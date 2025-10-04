package src

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#fc40fc")).
			Bold(true).
			MarginBottom(1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262")).
			MarginTop(1)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff0000")).
			Bold(true).
			MarginTop(1)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00ff00")).
			Bold(true).
			MarginTop(1)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			MarginBottom(1)
)

type model struct {
	db          *DB
	textInput   textinput.Model
	message     string
	messageType string // "error" or "success"
	processing  bool
}

type urlProcessedMsg struct {
	success bool
	message string
}

func processURL(db *DB, url string) tea.Cmd {
	return func() tea.Msg {
		// Determine if it's a playlist/channel or single video
		if IsPlaylistURL(url) {
			err := ExtractPlaylistToDB(url, db)
			if err != nil {
				return urlProcessedMsg{
					success: false,
					message: fmt.Sprintf("Failed to add playlist/channel: %v", err),
				}
			}
			return urlProcessedMsg{
				success: true,
				message: "Playlist/Channel added successfully!",
			}
		} else {
			// Single video - download immediately
			err := RunHeadless(url, []string{}, db)
			if err != nil {
				return urlProcessedMsg{
					success: false,
					message: fmt.Sprintf("Download failed: %v", err),
				}
			}
			return urlProcessedMsg{
				success: true,
				message: "Video downloaded successfully!",
			}
		}
	}
}

func newModel(db *DB) model {
	ti := textinput.New()
	ti.Placeholder = "https://youtube.com/..."
	ti.Focus()
	ti.Width = 60
	ti.CharLimit = 200

	return model{
		db:        db,
		textInput: ti,
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit

		case tea.KeyEnter:
			url := m.textInput.Value()
			if url != "" && !m.processing {
				m.processing = true
				m.message = "Processing..."
				m.messageType = "info"
				return m, processURL(m.db, url)
			}
		}

	case urlProcessedMsg:
		m.processing = false
		m.message = msg.message
		if msg.success {
			m.messageType = "success"
			m.textInput.SetValue("")
		} else {
			m.messageType = "error"
		}
		return m, nil
	}

	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m model) View() string {
	s := titleStyle.Render("🎬 yt-dlp Wrapper - Add URL")
	s += "\n\n"

	s += infoStyle.Render("Enter a YouTube URL:")
	s += "\n"
	s += infoStyle.Render("• Single video → downloads immediately")
	s += "\n"
	s += infoStyle.Render("• Playlist/Channel → saves to database")
	s += "\n\n"

	s += m.textInput.View()
	s += "\n"

	if m.message != "" {
		s += "\n"
		switch m.messageType {
		case "error":
			s += errorStyle.Render("✗ " + m.message)
		case "success":
			s += successStyle.Render("✓ " + m.message)
		default:
			s += infoStyle.Render(m.message)
		}
	}

	s += "\n"
	s += helpStyle.Render("enter: submit • esc/ctrl+c: quit")

	return "\n" + s + "\n"
}

func NewProgram(db *DB) *tea.Program {
	return tea.NewProgram(newModel(db))
}
