package teamserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"infinite-canvas/backend/internal/accounts"
	"infinite-canvas/backend/internal/kernelpool"
)

type failedRelayPool struct {
	WorkspacePool
	closed bool
}

// Like the real pool, a failed reprovision closes the open kernel.
func (p *failedRelayPool) Reprovision(spec kernelpool.Spec) error {
	_ = p.Close(context.Background(), spec.ID)
	return errors.New("synthetic disk failure")
}
func (p *failedRelayPool) Close(context.Context, string) error { p.closed = true; return nil }

func TestRelayBindingReportsSavedButNotApplied(t *testing.T) {
	var pool *failedRelayPool
	h := newHarness(t, func(o *Options) { pool = &failedRelayPool{WorkspacePool: o.Pool}; o.Pool = pool })
	u, err := h.users.CreateUser(context.Background(), "", accounts.CreateUserInput{Username: "admin", Password: "admin-password", Role: accounts.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	cookie := h.login("admin", "admin-password")
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		pool.closed = false
		body := ""
		if method == http.MethodPut {
			body = `{"key":"SYNTHETIC_BINDING_KEY"}`
		}
		r := h.do(call{method: method, path: "/api/platform/admin/users/" + u.ID + "/relay-key", body: body, cookie: cookie})
		var envelope struct {
			Data struct {
				User    json.RawMessage `json:"user"`
				Applied *bool           `json:"applied"`
				Message string          `json:"message"`
			} `json:"data"`
		}
		if err := json.Unmarshal(r.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if r.Code != 200 || envelope.Data.Applied == nil || *envelope.Data.Applied || len(envelope.Data.User) == 0 || !strings.Contains(envelope.Data.Message, map[string]string{http.MethodPut: "绑定已保存", http.MethodDelete: "密钥已清除"}[method]) || !pool.closed {
			t.Fatalf("failure not communicated or fenced: %s", r.Body.String())
		}
		w, err := h.users.WorkspaceForUser(context.Background(), u.ID)
		if err != nil {
			t.Fatal(err)
		}
		key, err := h.users.RelayKey(*w)
		want := ""
		if method == http.MethodPut {
			want = "SYNTHETIC_BINDING_KEY"
		}
		if err != nil || key != want {
			t.Fatal("binding was not saved")
		}
	}
}
