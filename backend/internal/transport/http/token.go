package httptransport

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
)

const LaunchTokenHeader = "X-Desktop-Token"

type desktopAuthenticatedKey struct{}

// DesktopAuthenticated can only be set after the launch credential was verified.
func DesktopAuthenticated(r *http.Request) bool {
	trusted, _ := r.Context().Value(desktopAuthenticatedKey{}).(bool)
	return trusted
}

type hostedOwnerKey struct{}

// WithHostedOwner marks a request the team-server gateway has already bound to
// this workspace's owner. Only in-process code can set a context value, so a
// browser cannot forge it with headers, Host or Origin.
func WithHostedOwner(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), hostedOwnerKey{}, true))
}

// HostedOwner reports whether the gateway authenticated this request as the
// workspace owner. It replaces the loopback same-origin rule for hosted kernels.
func HostedOwner(r *http.Request) bool {
	if r == nil {
		return false
	}
	trusted, _ := r.Context().Value(hostedOwnerKey{}).(bool)
	return trusted
}

func NewLaunchToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func RequireLaunchToken(token string) func(http.Handler) http.Handler {
	want := []byte(token)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodOptions || r.URL.Path == "/api/health/live" {
				next.ServeHTTP(w, r)
				return
			}
			got := []byte(r.Header.Get(LaunchTokenHeader))
			if len(want) == 0 || len(got) != len(want) || subtle.ConstantTimeCompare(got, want) != 1 {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code":   http.StatusForbidden,
					"data":   nil,
					"msg":    "桌面运行时授权无效",
					"reason": "desktop_token_required",
				})
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), desktopAuthenticatedKey{}, true)))
		})
	}
}
