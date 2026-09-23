package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/middleware"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/record"
)

// RecordHandler が扱うエンドポイントはすべてログイン必須（middleware.Auth.RequireUser でラップされる）。
type RecordHandler struct {
	usecase *record.Usecase
}

func NewRecordHandler(u *record.Usecase) *RecordHandler {
	return &RecordHandler{usecase: u}
}

// List handles GET /api/records?type=anime&status=active
func (h *RecordHandler) List(c echo.Context) error {
	userID := middleware.UserFromContext(c).ID
	mediaType := domain.MediaType(c.QueryParam("type"))
	status := domain.Status(c.QueryParam("status"))

	records, err := h.usecase.List(c.Request().Context(), userID, mediaType, status)
	if err != nil {
		return writeError(c, http.StatusBadRequest, err.Error())
	}
	if records == nil {
		records = []*domain.Record{}
	}
	return c.JSON(http.StatusOK, records)
}

// Create handles POST /api/records — 検索結果をライブラリに追加、またはステータス変更。
func (h *RecordHandler) Create(c echo.Context) error {
	userID := middleware.UserFromContext(c).ID

	var in domain.NewRecordInput
	if err := c.Bind(&in); err != nil {
		return writeError(c, http.StatusBadRequest, "invalid request body")
	}

	rec, err := h.usecase.AddOrUpdateStatus(c.Request().Context(), userID, in)
	if err != nil {
		return writeError(c, http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusCreated, rec)
}

// Update handles PATCH /api/records/:id
func (h *RecordHandler) Update(c echo.Context) error {
	userID := middleware.UserFromContext(c).ID

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "invalid id")
	}

	var in domain.UpdateRecordInput
	if err := c.Bind(&in); err != nil {
		return writeError(c, http.StatusBadRequest, "invalid request body")
	}

	rec, err := h.usecase.UpdateProgressOrStatus(c.Request().Context(), userID, id, in)
	if err != nil {
		if errors.Is(err, record.ErrNotFound) {
			return writeError(c, http.StatusNotFound, "record not found")
		}
		return writeError(c, http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, rec)
}

// Delete handles DELETE /api/records/:id — 詳細モーダルの「記録から外す」で使う。
func (h *RecordHandler) Delete(c echo.Context) error {
	userID := middleware.UserFromContext(c).ID

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "invalid id")
	}

	if err := h.usecase.Delete(c.Request().Context(), userID, id); err != nil {
		if errors.Is(err, record.ErrNotFound) {
			return writeError(c, http.StatusNotFound, "record not found")
		}
		return writeError(c, http.StatusBadRequest, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}
