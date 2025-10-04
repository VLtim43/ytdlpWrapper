package src

import (
	"fmt"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	db             *DB
}

type ytdlpCheckMsg struct {
	installed bool
}

func checkYtdlp() tea.Msg {
	_, err := exec.LookPath("yt-dlp")
	return ytdlpCheckMsg{installed: err == nil}
}

func newModel(db *DB) model {
	return model{
		ytdlpChecked: false,
		db:           db,
	}
}

func (m model) Init() tea.Cmd {
	return checkYtdlp
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
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

	content := fmt.Sprintf("🎬 yt-dlp Wrapper\n\n%s\n\nPress Ctrl+C or q to quit", status)
	return "\n" + borderStyle.Render(content) + "\n"
}

func NewProgram(db *DB) *tea.Program {
	return tea.NewProgram(newModel(db))
}
