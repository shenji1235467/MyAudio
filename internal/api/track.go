package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"myaudio/internal/model"
	"myaudio/internal/scanner"
)

// scanTimeout 手动扫描的整体超时兜底，防止任何意外阻塞导致扫描标志永久不复位
const scanTimeout = 10 * time.Minute

// listTracks GET /api/tracks?keyword=&order=&page=&per_page=
func (h *Handler) listTracks(c *gin.Context) {
	q := model.TrackQuery{
		Keyword: c.Query("keyword"),
		Dir:     c.Query("dir"),
		Order:   c.DefaultQuery("order", "created_at"),
	}
	q.Page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	q.PerPage, _ = strconv.Atoi(c.DefaultQuery("per_page", "48"))

	result, err := h.store.ListTracks(q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// getTrack GET /api/tracks/:id
func (h *Handler) getTrack(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	t, err := h.store.GetTrack(id)
	if errors.Is(err, os.ErrNotExist) || t == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "曲目不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, t)
}

// deleteTrack DELETE /api/tracks/:id?delete_file=true
func (h *Handler) deleteTrack(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	// 删除前取出曲目，便于清理封面缓存
	t, err := h.store.GetTrack(id)
	if err != nil || t == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "曲目不存在"})
		return
	}
	deleteFile := c.Query("delete_file") == "true" || c.Query("delete_file") == "1"
	if err := h.store.DeleteTrack(id, deleteFile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if coverPath, ok := scanner.CoverPath(h.cfg.CoverDir(), t.Path); ok {
		_ = os.Remove(coverPath)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// stream GET /api/stream/:id —— 支持 Range 的流式播放（拖动进度/断点续播）
func (h *Handler) stream(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	t, err := h.store.GetTrack(id)
	if err != nil || t == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "曲目不存在"})
		return
	}
	f, err := os.Open(t.Path)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "音频文件不存在或已被移动"})
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Accept-Ranges", "bytes")
	c.Header("Cache-Control", "private, max-age=3600")
	if t.Title != "" {
		// 文件名 ASCII 兜底，避免非 ASCII 在 Content-Disposition 中出错
		c.Header("Content-Disposition", `inline; filename="track-`+strconv.FormatInt(id, 10)+`."`+t.Format)
	}
	// http.ServeContent 自动处理 Range、If-Modified-Since 与 206/304
	c.Header("Content-Type", mimeByFormat(t.Format))
	http.ServeContent(c.Writer, c.Request, stat.Name(), stat.ModTime(), f)
}

// cover GET /api/cover/:id —— 内嵌封面（带浏览器缓存）
func (h *Handler) cover(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	t, err := h.store.GetTrack(id)
	if err != nil || t == nil {
		c.Status(http.StatusNotFound)
		return
	}
	path, ok := scanner.CoverPath(h.cfg.CoverDir(), t.Path)
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "private, max-age=86400")
	c.File(path)
}

// stats GET /api/stats
func (h *Handler) stats(c *gin.Context) {
	count, duration, err := h.store.Stats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"track_count":    count,
		"total_duration": duration,
		"scanning":       h.scanner.IsRunning(),
	})
}

// triggerScan POST /api/scan —— 手动触发扫描（防重入）
func (h *Handler) triggerScan(c *gin.Context) {
	if h.scanner.IsRunning() {
		c.JSON(http.StatusConflict, gin.H{"error": "扫描任务正在进行中"})
		return
	}
	// 必须使用独立 context：c.Request.Context() 会在 HTTP 响应返回后被取消，
	// 若传给后台 goroutine，worker 会因 ctx 取消而提前退出，进而可能让扫描流程卡死。
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
		defer cancel()
		if _, err := h.scanner.Scan(ctx); err != nil {
			_ = err // 错误已在扫描器内记录日志
		}
	}()
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "message": "扫描已启动"})
}

// ---------------- 辅助 ----------------

func parseID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return 0, errors.New("invalid id")
	}
	return id, nil
}

func mimeByFormat(format string) string {
	switch strings.ToLower(format) {
	case "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "flac":
		return "audio/flac"
	case "ogg", "oga":
		return "audio/ogg"
	case "m4a", "aac":
		return "audio/mp4"
	default:
		return "application/octet-stream"
	}
}
