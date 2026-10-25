package handler_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/handler"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/volumes"
)

// 既刊の巻数の title の入力を検証することを保証する。空と長すぎる題名は楽天に問い合わせずに 400 にする。
// 楽天のキーが無い環境は 200 で configured=false を返す。
func TestLatestVolume_ValidatesTitle(t *testing.T) {
	h := handler.NewVolumesHandler(volumes.NewUsecase(nil, time.Now))

	tests := []struct {
		name     string
		title    string
		wantCode int
	}{
		{name: "空は弾く", title: "", wantCode: http.StatusBadRequest},
		{name: "上限を超える長さは弾く", title: strings.Repeat("あ", 101), wantCode: http.StatusBadRequest},
		{name: "上限ちょうどは通す（キー無しなので未設定を返す）", title: strings.Repeat("あ", 100), wantCode: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := call(t, h.LatestVolume, "/api/books/latest-volume?title="+url.QueryEscape(tt.title))
			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantCode, rec.Body.String())
			}
		})
	}
}
