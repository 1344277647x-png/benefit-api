package controller

import (
	"net/http"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// Native bearer-token clients generally omit Origin. Browser clients must
// match the configured public address, a trusted console URL or an explicit
// exact origin. Do not trust client-controlled forwarded headers here.
func realtimeOriginAllowed(r *http.Request) bool {
	values := r.Header.Values("Origin")
	if len(values) == 0 {
		return true
	}
	if len(values) != 1 {
		return false
	}
	origin, err := common.NormalizeOrigin(values[0])
	if err != nil {
		return false
	}
	allowed := []string{system_setting.ServerAddress}
	allowed = append(allowed, common.SessionCookieTrustedURLs...)
	allowed = append(allowed, strings.Split(os.Getenv("REALTIME_ALLOWED_ORIGINS"), ",")...)
	for _, raw := range allowed {
		candidate, err := common.NormalizeOrigin(strings.TrimRight(strings.TrimSpace(raw), "/"))
		if err == nil && origin == candidate {
			return true
		}
	}
	return false
}
