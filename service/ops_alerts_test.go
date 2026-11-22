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
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpsAlertsTLSRealExpiredHandshake(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(server.Close)
	leaf := server.Certificate()
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	dialer := net.Dialer{}
	checkTime := leaf.NotAfter.Add(time.Hour)
	conn, err := dialer.DialContext(t.Context(), "tcp", server.Listener.Addr().String())
	require.NoError(t, err)
	cert, err := inspectTLSCertificate(t.Context(), conn, &tls.Config{
		RootCAs: roots, ServerName: "127.0.0.1", Time: func() time.Time { return checkTime },
	})
	require.NoError(t, err)
	assert.Equal(t, leaf.NotAfter, cert.NotAfter)

	conn, err = dialer.DialContext(t.Context(), "tcp", server.Listener.Addr().String())
	require.NoError(t, err)
	_, err = inspectTLSCertificate(t.Context(), conn, &tls.Config{
		RootCAs: roots, ServerName: "unrelated.test", Time: func() time.Time { return leaf.NotBefore.Add(time.Hour) },
	})
	require.ErrorContains(t, err, "hostname mismatch")
}

func TestOpsAlertsTLSCertificateThresholdsAndSSRFGuard(t *testing.T) {
	oldAddr := system_setting.ServerAddress
	oldProber := opsAlertsTLSProber
	t.Cleanup(func() {
		system_setting.ServerAddress = oldAddr
		opsAlertsTLSProber = oldProber
		ResetOpsAlertsCacheForTest()
	})

	now := time.Unix(1760000000, 0)

	// 1. HTTP URL skips TLS dialing cleanly.
	ResetOpsAlertsCacheForTest()
	called := 0
	opsAlertsTLSProber = func(ctx context.Context, host, port string) (tlsProbeResult, error) {
		called++
		return tlsProbeResult{}, nil
	}
	httpStatus := checkServerAddressTLS(t.Context(), "http://localhost:3000", false, now)
	assert.Equal(t, "not_configured", httpStatus.Status)
	assert.Zero(t, called)

	// 2. Private/loopback HTTPS hosts are rejected by production prober without dialing.
	_, err := probePublicTLSCertificate(t.Context(), "127.0.0.1", "443")
	require.Error(t, err)
	assert.Equal(t, "non-public host skipped", err.Error())
	_, err = probePublicTLSCertificate(t.Context(), "localhost", "443")
	require.Error(t, err)
	assert.Equal(t, "non-public host skipped", err.Error())

	// 3. Certificate expiring in 10 days -> warning, cached within TTL.
	ResetOpsAlertsCacheForTest()
	opsAlertsTLSProber = func(ctx context.Context, host, port string) (tlsProbeResult, error) {
		called++
		assert.Equal(t, "xbenefitapi.xyz", host)
		assert.Equal(t, "443", port)
		return tlsProbeResult{
			Issuer:   "Let's Encrypt R10",
			NotAfter: now.Add(10*24*time.Hour + time.Hour),
		}, nil
	}
	warnStatus := checkServerAddressTLS(t.Context(), "https://xbenefitapi.xyz", false, now)
	assert.Equal(t, "warning", warnStatus.Status)
	assert.Equal(t, 10, warnStatus.DaysRemaining)
	assert.Equal(t, "Let's Encrypt R10", warnStatus.Issuer)
	assert.Equal(t, 1, called)

	// Cached hit within TTL does not dial again.
	cachedStatus := checkServerAddressTLS(t.Context(), "https://xbenefitapi.xyz", false, now.Add(2*time.Minute))
	assert.Equal(t, warnStatus, cachedStatus)
	assert.Equal(t, 1, called)

	// 4. Certificate expiring in 3 days -> critical after force refresh past cooldown.
	opsAlertsTLSProber = func(ctx context.Context, host, port string) (tlsProbeResult, error) {
		called++
		return tlsProbeResult{
			Issuer:   "Let's Encrypt R10",
			NotAfter: now.Add(3*24*time.Hour + time.Hour),
		}, nil
	}
	critStatus := checkServerAddressTLS(t.Context(), "https://xbenefitapi.xyz", true, now.Add(time.Minute))
	assert.Equal(t, "critical", critStatus.Status)
	assert.Equal(t, 3, critStatus.DaysRemaining)
	assert.Equal(t, 2, called)

	// 5. Healthy certificate (> 14 days) -> ok.
	ResetOpsAlertsCacheForTest()
	opsAlertsTLSProber = func(ctx context.Context, host, port string) (tlsProbeResult, error) {
		return tlsProbeResult{
			Issuer:   "Let's Encrypt R10",
			NotAfter: now.Add(60 * 24 * time.Hour),
		}, nil
	}
	okStatus := checkServerAddressTLS(t.Context(), "https://xbenefitapi.xyz", false, now)
	assert.Equal(t, "ok", okStatus.Status)
	assert.Equal(t, 60, okStatus.DaysRemaining)

	// 6. Dial failure surfaces error status cleanly.
	ResetOpsAlertsCacheForTest()
	opsAlertsTLSProber = func(ctx context.Context, host, port string) (tlsProbeResult, error) {
		return tlsProbeResult{}, errors.New("tls handshake failed")
	}
	errStatus := checkServerAddressTLS(t.Context(), "https://xbenefitapi.xyz", false, now)
	assert.Equal(t, "error", errStatus.Status)
	assert.Equal(t, "tls handshake failed", errStatus.Error)

	// 7. GetOpsAlertsSnapshot combines TLS and empty queues cleanly without DB panic.
	oldDB := model.DB
	model.DB = nil
	t.Cleanup(func() { model.DB = oldDB })
	system_setting.ServerAddress = "https://xbenefitapi.xyz"
	ResetOpsAlertsCacheForTest()
	opsAlertsTLSProber = func(ctx context.Context, host, port string) (tlsProbeResult, error) {
		return tlsProbeResult{Issuer: "Let's Encrypt", NotAfter: time.Now().Add(5 * 24 * time.Hour)}, nil
	}
	snap, err := GetOpsAlertsSnapshot(t.Context(), false)
	require.NoError(t, err)
	assert.Equal(t, "critical", snap.AlertLevel)
	require.Len(t, snap.Alerts, 1)
	assert.Equal(t, "tls_cert_critical", snap.Alerts[0].Code)
}
