package src

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

func IsInstalled() bool {
	_, err := exec.LookPath("yt-dlp")
	return err == nil
}

func IsPlaylistURL(urlStr string) bool {
	// Check for common playlist indicators
	return strings.Contains(urlStr, "/playlist") ||
		strings.Contains(urlStr, "list=") ||
		strings.Contains(urlStr, "/playlists/")
}

func NormalizeFilename(filename string) string {
	// Replace spaces with underscores
	filename = strings.ReplaceAll(filename, " ", "_")

	// Remove invalid characters (keep only alphanumeric, underscore, hyphen, dot)
	reg := regexp.MustCompile(`[^a-zA-Z0-9_\-\.]`)
	filename = reg.ReplaceAllString(filename, "")

	// Remove multiple consecutive underscores/hyphens
	reg = regexp.MustCompile(`[_\-]{2,}`)
	filename = reg.ReplaceAllString(filename, "_")

	// Trim leading/trailing underscores and hyphens
	filename = strings.Trim(filename, "_-")

	return filename
}

// DownloadOptions contains options for downloading videos
type DownloadOptions struct {
	URL        string
	OutputPath string
	ExtraArgs  []string
	Context    context.Context
}

func Download(opts DownloadOptions) error {
	args := []string{}

	args = append(args, "--restrict-filenames")

	if opts.OutputPath != "" {
		args = append(args, "-o", opts.OutputPath)
	}

	args = append(args, opts.ExtraArgs...)
	args = append(args, opts.URL)

	var cmd *exec.Cmd
	if opts.Context != nil {
		cmd = exec.CommandContext(opts.Context, "yt-dlp", args...)
	} else {
		cmd = exec.Command("yt-dlp", args...)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// DownloadWithCallback executes yt-dlp and calls the callback for each output line
func DownloadWithCallback(opts DownloadOptions, callback func(string)) error {
	args := []string{}

	args = append(args, "--restrict-filenames")

	if opts.OutputPath != "" {
		args = append(args, "-o", opts.OutputPath)
	}

	args = append(args, opts.ExtraArgs...)
	args = append(args, opts.URL)

	var cmd *exec.Cmd
	if opts.Context != nil {
		cmd = exec.CommandContext(opts.Context, "yt-dlp", args...)
	} else {
		cmd = exec.Command("yt-dlp", args...)
	}

	// Create pipes for stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return err
	}

	// Read from both stdout and stderr
	go readAndCallback(stdout, callback)
	go readAndCallback(stderr, callback)

	return cmd.Wait()
}

func readAndCallback(r io.Reader, callback func(string)) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		callback(scanner.Text())
	}
}

type PlaylistInfo struct {
	Title      string
	Channel    string
	ChannelURL string
	Videos     []VideoInfo
}

type VideoInfo struct {
	URL        string
	Title      string
	ID         string
	Channel    string
	ChannelURL string
}

func ExtractPlaylist(playlistURL string) (*PlaylistInfo, error) {
	args := []string{
		"--flat-playlist",
		"--get-url",
		"--print", "%(playlist_title,playlist)s|%(playlist_channel,channel)s|%(playlist_channel_url,channel_url)s|%(playlist_index)s|%(id)s|%(title)s|%(channel)s|%(channel_url)s|%(url)s",
		playlistURL,
	}

	cmd := exec.Command("yt-dlp", args...)

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(output), "\n")
	info := &PlaylistInfo{
		Videos: make([]VideoInfo, 0),
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse format: playlist_title|playlist_channel|playlist_channel_url|index|id|title|channel|channel_url|url
		parts := strings.SplitN(line, "|", 9)
		if len(parts) == 9 {
			// Extract playlist info from first video
			if info.Title == "" {
				info.Title = parts[0]
				info.Channel = parts[1]
				info.ChannelURL = parts[2]
			}

			video := VideoInfo{
				ID:         parts[4],
				Title:      parts[5],
				Channel:    parts[6],
				ChannelURL: parts[7],
				URL:        parts[8],
			}
			info.Videos = append(info.Videos, video)
		}
	}

	// Fallback: Extract playlist title from URL if still empty
	if info.Title == "" && len(info.Videos) > 0 {
		info.Title = extractTitleFromURL(playlistURL)
	}

	return info, nil
}
