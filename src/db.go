package src

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

type DownloadStatus string

const (
	StatusCompleted DownloadStatus = "completed"
	StatusFailed    DownloadStatus = "failed"
	StatusPending   DownloadStatus = "pending"
	StatusCancelled DownloadStatus = "cancelled"
)

type DownloadRecord struct {
	ID         string
	URL        string
	Title      string
	Channel    string
	ChannelURL string
	FilePath   string
	Status     DownloadStatus
	Error      string
	PlaylistID string // Empty for orphan videos
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type PlaylistRecord struct {
	ID               string
	URL              string
	Title            string
	Channel          string
	ChannelURL       string
	TotalVideos      int
	VideosSaved      int
	VideosDownloaded int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type PlaylistVideo struct {
	ID           string
	PlaylistID   string
	PlaylistName string
	VideoURL     string
	VideoTitle   string
	VideoID      string
	Channel      string
	ChannelURL   string
	Index        int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type DB struct {
	conn *sql.DB
}

func Open(dbPath string) (*DB, error) {
	// Check if database file exists
	_, err := os.Stat(dbPath)
	isNewDB := os.IsNotExist(err)

	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	if err := conn.Ping(); err != nil {
		return nil, err
	}

	db := &DB{conn: conn}

	if isNewDB {
		fmt.Printf("Creating %s...\n", dbPath)
	}

	if err := db.createTables(); err != nil {
		return nil, err
	}

	return db, nil
}

func (db *DB) createTables() error {
	schema := `
	CREATE TABLE IF NOT EXISTS downloads (
		id TEXT PRIMARY KEY,
		url TEXT NOT NULL,
		title TEXT NOT NULL,
		channel TEXT NOT NULL,
		channel_url TEXT NOT NULL,
		file_path TEXT,
		status TEXT NOT NULL,
		error TEXT,
		playlist_id TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (playlist_id) REFERENCES playlists(id) ON DELETE SET NULL
	);
	CREATE INDEX IF NOT EXISTS idx_url ON downloads(url);
	CREATE INDEX IF NOT EXISTS idx_status ON downloads(status);
	CREATE INDEX IF NOT EXISTS idx_playlist_id ON downloads(playlist_id);

	CREATE TABLE IF NOT EXISTS playlists (
		id TEXT PRIMARY KEY,
		url TEXT NOT NULL,
		title TEXT NOT NULL,
		channel TEXT NOT NULL,
		channel_url TEXT NOT NULL,
		total_videos INTEGER NOT NULL,
		videos_saved INTEGER NOT NULL DEFAULT 0,
		videos_downloaded INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_playlist_url ON playlists(url);

	CREATE TABLE IF NOT EXISTS playlist_videos (
		id TEXT PRIMARY KEY,
		playlist_id TEXT NOT NULL,
		playlist_name TEXT NOT NULL,
		video_url TEXT NOT NULL,
		video_title TEXT NOT NULL,
		video_id TEXT NOT NULL,
		channel TEXT NOT NULL,
		channel_url TEXT NOT NULL,
		idx INTEGER NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (playlist_id) REFERENCES playlists(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_playlist_videos_playlist_id ON playlist_videos(playlist_id);
	`

	_, err := db.conn.Exec(schema)
	return err
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) InsertDownload(urlStr, title string) (string, error) {
	return db.InsertDownloadWithPlaylist(urlStr, title, "")
}

func (db *DB) InsertDownloadWithPlaylist(urlStr, title, playlistID string) (string, error) {
	id := uuid.New().String()

	if title == "" {
		title = extractTitleFromURL(urlStr)
	}

	now := time.Now()
	_, err := db.conn.Exec(
		`INSERT INTO downloads (id, url, title, channel, channel_url, status, playlist_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, urlStr, title, "", "", StatusPending, playlistID, now, now,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (db *DB) UpdateDownloadChannel(id, channel string) error {
	_, err := db.conn.Exec(
		`UPDATE downloads SET channel = ?, updated_at = ? WHERE id = ?`,
		channel, time.Now(), id,
	)
	return err
}

func (db *DB) UpdateDownloadChannelURL(id, channelURL string) error {
	_, err := db.conn.Exec(
		`UPDATE downloads SET channel_url = ?, updated_at = ? WHERE id = ?`,
		channelURL, time.Now(), id,
	)
	return err
}

func extractTitleFromURL(urlStr string) string {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return urlStr
	}

	// Get the last part of the path
	basePath := path.Base(parsed.Path)
	if basePath != "" && basePath != "/" && basePath != "." {
		// Remove extension if present
		ext := path.Ext(basePath)
		if ext != "" {
			basePath = strings.TrimSuffix(basePath, ext)
		}
		return basePath
	}

	// Fallback to query parameters or hostname
	if parsed.RawQuery != "" {
		// Try to extract video ID from common patterns
		params := parsed.Query()
		if v := params.Get("v"); v != "" {
			return v
		}
		if id := params.Get("id"); id != "" {
			return id
		}
	}

	// Last resort: use hostname + path
	return strings.TrimPrefix(parsed.Host+parsed.Path, "www.")
}

func (db *DB) UpdateDownloadStatus(id string, status DownloadStatus, filePath, errorMsg string) error {
	_, err := db.conn.Exec(
		`UPDATE downloads SET status = ?, file_path = ?, error = ?, updated_at = ? WHERE id = ?`,
		status, filePath, errorMsg, time.Now(), id,
	)
	return err
}

func (db *DB) UpdateDownloadTitle(id, title string) error {
	_, err := db.conn.Exec(
		`UPDATE downloads SET title = ?, updated_at = ? WHERE id = ?`,
		title, time.Now(), id,
	)
	return err
}

func (db *DB) GetDownload(id string) (*DownloadRecord, error) {
	row := db.conn.QueryRow(
		`SELECT id, url, title, channel, channel_url, file_path, status, error, playlist_id, created_at, updated_at FROM downloads WHERE id = ?`,
		id,
	)

	var d DownloadRecord
	err := row.Scan(&d.ID, &d.URL, &d.Title, &d.Channel, &d.ChannelURL, &d.FilePath, &d.Status, &d.Error, &d.PlaylistID, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (db *DB) GetAllDownloads() ([]DownloadRecord, error) {
	rows, err := db.conn.Query(
		`SELECT id, url, title, channel, channel_url, file_path, status, error, playlist_id, created_at, updated_at FROM downloads ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var downloads []DownloadRecord
	for rows.Next() {
		var d DownloadRecord
		if err := rows.Scan(&d.ID, &d.URL, &d.Title, &d.Channel, &d.ChannelURL, &d.FilePath, &d.Status, &d.Error, &d.PlaylistID, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		downloads = append(downloads, d)
	}
	return downloads, rows.Err()
}

func (db *DB) InsertPlaylist(url, title, channel, channelURL string, totalVideos, videosSaved int) (string, error) {
	id := uuid.New().String()

	if title == "" {
		title = extractTitleFromURL(url)
	}

	now := time.Now()
	_, err := db.conn.Exec(
		`INSERT INTO playlists (id, url, title, channel, channel_url, total_videos, videos_saved, videos_downloaded, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, url, title, channel, channelURL, totalVideos, videosSaved, 0, now, now,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (db *DB) UpdatePlaylistCounts(id string, totalVideos, videosSaved, videosDownloaded int) error {
	_, err := db.conn.Exec(
		`UPDATE playlists SET total_videos = ?, videos_saved = ?, videos_downloaded = ?, updated_at = ? WHERE id = ?`,
		totalVideos, videosSaved, videosDownloaded, time.Now(), id,
	)
	return err
}

func (db *DB) GetPlaylist(id string) (*PlaylistRecord, error) {
	row := db.conn.QueryRow(
		`SELECT id, url, title, channel, channel_url, total_videos, videos_saved, videos_downloaded, created_at, updated_at FROM playlists WHERE id = ?`,
		id,
	)

	var p PlaylistRecord
	err := row.Scan(&p.ID, &p.URL, &p.Title, &p.Channel, &p.ChannelURL, &p.TotalVideos, &p.VideosSaved, &p.VideosDownloaded, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (db *DB) GetPlaylistByURL(url string) (*PlaylistRecord, error) {
	row := db.conn.QueryRow(
		`SELECT id, url, title, channel, channel_url, total_videos, videos_saved, videos_downloaded, created_at, updated_at FROM playlists WHERE url = ?`,
		url,
	)

	var p PlaylistRecord
	err := row.Scan(&p.ID, &p.URL, &p.Title, &p.Channel, &p.ChannelURL, &p.TotalVideos, &p.VideosSaved, &p.VideosDownloaded, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (db *DB) InsertPlaylistVideo(playlistID, playlistName, videoURL, videoTitle, videoID, channel, channelURL string, index int) error {
	id := uuid.New().String()
	now := time.Now()
	_, err := db.conn.Exec(
		`INSERT INTO playlist_videos (id, playlist_id, playlist_name, video_url, video_title, video_id, channel, channel_url, idx, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, playlistID, playlistName, videoURL, videoTitle, videoID, channel, channelURL, index, now, now,
	)
	return err
}

func (db *DB) GetAllPlaylists() ([]PlaylistRecord, error) {
	rows, err := db.conn.Query(
		`SELECT id, url, title, channel, channel_url, total_videos, videos_saved, videos_downloaded, created_at, updated_at FROM playlists ORDER BY updated_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var playlists []PlaylistRecord
	for rows.Next() {
		var p PlaylistRecord
		if err := rows.Scan(&p.ID, &p.URL, &p.Title, &p.Channel, &p.ChannelURL, &p.TotalVideos, &p.VideosSaved, &p.VideosDownloaded, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		playlists = append(playlists, p)
	}
	return playlists, rows.Err()
}

func (db *DB) VideoExistsInPlaylist(playlistID, videoID string) (bool, error) {
	var count int
	err := db.conn.QueryRow(
		`SELECT COUNT(*) FROM playlist_videos WHERE playlist_id = ? AND video_id = ?`,
		playlistID, videoID,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (db *DB) GetPlaylistVideos(playlistID string) ([]PlaylistVideo, error) {
	rows, err := db.conn.Query(
		`SELECT id, playlist_id, playlist_name, video_url, video_title, video_id, channel, channel_url, idx, created_at, updated_at FROM playlist_videos WHERE playlist_id = ? ORDER BY idx`,
		playlistID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var videos []PlaylistVideo
	for rows.Next() {
		var v PlaylistVideo
		if err := rows.Scan(&v.ID, &v.PlaylistID, &v.PlaylistName, &v.VideoURL, &v.VideoTitle, &v.VideoID, &v.Channel, &v.ChannelURL, &v.Index, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		videos = append(videos, v)
	}
	return videos, rows.Err()
}
