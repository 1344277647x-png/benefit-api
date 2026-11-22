/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

const (
	tlsCertWarningDays          = 14
	tlsCertCriticalDays         = 7
	tlsProbeTimeout             = 5 * time.Second
	tlsCacheTTL                 = 10 * time.Minute
	tlsForceRefreshCooldown     = 30 * time.Second
	opsQueuePendingWarnAgeSec   = 300
	opsQueuePendingCritCount    = 100
	businessQueuePendingWarnCnt = 20
)

type OpsAlertTLSCertStatus struct {
	Status               string `json:"status"`
	Domain               string `json:"domain"`
	Issuer               string `json:"issuer,omitempty"`
	NotAfter             int64  `json:"not_after,omitempty"`
	DaysRemaining        int    `json:"days_remaining"`
	WarningThresholdDays int    `json:"warning_threshold_days"`
	CheckedAt            int64  `json:"checked_at"`
	Error                string `json:"error,omitempty"`
}

type OpsAlertItem struct {
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	Title      string `json:"title"`
	Detail     string `json:"detail"`
	Count      int64  `json:"count,omitempty"`
	ActionPath string `json:"action_path,omitempty"`
}

type OpsAlertsSnapshot struct {
	GeneratedAt int64                 `json:"generated_at"`
	AlertLevel  string                `json:"alert_level"`
	TLS         OpsAlertTLSCertStatus `json:"tls"`
	Queues      model.OpsQueueSummary `json:"queues"`
	Alerts      []OpsAlertItem        `json:"alerts"`
}

type tlsProbeResult struct {
	Issuer   string
	NotAfter time.Time
}

var (
	opsAlertsCacheMu    sync.Mutex
	opsAlertsCachedAddr string
	opsAlertsCachedAt   time.Time
	opsAlertsCachedTLS  OpsAlertTLSCertStatus
	opsAlertsTLSProber  = probePublicTLSCertificate
)

func ResetOpsAlertsCacheForTest() {
	opsAlertsCacheMu.Lock()
	defer opsAlertsCacheMu.Unlock()
	opsAlertsCachedAddr = ""
	opsAlertsCachedAt = time.Time{}
	opsAlertsCachedTLS = OpsAlertTLSCertStatus{}
}

func isPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return !ip.IsLoopback() &&
		!ip.IsPrivate() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() &&
		!ip.IsMulticast() &&
		!ip.IsUnspecified()
}

func isDisallowedTLSHostname(host string) bool {
	lower := strings.ToLower(strings.TrimSpace(host))
	if lower == "" || lower == "localhost" || strings.HasSuffix(lower, ".localhost") ||
		strings.HasSuffix(lower, ".local") || strings.HasSuffix(lower, ".internal") {
		return true
	}
	if ip := net.ParseIP(lower); ip != nil {
		return !isPublicIP(ip)
	}
	return false
}

func probePublicTLSCertificate(ctx context.Context, host string, port string) (tlsProbeResult, error) {
	if isDisallowedTLSHostname(host) {
		return tlsProbeResult{}, errors.New("non-public host skipped")
	}
	lookupCtx, cancel := context.WithTimeout(ctx, tlsProbeTimeout)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(lookupCtx, host)
	if err != nil || len(ips) == 0 {
		return tlsProbeResult{}, errors.New("dns lookup failed")
	}
	var dialIP net.IP
	for _, entry := range ips {
		if !isPublicIP(entry.IP) {
			return tlsProbeResult{}, errors.New("non-public host skipped")
		}
		if dialIP == nil {
			dialIP = entry.IP
		}
	}
	dialer := &net.Dialer{Timeout: tlsProbeTimeout}
	rawConn, err := dialer.DialContext(lookupCtx, "tcp", net.JoinHostPort(dialIP.String(), port))
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return tlsProbeResult{}, errors.New("connection timed out")
		}
		return tlsProbeResult{}, errors.New("tls connection failed")
	}
	return inspectTLSCertificate(lookupCtx, rawConn, &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	})
}

