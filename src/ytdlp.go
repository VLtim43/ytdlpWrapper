package src

import (
	"bufio"
	"context"
	"fmt"
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
	// If it's a channel URL, try to get the canonical channel ID/URL first
	var canonicalChannelURL string
	if IsChannelURL(playlistURL) {
		canonicalChannelURL = extractChannelURL(playlistURL)
	}

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
				// Clean the playlist channel URL immediately
				info.ChannelURL = CleanChannelURL(parts[2])
			}

			videoChannel := parts[6]
			videoChannelURL := parts[7]

			// Fallback: Use playlist channel info if video channel is missing or NA
			if videoChannel == "" || videoChannel == "NA" {
				videoChannel = parts[1] // Use playlist_channel
			}
			if videoChannelURL == "" || videoChannelURL == "NA" {
				// If we have a canonical channel URL, use it; otherwise use playlist_channel_url
				if canonicalChannelURL != "" {
					videoChannelURL = canonicalChannelURL
				} else {
					videoChannelURL = parts[2] // Use playlist_channel_url
				}
			}

			// Clean the video channel URL
			videoChannelURL = CleanChannelURL(videoChannelURL)

			// Ensure video channel name is never empty
			if videoChannel == "" || videoChannel == "NA" {
				if videoChannelURL != "" && videoChannelURL != "NA" {
					videoChannel = extractChannelNameFromURL(videoChannelURL)
				} else {
					videoChannel = "Unknown Channel"
				}
			}

			// Ensure video channel URL is never empty
			if videoChannelURL == "" || videoChannelURL == "NA" {
				// This shouldn't happen after fallbacks, but just in case
				videoChannelURL = ""
			}

			video := VideoInfo{
				ID:         parts[4],
				Title:      parts[5],
				Channel:    videoChannel,
				ChannelURL: videoChannelURL,
				URL:        parts[8],
			}
			info.Videos = append(info.Videos, video)
		}
	}

	// Fallback: Extract playlist title from URL if still empty
	if info.Title == "" && len(info.Videos) > 0 {
		info.Title = extractTitleFromURL(playlistURL)
	}

	// Use canonical channel URL if we extracted it
	if canonicalChannelURL != "" {
		info.ChannelURL = canonicalChannelURL
	} else if (info.ChannelURL == "" || info.ChannelURL == "NA") && IsChannelURL(playlistURL) {
		// Fallback: use the original URL if it's a channel URL
		info.ChannelURL = CleanChannelURL(playlistURL)
	}

	// Ensure channel name is never empty
	if info.Channel == "" || info.Channel == "NA" {
		// Extract from channel URL if available
		if info.ChannelURL != "" {
			info.Channel = extractChannelNameFromURL(info.ChannelURL)
		}
	}

	// Ensure channel URL is never empty if we have videos
	if (info.ChannelURL == "" || info.ChannelURL == "NA") && len(info.Videos) > 0 {
		// Use the first video's channel URL
		for _, video := range info.Videos {
			if video.ChannelURL != "" && video.ChannelURL != "NA" {
				info.ChannelURL = video.ChannelURL
				if info.Channel == "" || info.Channel == "NA" {
					info.Channel = video.Channel
				}
				break
			}
		}
	}

	return info, nil
}

// extractChannelNameFromURL extracts a readable channel name from a URL
func extractChannelNameFromURL(urlStr string) string {
	// For @handle format
	if strings.Contains(urlStr, "/@") {
		parts := strings.Split(urlStr, "/@")
		if len(parts) > 1 {
			return "@" + strings.Split(parts[1], "/")[0]
		}
	}
	// For /channel/ID format
	if strings.Contains(urlStr, "/channel/") {
		parts := strings.Split(urlStr, "/channel/")
		if len(parts) > 1 {
			return strings.Split(parts[1], "/")[0]
		}
	}
	// For /c/ or /user/ format
	if strings.Contains(urlStr, "/c/") {
		parts := strings.Split(urlStr, "/c/")
		if len(parts) > 1 {
			return strings.Split(parts[1], "/")[0]
		}
	}
	if strings.Contains(urlStr, "/user/") {
		parts := strings.Split(urlStr, "/user/")
		if len(parts) > 1 {
			return strings.Split(parts[1], "/")[0]
		}
	}
	return "Unknown Channel"
}


// extractChannelURL gets the canonical channel URL (with ID) from any channel URL format
func extractChannelURL(channelURL string) string {
	args := []string{
		"--print", "%(channel_id)s",
		"--playlist-items", "1",
		channelURL,
	}

	cmd := exec.Command("yt-dlp", args...)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	channelID := strings.TrimSpace(string(output))
	if channelID == "" || channelID == "NA" {
		return ""
	}

	// Return the canonical channel URL format
	return "https://www.youtube.com/channel/" + channelID
}

func ExtractVideoMetadata(videoURL string) (*VideoInfo, error) {
	args := []string{
		"--print", "%(id)s|%(title)s|%(channel)s|%(channel_url)s",
		videoURL,
	}

	cmd := exec.Command("yt-dlp", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	line := strings.TrimSpace(string(output))
	parts := strings.SplitN(line, "|", 4)
	if len(parts) != 4 {
		return nil, fmt.Errorf("invalid metadata format")
	}

	channelURL := parts[3]
	if channelURL == "NA" || channelURL == "" {
		channelURL = ""
	} else {
		channelURL = CleanChannelURL(channelURL)
	}

	return &VideoInfo{
		ID:         parts[0],
		Title:      parts[1],
		Channel:    parts[2],
		ChannelURL: channelURL,
		URL:        videoURL,
	}, nil
}
