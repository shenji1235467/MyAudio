// Package model 定义业务实体与传输对象。
package model

import "time"

// Track 音频曲目（对应 tracks 表）
type Track struct {
	ID         int64     `json:"id"`
	Path       string    `json:"-"`              // 服务器本地绝对路径，不下发给前端
	Dir        string    `json:"dir"`            // 相对音频根目录的所在目录（POSIX 风格，根目录为空串）
	Filename   string    `json:"filename"`       // 文件名（含扩展名，如 P30.mp3），用于按文件名排序/展示
	Title      string    `json:"title"`          // 标题（标签缺失时取文件名）
	Artist     string    `json:"artist"`         // 艺术家
	Album      string    `json:"album"`          // 专辑
	Format     string    `json:"format"`         // mp3 / wav / flac ...
	Size       int64     `json:"size"`           // 文件字节数
	Duration   float64   `json:"duration"`       // 时长（秒）
	MTime      time.Time `json:"mtime"`          // 文件修改时间（增量扫描依据）
	HasCover   bool      `json:"has_cover"`      // 是否存在内嵌封面
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// DirInfo 目录导航项（按曲目所在相对目录聚合）
type DirInfo struct {
	Dir   string `json:"dir"`    // 相对目录，根目录为空串
	Name  string `json:"name"`   // 展示名（根目录为“根目录”，其余为目录最后一段）
	Count int64  `json:"count"`  // 该目录（含子目录）下的曲目数
}

// Playlist 播放列表
type Playlist struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	TrackCount int      `json:"track_count"` // 联表统计，非表字段
}

// PlaylistTrack 播放列表与曲目的关联（带顺序）
type PlaylistTrack struct {
	PlaylistID int64 `json:"playlist_id"`
	TrackID    int64 `json:"track_id"`
	Position   int   `json:"position"`
}

// ScanResult 一次扫描的统计结果
type ScanResult struct {
	Scanned  int `json:"scanned"`  // 扫描到的音频文件数
	Added    int `json:"added"`    // 新增曲目
	Updated  int `json:"updated"`  // 更新曲目
	Removed  int `json:"removed"`  // 清理的失效记录
	DurationMS int64 `json:"duration_ms"`
}

// TrackQuery 曲目列表查询参数
type TrackQuery struct {
	Keyword string // 标题/艺术家/专辑模糊匹配
	Dir     string // 相对目录过滤（精确匹配该目录及其子目录），空串表示全部
	Order   string // filename / title / artist / album / created_at（默认 created_at 倒序）
	Page    int
	PerPage int
}

// Paginated 分页结果
type Paginated[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Pages int   `json:"pages"`
}