func inspectTLSCertificate(ctx context.Context, conn net.Conn, config *tls.Config) (tlsProbeResult, error) {
	tlsConn := tls.Client(conn, config)
	defer func() { _ = tlsConn.Close() }()
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		var invalid x509.CertificateInvalidError
		if errors.As(err, &invalid) && invalid.Reason == x509.Expired && invalid.Cert != nil {
			now := time.Now()
			if config.Time != nil {
				now = config.Time()
			}
			if now.Before(invalid.Cert.NotBefore) {
				return tlsProbeResult{}, errors.New("certificate not yet valid")
			}
			if !now.Before(invalid.Cert.NotAfter) {
				return tlsProbeResult{Issuer: invalid.Cert.Issuer.CommonName, NotAfter: invalid.Cert.NotAfter}, nil
			}
		}
		var hostnameErr x509.HostnameError
		if errors.As(err, &hostnameErr) {
			return tlsProbeResult{}, errors.New("certificate hostname mismatch")
		}
		return tlsProbeResult{}, errors.New("tls handshake failed")
	}
	certs := tlsConn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return tlsProbeResult{}, errors.New("peer certificate missing")
	}
	leaf := certs[0]
	issuer := strings.TrimSpace(leaf.Issuer.CommonName)
	if issuer == "" && len(leaf.Issuer.Organization) > 0 {
		issuer = strings.TrimSpace(leaf.Issuer.Organization[0])
	}
	return tlsProbeResult{
		Issuer:   issuer,
		NotAfter: leaf.NotAfter,
	}, nil
}

