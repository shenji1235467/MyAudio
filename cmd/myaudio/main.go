// MyAudio 服务入口：加载配置 → 初始化存储/认证/扫描器 → 启动 HTTP 服务 → 优雅退出。
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"myaudio/internal/api"
	"myaudio/internal/auth"
	"myaudio/internal/config"
	"myaudio/internal/scanner"
	"myaudio/internal/store"
	"myaudio/web"
)

func main() {
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	// 1. 配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("[fatal] 加载配置失败: %v", err)
	}

	// 2. 存储层
	st, err := store.New(cfg.DBPath())
	if err != nil {
		log.Fatalf("[fatal] 初始化数据库失败: %v", err)
	}
	defer st.Close()

	// 3. 认证（明文密码在首次启动时转为 bcrypt 哈希并写回配置）
	am, err := auth.New(cfg.Password, cfg.PasswordHash, cfg.SessionTTL)
	if err != nil {
		log.Fatalf("[fatal] 初始化认证失败: %v", err)
	}
	defer am.Close()
	if cfg.PasswordHash == "" {
		cfg.PasswordHash = am.Hash()
		cfg.Password = ""
		if err := cfg.Save(*configPath); err != nil {
			log.Printf("[warn] 密码哈希回写配置失败: %v", err)
		} else {
			log.Printf("[auth] 密码已转为 bcrypt 哈希并写回 %s", *configPath)
		}
	}

	// 4. 扫描器：启动即扫描 + 定时扫描
	sc := scanner.New(cfg.MusicDir, cfg.CoverDir(), st)
	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		result, err := sc.Scan(rootCtx)
		if err != nil {
			log.Printf("[warn] 启动扫描失败: %v", err)
			return
		}
		log.Printf("[scanner] 启动扫描完成: 发现 %d, 新增 %d, 移除 %d",
			result.Scanned, result.Added, result.Removed)
	}()
	sc.StartTicker(rootCtx, cfg.ScanInterval)

	// 5. 嵌入式前端（资源已在 web 包根目录，无需再剥离前缀）
	webFS := web.Assets

	// 6. HTTP 服务
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.Use(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Next()
	})

	h := api.NewHandler(cfg, st, am, sc, webFS)
	h.Register(r)

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       0, // 流式播放可能持续较久，不设整体读超时
		WriteTimeout:      0,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("[server] MyAudio 监听 %s （音频目录: %s）", cfg.Listen, cfg.MusicDir)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[fatal] HTTP 服务异常: %v", err)
		}
	}()

	// 7. 优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Printf("[server] 收到退出信号，正在关闭…")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[warn] 优雅关闭超时: %v", err)
	}
	log.Printf("[server] 已退出")
}
