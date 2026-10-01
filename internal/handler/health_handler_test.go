package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

type stubPinger struct {
	err error
	// gotDeadline は Ping に渡された ctx が期限付きだったかを記録する。
	gotDeadline bool
}

func (s *stubPinger) Ping(ctx context.Context) error {
	_, s.gotDeadline = ctx.Deadline()
	return s.err
}

func TestHealthzOK(t *testing.T) {
	pinger := &stubPinger{}
	rec := doHealthz(t, pinger)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := decodeStatus(t, rec); got != "ok" {
		t.Errorf("status field = %q, want %q", got, "ok")
	}
	if !pinger.gotDeadline {
		t.Error("Ping received a context without a deadline; want healthCheckTimeout applied")
	}
}

// DBが落ちているときは 503 を返す。ECS/ALB はこれを見てタスクを入れ替える。
func TestHealthzDBDown(t *testing.T) {
	rec := doHealthz(t, &stubPinger{err: errors.New("connection refused")})

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if got := decodeStatus(t, rec); got != "unhealthy" {
		t.Errorf("status field = %q, want %q", got, "unhealthy")
	}
	// 未認証で叩けるエンドポイントなので、内部エラーの詳細を漏らさないこと。
	if body := rec.Body.String(); strings.Contains(body, "connection refused") {
		t.Errorf("body leaked internal error detail: %q", body)
	}
}

func doHealthz(t *testing.T, p DBPinger) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	if err := NewHealthHandler(p).Healthz(e.NewContext(req, rec)); err != nil {
		t.Fatalf("Healthz returned error: %v", err)
	}
	return rec
}

func decodeStatus(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return body["status"]
}

// /healthz が静的ファイル配信のワイルドカード（/*）に食われていないことを確かめる。
func TestRouterHealthzNotShadowedByStatic(t *testing.T) {
	e := NewRouter(Handlers{
		Health: NewHealthHandler(&stubPinger{}),
		WebDir: t.TempDir(),
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q, want %d", rec.Code, rec.Body.String(), http.StatusOK)
	}
	if got := decodeStatus(t, rec); got != "ok" {
		t.Errorf("status field = %q, want %q", got, "ok")
	}
}
