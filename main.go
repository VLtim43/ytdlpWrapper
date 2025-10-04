package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ytdlpWrapper/src"
)


func main() {
	// Parse command line arguments manually to allow all ytdlp flags to pass through
	var url string
	var playlistURL string
	var listMode bool
	var listPlaylists bool
	var ytdlpArgs []string

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		if args[i] == "-url" || args[i] == "--url" {
			if i+1 < len(args) {
				url = args[i+1]
				i++
			}
		} else if args[i] == "-playlist" || args[i] == "--playlist" {
			if i+1 < len(args) {
				playlistURL = args[i+1]
				i++
			}
		} else if args[i] == "-list" || args[i] == "--list" {
			listMode = true
		} else if args[i] == "-list-playlists" || args[i] == "--list-playlists" {
			listPlaylists = true
		} else if !strings.HasPrefix(args[i], "-") && url == "" && playlistURL == "" {
			// Auto-detect playlist URLs
			if src.IsPlaylistURL(args[i]) {
				playlistURL = args[i]
			} else {
				url = args[i]
			}
		} else {
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
	dbPath := filepath.Join(".", "db", "data.db")
	db, err := src.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Handle different modes
	if listMode {
		if err := src.ListDownloads(db); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if listPlaylists {
		if err := src.ListPlaylists(db); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if playlistURL != "" {
		if err := src.ExtractPlaylistToDB(playlistURL, db); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if url != "" {
		if err := src.RunHeadless(url, ytdlpArgs, db); err != nil {
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
