package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"myaudio/internal/model"
)

// listPositions GET /api/positions —— 返回全部已记忆的播放位置（按最近更新倒序）
func (h *Handler) listPositions(c *gin.Context) {
	list, err := h.store.ListPlayPositions()
	if err != nil {
		apiErr(c, http.StatusInternalServerError, codeQueryFailed, err.Error())
		return
	}
	if list == nil {
		list = []model.PlayPosition{}
	}
	c.JSON(http.StatusOK, list)
}

// getPosition GET /api/positions/:id —— 返回单条播放位置（无记录时 position=0）
func (h *Handler) getPosition(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	pos, err := h.store.GetPlayPosition(id)
	if err != nil {
		apiErr(c, http.StatusInternalServerError, codeQueryFailed, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"track_id": id, "position": pos})
}

// putPosition PUT /api/positions/:id —— 保存（更新）某曲目的播放位置
func (h *Handler) putPosition(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	var in model.SavePositionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		apiErr(c, http.StatusBadRequest, codeBadRequest, err.Error())
		return
	}
	if in.Position < 0 {
		apiErr(c, http.StatusBadRequest, codeInvalidPosition, "position must be >= 0")
		return
	}
	// 曲目必须存在，否则外键约束失败 → 视为 404
	if t, gerr := h.store.GetTrack(id); errors.Is(gerr, sql.ErrNoRows) || (gerr == nil && t == nil) {
		apiErr(c, http.StatusNotFound, codeTrackNotFound, "")
		return
	} else if gerr != nil {
		apiErr(c, http.StatusInternalServerError, codeQueryFailed, gerr.Error())
		return
	}
	if err := h.store.UpsertPlayPosition(id, in.Position); err != nil {
		if strings.Contains(err.Error(), "FOREIGN KEY") || strings.Contains(err.Error(), "constraint") {
			apiErr(c, http.StatusNotFound, codeTrackNotFound, "")
			return
		}
		apiErr(c, http.StatusInternalServerError, codeQueryFailed, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// deletePosition DELETE /api/positions/:id —— 清除某曲目的播放位置（完整播完时调用）
func (h *Handler) deletePosition(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if err := h.store.DeletePlayPosition(id); err != nil {
		apiErr(c, http.StatusInternalServerError, codeQueryFailed, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
