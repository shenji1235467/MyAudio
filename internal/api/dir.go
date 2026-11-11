package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"myaudio/internal/model"
)

// listDirs GET /api/dirs —— 返回曲目所在的相对目录导航（根目录 + 各子目录，含曲目数）
func (h *Handler) listDirs(c *gin.Context) {
	dirs, err := h.store.ListDirs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询目录失败: " + err.Error()})
		return
	}
	if dirs == nil {
		dirs = []model.DirInfo{}
	}
	c.JSON(http.StatusOK, dirs)
}
