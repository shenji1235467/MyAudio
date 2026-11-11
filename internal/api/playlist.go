package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"myaudio/internal/model"
	"myaudio/internal/store"
)

// listPlaylists GET /api/playlists
func (h *Handler) listPlaylists(c *gin.Context) {
	pls, err := h.store.ListPlaylists()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if pls == nil {
		pls = []model.Playlist{}
	}
	c.JSON(http.StatusOK, pls)
}

type playlistRequest struct {
	Name string `json:"name" binding:"required"`
}

// createPlaylist POST /api/playlists
func (h *Handler) createPlaylist(c *gin.Context) {
	var req playlistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "名称不能为空"})
		return
	}
	pl, err := h.store.CreatePlaylist(req.Name)
	if err != nil {
		h.writePlaylistError(c, err)
		return
	}
	c.JSON(http.StatusCreated, pl)
}

// renamePlaylist PUT /api/playlists/:id
func (h *Handler) renamePlaylist(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	var req playlistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "名称不能为空"})
		return
	}
	if err := h.store.RenamePlaylist(id, req.Name); err != nil {
		h.writePlaylistError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// deletePlaylist DELETE /api/playlists/:id
func (h *Handler) deletePlaylist(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	if err := h.store.DeletePlaylist(id); err != nil {
		h.writePlaylistError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// playlistTracks GET /api/playlists/:id/tracks
func (h *Handler) playlistTracks(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		return
	}
	tracks, err := h.store.PlaylistTracks(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tracks)
}

// addPlaylistTrack POST /api/playlists/:id/tracks/:trackID
func (h *Handler) addPlaylistTrack(c *gin.Context) {
	playlistID, err := parseID(c)
	if err != nil {
		return
	}
	trackID, err := strconv.ParseInt(c.Param("trackID"), 10, 64)
	if err != nil || trackID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的曲目 ID"})
		return
	}
	if err := h.store.AddTrackToPlaylist(playlistID, trackID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// removePlaylistTrack DELETE /api/playlists/:id/tracks/:trackID
func (h *Handler) removePlaylistTrack(c *gin.Context) {
	playlistID, err := parseID(c)
	if err != nil {
		return
	}
	trackID, err := strconv.ParseInt(c.Param("trackID"), 10, 64)
	if err != nil || trackID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的曲目 ID"})
		return
	}
	if err := h.store.RemoveTrackFromPlaylist(playlistID, trackID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) writePlaylistError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, store.ErrEmptyName):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, store.ErrPlaylistExists):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, store.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
