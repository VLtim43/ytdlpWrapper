package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"ytdlpWrapper/src"
)

var (
	borderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#fc40fcff")).
		Padding(1, 2)
)

type model struct {
	ytdlpInstalled bool
	ytdlpChecked   bool
}

type ytdlpCheckMsg struct {
	installed bool
}

func checkYtdlp() tea.Msg {
	_, err := exec.LookPath("yt-dlp")
	return ytdlpCheckMsg{installed: err == nil}
}

func initialModel() model {
	return model{
		ytdlpChecked: false,
	}
}

func (m model) Init() tea.Cmd {
	return checkYtdlp
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		}

	case ytdlpCheckMsg:
		m.ytdlpInstalled = msg.installed
		m.ytdlpChecked = true
	}
	return m, nil
}

func (m model) View() string {
	var status string

	if !m.ytdlpChecked {
		status = "Checking yt-dlp installation..."
	} else if m.ytdlpInstalled {
		status = "✓ yt-dlp is installed"
	} else {
		status = "✗ yt-dlp is not installed"
	}

	content := fmt.Sprintf("🎬 yt-dlp Wrapper\n\n%s\n\nPress Ctrl+C to quit", status)
	return "\n" + borderStyle.Render(content) + "\n"
}

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

func headlessMode(url string, cookiesFile string, extraArgs []string, database *src.DB) error {
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
	downloadID, err := database.InsertDownload(url, "")
	if err != nil {
		return fmt.Errorf("failed to insert download record: %w", err)
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("                          yt-dlp Output")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	opts := src.DownloadOptions{
		URL:         url,
		CookiesFile: cookiesFile,
		OutputPath:  filepath.Join(downloadsDir, "%(title)s.%(ext)s"),
		ExtraArgs:   extraArgs,
	}

	if err := src.Download(opts); err != nil {
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		// Update status to failed
		if dbErr := database.UpdateDownloadStatus(downloadID, src.StatusFailed, "", err.Error()); dbErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update download status: %v\n", dbErr)
		}
		return fmt.Errorf("download failed: %w", err)
	}

	// Update status to completed
	if err := database.UpdateDownloadStatus(downloadID, src.StatusCompleted, filepath.Join(downloadsDir, "%(title)s.%(ext)s"), ""); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to update download status: %v\n", err)
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("✓ Download completed successfully!")
	return nil
}

func main() {
	// Define flags
	url := flag.String("url", "", "Video URL to download")
	cookiesFile := flag.String("cookies", "", "Path to cookies file")
	flag.Parse()

	// Get any additional arguments to pass to yt-dlp
	extraArgs := flag.Args()

	// Initialize database
	dbPath := filepath.Join(".", "db", "downloads.db")
	database, err := src.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	// If URL is provided, run in headless mode
	if *url != "" {
		if err := headlessMode(*url, *cookiesFile, extraArgs, database); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Otherwise, run TUI mode
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}
