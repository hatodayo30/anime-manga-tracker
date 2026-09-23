package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// NewStaticHandler は webDir 配下（Reactのビルド成果物）を配信するハンドラを返す。
// リクエストされたパスに一致するファイルが無い場合はindex.htmlを返す
// （React Routerによるクライアントサイドルーティングのため）。ただし/api/配下は、
// 登録済みルートに一致しなかった場合でもindex.htmlへフォールバックさせず404のままにする。
// Cache-Control: no-cache を付け、ブラウザが更新確認なしに古いJS/CSSを
// 使い回す（ヒューリスティックキャッシュ）のを防ぐ。ファイル自体は毎回配信されるが、
// ETag/Last-Modifiedによる条件付きGETは効くので304で済む場合も多い。
func NewStaticHandler(webDir string) http.Handler {
	fileServer := http.FileServer(http.Dir(webDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")

		path := filepath.Join(webDir, filepath.Clean(r.URL.Path))
		if _, err := os.Stat(path); os.IsNotExist(err) && !strings.HasPrefix(r.URL.Path, "/api/") {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