func checkServerAddressTLS(ctx context.Context, serverAddress string, forceRefresh bool, now time.Time) OpsAlertTLSCertStatus {
	trimmed := strings.TrimSpace(serverAddress)
	base := OpsAlertTLSCertStatus{
		Status:               "not_configured",
		WarningThresholdDays: tlsCertWarningDays,
		CheckedAt:            now.Unix(),
	}
	if trimmed == "" {
		return base
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Hostname() == "" {
		base.Domain = parsedHostOrEmpty(parsed)
		return base
	}
	host := parsed.Hostname()
	port := parsed.Port()
	if port == "" {
		port = "443"
	}
	base.Domain = host

	opsAlertsCacheMu.Lock()
	if opsAlertsCachedAddr == trimmed && !opsAlertsCachedAt.IsZero() {
		age := now.Sub(opsAlertsCachedAt)
		if (!forceRefresh && age < tlsCacheTTL) || (forceRefresh && age < tlsForceRefreshCooldown) {
			cached := opsAlertsCachedTLS
			opsAlertsCacheMu.Unlock()
			return cached
		}
	}
	opsAlertsCacheMu.Unlock()

	probe, err := opsAlertsTLSProber(ctx, host, port)
	result := base
	result.CheckedAt = now.Unix()
	if err != nil {
		if err.Error() == "non-public host skipped" {
			result.Status = "not_configured"
			result.Error = err.Error()
		} else {
			result.Status = "error"
			result.Error = err.Error()
		}
	} else {
		result.Issuer = probe.Issuer
		result.NotAfter = probe.NotAfter.Unix()
		remaining := probe.NotAfter.Sub(now)
		days := int(remaining.Hours() / 24)
		if remaining < 0 && days == 0 {
			days = -1
		}
		result.DaysRemaining = days
		switch {
		case remaining <= 0 || days < tlsCertCriticalDays:
			result.Status = "critical"
		case days < tlsCertWarningDays:
			result.Status = "warning"
		default:
			result.Status = "ok"
		}
	}

	opsAlertsCacheMu.Lock()
	opsAlertsCachedAddr = trimmed
	opsAlertsCachedAt = now
	opsAlertsCachedTLS = result
	opsAlertsCacheMu.Unlock()
	return result
}

func parsedHostOrEmpty(u *url.URL) string {
	if u == nil {
		return ""
	}
	return u.Hostname()
}

func GetOpsAlertsSnapshot(ctx context.Context, forceRefresh bool) (*OpsAlertsSnapshot, error) {
	now := time.Now()
	queues, err := model.GetOpsQueueSummary(ctx)
	if err != nil {
		return nil, err
	}
	tlsStatus := checkServerAddressTLS(ctx, system_setting.ServerAddress, forceRefresh, now)

	alerts := make([]OpsAlertItem, 0, 5)
	switch tlsStatus.Status {
	case "critical":
		title := "TLS certificate expiring soon"
		detail := fmt.Sprintf("%s expires in %d day(s).", tlsStatus.Domain, tlsStatus.DaysRemaining)
		if tlsStatus.DaysRemaining <= 0 {
			title = "TLS certificate expired"
			detail = fmt.Sprintf("%s certificate has expired.", tlsStatus.Domain)
		}
		alerts = append(alerts, OpsAlertItem{
			Code:     "tls_cert_critical",
			Severity: "critical",
			Title:    title,
			Detail:   detail,
		})
	case "warning":
		alerts = append(alerts, OpsAlertItem{
			Code:     "tls_cert_expiring",
			Severity: "warning",
			Title:    "TLS certificate expiring soon",
			Detail:   fmt.Sprintf("%s expires in %d day(s).", tlsStatus.Domain, tlsStatus.DaysRemaining),
		})
	case "error":
		alerts = append(alerts, OpsAlertItem{
			Code:     "tls_cert_check_failed",
			Severity: "warning",
			Title:    "TLS certificate check failed",
			Detail:   fmt.Sprintf("%s: %s", tlsStatus.Domain, tlsStatus.Error),
		})
	}

	if queues.BusinessDeadLetter > 0 {
		alerts = append(alerts, OpsAlertItem{
			Code:       "business_event_dead_letter",
			Severity:   "critical",
			Title:      "Business outbox dead letters detected",
			Detail:     fmt.Sprintf("%d dead-letter event(s) require inspection.", queues.BusinessDeadLetter),
			Count:      queues.BusinessDeadLetter,
			ActionPath: "/dashboard/reconciliation",
		})
	}

	if queues.OperationsPending > 0 {
		severity := "warning"
		age := int64(0)
		if queues.OperationsOldestPendingAt > 0 {
			age = now.Unix() - queues.OperationsOldestPendingAt
		}
		if queues.OperationsRetrying > 0 || queues.OperationsPending >= opsQueuePendingCritCount || age >= opsQueuePendingWarnAgeSec {
			severity = "critical"
		}
		alerts = append(alerts, OpsAlertItem{
			Code:       "operations_queue_backlog",
			Severity:   severity,
			Title:      "Operations accounting queue backlog",
			Detail:     fmt.Sprintf("%d pending (%d retrying).", queues.OperationsPending, queues.OperationsRetrying),
			Count:      queues.OperationsPending,
			ActionPath: "/dashboard/operations",
		})
	}

	if queues.BusinessRetrying > 0 || queues.BusinessPending >= businessQueuePendingWarnCnt {
		alerts = append(alerts, OpsAlertItem{
			Code:       "business_event_backlog",
			Severity:   "warning",
			Title:      "Business event outbox backlog",
			Detail:     fmt.Sprintf("%d pending (%d retrying).", queues.BusinessPending, queues.BusinessRetrying),
			Count:      queues.BusinessPending,
			ActionPath: "/dashboard/reconciliation",
		})
	}

	if queues.BillingExceptionsCount > 0 || queues.UnsettledPreConsumeCount > 0 {
		total := queues.BillingExceptionsCount + queues.UnsettledPreConsumeCount
		alerts = append(alerts, OpsAlertItem{
			Code:       "billing_reconciliation_pending",
			Severity:   "warning",
			Title:      "Billing settlement anomalies need review",
			Detail:     fmt.Sprintf("%d billing exception(s), %d unsettled pre-consume record(s).", queues.BillingExceptionsCount, queues.UnsettledPreConsumeCount),
			Count:      total,
			ActionPath: "/dashboard/reconciliation",
		})
	}

	level := "ok"
	for _, alert := range alerts {
		if alert.Severity == "critical" {
			level = "critical"
			break
		}
		if alert.Severity == "warning" {
			level = "warning"
		}
	}

	return &OpsAlertsSnapshot{
		GeneratedAt: now.Unix(),
		AlertLevel:  level,
		TLS:         tlsStatus,
		Queues:      queues,
		Alerts:      alerts,
	}, nil
}
