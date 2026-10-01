package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// DBPinger は /healthz が必要とするDB疎通確認だけを切り出したもの。
// *pgxpool.Pool がそのまま満たす。
type DBPinger interface {
	Ping(ctx context.Context) error
}

// healthCheckTimeout はDB疎通確認の上限時間。
// ECSのコンテナヘルスチェックは既定5秒でタイムアウトするため、それより短くして
// 「DBが応答しない」ことをこちら側のレスポンスとして返せるようにする。
const healthCheckTimeout = 2 * time.Second

type HealthHandler struct {
	db DBPinger
}

func NewHealthHandler(db DBPinger) *HealthHandler {
	return &HealthHandler{db: db}
}

// Healthz handles GET /healthz
// ECSのコンテナヘルスチェックと、ALBターゲットグループのヘルスチェックから叩かれる。
// 確認はDBの疎通までに留め、AniList / Jikan などの上流APIは叩かない
// （ヘルスチェックのたびに上流のレート制限を消費してしまうため）。
func (h *HealthHandler) Healthz(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), healthCheckTimeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		// 詳細は外部に出さずログにだけ残す。/healthz は未認証で叩けるため。
		c.Logger().Errorf("healthz: ping db: %v", err)
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "unhealthy"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
