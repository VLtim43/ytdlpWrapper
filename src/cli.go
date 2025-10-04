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
	titleRegex       = regexp.MustCompile(`\[download\] (.+?) has already been downloaded`)
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

	downloadID, err := db.InsertDownload(url, "")
	if err != nil {
		return fmt.Errorf("failed to insert download record: %w", err)
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
	var videoTitle string

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
			cleanupPartFiles(downloadsDir, downloadID)
			if dbErr := db.UpdateDownloadStatus(downloadID, StatusCancelled, "", "Download cancelled by user"); dbErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to update download status: %v\n", dbErr)
			}
			return fmt.Errorf("download cancelled")
		}

		// Clean up .part files on failure too
		cleanupPartFiles(downloadsDir, downloadID)
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

func cleanupPartFiles(downloadsDir, downloadID string) {
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
