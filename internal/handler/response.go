package handler

import "github.com/labstack/echo/v4"

// writeError は共通のエラーレスポンス形式（{"error": message}）を書き込む。
func writeError(c echo.Context, status int, message string) error {
	return c.JSON(status, map[string]string{"error": message})
}
