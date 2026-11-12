// Package store 封装 SQLite 持久化：建表迁移、曲目与播放列表 CRUD、搜索。
// 使用 modernc.org/sqlite 纯 Go 驱动（无 CGO），WAL 模式提升并发读性能。
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"myaudio/internal/model"
)

// Store 数据访问层
type Store struct {
	db *sql.DB
}

// New 打开数据库并执行迁移
func New(dbPath string) (*Store, error) {
	// 忙等待 5s，降低 WAL 下偶发 SQLITE_BUSY 概率
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	// 单连接写、多连接读；modernc 驱动下 WAL 配合受限连接数最稳
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.pragmas(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭数据库
func (s *Store) Close() error { return s.db.Close() }

// DB 暴露底层连接（扫描器批量写入等场景使用）
func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) pragmas() error {
	for _, p := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA temp_store=MEMORY",
	} {
		if _, err := s.db.Exec(p); err != nil {
			return fmt.Errorf("设置 %s 失败: %w", p, err)
		}
	}
	return nil
}

// migrate 建表（IF NOT EXISTS，幂等）
func (s *Store) migrate() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS tracks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    path        TEXT NOT NULL UNIQUE,
    title       TEXT NOT NULL DEFAULT '',
    artist      TEXT NOT NULL DEFAULT '',
    album       TEXT NOT NULL DEFAULT '',
    format      TEXT NOT NULL DEFAULT '',
    size        INTEGER NOT NULL DEFAULT 0,
    duration    REAL NOT NULL DEFAULT 0,
    mtime       TEXT NOT NULL,
    has_cover   INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_tracks_title  ON tracks(title);
CREATE INDEX IF NOT EXISTS idx_tracks_artist ON tracks(artist);
CREATE INDEX IF NOT EXISTS idx_tracks_album  ON tracks(album);

