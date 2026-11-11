// Package auth 提供单用户密码登录、内存 Session 管理与 Gin 认证中间件。
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

const (
	// CookieName 会话 Cookie 名称
	CookieName = "myaudio_session"
	// maxFailures 触发锁定的连续失败次数
	maxFailures = 5
	// lockDuration 锁定时长
	lockDuration = 10 * time.Minute
	// failRecordTTL 失败记录在“无锁定且长期无活动”后的保留时长，用于 GC 回收，避免 map 无限增长
	failRecordTTL = 30 * time.Minute
)

var (
	ErrInvalidPassword = errors.New("密码错误")
	ErrTooManyAttempts = errors.New("失败次数过多，请 10 分钟后再试")
)

// session 单个登录会话
type session struct {
	expiresAt time.Time
}

// failRecord 按 IP 记录失败与锁定状态
type failRecord struct {
	fails     int
	lockUntil time.Time
	lastSeen  time.Time // 最近一次失败时间，供 GC 判断无活动记录
}

// Manager 认证管理器（内存实现，单实例部署足够；重启后需重新登录）
type Manager struct {
	hash     []byte
	ttl      time.Duration
	mu       sync.RWMutex
	sessions map[string]session
	failMu   sync.Mutex
	failures map[string]*failRecord
	quit     chan struct{}
}

// New 创建认证管理器。passwordHash 为空时用明文 password 生成 bcrypt 哈希。
func New(password, passwordHash string, ttl time.Duration) (*Manager, error) {
	var hash []byte
	if passwordHash != "" {
		hash = []byte(passwordHash)
	} else {
		h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		hash = h
	}
	m := &Manager{
		hash:     hash,
		ttl:      ttl,
		sessions: make(map[string]session),
		failures: make(map[string]*failRecord),
		quit:     make(chan struct{}),
	}
	go m.gcLoop()
	return m, nil
}

// Hash 返回当前 bcrypt 哈希（首次启动写回配置用）
func (m *Manager) Hash() string { return string(m.hash) }

// Login 校验密码，成功则创建会话并返回 Cookie 值；失败按 IP 累计计数
func (m *Manager) Login(ip, password string) (string, error) {
	m.failMu.Lock()
	rec := m.failures[ip]
	if rec != nil && time.Now().Before(rec.lockUntil) {
		m.failMu.Unlock()
		return "", ErrTooManyAttempts
	}
	m.failMu.Unlock()

	if err := bcrypt.CompareHashAndPassword(m.hash, []byte(password)); err != nil {
		m.recordFailure(ip)
		return "", ErrInvalidPassword
	}

	m.clearFailures(ip)
	token, err := randomToken(32)
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	m.sessions[token] = session{expiresAt: time.Now().Add(m.ttl)}
	m.mu.Unlock()
	return token, nil
}

// Logout 注销会话
func (m *Manager) Logout(token string) {
	m.mu.Lock()
	delete(m.sessions, token)
	m.mu.Unlock()
}

// Valid 校验会话是否有效，有效时自动续期
func (m *Manager) Valid(token string) bool {
	if token == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[token]
	if !ok || time.Now().After(s.expiresAt) {
		delete(m.sessions, token)
		return false
	}
	m.sessions[token] = session{expiresAt: time.Now().Add(m.ttl)}
	return true
}

// Middleware Gin 路由保护：未登录返回 401（API）或跳转登录页（浏览器页面）
func (m *Manager) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(CookieName)
		if err != nil || !m.Valid(token) {
			if isAPIRequest(c) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "UNAUTHORIZED", "error": "UNAUTHORIZED"})
				return
			}
			// 临时跳转并禁止缓存，防止浏览器记住重定向导致登录态异常时的跳转死循环
			c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
			c.Header("Pragma", "no-cache")
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		c.Next()
	}
}

// SetCookie 下发会话 Cookie。SameSite=Lax 兼顾跨站顶层导航与 CSRF 防护；
// HttpOnly 防止 JS 读取，Secure 由部署在 HTTPS 反代后按需开启（当前反代负责 TLS）。
func SetCookie(c *gin.Context, token string, ttl time.Duration) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearCookie 清除会话 Cookie
func ClearCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (m *Manager) recordFailure(ip string) {
	m.failMu.Lock()
	defer m.failMu.Unlock()
	rec := m.failures[ip]
	if rec == nil {
		rec = &failRecord{}
		m.failures[ip] = rec
	}
	rec.fails++
	rec.lastSeen = time.Now()
	if rec.fails >= maxFailures {
		rec.lockUntil = time.Now().Add(lockDuration)
		rec.fails = 0
	}
}

func (m *Manager) clearFailures(ip string) {
	m.failMu.Lock()
	delete(m.failures, ip)
	m.failMu.Unlock()
}

// gcLoop 定期清理过期会话与失败记录
func (m *Manager) gcLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-m.quit:
			return
		case now := <-ticker.C:
			m.mu.Lock()
			for k, s := range m.sessions {
				if now.After(s.expiresAt) {
					delete(m.sessions, k)
				}
			}
			m.mu.Unlock()
			m.failMu.Lock()
			for ip, rec := range m.failures {
				// 仍在锁定期内：保留；否则仅当长期无活动才回收，避免 map 随失败 IP 无限增长。
				if now.Before(rec.lockUntil) {
					continue
				}
				if now.Sub(rec.lastSeen) > failRecordTTL {
					delete(m.failures, ip)
				}
			}
			m.failMu.Unlock()
		}
	}
}

// Close 停止后台清理
func (m *Manager) Close() { close(m.quit) }

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func isAPIRequest(c *gin.Context) bool {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		return true
	}
	return c.GetHeader("Accept") == "application/json" ||
		c.GetHeader("X-Requested-With") == "XMLHttpRequest"
}
