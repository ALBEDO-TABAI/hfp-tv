package accounts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testOrigin = "https://team.example"

func testHandler(t *testing.T, s *Service, config HTTPConfig) http.Handler {
	t.Helper()
	h, err := NewHandler(s, config)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func apiRequest(h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, testOrigin+path, strings.NewReader(body))
	req.RemoteAddr = "192.0.2.100:12345"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", testOrigin)
	req.Header.Set("X-Requested-With", "hfp")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	}
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, req)
	return recorder
}

func assertStatus(t *testing.T, response *httptest.ResponseRecorder, status int) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status=%d, want %d: %s", response.Code, status, response.Body.String())
	}
	var envelope struct {
		Code   int    `json:"code"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if status >= 400 && (envelope.Code != status || envelope.Reason == "") {
		t.Fatalf("failure envelope: %s", response.Body.String())
	}
	if status == 200 && envelope.Code != 0 {
		t.Fatalf("success envelope: %s", response.Body.String())
	}
}

func TestCSRFAndPreflight(t *testing.T) {
	s := testService(t)
	h := testHandler(t, s, HTTPConfig{AllowedOrigins: []string{testOrigin}})
	for _, tc := range []struct{ name, origin, header string }{
		{"missing header", testOrigin, ""},
		{"wrong header", testOrigin, "ajax"},
		{"missing origin", "", "hfp"},
		{"wrong origin", "https://evil.example", "hfp"},
		{"wrong scheme", "http://team.example", "hfp"},
		{"wrong port", "https://team.example:444", "hfp"},
		{"null origin", "null", "hfp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, testOrigin+"/api/platform/logout", nil)
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("X-Requested-With", tc.header)
			req.Header.Set("X-Forwarded-Host", "evil.example")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			assertStatus(t, rec, http.StatusForbidden)
		})
	}
	assertStatus(t, apiRequest(h, "POST", "/api/platform/logout", "", ""), http.StatusOK)
	empty := testHandler(t, s, HTTPConfig{})
	assertStatus(t, apiRequest(empty, "POST", "/api/platform/logout", "", ""), http.StatusForbidden)
	for _, invalid := range []string{"*", "https://*.example", "null", "https://team.example/path", "https://user:pass@team.example"} {
		if _, err := NewHandler(s, HTTPConfig{AllowedOrigins: []string{invalid}}); err == nil {
			t.Fatalf("accepted origin %s", invalid)
		}
	}
	for _, origin := range []string{testOrigin, "https://evil.example"} {
		req := httptest.NewRequest(http.MethodOptions, testOrigin+"/api/platform/login", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "Content-Type, X-Requested-With")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if origin == testOrigin {
			if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != testOrigin || rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
				t.Fatalf("preflight: %+v", rec)
			}
		} else {
			assertStatus(t, rec, 403)
		}
	}
}

func TestCookiesPrivacyAndHTTPAuthorization(t *testing.T) {
	s := testService(t)
	_, admin := testAdmin(t, s)
	h := testHandler(t, s, HTTPConfig{AllowedOrigins: []string{testOrigin}})
	created := apiRequest(h, "POST", "/api/platform/admin/users", `{"username":"member","password":"`+testPassword+`"}`, admin)
	assertStatus(t, created, 200)
	for _, insecure := range []bool{false, true} {
		h := testHandler(t, s, HTTPConfig{AllowedOrigins: []string{testOrigin}, InsecureCookie: insecure})
		login := apiRequest(h, "POST", "/api/platform/login", `{"username":"member","password":"`+testPassword+`"}`, "")
		assertStatus(t, login, 200)
		cookies := login.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatalf("cookies: %v", cookies)
		}
		cookie := cookies[0]
		if cookie.Secure == insecure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.Domain != "" || cookie.MaxAge <= 0 {
			t.Fatalf("cookie policy: %+v", cookie)
		}
		me := apiRequest(h, "GET", "/api/platform/me", "", cookie.Value)
		assertStatus(t, me, 200)
		for _, secret := range []string{"password", "token_hash", "relay_key", "data_dir", cookie.Value} {
			if strings.Contains(me.Body.String(), secret) || strings.Contains(login.Body.String(), secret) {
				t.Fatalf("response exposes %s", secret)
			}
		}
		for _, endpoint := range []struct{ method, path, body string }{
			{"GET", "/api/platform/admin/users", ""},
			{"POST", "/api/platform/admin/users", `{}`},
			{"POST", "/api/platform/admin/invitations", `{}`},
			{"POST", "/api/platform/admin/users/missing/disable", ""},
			{"POST", "/api/platform/admin/users/missing/reset-password", `{}`},
		} {
			assertStatus(t, apiRequest(h, endpoint.method, endpoint.path, endpoint.body, cookie.Value), 403)
		}
		logout := apiRequest(h, "POST", "/api/platform/logout", "", cookie.Value)
		assertStatus(t, logout, 200)
		deleted := logout.Result().Cookies()[0]
		if deleted.MaxAge != -1 || deleted.Path != "/" || deleted.Secure != cookie.Secure || !deleted.HttpOnly {
			t.Fatalf("cookie not cleared: %+v", deleted)
		}
		assertStatus(t, apiRequest(h, "GET", "/api/platform/me", "", cookie.Value), 401)
	}
	assertStatus(t, apiRequest(h, "GET", "/api/tasks", "", admin), 404)
	assertStatus(t, apiRequest(h, "POST", "/api/tasks", `{}`, admin), 404)
	assertStatus(t, apiRequest(h, "POST", "/api/platform/admin/users", `{}`, ""), 401)
}

func TestAdminHTTPWritesAndLoginLimits(t *testing.T) {
	s := testService(t)
	_, admin := testAdmin(t, s)
	h := testHandler(t, s, HTTPConfig{AllowedOrigins: []string{testOrigin}})
	invite := apiRequest(h, "POST", "/api/platform/admin/invitations", `{"role":"member"}`, admin)
	assertStatus(t, invite, 200)
	var issued struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(invite.Body.Bytes(), &issued); err != nil || issued.Data.Token == "" {
		t.Fatalf("invitation: %v", err)
	}
	path := "/api/platform/invitations/" + issued.Data.Token + "/accept"
	accepted := apiRequest(h, "POST", path, `{"username":"invited","password":"`+testPassword+`","role":"admin"}`, "")
	assertStatus(t, accepted, 200)
	var identity struct {
		Data Identity `json:"data"`
	}
	if err := json.Unmarshal(accepted.Body.Bytes(), &identity); err != nil {
		t.Fatal(err)
	}
	if identity.Data.User.Role != RoleMember {
		t.Fatal("invitation escalated role")
	}
	assertStatus(t, apiRequest(h, "POST", path, `{"username":"reused","password":"`+testPassword+`"}`, ""), 400)
	assertStatus(t, apiRequest(h, "GET", "/api/platform/admin/users", "", admin), 200)
	userPath := "/api/platform/admin/users/" + identity.Data.User.ID
	assertStatus(t, apiRequest(h, "POST", userPath+"/reset-password", `{"password":"new long password"}`, admin), 200)
	login := apiRequest(h, "POST", "/api/platform/login", `{"username":"invited","password":"new long password"}`, "")
	assertStatus(t, login, 200)
	memberToken := login.Result().Cookies()[0].Value
	assertStatus(t, apiRequest(h, "POST", userPath+"/disable", "", admin), 200)
	assertStatus(t, apiRequest(h, "GET", "/api/platform/me", "", memberToken), 401)
	for i := 0; i <= loginFailureLimit; i++ {
		req := httptest.NewRequest("POST", testOrigin+"/api/platform/login", strings.NewReader(`{"username":"unknown","password":"incorrect"}`))
		req.RemoteAddr = "192.0.2.123:54321"
		req.Header.Set("Origin", testOrigin)
		req.Header.Set("X-Requested-With", "hfp")
		// 客户端伪造的代理头不能改变限速 IP。
		req.Header.Set("X-Forwarded-For", "192.0.2."+strings.Repeat("1", i+1))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		status := 401
		if i == loginFailureLimit {
			status = 429
		}
		assertStatus(t, rec, status)
		if !strings.Contains(rec.Body.String(), ErrCredentials.Message) {
			t.Fatal("login message differs")
		}
	}
}
