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
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type PlaylistRecord struct {
	ID          string
	URL         string
	Title       string
	Channel     string
	ChannelURL  string
	VideoCount  int
	ExtractedAt time.Time
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
		channel TEXT,
		channel_url TEXT,
		file_path TEXT,
		status TEXT NOT NULL,
		error TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_url ON downloads(url);
	CREATE INDEX IF NOT EXISTS idx_status ON downloads(status);

	CREATE TABLE IF NOT EXISTS playlists (
		id TEXT PRIMARY KEY,
		url TEXT NOT NULL,
		title TEXT NOT NULL,
		channel TEXT,
		channel_url TEXT,
		video_count INTEGER NOT NULL,
		extracted_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_playlist_url ON playlists(url);

	CREATE TABLE IF NOT EXISTS playlist_videos (
		id TEXT PRIMARY KEY,
		playlist_id TEXT NOT NULL,
		playlist_name TEXT NOT NULL,
		video_url TEXT NOT NULL,
		video_title TEXT NOT NULL,
		video_id TEXT NOT NULL,
		channel TEXT,
		channel_url TEXT,
		idx INTEGER NOT NULL,
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
	id := uuid.New().String()

	if title == "" {
		title = extractTitleFromURL(urlStr)
	}

	now := time.Now()
	_, err := db.conn.Exec(
		`INSERT INTO downloads (id, url, title, channel, channel_url, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, urlStr, title, "", "", StatusPending, now, now,
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
		`SELECT id, url, title, channel, channel_url, file_path, status, error, created_at, updated_at FROM downloads WHERE id = ?`,
		id,
	)

	var d DownloadRecord
	err := row.Scan(&d.ID, &d.URL, &d.Title, &d.Channel, &d.ChannelURL, &d.FilePath, &d.Status, &d.Error, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (db *DB) GetAllDownloads() ([]DownloadRecord, error) {
	rows, err := db.conn.Query(
		`SELECT id, url, title, channel, channel_url, file_path, status, error, created_at, updated_at FROM downloads ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var downloads []DownloadRecord
	for rows.Next() {
		var d DownloadRecord
		if err := rows.Scan(&d.ID, &d.URL, &d.Title, &d.Channel, &d.ChannelURL, &d.FilePath, &d.Status, &d.Error, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		downloads = append(downloads, d)
	}
	return downloads, rows.Err()
}

func (db *DB) InsertPlaylist(url, title, channel, channelURL string, videoCount int) (string, error) {
	id := uuid.New().String()

	if title == "" {
		title = extractTitleFromURL(url)
	}

	now := time.Now()
	_, err := db.conn.Exec(
		`INSERT INTO playlists (id, url, title, channel, channel_url, video_count, extracted_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, url, title, channel, channelURL, videoCount, now,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (db *DB) InsertPlaylistVideo(playlistID, playlistName, videoURL, videoTitle, videoID, channel, channelURL string, index int) error {
	id := uuid.New().String()
	_, err := db.conn.Exec(
		`INSERT INTO playlist_videos (id, playlist_id, playlist_name, video_url, video_title, video_id, channel, channel_url, idx) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, playlistID, playlistName, videoURL, videoTitle, videoID, channel, channelURL, index,
	)
	return err
}

func (db *DB) GetAllPlaylists() ([]PlaylistRecord, error) {
	rows, err := db.conn.Query(
		`SELECT id, url, title, channel, channel_url, video_count, extracted_at FROM playlists ORDER BY extracted_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var playlists []PlaylistRecord
	for rows.Next() {
		var p PlaylistRecord
		if err := rows.Scan(&p.ID, &p.URL, &p.Title, &p.Channel, &p.ChannelURL, &p.VideoCount, &p.ExtractedAt); err != nil {
			return nil, err
		}
		playlists = append(playlists, p)
	}
	return playlists, rows.Err()
}

func (db *DB) GetPlaylistVideos(playlistID string) ([]PlaylistVideo, error) {
	rows, err := db.conn.Query(
		`SELECT id, playlist_id, playlist_name, video_url, video_title, video_id, channel, channel_url, idx FROM playlist_videos WHERE playlist_id = ? ORDER BY idx`,
		playlistID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var videos []PlaylistVideo
	for rows.Next() {
		var v PlaylistVideo
		if err := rows.Scan(&v.ID, &v.PlaylistID, &v.PlaylistName, &v.VideoURL, &v.VideoTitle, &v.VideoID, &v.Channel, &v.ChannelURL, &v.Index); err != nil {
			return nil, err
		}
		videos = append(videos, v)
	}
	return videos, rows.Err()
}
