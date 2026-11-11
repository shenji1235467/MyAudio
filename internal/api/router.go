// Package api 组装 Gin 路由与 HTTP Handler。
package api

import (
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"myaudio/internal/auth"
	"myaudio/internal/config"
	"myaudio/internal/scanner"
	"myaudio/internal/store"
)

// Handler 聚合各模块依赖
type Handler struct {
	cfg     *config.Config
	store   *store.Store
	auth    *auth.Manager
	scanner *scanner.Scanner
	webFS   fs.FS // 嵌入式前端资源
}

// NewHandler 创建 Handler
func NewHandler(cfg *config.Config, st *store.Store, am *auth.Manager, sc *scanner.Scanner, webFS fs.FS) *Handler {
	return &Handler{cfg: cfg, store: st, auth: am, scanner: sc, webFS: webFS}
}

// Register 注册全部路由
func (h *Handler) Register(r *gin.Engine) {
	// 健康检查（无需登录，便于 systemd/探活）
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	// 登录相关（无需认证）
	r.GET("/login", h.serveLoginPage)
	r.POST("/api/auth/login", h.login)
	r.POST("/api/auth/logout", h.logout)
	r.GET("/api/auth/status", h.authStatus)

	// 受保护 API
	api := r.Group("/api", h.auth.Middleware())
	{
		api.GET("/tracks", h.listTracks)
		api.GET("/dirs", h.listDirs)
		api.GET("/tracks/:id", h.getTrack)
		api.DELETE("/tracks/:id", h.deleteTrack)
		api.GET("/stream/:id", h.stream)
		api.GET("/cover/:id", h.cover)
		api.GET("/stats", h.stats)
		api.POST("/scan", h.triggerScan)

		api.GET("/playlists", h.listPlaylists)
		api.POST("/playlists", h.createPlaylist)
		api.PUT("/playlists/:id", h.renamePlaylist)
		api.DELETE("/playlists/:id", h.deletePlaylist)
		api.GET("/playlists/:id/tracks", h.playlistTracks)
		api.POST("/playlists/:id/tracks/:trackID", h.addPlaylistTrack)
		api.DELETE("/playlists/:id/tracks/:trackID", h.removePlaylistTrack)
	}

	// 静态资源（CSS/JS 等）无需登录即可访问，保证登录页样式可用；
	// 页面路由（HTML/SPA 回退）经认证中间件保护。
	r.NoRoute(h.serveFrontend)
}

// publicAssets 无需认证即可访问的静态资源
var publicAssets = map[string]bool{
	"style.css":           true,
	"app.js":              true,
	"i18n.js":             true,
	"favicon.ico":         true,
	"locales/zh-CN.js":    true,
	"locales/en.js":       true,
}

// serveFrontend 区分静态资源与 SPA 页面：资源放行，页面需登录
func (h *Handler) serveFrontend(c *gin.Context) {
	p := strings.TrimPrefix(c.Request.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}
	// 防路径穿越
	if strings.Contains(p, "..") {
		c.Status(http.StatusNotFound)
		return
	}

	if f, err := h.webFS.Open(p); err == nil {
		f.Close()
		// 登录页本身放行，其余页面需登录
		if !publicAssets[p] && p != "login.html" {
			h.auth.Middleware()(c)
			if c.IsAborted() {
				return
			}
		}
		// 主页 index.html 直接内联返回。不能走 FileFromFS：底层 http.FileServer
		// 对索引/目录会 301 到 "./"，在根路径 "/" 上会与自身形成重定向死循环。
		if p == "index.html" {
			h.serveIndex(c)
			return
		}
		c.FileFromFS(p, http.FS(h.webFS))
		return
	}

	// 未知路径：SPA 回退到 index.html（需登录，未登录由中间件跳转 /login）
	h.auth.Middleware()(c)
	if c.IsAborted() {
		return
	}
	h.serveIndex(c)
}

func (h *Handler) serveIndex(c *gin.Context) {
	data, err := fs.ReadFile(h.webFS, "index.html")
	if err != nil {
		c.String(http.StatusInternalServerError, "frontend asset missing")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

// safeRedirect 统一的临时跳转：强制 no-store，避免浏览器或反代缓存重定向
// （历史上 http.FileServer 产生的 301 永久重定向曾导致 ERR_TOO_MANY_REDIRECTS）。
func safeRedirect(c *gin.Context, location string) {
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Redirect(http.StatusFound, location)
}

func (h *Handler) serveLoginPage(c *gin.Context) {
	// 已登录直接进入主页
	if token, err := c.Cookie(auth.CookieName); err == nil && h.auth.Valid(token) {
		safeRedirect(c, "/")
		return
	}
	data, err := fs.ReadFile(h.webFS, "login.html")
	if err != nil {
		c.String(http.StatusInternalServerError, "login page missing")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

// ---------------- 认证 Handler ----------------

type loginRequest struct {
	Password string `json:"password" binding:"required"`
}

func (h *Handler) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiErr(c, http.StatusBadRequest, codeBadRequest, "")
		return
	}
	token, err := h.auth.Login(c.ClientIP(), req.Password)
	if err != nil {
		if err == auth.ErrTooManyAttempts {
			apiErr(c, http.StatusTooManyRequests, codeTooManyAttempts, "")
			return
		}
		apiErr(c, http.StatusUnauthorized, codeInvalidPassword, "")
		return
	}
	auth.SetCookie(c, token, h.cfg.SessionTTL)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) logout(c *gin.Context) {
	if token, err := c.Cookie(auth.CookieName); err == nil {
		h.auth.Logout(token)
	}
	auth.ClearCookie(c)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) authStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"authenticated": true, "time": time.Now().Unix()})
}
