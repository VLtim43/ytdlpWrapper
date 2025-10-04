package src

import (
	"database/sql"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DownloadStatus string

const (
	StatusCompleted DownloadStatus = "completed"
	StatusFailed    DownloadStatus = "failed"
	StatusPending   DownloadStatus = "pending"
)

type DownloadRecord struct {
	ID        int64
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
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		url TEXT NOT NULL,
		title TEXT,
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

func (db *DB) InsertDownload(url, title string) (int64, error) {
	now := time.Now()
	result, err := db.conn.Exec(
		`INSERT INTO downloads (url, title, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		url, title, StatusPending, now, now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (db *DB) UpdateDownloadStatus(id int64, status DownloadStatus, filePath, errorMsg string) error {
	_, err := db.conn.Exec(
		`UPDATE downloads SET status = ?, file_path = ?, error = ?, updated_at = ? WHERE id = ?`,
		status, filePath, errorMsg, time.Now(), id,
	)
	return err
}

func (db *DB) GetDownload(id int64) (*DownloadRecord, error) {
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
