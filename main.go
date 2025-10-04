package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ytdlpWrapper/src"
)

func ensureDownloadsFolder() (string, error) {
	// Use current working directory as base
	baseDir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	downloadsDir := filepath.Join(baseDir, "downloads")

	if err := os.MkdirAll(downloadsDir, 0755); err != nil {
		return "", err
	}

	return downloadsDir, nil
}

func headlessMode(url string, ytdlpArgs []string, db *src.DB) error {
	if !src.IsInstalled() {
		return fmt.Errorf("yt-dlp is not installed")
	}

	downloadsDir, err := ensureDownloadsFolder()
	if err != nil {
		return fmt.Errorf("failed to create downloads folder: %w", err)
	}

	fmt.Printf("Downloading to: %s\n", downloadsDir)
	fmt.Printf("URL: %s\n\n", url)

	// Insert download record
	downloadID, err := db.InsertDownload(url, "")
	if err != nil {
		return fmt.Errorf("failed to insert download record: %w", err)
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("                          yt-dlp Output")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	opts := src.DownloadOptions{
		URL:        url,
		OutputPath: filepath.Join(downloadsDir, "%(title)s.%(ext)s"),
		ExtraArgs:  ytdlpArgs,
	}

	if err := src.Download(opts); err != nil {
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		// Update status to failed
		if dbErr := db.UpdateDownloadStatus(downloadID, src.StatusFailed, "", err.Error()); dbErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update download status: %v\n", dbErr)
		}
		return fmt.Errorf("download failed: %w", err)
	}

	// Update status to completed
	if err := db.UpdateDownloadStatus(downloadID, src.StatusCompleted, filepath.Join(downloadsDir, "%(title)s.%(ext)s"), ""); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to update download status: %v\n", err)
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("✓ Download completed successfully!")
	return nil
}

func main() {
	// Parse command line arguments manually to allow all ytdlp flags to pass through
	// Look for URL (first non-flag argument or after -url)
	var url string
	var ytdlpArgs []string

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		if args[i] == "-url" || args[i] == "--url" {
			if i+1 < len(args) {
				url = args[i+1]
				i++
			}
		} else if !strings.HasPrefix(args[i], "-") && url == "" {
			// First non-flag argument is the URL
			url = args[i]
		} else {
			// Everything else gets passed to ytdlp
			ytdlpArgs = append(ytdlpArgs, args[i])
		}
	}

	// Ensure required directories exist
	if err := os.MkdirAll("db", 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating db directory: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll("downloads", 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating downloads directory: %v\n", err)
		os.Exit(1)
	}

	// Initialize database
	dbPath := filepath.Join(".", "db", "downloads.db")
	db, err := src.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// If URL is provided, run in headless mode
	if url != "" {
		if err := headlessMode(url, ytdlpArgs, db); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Otherwise, run TUI mode
	p := src.NewProgram(db)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}