CREATE TABLE IF NOT EXISTS playlists (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL UNIQUE,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS playlist_tracks (
    playlist_id INTEGER NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    track_id    INTEGER NOT NULL REFERENCES tracks(id)    ON DELETE CASCADE,
    position    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (playlist_id, track_id)
);
CREATE INDEX IF NOT EXISTS idx_pt_playlist ON playlist_tracks(playlist_id, position);

-- 服务器端播放位置记忆：单用户服务，全局一份；曲目删除时随外键级联清除
CREATE TABLE IF NOT EXISTS play_positions (
    track_id    INTEGER PRIMARY KEY REFERENCES tracks(id) ON DELETE CASCADE,
    position    REAL NOT NULL DEFAULT 0,
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("数据库迁移失败: %w", err)
	}
	// 增量迁移：为旧版 tracks 表补充 dir / filename 列（按目录浏览、按文件名排序）。幂等，列已存在则跳过。
	if err := s.ensureColumn("tracks", "dir", "TEXT"); err != nil {
		return fmt.Errorf("迁移 tracks.dir 失败: %w", err)
	}
	if err := s.ensureColumn("tracks", "filename", "TEXT"); err != nil {
		return fmt.Errorf("迁移 tracks.filename 失败: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_tracks_dir ON tracks(dir)`); err != nil {
		return fmt.Errorf("创建 dir 索引失败: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_tracks_filename ON tracks(filename)`); err != nil {
		return fmt.Errorf("创建 filename 索引失败: %w", err)
	}
	return nil
}

// ensureColumn 幂等地为表添加列（SQLite 不支持 ADD COLUMN IF NOT EXISTS，先查 table_info）
func (s *Store) ensureColumn(table, column, colType string) error {
	rows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == column {
			rows.Close()
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	_, err = s.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s NOT NULL DEFAULT ''", table, column, colType))
	return err
}

// ---------------- Track ----------------

const trackCols = `id, path, dir, filename, title, artist, album, format, size, duration, mtime, has_cover, created_at, updated_at`

func scanTrack(row interface {
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

// GetTrack 按 ID 查询
func (s *Store) GetTrack(id int64) (*model.Track, error) {
	row := s.db.QueryRow(`SELECT `+trackCols+` FROM tracks WHERE id = ?`, id)
	t, err := scanTrack(row)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// GetTrackByPath 按路径查询（增量扫描用）
func (s *Store) GetTrackByPath(path string) (*model.Track, error) {
	row := s.db.QueryRow(`SELECT `+trackCols+` FROM tracks WHERE path = ?`, path)
	t, err := scanTrack(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

// ListTracks 分页 + 关键词搜索 + 目录过滤
func (s *Store) ListTracks(q model.TrackQuery) (*model.Paginated[model.Track], error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PerPage <= 0 || q.PerPage > 200 {
		q.PerPage = 48
	}
	where, args := buildTrackFilter(q.Keyword, q.Dir)

	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM tracks`+where, args...).Scan(&total); err != nil {
		return nil, err
	}

	order := "t.created_at DESC, t.id DESC"
	switch q.Order {
	case "filename":
		// 按文件名（含扩展名）不区分大小写排序；同目录下同名兜底按 id 稳定排序
		order = "t.filename COLLATE NOCASE ASC, t.id ASC"
	case "title":
		order = "t.title COLLATE NOCASE ASC"
	case "artist":
		order = "t.artist COLLATE NOCASE ASC, t.title ASC"
	case "album":
		order = "t.album COLLATE NOCASE ASC, t.title ASC"
	}
	args = append(args, q.PerPage, (q.Page-1)*q.PerPage)

	rows, err := s.db.Query(
		`SELECT `+prefixedTrackCols()+` FROM tracks t`+where+
			` ORDER BY `+order+` LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.Track, 0, q.PerPage)
	for rows.Next() {
		t, err := scanTrackPrefixed(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *t)
	}
	pages := int((total + int64(q.PerPage) - 1) / int64(q.PerPage))
	return &model.Paginated[model.Track]{Items: items, Total: total, Page: q.Page, Pages: pages}, rows.Err()
}

// ListDirs 聚合所有曲目所在的相对目录及曲目数（含子目录，即前缀匹配），用于前端目录导航。
// 根目录用空串表示，并始终排在首位。
func (s *Store) ListDirs() ([]model.DirInfo, error) {
	rows, err := s.db.Query(`SELECT dir, COUNT(*) AS cnt FROM tracks GROUP BY dir ORDER BY dir COLLATE NOCASE ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dirs []string
	for rows.Next() {
		var dir string
		var cnt int64
		if err := rows.Scan(&dir, &cnt); err != nil {
			return nil, err
		}
		if dir == "" {
			continue // 根目录单独以 total 统计
		}
		dirs = append(dirs, dir)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]model.DirInfo, 0, len(dirs)+1)
	// 根目录：统计全部曲目
	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM tracks`).Scan(&total); err != nil {
		return nil, err
	}
	// 根目录 Name 留空，由前端按当前语言本地化展示（i18n key: common.rootDir）
	out = append(out, model.DirInfo{Dir: "", Name: "", Count: total})

	// 每个目录的 Count 取“该目录及其子目录”的曲目数，点击父目录可看到下级内容
	for _, dir := range dirs {
		var cnt int64
		if err := s.db.QueryRow(
			`SELECT COUNT(*) FROM tracks WHERE dir = ? OR dir LIKE ?`, dir, dir+"/%",
		).Scan(&cnt); err != nil {
			return nil, err
		}
		out = append(out, model.DirInfo{
			Dir:   dir,
			Name:  dirDisplayName(dir),
			Count: cnt,
		})
	}
	return out, nil
}

// dirDisplayName 取相对目录的最后一段作为展示名
func dirDisplayName(dir string) string {
	dir = strings.TrimSuffix(dir, "/")
	if i := strings.LastIndex(dir, "/"); i >= 0 {
		return dir[i+1:]
	}
	return dir
}

// AllTrackPaths 返回库内全部曲目路径（扫描清理失效记录用）
func (s *Store) AllTrackPaths() (map[string]int64, error) {
	rows, err := s.db.Query(`SELECT id, path FROM tracks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int64, 256)
	for rows.Next() {
		var id int64
		var path string
		if err := rows.Scan(&id, &path); err != nil {
			return nil, err
		}
		out[path] = id
	}
	return out, rows.Err()
}

// DeleteTrack 删除曲目记录；deleteFile=true 时同时删除磁盘文件
func (s *Store) DeleteTrack(id int64, deleteFile bool) error {
	var path string
	err := s.db.QueryRow(`SELECT path FROM tracks WHERE id = ?`, id).Scan(&path)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM tracks WHERE id = ?`, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if deleteFile && path != "" {
		_ = removeIgnored(path) // 磁盘文件删除失败不影响库操作结果
	}
	return nil
}

// Stats 返回库统计
func (s *Store) Stats() (trackCount int64, totalDuration float64, err error) {
	err = s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(duration),0) FROM tracks`).Scan(&trackCount, &totalDuration)
	return
}

// ---------------- 扫描写入（Upsert / 批量删除） ----------------

// UpsertTrack 新增或更新曲目，返回是否新增
func (s *Store) UpsertTrack(t *model.Track) (added bool, err error) {
	existing, err := s.GetTrackByPath(t.Path)
	if err != nil {
		return false, err
	}
	hc := boolToInt(t.HasCover)
	if existing == nil {
		_, err = s.db.Exec(`INSERT INTO tracks
			(path,dir,filename,title,artist,album,format,size,duration,mtime,has_cover,updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,datetime('now'))`,
			t.Path, t.Dir, t.Filename, t.Title, t.Artist, t.Album, t.Format, t.Size, t.Duration,
			t.MTime.UTC().Format(sqliteTimeLayout), hc)
		return true, err
	}
	_, err = s.db.Exec(`UPDATE tracks SET
		dir=?, filename=?, title=?, artist=?, album=?, format=?, size=?, duration=?, mtime=?, has_cover=?, updated_at=datetime('now')
		WHERE id=?`,
		t.Dir, t.Filename, t.Title, t.Artist, t.Album, t.Format, t.Size, t.Duration,
		t.MTime.UTC().Format(sqliteTimeLayout), hc, existing.ID)
	t.ID = existing.ID
	return false, err
}

// DeleteMissing 批量删除失效记录
func (s *Store) DeleteMissing(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`DELETE FROM tracks WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, id := range ids {
		if _, err := stmt.Exec(id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------------- Playlist ----------------

// ListPlaylists 返回全部播放列表（含曲目数）
func (s *Store) ListPlaylists() ([]model.Playlist, error) {
	rows, err := s.db.Query(`
		SELECT p.id, p.name, p.created_at, p.updated_at, COUNT(pt.track_id)
		FROM playlists p
		LEFT JOIN playlist_tracks pt ON pt.playlist_id = p.id
		GROUP BY p.id ORDER BY p.updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Playlist
	for rows.Next() {
		var p model.Playlist
		var c, u string
		if err := rows.Scan(&p.ID, &p.Name, &c, &u, &p.TrackCount); err != nil {
			return nil, err
		}
		p.CreatedAt, p.UpdatedAt = parseSQLiteTime(c), parseSQLiteTime(u)
		out = append(out, p)
	}
	return out, rows.Err()
}

// CreatePlaylist 新建播放列表
func (s *Store) CreatePlaylist(name string) (*model.Playlist, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrEmptyName
	}
	res, err := s.db.Exec(`INSERT INTO playlists(name) VALUES (?)`, name)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrPlaylistExists
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &model.Playlist{ID: id, Name: name, TrackCount: 0}, nil
}

// RenamePlaylist 重命名
func (s *Store) RenamePlaylist(id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrEmptyName
	}
	res, err := s.db.Exec(`UPDATE playlists SET name=?, updated_at=datetime('now') WHERE id=?`, name, id)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return ErrPlaylistExists
		}
		return err
	}
	return checkAffected(res)
}

// DeletePlaylist 删除播放列表（关联表随外键级联删除）
func (s *Store) DeletePlaylist(id int64) error {
	res, err := s.db.Exec(`DELETE FROM playlists WHERE id=?`, id)
	if err != nil {
		return err
	}
	return checkAffected(res)
}

// PlaylistTracks 返回列表内曲目（按 position 排序）
func (s *Store) PlaylistTracks(playlistID int64) ([]model.Track, error) {
	rows, err := s.db.Query(`SELECT `+prefixedTrackCols()+`
		FROM tracks t JOIN playlist_tracks pt ON pt.track_id = t.id
		WHERE pt.playlist_id = ? ORDER BY pt.position ASC, pt.rowid ASC`, playlistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Track
	for rows.Next() {
		t, err := scanTrackPrefixed(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// AddTrackToPlaylist 追加曲目到列表末尾；已存在则忽略
func (s *Store) AddTrackToPlaylist(playlistID, trackID int64) error {
	// 前置存在性校验：把外键约束失败转成语义化的 ErrNotFound，避免底层错误冒泡成 500。
	var ok int
	if err := s.db.QueryRow(`SELECT 1 FROM playlists WHERE id=?`, playlistID).Scan(&ok); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if err := s.db.QueryRow(`SELECT 1 FROM tracks WHERE id=?`, trackID).Scan(&ok); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	var maxPos sql.NullInt64
	if err := s.db.QueryRow(`SELECT MAX(position) FROM playlist_tracks WHERE playlist_id=?`,
		playlistID).Scan(&maxPos); err != nil {
		return err
	}
	next := 0
	if maxPos.Valid {
		next = int(maxPos.Int64) + 1
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO playlist_tracks(playlist_id, track_id, position)
		VALUES (?,?,?)`, playlistID, trackID, next)
	if err == nil {
		_, _ = s.db.Exec(`UPDATE playlists SET updated_at=datetime('now') WHERE id=?`, playlistID)
	}
	return err
}

// RemoveTrackFromPlaylist 从列表移除并收紧 position
func (s *Store) RemoveTrackFromPlaylist(playlistID, trackID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`DELETE FROM playlist_tracks WHERE playlist_id=? AND track_id=?`,
		playlistID, trackID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil
	}
	if _, err := tx.Exec(`UPDATE playlists SET updated_at=datetime('now') WHERE id=?`, playlistID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// 重新编号，保持 position 连续
	return s.resequence(playlistID)
}

// resequence 重排列表内 position
func (s *Store) resequence(playlistID int64) error {
	rows, err := s.db.Query(`SELECT track_id FROM playlist_tracks
		WHERE playlist_id=? ORDER BY position ASC, rowid ASC`, playlistID)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`UPDATE playlist_tracks SET position=? WHERE playlist_id=? AND track_id=?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for i, id := range ids {
		if _, err := stmt.Exec(i, playlistID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------------- 播放位置（服务器端记忆，跨设备同步） ----------------

// maxPlayPositions 记忆条目的容量上限，超出后淘汰最久未更新的记录
const maxPlayPositions = 200

// UpsertPlayPosition 保存（或更新）某曲目的播放位置；存在外键约束，曲目不存在会返回错误
func (s *Store) UpsertPlayPosition(trackID int64, position float64) error {
	if position < 0 {
		position = 0
	}
	if _, err := s.db.Exec(`
		INSERT INTO play_positions (track_id, position, updated_at)
		VALUES (?, ?, datetime('now'))
		ON CONFLICT(track_id) DO UPDATE SET position=excluded.position, updated_at=datetime('now')`,
		trackID, position); err != nil {
		return err
	}
	// 容量兜底：超出上限时删除最久未更新的若干条
	if _, err := s.db.Exec(`
		DELETE FROM play_positions WHERE track_id IN (
			SELECT track_id FROM play_positions ORDER BY updated_at ASC, track_id ASC
			LIMIT (SELECT MAX(0, COUNT(*) - ?) FROM play_positions)
		)`, maxPlayPositions); err != nil {
		return err
	}
	return nil
}

// GetPlayPosition 读取单条播放位置；无记录时返回 (0, nil)
func (s *Store) GetPlayPosition(trackID int64) (float64, error) {
	var pos float64
	err := s.db.QueryRow(`SELECT position FROM play_positions WHERE track_id=?`, trackID).Scan(&pos)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return pos, err
}

// ListPlayPositions 返回全部播放位置，按最近更新时间倒序（首条即“上次播放”）
func (s *Store) ListPlayPositions() ([]model.PlayPosition, error) {
	rows, err := s.db.Query(`SELECT track_id, position, updated_at FROM play_positions ORDER BY updated_at DESC, track_id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.PlayPosition, 0)
	for rows.Next() {
		var p model.PlayPosition
		var updatedAt string
		if err := rows.Scan(&p.TrackID, &p.Position, &updatedAt); err != nil {
			return nil, err
		}
		if t, perr := time.Parse(sqliteTimeLayout, updatedAt); perr == nil {
			p.UpdatedAt = t
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeletePlayPosition 清除某曲目的播放位置；无记录时不报错
func (s *Store) DeletePlayPosition(trackID int64) error {
	_, err := s.db.Exec(`DELETE FROM play_positions WHERE track_id=?`, trackID)
	return err
}
