// Package config 负责加载与校验服务配置。
// 配置文件为 YAML；首次启动缺少配置或密码时自动生成默认配置与随机密码。
package config

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 服务配置
type Config struct {
	// Listen 监听地址，如 :8083（默认端口）
	Listen string `yaml:"listen"`
	// MusicDir 本地音频目录（递归扫描）
	MusicDir string `yaml:"music_dir"`
	// DataDir 数据库与封面缓存目录
	DataDir string `yaml:"data_dir"`
	// Password 明文密码（仅用于首次写入；运行期校验使用 PasswordHash）
	Password string `yaml:"password,omitempty"`
	// PasswordHash bcrypt 哈希，优先级高于 Password
	PasswordHash string `yaml:"password_hash"`
	// ScanInterval 定时扫描间隔，0 表示关闭定时扫描
	ScanInterval time.Duration `yaml:"scan_interval"`
	// SessionTTL 会话有效期
	SessionTTL time.Duration `yaml:"session_ttl"`
	// MaxUploadMB 预留：后续网页上传功能的单文件大小上限
	MaxUploadMB int `yaml:"max_upload_mb"`
}

// Default 返回带默认值的配置
func Default(baseDir string) *Config {
	return &Config{
		Listen:       ":8083",
		MusicDir:     filepath.Join(baseDir, "data", "music"),
		DataDir:      filepath.Join(baseDir, "data"),
		ScanInterval: 5 * time.Minute,
		SessionTTL:   7 * 24 * time.Hour,
		MaxUploadMB:  100,
	}
}

// Load 读取配置文件；文件不存在时生成默认配置（含随机密码）并落盘
func Load(path string) (*Config, error) {
	baseDir := filepath.Dir(path)
	cfg := Default(baseDir)

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
		}
	case errors.Is(err, os.ErrNotExist):
		// 生成随机初始密码，并为首次运行赋予定时扫描默认值（0 才表示关闭）
		cfg.Password = randomPassword(10)
		cfg.ScanInterval = 5 * time.Minute
		if err := cfg.Save(path); err != nil {
			return nil, err
		}
		fmt.Printf("[config] 未发现配置文件，已生成默认配置: %s\n", path)
		fmt.Printf("[config] 初始访问密码: %s （请尽快修改 config.yaml 中的 password 后重启）\n", cfg.Password)
	default:
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	if err := cfg.normalizeAndCheck(baseDir); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Save 将配置写回文件（权限 0600，避免密码泄露）
func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	out, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o600)
}

// normalizeAndCheck 补全默认值、创建必要目录并校验关键字段
func (c *Config) normalizeAndCheck(baseDir string) error {
	if c.Listen == "" {
		c.Listen = ":8083"
	}
	if c.MusicDir == "" {
		c.MusicDir = filepath.Join(baseDir, "data", "music")
	}
	if c.DataDir == "" {
		c.DataDir = filepath.Join(baseDir, "data")
	}
	// 扫描间隔：0 表示关闭定时扫描（仍可手动触发）；负值归一化为 0。
	// 需要定时扫描请在配置中显式填写，如 scan_interval: 5m。
	if c.ScanInterval < 0 {
		c.ScanInterval = 0
	}
	if c.SessionTTL <= 0 {
		c.SessionTTL = 7 * 24 * time.Hour
	}
	if c.MaxUploadMB <= 0 {
		c.MaxUploadMB = 100
	}
	for _, dir := range []string{c.MusicDir, c.DataDir, filepath.Join(c.DataDir, "covers")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", dir, err)
		}
	}
	if c.Password == "" && c.PasswordHash == "" {
		return errors.New("必须配置 password 或 password_hash")
	}
	return nil
}

// randomPassword 生成 URL 安全的随机密码
func randomPassword(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// 极端情况下退化为时间戳派生，保证服务仍可启动
		return fmt.Sprintf("myaudio-%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(b)[:n]
}

// DBPath 返回 SQLite 数据库文件路径
func (c *Config) DBPath() string {
	return filepath.Join(c.DataDir, "myaudio.db")
}

// CoverDir 返回封面缓存目录
func (c *Config) CoverDir() string {
	return filepath.Join(c.DataDir, "covers")
}
