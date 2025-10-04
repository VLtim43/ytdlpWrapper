package src

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

var (
	progressRegex    = regexp.MustCompile(`(\d+\.?\d*)%`)
	etaRegex         = regexp.MustCompile(`ETA\s+(\d{2}:\d{2}(?::\d{2})?)`)
	destinationRegex = regexp.MustCompile(`\[download\] Destination: (.+)`)
)

func RunHeadless(url string, ytdlpArgs []string, db *DB) error {
	if !IsInstalled() {
		return fmt.Errorf("yt-dlp is not installed")
	}

	downloadsDir, err := ensureDownloadsFolder()
	if err != nil {
		return fmt.Errorf("failed to create downloads folder: %w", err)
	}

	fmt.Printf("Downloading: %s\n", url)
	fmt.Printf("Destination: %s\n\n", downloadsDir)

	// Extract video metadata first
	videoInfo, err := ExtractVideoMetadata(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to extract metadata: %v\n", err)
		videoInfo = &VideoInfo{URL: url} // Continue with minimal info
	}

	downloadID, err := db.InsertDownload(url, videoInfo.Title)
	if err != nil {
		return fmt.Errorf("failed to insert download record: %w", err)
	}

	// Update channel info if available
	if videoInfo.Channel != "" {
		db.UpdateDownloadChannel(downloadID, videoInfo.Channel)
	}
	if videoInfo.ChannelURL != "" {
		db.UpdateDownloadChannelURL(downloadID, videoInfo.ChannelURL)
	}

	// Setup signal handling for Ctrl+C
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	cancelled := false
	go func() {
		<-sigChan
		fmt.Println("\n\nCancelling download...")
		cancelled = true
		cancel()
	}()

	// Add --newline flag to force ytdlp to output progress on new lines
	ytdlpArgs = append([]string{"--newline"}, ytdlpArgs...)

	opts := DownloadOptions{
		URL:        url,
		OutputPath: filepath.Join(downloadsDir, "%(title)s.%(ext)s"),
		ExtraArgs:  ytdlpArgs,
		Context:    ctx,
	}

	var lastOutput string
	var videoTitle, videoChannel string

	err = DownloadWithCallback(opts, func(line string) {
		// Extract title from destination line
		if videoTitle == "" {
			if matches := destinationRegex.FindStringSubmatch(line); len(matches) > 1 {
				fullPath := matches[1]
				filename := filepath.Base(fullPath)
				ext := filepath.Ext(filename)
				videoTitle = strings.TrimSuffix(filename, ext)
				db.UpdateDownloadTitle(downloadID, videoTitle)
			}
		}

		// Extract channel info
		if videoChannel == "" && strings.Contains(line, "[info]") && strings.Contains(line, "Extracting URL:") {
			// This is a simple heuristic - ytdlp doesn't easily expose channel in progress output
			// Channel will be captured more reliably from playlist extraction
		}

		// Look for download progress lines
		if strings.Contains(line, "[download]") && strings.Contains(line, "%") {
			var progress, eta string

			if matches := progressRegex.FindStringSubmatch(line); len(matches) > 0 {
				progress = matches[1]
			}

			if matches := etaRegex.FindStringSubmatch(line); len(matches) > 0 {
				eta = matches[1]
			}

			if progress != "" {
				output := fmt.Sprintf("Progress: %s%%", progress)
				if eta != "" {
					output += fmt.Sprintf(" | ETA: %s", eta)
				}

				if output != lastOutput {
					fmt.Printf("\r%-60s", output)
					lastOutput = output
				}
			}
		}
	})

	fmt.Println()

	if err != nil {
		if cancelled {
			// Clean up .part files
			cleanupPartFiles(downloadsDir)
			if dbErr := db.UpdateDownloadStatus(downloadID, StatusCancelled, "", "Download cancelled by user"); dbErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to update download status: %v\n", dbErr)
			}
			return fmt.Errorf("download cancelled")
		}

		// Clean up .part files on failure too
		cleanupPartFiles(downloadsDir)
		if dbErr := db.UpdateDownloadStatus(downloadID, StatusFailed, "", err.Error()); dbErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update download status: %v\n", dbErr)
		}
		return fmt.Errorf("download failed: %w", err)
	}

	if err := db.UpdateDownloadStatus(downloadID, StatusCompleted, filepath.Join(downloadsDir, "%(title)s.%(ext)s"), ""); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to update download status: %v\n", err)
	}

	fmt.Println("✓ Download completed successfully!")
	return nil
}

func ensureDownloadsFolder() (string, error) {
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

func cleanupPartFiles(downloadsDir string) {
	entries, err := os.ReadDir(downloadsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to read downloads directory: %v\n", err)
		return
	}

	cleaned := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasSuffix(name, ".part") || strings.HasSuffix(name, ".ytdl") || strings.HasSuffix(name, ".temp") {
			filePath := filepath.Join(downloadsDir, name)
			if err := os.Remove(filePath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to remove %s: %v\n", name, err)
			} else {
				cleaned++
			}
		}
	}

	if cleaned > 0 {
		fmt.Printf("Cleaned up %d partial file(s)\n", cleaned)
	}
}

