package api

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"runtime/debug"
	"strings"
)

// assetETag 计算内嵌前端资源的内容指纹（强 ETag）。
// 取 SHA-256 前 8 字节，足够区分构建版本；前端一改，ETag 必然变。
func assetETag(data []byte) string {
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:8]) + `"`
}

// loadAssetETags 启动时把内嵌资源逐个算一遍指纹，请求期不再做哈希。
func loadAssetETags(fsys fs.FS) (map[string]string, error) {
	etags := make(map[string]string)
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		etags[p] = assetETag(data)
		return nil
	})
	return etags, err
}

// etagMatches 判断 If-None-Match 是否命中当前资源，支持弱校验与通配符。
func etagMatches(header, etag string) bool {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "*" || strings.TrimPrefix(part, "W/") == etag {
			return true
		}
	}
	return false
}

// BuildRevision 返回编译时注入的 Git 提交号，用于确认线上跑的到底是哪次构建。
// 走 go build 的 vcs 标记；-buildvcs=false 或非 Git 环境时返回 unknown。
func BuildRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			if len(s.Value) > 12 {
				return s.Value[:12]
			}
			return s.Value
		}
	}
	return "unknown"
}
