// Package scanner 负责递归扫描本地音频目录，解析标签与封面，增量同步到 SQLite。
package scanner

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dhowden/tag"

	"myaudio/internal/model"
	"myaudio/internal/store"
)

// 支持的音频扩展名（小写）
var audioExts = map[string]bool{
	".mp3": true, ".wav": true, ".flac": true,
	".ogg": true, ".oga": true, ".m4a": true, ".aac": true,
}

// Scanner 目录扫描器
type Scanner struct {
	musicDir string
	coverDir string
	store    *store.Store

	mu      sync.Mutex
	running bool
}

// New 创建扫描器
func New(musicDir, coverDir string, st *store.Store) *Scanner {
	return &Scanner{musicDir: musicDir, coverDir: coverDir, store: st}
}

// IsRunning 是否正在扫描（前端手动触发时防重入）
func (s *Scanner) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// StartTicker 启动定时扫描；interval<=0 时仅保留手动扫描
func (s *Scanner) StartTicker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := s.Scan(ctx); err != nil {
					log.Printf("[scanner] 定时扫描失败: %v", err)
				}
			}
		}
	}()
}

// Scan 执行一次完整增量扫描
func (s *Scanner) Scan(ctx context.Context) (model.ScanResult, error) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return model.ScanResult{}, errors.New("扫描任务正在进行中")
	}
	s.running = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	start := time.Now()
	res := model.ScanResult{}

	// 1. 发现全部音频文件
	paths, err := s.discover()
	if err != nil {
		return res, err
	}
	res.Scanned = len(paths)

	// 2. 受限并发解析标签（避免大目录下 fd/内存飙升）
	workers := runtime.NumCPU()
	if workers > 4 {
		workers = 4
	}
	// 带缓冲 channel：容量等于任务数，投递永不阻塞，从根源避免 worker 提前退出
	// （如 ctx 取消）导致主流程在“向无缓冲 channel 发送”上永久卡死、running 不复位。
	jobs := make(chan string, len(paths))
	var wg sync.WaitGroup
	var added, updated int64

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				if ctx.Err() != nil {
					// 排空剩余任务后退出，保证 channel 可被正常关闭
					continue
				}
				t, err := s.parseFile(path)
				if err != nil {
					log.Printf("[scanner] 解析失败 %s: %v", path, err)
					continue
				}
				isAdd, err := s.store.UpsertTrack(t)
				if err != nil {
					log.Printf("[scanner] 入库失败 %s: %v", path, err)
					continue
				}
				if isAdd {
					atomic.AddInt64(&added, 1)
				} else {
					atomic.AddInt64(&updated, 1)
				}
			}
		}()
	}
	// 缓冲容量=任务数，这里的投递不会阻塞；投递与等待放在同一流程，确保必能走到收尾
	for _, p := range paths {
		jobs <- p
	}
	close(jobs)
	wg.Wait()
	res.Added, res.Updated = int(added), int(updated)

	// 3. 清理库中已不存在的文件
	known, err := s.store.AllTrackPaths()
	if err != nil {
		return res, err
	}
	exist := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		exist[p] = struct{}{}
	}
	var staleIDs []int64
	for path, id := range known {
		if _, ok := exist[path]; !ok {
			staleIDs = append(staleIDs, id)
		}
	}
	if err := s.store.DeleteMissing(staleIDs); err != nil {
		log.Printf("[scanner] 清理失效记录失败: %v", err)
	}
	res.Removed = len(staleIDs)
	res.DurationMS = time.Since(start).Milliseconds()

	log.Printf("[scanner] 扫描完成: 发现 %d, 新增 %d, 更新 %d, 移除 %d, 耗时 %dms",
		res.Scanned, res.Added, res.Updated, res.Removed, res.DurationMS)
	return res, nil
}

// discover 递归遍历音频目录
func (s *Scanner) discover() ([]string, error) {
	var paths []string
	err := filepath.WalkDir(s.musicDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("[scanner] 遍历跳过 %s: %v", path, err)
			return nil // 单个文件/目录失败不中断整体扫描
		}
		if d.IsDir() {
			return nil
		}
		if audioExts[strings.ToLower(filepath.Ext(path))] {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("音频目录不存在: " + s.musicDir)
	}
	return paths, err
}

// parseFile 解析单个文件：标签、时长、封面
func (s *Scanner) parseFile(path string) (*model.Track, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}

	t := &model.Track{
		Path:     path,
		Filename: filepath.Base(path),
		Title:    strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		Format:   strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."),
		Size:     stat.Size(),
		MTime:    stat.ModTime(),
	}
	// 相对音频根目录的所在目录（POSIX 风格）；文件位于根目录时 Dir 为空串。
	if rel, err := filepath.Rel(s.musicDir, path); err == nil {
		rel = filepath.ToSlash(rel)
		if i := strings.LastIndex(rel, "/"); i >= 0 {
			t.Dir = rel[:i]
		}
	}

	// 时长：WAV 可从文件头精确读取；其余格式按平均码率估算
	if t.Format == "wav" {
		if d, err := wavDuration(path); err == nil {
			t.Duration = d
		} else {
			t.Duration = estimateDuration(t.Format, t.Size)
		}
	} else {
		t.Duration = estimateDuration(t.Format, t.Size)
	}

	// 标签解析失败（常见于裸 WAV）不报错，仅使用文件名作为标题
	if md, err := tag.ReadFrom(f); err == nil {
		if v := md.Title(); v != "" {
			t.Title = v
		}
		t.Artist = md.Artist()
		t.Album = md.Album()

		// 内嵌封面：提取落盘，供 /api/cover/:id 提供
		if pic := md.Picture(); pic != nil && len(pic.Data) > 0 {
			if err := s.saveCover(t, pic.Data, pic.Ext); err == nil {
				t.HasCover = true
			}
		}
	}
	return t, nil
}

// saveCover 封面按 track 路径哈希命名；此处先落盘，Upsert 后可按 id 重命名。
// 为简化链路，直接以文件路径的稳定哈希命名，API 侧通过 track id 反查路径后读取。
func (s *Scanner) saveCover(t *model.Track, data []byte, ext string) error {
	if ext == "" {
		ext = ".jpg"
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	name := coverName(t.Path) + ext
	dst := filepath.Join(s.coverDir, name)
	return os.WriteFile(dst, data, 0o644)
}

// CoverPath 根据曲目本地路径返回封面文件路径（存在时）
func CoverPath(coverDir, trackPath string) (string, bool) {
	entries, err := os.ReadDir(coverDir)
	if err != nil {
		return "", false
	}
	base := coverName(trackPath) + "."
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), base) {
			return filepath.Join(coverDir, e.Name()), true
		}
	}
	return "", false
}

// estimateDuration 标签库不提供时长时的粗略估算（秒）
func estimateDuration(format string, size int64) float64 {
	// 常见格式平均码率（字节/秒），仅用于无标签兜底展示
	kbps := map[string]int{
		"mp3": 192, "m4a": 256, "aac": 192,
		"ogg": 192, "oga": 192, "flac": 900, "wav": 1411,
	}[format]
	if kbps == 0 {
		kbps = 192
	}
	return float64(size) / float64(kbps*128)
}