func ListDownloads(db *DB) error {
	downloads, err := db.GetAllDownloads()
	if err != nil {
		return fmt.Errorf("failed to get downloads: %w", err)
	}

	if len(downloads) == 0 {
		fmt.Println("No downloads yet")
		return nil
	}

	fmt.Println("Download History:")
	fmt.Println(strings.Repeat("─", 80))

	for _, d := range downloads {
		var statusIcon string
		switch d.Status {
		case StatusCompleted:
			statusIcon = "✓"
		case StatusFailed:
			statusIcon = "✗"
		case StatusPending:
			statusIcon = "⏳"
		case StatusCancelled:
			statusIcon = "⊘"
		default:
			statusIcon = "?"
		}

		fmt.Printf("%s [%s] %s\n", statusIcon, d.ID, d.URL)
		if d.Title != "" {
			fmt.Printf("   Title: %s\n", d.Title)
		}
		if d.Channel != "" {
			fmt.Printf("   Channel: %s\n", d.Channel)
		}
		if d.PlaylistID != "" {
			// Get playlist info to show which playlist this came from
			playlist, err := db.GetPlaylist(d.PlaylistID)
			if err == nil && playlist != nil {
				fmt.Printf("   Playlist: %s\n", playlist.Title)
			}
		} else {
			fmt.Printf("   Source: Direct download (orphan)\n")
		}
		if d.FilePath != "" {
			fmt.Printf("   Path: %s\n", d.FilePath)
		}
		if d.Error != "" {
			fmt.Printf("   Error: %s\n", d.Error)
		}
		fmt.Printf("   Created: %s\n", d.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Println()
	}

	return nil
}

func ExtractPlaylistToDB(urlStr string, db *DB) error {
	if !IsInstalled() {
		return fmt.Errorf("yt-dlp is not installed")
	}

	info, err := ExtractPlaylist(urlStr)
	if err != nil {
		return fmt.Errorf("failed to extract videos: %w", err)
	}

	if len(info.Videos) == 0 {
		return fmt.Errorf("no videos found")
	}

	title := info.Title
	if title == "" {
		title = "Unknown Playlist"
	}

	totalVideos := len(info.Videos)
	channel := info.Channel
	channelURL := info.ChannelURL

	// Check if playlist already exists
	existingPlaylist, err := db.GetPlaylistByURL(urlStr)
	var playlistID string
	var newVideosAdded int

	if err == nil && existingPlaylist != nil {
		// Playlist exists - update it
		playlistID = existingPlaylist.ID
		fmt.Printf("Updating existing playlist: %s\n", title)

		// Add only new videos
		for i, video := range info.Videos {
			exists, err := db.VideoExistsInPlaylist(playlistID, video.ID)
			if err != nil {
				continue
			}
			if !exists {
				if err := db.InsertPlaylistVideo(playlistID, title, video.URL, video.Title, video.ID, video.Channel, video.ChannelURL, i+1); err == nil {
					newVideosAdded++
				}
			}
		}

		// Update counts
		currentSaved := existingPlaylist.VideosSaved + newVideosAdded
		db.UpdatePlaylistCounts(playlistID, totalVideos, currentSaved, existingPlaylist.VideosDownloaded)

		fmt.Printf("Playlist: %s\n", title)
		fmt.Printf("Total videos in playlist: %d\n", totalVideos)
		fmt.Printf("New videos added: %d\n", newVideosAdded)
		fmt.Printf("Total saved: %d\n", currentSaved)
	} else {
		// New playlist
		savedCount := 0
		for i, video := range info.Videos {
			if err := db.InsertPlaylistVideo("", title, video.URL, video.Title, video.ID, video.Channel, video.ChannelURL, i+1); err == nil {
				savedCount++
			}
		}

		playlistID, err = db.InsertPlaylist(urlStr, title, channel, channelURL, totalVideos, savedCount)
		if err != nil {
			return fmt.Errorf("failed to insert playlist: %w", err)
		}

		// Update playlist_id for the videos
		for _, video := range info.Videos {
			db.conn.Exec(`UPDATE playlist_videos SET playlist_id = ? WHERE video_id = ? AND playlist_id = ''`, playlistID, video.ID)
		}

		fmt.Printf("Playlist: %s\n", title)
		fmt.Printf("Videos in playlist: %d\n", totalVideos)
		fmt.Printf("Videos saved to database: %d\n", savedCount)

		if savedCount < totalVideos {
			fmt.Fprintf(os.Stderr, "Warning: Only %d/%d videos were saved\n", savedCount, totalVideos)
		}
	}

	return nil
}

func ListPlaylists(db *DB) error {
	playlists, err := db.GetAllPlaylists()
	if err != nil {
		return fmt.Errorf("failed to get playlists: %w", err)
	}

	if len(playlists) == 0 {
		fmt.Println("No playlists yet")
		return nil
	}

	fmt.Println("Playlists:")
	fmt.Println(strings.Repeat("─", 80))

	for _, p := range playlists {
		fmt.Printf("📋 [%s] %s\n", p.ID, p.Title)
		if p.Channel != "" {
			fmt.Printf("   Channel: %s\n", p.Channel)
		}
		fmt.Printf("   URL: %s\n", p.URL)
		fmt.Printf("   Total videos: %d | Saved: %d | Downloaded: %d\n", p.TotalVideos, p.VideosSaved, p.VideosDownloaded)
		fmt.Printf("   Created: %s | Updated: %s\n", p.CreatedAt.Format("2006-01-02 15:04:05"), p.UpdatedAt.Format("2006-01-02 15:04:05"))
		fmt.Println()
	}

	return nil
}
