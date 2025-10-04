package src

import (
	"bufio"
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
}

func Download(opts DownloadOptions) error {
	args := []string{}

	args = append(args, "--restrict-filenames")

	if opts.OutputPath != "" {
		args = append(args, "-o", opts.OutputPath)
	}

	args = append(args, opts.ExtraArgs...)
	args = append(args, opts.URL)

	cmd := exec.Command("yt-dlp", args...)

	// Inherit stdout and stderr to show yt-dlp output
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// DownloadWithCallback executes yt-dlp and calls the callback for each output line
func DownloadWithCallback(opts DownloadOptions, callback func(string)) error {
	args := []string{}

	// Restrict filenames to ASCII characters and normalize
	args = append(args, "--restrict-filenames")

	if opts.OutputPath != "" {
		args = append(args, "-o", opts.OutputPath)
	}

	args = append(args, opts.ExtraArgs...)
	args = append(args, opts.URL)

	cmd := exec.Command("yt-dlp", args...)

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
