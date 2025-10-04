package src

import (
	"database/sql"
	"net/url"
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
	ID        string
	URL       string
	Title     string
	FilePath  string
	Status    DownloadStatus
	Error     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type DB struct {
	conn *sql.DB
}

func Open(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	if err := conn.Ping(); err != nil {
		return nil, err
	}

	db := &DB{conn: conn}
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
		file_path TEXT,
		status TEXT NOT NULL,
		error TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_url ON downloads(url);
	CREATE INDEX IF NOT EXISTS idx_status ON downloads(status);
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
		`INSERT INTO downloads (id, url, title, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, urlStr, title, StatusPending, now, now,
	)
	if err != nil {
		return "", err
	}
	return id, nil
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
		`SELECT id, url, title, file_path, status, error, created_at, updated_at FROM downloads WHERE id = ?`,
		id,
	)

	var d DownloadRecord
	err := row.Scan(&d.ID, &d.URL, &d.Title, &d.FilePath, &d.Status, &d.Error, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (db *DB) GetAllDownloads() ([]DownloadRecord, error) {
	rows, err := db.conn.Query(
		`SELECT id, url, title, file_path, status, error, created_at, updated_at FROM downloads ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var downloads []DownloadRecord
	for rows.Next() {
		var d DownloadRecord
		if err := rows.Scan(&d.ID, &d.URL, &d.Title, &d.FilePath, &d.Status, &d.Error, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		downloads = append(downloads, d)
	}
	return downloads, rows.Err()
}
