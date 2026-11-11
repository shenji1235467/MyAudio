package api

import (
	"context"
	"database/sql"
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
		apiErr(c, http.StatusInternalServerError, codeQueryFailed, err.Error())
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
	if errors.Is(err, sql.ErrNoRows) || (err == nil && t == nil) {
		apiErr(c, http.StatusNotFound, codeTrackNotFound, "")
		return
	}
	if err != nil {
		apiErr(c, http.StatusInternalServerError, codeQueryFailed, err.Error())
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
	if errors.Is(err, sql.ErrNoRows) || (err == nil && t == nil) {
		apiErr(c, http.StatusNotFound, codeTrackNotFound, "")
		return
	}
	if err != nil {
		apiErr(c, http.StatusInternalServerError, codeQueryFailed, err.Error())
		return
	}
	deleteFile := c.Query("delete_file") == "true" || c.Query("delete_file") == "1"
	if err := h.store.DeleteTrack(id, deleteFile); err != nil {
		apiErr(c, http.StatusInternalServerError, codeQueryFailed, err.Error())
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
	if errors.Is(err, sql.ErrNoRows) || (err == nil && t == nil) {
		apiErr(c, http.StatusNotFound, codeTrackNotFound, "")
		return
	}
	if err != nil {
		apiErr(c, http.StatusInternalServerError, codeQueryFailed, err.Error())
		return
	}
	f, err := os.Open(t.Path)
	if err != nil {
		// 记录存在但磁盘文件缺失：对客户端而言资源确不可用，返回 404 合理
		apiErr(c, http.StatusNotFound, codeFileMissing, "")
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		apiErr(c, http.StatusInternalServerError, codeQueryFailed, err.Error())
		return
	}

	c.Header("Accept-Ranges", "bytes")
	c.Header("Cache-Control", "private, max-age=3600")
	// ASCII 兜底文件名，避免非 ASCII 在 Content-Disposition 中出错（与标题无关，恒设置）
	c.Header("Content-Disposition", `inline; filename="track-`+strconv.FormatInt(id, 10)+`.`+t.Format+`"`)
	// http.ServeContent 自动处理 Range、If-Modified-Since 与 206/304，Content-Range 单位由标准库写为 bytes
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
	if errors.Is(err, sql.ErrNoRows) || (err == nil && t == nil) {
		c.Status(http.StatusNotFound)
		return
	}
	if err != nil {
		apiErr(c, http.StatusInternalServerError, codeQueryFailed, err.Error())
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
		apiErr(c, http.StatusInternalServerError, codeQueryFailed, err.Error())
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
		apiErr(c, http.StatusConflict, codeScanInProgress, "")
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
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "message": "scan started"})
}

// ---------------- 辅助 ----------------

func parseID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		apiErr(c, http.StatusBadRequest, codeInvalidID, "")
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
