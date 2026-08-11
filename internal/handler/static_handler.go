package handler

import "net/http"

// NewStaticHandler は web/ 配下（素のHTML/CSS/JS）を配信するハンドラを返す。
// Cache-Control: no-cache を付け、ブラウザが更新確認なしに古いJS/CSSを
// 使い回す（ヒューリスティックキャッシュ）のを防ぐ。ファイル自体は毎回配信されるが、
// ETag/Last-Modifiedによる条件付きGETは効くので304で済む場合も多い。
func NewStaticHandler(webDir string) http.Handler {
	fileServer := http.FileServer(http.Dir(webDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		fileServer.ServeHTTP(w, r)
	})
}
