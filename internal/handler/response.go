package handler

import "github.com/labstack/echo/v4"

// jsonError は echo.Context を使うハンドラ向けの共通エラーレスポンス。
func jsonError(c echo.Context, status int, message string) error {
	return c.JSON(status, map[string]string{"error": message})
}
