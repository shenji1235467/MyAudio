package store

import (
	"database/sql"
	"errors"
	"os"
	"strings"
	"time"

	"myaudio/internal/model"
)

// 业务错误
var (
	ErrEmptyName      = errors.New("名称不能为空")
	ErrPlaylistExists = errors.New("播放列表名称已存在")
	ErrNotFound       = errors.New("记录不存在")
)

const sqliteTimeLayout = "2006-01-02 15:04:05"

// parseSQLiteTime 解析 SQLite datetime('now') 输出（UTC）
func parseSQLiteTime(s string) time.Time {
	if t, err := time.ParseInLocation(sqliteTimeLayout, s, time.UTC); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func removeIgnored(path string) error { return os.Remove(path) }

func checkAffected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// buildTrackFilter 构造曲目列表的 WHERE 子句：关键词（标题/艺术家/专辑）+ 目录过滤。
// 条件均使用裸列名，可同时用于带别名 t. 的查询与 COUNT(*) 统计。
func buildTrackFilter(kw, dir string) (string, []any) {
	var conds []string
	var args []any

	if kw = strings.TrimSpace(kw); kw != "" {
		like := "%" + kw + "%"
		conds = append(conds, "(title LIKE ? OR artist LIKE ? OR album LIKE ?)")
		args = append(args, like, like, like)
	}
	if dir = strings.TrimSpace(dir); dir != "" {
		// 精确命中该目录，或命中其任意层级子目录（dir 以 "dir/" 开头）
		conds = append(conds, "(dir = ? OR dir LIKE ?)")
		args = append(args, dir, dir+"/%")
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// prefixedTrackCols 带表别名 t. 的列清单
func prefixedTrackCols() string {
	return "t.id, t.path, t.dir, t.filename, t.title, t.artist, t.album, t.format, t.size, " +
		"t.duration, t.mtime, t.has_cover, t.created_at, t.updated_at"
}

// scanTrackPrefixed 扫描带别名的查询结果
func scanTrackPrefixed(row interface {
	Scan(dest ...any) error
}) (*model.Track, error) {
	var t model.Track
	var mtime, createdAt, updatedAt string
	var hasCover int
	if err := row.Scan(&t.ID, &t.Path, &t.Dir, &t.Filename, &t.Title, &t.Artist, &t.Album, &t.Format,
		&t.Size, &t.Duration, &mtime, &hasCover, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	t.MTime = parseSQLiteTime(mtime)
	t.CreatedAt = parseSQLiteTime(createdAt)
	t.UpdatedAt = parseSQLiteTime(updatedAt)
	t.HasCover = hasCover == 1
	return &t, nil
}
