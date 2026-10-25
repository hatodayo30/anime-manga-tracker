package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/handler"
	"github.com/hatodayo30/anime-manga-tracker/internal/middleware"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/record"
)

// fakeRecordRepo は Upsert だけが決めたエラーを返す record.Repository。
type fakeRecordRepo struct {
	upsertErr error
}

func (f *fakeRecordRepo) List(context.Context, int64, domain.MediaType, domain.Status, domain.SortKey) ([]*domain.Record, error) {
	return nil, nil
}

func (f *fakeRecordRepo) Upsert(context.Context, int64, domain.NewRecordInput) (*domain.Record, error) {
	if f.upsertErr != nil {
		return nil, f.upsertErr
	}
	return &domain.Record{}, nil
}

func (f *fakeRecordRepo) Update(context.Context, int64, int64, domain.UpdateRecordInput) (*domain.Record, error) {
	return nil, record.ErrNotFound
}

func (f *fakeRecordRepo) Delete(context.Context, int64, int64) error { return nil }

// loggedInAuth はどのトークンでも ID 1 の利用者として通す。
type loggedInAuth struct{}

func (loggedInAuth) CurrentUser(context.Context, string) (*domain.User, error) {
	return &domain.User{ID: 1}, nil
}

func callRecord(t *testing.T, h echo.HandlerFunc, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.AddCookie(&http.Cookie{Name: middleware.SessionCookieName, Value: "token"})
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)
	if err := middleware.NewAuth(loggedInAuth{}).RequireUser(h)(c); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	return rec
}

// 記録の保存の失敗を、入力の誤り・見つからない・それ以外に振り分けることを保証する。
// それ以外（DB の失敗）のエラー文は画面に出さない。テーブル名・制約名が利用者に見えるため（BACKLOG 10）。
func TestRecordHandler_ErrorResponses(t *testing.T) {
	dbErr := errors.New(`upsert record: ERROR: new row for relation "records" violates check constraint "records_progress_unit_matches_media_type" (SQLSTATE 23514)`)

	tests := []struct {
		name       string
		repoErr    error
		method     string
		target     string
		body       string
		param      string
		wantCode   int
		wantInBody string
	}{
		{
			name:       "DB の失敗は定型文の 500",
			repoErr:    dbErr,
			method:     http.MethodPost,
			target:     "/api/records",
			body:       `{"anilistId":1,"mediaType":"manga","status":"want","title":"X"}`,
			wantCode:   http.StatusInternalServerError,
			wantInBody: "record request failed",
		},
		{
			name:       "入力の誤りはメッセージ付きの 400",
			method:     http.MethodPost,
			target:     "/api/records",
			body:       `{"anilistId":1,"mediaType":"movie","status":"want","title":"X"}`,
			wantCode:   http.StatusBadRequest,
			wantInBody: "invalid media type",
		},
		{
			name:     "見つからなければ 404",
			method:   http.MethodPatch,
			target:   "/api/records/9",
			body:     `{"progress":1}`,
			param:    "9",
			wantCode: http.StatusNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := handler.NewRecordHandler(record.NewUsecase(&fakeRecordRepo{upsertErr: tt.repoErr}))
			fn := h.Create
			if tt.param != "" {
				fn = func(c echo.Context) error {
					c.SetParamNames("id")
					c.SetParamValues(tt.param)
					return h.Update(c)
				}
			}

			rec := callRecord(t, fn, tt.method, tt.target, tt.body)

			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantCode, rec.Body.String())
			}
			if tt.wantInBody != "" && !strings.Contains(rec.Body.String(), tt.wantInBody) {
				t.Errorf("body = %s, want it to contain %q", rec.Body.String(), tt.wantInBody)
			}
			for _, leaked := range []string{"SQLSTATE", "records_progress_unit", "upsert record"} {
				if strings.Contains(rec.Body.String(), leaked) {
					t.Errorf("body leaks DB detail %q: %s", leaked, rec.Body.String())
				}
			}
		})
	}
}
