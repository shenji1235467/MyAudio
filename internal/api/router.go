// Package api 组装 Gin 路由与 HTTP Handler。
package api

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
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
	cfg        *config.Config
	store      *store.Store
	auth       *auth.Manager
	scanner    *scanner.Scanner
	webFS      fs.FS             // 嵌入式前端资源
	assetETags map[string]string // 资源内容指纹：内容一变 ETag 就变
}

// NewHandler 创建 Handler
func NewHandler(cfg *config.Config, st *store.Store, am *auth.Manager, sc *scanner.Scanner, webFS fs.FS) *Handler {
	// 内嵌资源启动即算指纹，请求期只做字符串比对
	etags, err := loadAssetETags(webFS)
	if err != nil {
		etags = map[string]string{}
	}
	return &Handler{cfg: cfg, store: st, auth: am, scanner: sc, webFS: webFS, assetETags: etags}
}

// Register 注册全部路由
func (h *Handler) Register(r *gin.Engine) {
	// 健康检查（无需登录，便于 systemd/探活）。
	// 顺带回报前端资源指纹：怀疑"改了没生效"时，比对它就知道跑的是哪次构建。
	r.GET("/healthz", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{
			"status":   "ok",
			"revision": BuildRevision(),
			"assets":   h.assetETags["style.css"],
		})
	})

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

		// 服务器端播放位置记忆（跨设备同步）
		api.GET("/positions", h.listPositions)
		api.GET("/positions/:id", h.getPosition)
		api.PUT("/positions/:id", h.putPosition)
		api.DELETE("/positions/:id", h.deletePosition)

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
	"style.css":        true,
	"app.js":           true,
	"i18n.js":          true,
	"favicon.ico":      true,
	"locales/zh-CN.js": true,
	"locales/en.js":    true,
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
		h.serveAsset(c, p)
		return
	}

	// 未知路径：SPA 回退到 index.html（需登录，未登录由中间件跳转 /login）
	h.auth.Middleware()(c)
	if c.IsAborted() {
		return
	}
	h.serveIndex(c)
}

// serveIndex 返回主页。HTML 一律不缓存：页面结构随版本变化，缓存住旧页面
// 就会引用旧版 style.css/app.js，出现"改了前端却没生效"的假象。
func (h *Handler) serveIndex(c *gin.Context) {
	h.serveNoStoreHTML(c, "index.html")
}

// serveNoStoreHTML 内联返回页面并强制不缓存。
// 用内联而非 FileFromFS，是因为 http.FileServer 对索引/目录会 301 到 "./"，
// 在根路径 "/" 上会与自身形成重定向死循环。
func (h *Handler) serveNoStoreHTML(c *gin.Context, name string) {
	data, err := fs.ReadFile(h.webFS, name)
	if err != nil {
		c.String(http.StatusInternalServerError, "frontend asset missing")
		return
	}
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Data(http.StatusOK, "text/html; charset=utf-8", h.versionedHTML(data))
}

// versionedAssets HTML 中引用到的静态资源（路径相对内嵌根目录）
var versionedAssets = []string{
	"style.css", "app.js", "i18n.js", "locales/zh-CN.js", "locales/en.js",
}

// versionedHTML 给页面里的静态资源 URL 挂上内容指纹。
// 浏览器以完整 URL 为缓存键：CSS/JS 内容一变 URL 就变，用户升级后
// 必定拿到新资源，不会被旧缓存钉住。与 serveAsset 的 ETag 互为双保险。
func (h *Handler) versionedHTML(html []byte) []byte {
	s := string(html)
	for _, name := range versionedAssets {
		etag := h.assetETags[name]
		if etag == "" {
			continue
		}
		s = strings.ReplaceAll(s, `"/`+name+`"`, `"/`+name+`?v=`+strings.Trim(etag, `"`)+`"`)
	}
	return []byte(s)
}

// serveAsset 返回静态资源：带 ETag 走协商缓存。
// 前端资源被打进二进制、文件名不带内容哈希，所以不能用长 max-age：
// 一旦缓存住旧 CSS，用户升级后仍会看到旧布局。这里用 no-cache + ETag，
// 每次请求回源校验，内容没变就走 304，既不浪费带宽也不会锁死旧版本。
func (h *Handler) serveAsset(c *gin.Context, p string) {
	data, err := fs.ReadFile(h.webFS, p)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	etag := h.assetETags[p]
	if etag == "" {
		etag = assetETag(data)
	}
	c.Header("Cache-Control", "no-cache")
	c.Header("ETag", etag)
	if etagMatches(c.GetHeader("If-None-Match"), etag) {
		c.Status(http.StatusNotModified)
		return
	}
	ctype := mime.TypeByExtension(path.Ext(p))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	c.Data(http.StatusOK, ctype, data)
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
	h.serveNoStoreHTML(c, "login.html")
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
