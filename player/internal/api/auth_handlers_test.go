package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/torwin-job/musik/player/internal/auth"
)

func TestAuthDisabledLoginAndMe(t *testing.T) {
	server := openTestServer(t)

	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader([]byte(`{"password":"x"}`)))
	rec := httptest.NewRecorder()
	server.handleAuthLogin(rec, req)
	if rec.Code != 200 {
		t.Fatalf("login status=%d", rec.Code)
	}
	var login map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &login); err != nil {
		t.Fatal(err)
	}
	if login["auth"] != false {
		t.Fatalf("expected auth=false when disabled: %#v", login)
	}

	req = httptest.NewRequest("GET", "/api/auth/me", nil)
	rec = httptest.NewRecorder()
	server.handleAuthMe(rec, req)
	var me struct {
		OK          bool `json:"ok"`
		AuthEnabled bool `json:"auth_enabled"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if !me.OK || me.AuthEnabled {
		t.Fatalf("me=%+v", me)
	}

	req = httptest.NewRequest("POST", "/api/auth/logout", nil)
	rec = httptest.NewRecorder()
	server.handleAuthLogout(rec, req)
	if rec.Code != 200 {
		t.Fatalf("logout status=%d", rec.Code)
	}
}

func TestAuthPasswordLoginCookieAndReject(t *testing.T) {
	server := openTestServer(t)
	server.Cfg.AuthDisabled = false
	server.Cfg.Password = "secret"
	server.Auth = auth.New(auth.Config{
		Password: "secret", APIToken: "tok", Disabled: false,
	})
	server.loginLimiter = auth.NewLoginLimiter(20, time.Minute)

	bad := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader([]byte(`{"password":"nope"}`)))
	bad.RemoteAddr = "10.0.0.1:1234"
	rec := httptest.NewRecorder()
	server.handleAuthLogin(rec, bad)
	if rec.Code != 401 {
		t.Fatalf("bad password status=%d", rec.Code)
	}

	okReq := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader([]byte(`{"password":"secret"}`)))
	okReq.RemoteAddr = "10.0.0.2:1234"
	rec = httptest.NewRecorder()
	server.handleAuthLogin(rec, okReq)
	if rec.Code != 200 {
		t.Fatalf("good password status=%d body=%s", rec.Code, rec.Body.String())
	}
	cookie := rec.Result().Cookies()
	if len(cookie) == 0 || cookie[0].Name != auth.CookieName {
		t.Fatalf("expected session cookie, got %#v", cookie)
	}

	meReq := httptest.NewRequest("GET", "/api/auth/me", nil)
	meReq.AddCookie(cookie[0])
	rec = httptest.NewRecorder()
	server.handleAuthMe(rec, meReq)
	var me struct {
		OK          bool `json:"ok"`
		AuthEnabled bool `json:"auth_enabled"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if !me.OK || !me.AuthEnabled {
		t.Fatalf("authorized me=%+v", me)
	}

	bearer := httptest.NewRequest("GET", "/api/auth/me", nil)
	bearer.Header.Set("Authorization", "Bearer tok")
	rec = httptest.NewRecorder()
	server.handleAuthMe(rec, bearer)
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if !me.OK {
		t.Fatal("bearer token should authorize /me")
	}
}

func TestAuthLoginRateLimit(t *testing.T) {
	server := openTestServer(t)
	server.Auth = auth.New(auth.Config{Password: "secret"})
	server.loginLimiter = auth.NewLoginLimiter(2, time.Minute)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader([]byte(`{"password":"x"}`)))
		req.RemoteAddr = "10.9.9.9:1"
		rec := httptest.NewRecorder()
		server.handleAuthLogin(rec, req)
		if rec.Code != 401 {
			t.Fatalf("attempt %d status=%d", i, rec.Code)
		}
	}
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader([]byte(`{"password":"secret"}`)))
	req.RemoteAddr = "10.9.9.9:1"
	rec := httptest.NewRecorder()
	server.handleAuthLogin(rec, req)
	if rec.Code != 429 {
		t.Fatalf("rate limit status=%d, want 429", rec.Code)
	}
}
