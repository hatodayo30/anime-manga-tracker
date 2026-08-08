package handler

import "net/http"

// NewStaticHandler は web/ 配下（素のHTML/CSS/JS）を配信するハンドラを返す。
func NewStaticHandler(webDir string) http.Handler {
	return http.FileServer(http.Dir(webDir))
}
