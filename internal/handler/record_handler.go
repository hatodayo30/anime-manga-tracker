package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/hatodayo30/anime-manga-tracker/internal/middleware"
	"github.com/hatodayo30/anime-manga-tracker/internal/model"
	"github.com/hatodayo30/anime-manga-tracker/internal/repository"
	"github.com/hatodayo30/anime-manga-tracker/internal/service"
)

// RecordHandler が扱うエンドポイントはすべてログイン必須（middleware.Auth.RequireUser でラップされる）。
type RecordHandler struct {
	service *service.RecordService
}

func NewRecordHandler(s *service.RecordService) *RecordHandler {
	return &RecordHandler{service: s}
}

// List handles GET /api/records?type=anime&status=active
func (h *RecordHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserFromContext(r.Context()).ID
	mediaType := model.MediaType(r.URL.Query().Get("type"))
	status := model.Status(r.URL.Query().Get("status"))

	records, err := h.service.List(r.Context(), userID, mediaType, status)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if records == nil {
		records = []*model.Record{}
	}
	writeJSON(w, http.StatusOK, records)
}

// Create handles POST /api/records — 検索結果をライブラリに追加、またはステータス変更。
func (h *RecordHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserFromContext(r.Context()).ID

	var in model.NewRecordInput
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	record, err := h.service.AddOrUpdateStatus(r.Context(), userID, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

// Update handles PATCH /api/records/{id}
func (h *RecordHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserFromContext(r.Context()).ID

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var in model.UpdateRecordInput
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	record, err := h.service.UpdateProgressOrStatus(r.Context(), userID, id, in)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "record not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, record)
}

// Delete handles DELETE /api/records/{id} — 詳細モーダルの「記録から外す」で使う。
func (h *RecordHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserFromContext(r.Context()).ID

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.service.Delete(r.Context(), userID, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "record not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
