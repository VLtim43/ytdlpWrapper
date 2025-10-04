package src

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	progressRegex = regexp.MustCompile(`(\d+\.?\d*)%`)
	etaRegex      = regexp.MustCompile(`ETA\s+(\d{2}:\d{2}(?::\d{2})?)`)
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

	// Add --newline flag to force ytdlp to output progress on new lines
	ytdlpArgs = append([]string{"--newline"}, ytdlpArgs...)

	opts := DownloadOptions{
		URL:        url,
		OutputPath: filepath.Join(downloadsDir, "%(title)s.%(ext)s"),
		ExtraArgs:  ytdlpArgs,
	}

	var lastOutput string
	err = DownloadWithCallback(opts, func(line string) {
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
		default:
			statusIcon = "?"
		}

		fmt.Printf("%s [%d] %s\n", statusIcon, d.ID, d.URL)
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
