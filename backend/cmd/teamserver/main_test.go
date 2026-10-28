package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"infinite-canvas/backend/internal/accounts"
)

func cleanEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"TEAMSERVER_ADDR", "TEAMSERVER_DATA_DIR", "TEAMSERVER_ALLOWED_ORIGINS", "TEAMSERVER_INSECURE_COOKIE"} {
		t.Setenv(key, "")
	}
}

func TestAdminCreateLoginMeLogout(t *testing.T) {
	cleanEnv(t)
	root := t.TempDir()
	t.Setenv("TEAMSERVER_DATA_DIR", root)
	const password = "command line stdin password"
	var output bytes.Buffer
	args := []string{"admin", "create", "--username", "owner", "--display-name", "Team Owner"}
	if err := run(context.Background(), args, strings.NewReader(password+"\n"), &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), password) {
		t.Fatal("command exposed password")
	}
	service, err := accounts.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	handler, err := accounts.NewHandler(service, accounts.HTTPConfig{AllowedOrigins: []string{"https://team.example"}})
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body string, cookie *http.Cookie, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "https://team.example"+path, strings.NewReader(body))
		req.Header.Set("Origin", "https://team.example")
		req.Header.Set("X-Requested-With", "hfp")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body.String())
		}
		return rec
	}
	login := request("POST", "/api/platform/login", `{"username":"owner","password":"`+password+`"}`, nil, 200)
	cookie := login.Result().Cookies()[0]
	me := request("GET", "/api/platform/me", "", cookie, 200)
	var envelope struct {
		Code int               `json:"code"`
		Data accounts.Identity `json:"data"`
	}
	if err := json.Unmarshal(me.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != 0 || envelope.Data.User.Role != accounts.RoleAdmin || envelope.Data.User.DisplayName != "Team Owner" || envelope.Data.WorkspaceID == "" {
		t.Fatalf("identity: %+v", envelope)
	}
	request("POST", "/api/platform/logout", "", cookie, 200)
	request("GET", "/api/platform/me", "", cookie, 401)
	if _, err := os.Stat(filepath.Join(root, "workspaces")); !os.IsNotExist(err) {
		t.Fatal("command created kernel directory")
	}
	secondRoot := t.TempDir()
	args = append(args, "--data-dir", secondRoot)
	if err := run(context.Background(), args, strings.NewReader(password), &output); err != nil {
		t.Fatalf("flag must override environment: %v", err)
	}
	if _, err := os.Stat(filepath.Join(secondRoot, "platform.db")); err != nil {
		t.Fatal(err)
	}
	if err := run(context.Background(), []string{"admin", "create", "--username", "second"}, strings.NewReader(password), &output); err != nil {
		t.Fatalf("additional admin: %v", err)
	}
}

func TestCommandValidationAndPasswordInput(t *testing.T) {
	cleanEnv(t)
	for _, args := range [][]string{nil, {"unknown"}, {"admin", "create"}, {"serve"}, {"admin", "create", "--username", "owner", "--password", "must-not-appear"}} {
		var output bytes.Buffer
		err := run(context.Background(), args, strings.NewReader(""), &output)
		if err == nil {
			t.Fatalf("invalid command accepted: %v", args)
		}
		if strings.Contains(err.Error()+output.String(), "must-not-appear") {
			t.Fatal("argument leaked into error")
		}
	}
	for _, password := range []string{"", "short", strings.Repeat("p", 1025)} {
		if _, err := readPassword(strings.NewReader(password)); err == nil {
			t.Fatal("invalid stdin accepted")
		}
	}
	const spaces = "  password with spaces  "
	if password, err := readPassword(strings.NewReader(spaces + "\r\nignored")); err != nil || password != spaces {
		t.Fatalf("stdin password was altered: %q %v", password, err)
	}
	t.Setenv("TEAMSERVER_INSECURE_COOKIE", "invalid")
	if err := run(context.Background(), []string{"serve"}, strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Fatal("invalid boolean accepted")
	}
}
