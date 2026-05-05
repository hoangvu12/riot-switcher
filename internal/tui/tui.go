package tui

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"riot-switcher/internal/account"
	"riot-switcher/internal/riot"
)

type mode int

const (
	modeList mode = iota
	modeAddID
	modeAddLabel
	modeSaveID
	modeSaveLabel
	modeDeleteConfirm
)

type model struct {
	store        *account.Store
	profiles     []account.Profile
	cursor       int
	mode         mode
	input        string
	working      bool
	message      string
	log          string
	formID       string
	formLabel    string
	pendingID    string
	pendingLabel string
}

type profilesMsg struct {
	profiles []account.Profile
	err      error
}

type actionMsg struct {
	message string
	log     string
	err     error
}

var (
	baseStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	brandStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("44")).Bold(true)
	titleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Bold(true)
	faintStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	headerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(true)
	sectionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("44")).Bold(true)
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	rowStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("250")).PaddingLeft(1)
	activeStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("24")).PaddingLeft(1).PaddingRight(1)
	panelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Padding(0, 1)
	inputStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("24")).PaddingLeft(1).PaddingRight(1)
)

func Run(store *account.Store) error {
	_, err := tea.NewProgram(initialModel(store)).Run()
	return err
}

func initialModel(store *account.Store) model {
	return model{store: store, message: "Select a profile or start a new login."}
}

func (m model) Init() tea.Cmd {
	return m.loadProfiles
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case profilesMsg:
		m.working = false
		if msg.err != nil {
			m.message = msg.err.Error()
			return m, nil
		}
		m.profiles = msg.profiles
		if m.cursor >= len(m.profiles) {
			m.cursor = max(0, len(m.profiles)-1)
		}
		return m, nil
	case actionMsg:
		m.working = false
		m.log = msg.log
		if msg.err != nil {
			m.message = msg.err.Error()
			return m, nil
		}
		m.message = msg.message
		return m, m.loadProfiles
	case tea.KeyPressMsg:
		if m.working {
			return m, nil
		}
		if m.mode != modeList {
			return m.updateInput(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

func (m model) updateList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.profiles)-1 {
			m.cursor++
		}
	case "r":
		m.message = "Refreshing profile list..."
		m.working = true
		return m, m.loadProfiles
	case "a":
		m.mode = modeAddID
		m.input = ""
		m.message = "Step 1: choose a short id for this Riot account, like main or alt."
	case "s":
		m.mode = modeSaveID
		m.input = m.pendingID
		if m.pendingID == "" {
			m.message = "Step 2: capture the Riot account that is currently logged in. Enter the profile id to save it as."
		} else {
			m.message = "Step 2: capture the Riot account you just logged into. Press enter to save it as " + m.pendingID + "."
		}
	case "d":
		if len(m.profiles) == 0 {
			m.message = "No profiles to delete."
			break
		}
		m.mode = modeDeleteConfirm
		m.message = fmt.Sprintf("Delete %s? Press y to confirm, n or esc to cancel.", m.profiles[m.cursor].ID)
	case "enter":
		if len(m.profiles) == 0 {
			m.message = "No profiles yet. Press a to add one."
			break
		}
		m.working = true
		id := m.profiles[m.cursor].ID
		m.message = "Switching to " + id + "."
		return m, switchProfile(m.store, id)
	}
	return m, nil
}

func (m model) updateInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeDeleteConfirm {
		switch msg.String() {
		case "y":
			id := m.profiles[m.cursor].ID
			m.mode = modeList
			m.working = true
			m.message = "Deleting " + id + "."
			return m, deleteProfile(m.store, id)
		case "n", "esc":
			m.mode = modeList
			m.message = "Cancelled."
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "esc":
		m.mode = modeList
		m.input = ""
		m.message = "Cancelled."
		return m, nil
	case "backspace", "ctrl+h":
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
		return m, nil
	case "enter":
		return m.submitInput()
	}

	if text := msg.Key().Text; text != "" {
		m.input += text
	}
	return m, nil
}

func (m model) submitInput() (tea.Model, tea.Cmd) {
	value := strings.TrimSpace(m.input)
	switch m.mode {
	case modeAddID:
		if value == "" {
			m.message = "Profile id is required."
			return m, nil
		}
		m.formID = value
		m.formLabel = ""
		m.input = value
		m.mode = modeAddLabel
		m.message = "Optional display name. Press enter to use the same name as the id."
		return m, nil
	case modeAddLabel:
		m.formLabel = labelOrID(m.formID, value)
		m.pendingID = m.formID
		m.pendingLabel = m.formLabel
		m.mode = modeList
		m.input = ""
		m.working = true
		m.message = "Opening Riot for a clean manual login. This does not save anything yet."
		return m, beginSetup(m.formID, m.formLabel)
	case modeSaveID:
		if value == "" {
			m.message = "Profile id is required."
			return m, nil
		}
		m.formID = value
		if m.pendingID == value && m.pendingLabel != "" {
			m.input = m.pendingLabel
		} else {
			m.input = labelFor(m.profiles, value)
		}
		m.mode = modeSaveLabel
		m.message = "Optional display name for this captured login. Press enter to use the shown value."
		return m, nil
	case modeSaveLabel:
		m.formLabel = labelOrID(m.formID, value)
		if m.pendingID == m.formID {
			m.pendingID = ""
			m.pendingLabel = ""
		}
		m.mode = modeList
		m.input = ""
		m.working = true
		m.message = "Capturing the Riot account currently logged in on this PC."
		return m, captureProfile(m.store, m.formID, m.formLabel)
	}
	return m, nil
}

func (m model) View() tea.View {
	var b strings.Builder
	b.WriteString(brandStyle.Render("rsw"))
	b.WriteString(faintStyle.Render(" / "))
	b.WriteString(titleStyle.Render("Riot account switcher"))
	b.WriteString("\n\n")
	if line := m.workflowLine(); line != "" {
		b.WriteString(line)
		b.WriteString("\n\n")
	}

	if m.mode != modeList {
		b.WriteString(m.inputView())
	} else if len(m.profiles) == 0 {
		b.WriteString(panelStyle.Render("No profiles yet. Use Start Login to begin adding one."))
		b.WriteString("\n\n")
	} else {
		b.WriteString(headerStyle.Render(fmt.Sprintf("%-11s %-30s %s", "Profile", "Label", "Captured")))
		b.WriteString("\n")
		for i, profile := range m.profiles {
			row := fmt.Sprintf("%-11s %-30s %s", profile.ID, truncate(profile.Label, 29), profile.CapturedAt.Format("Jan 02  15:04"))
			if i == m.cursor {
				b.WriteString(activeStyle.Render("  " + row))
			} else {
				b.WriteString(rowStyle.Render("  " + row))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if m.working {
		b.WriteString("\n")
		b.WriteString(brandStyle.Render("Working"))
		b.WriteString(faintStyle.Render(" - please wait"))
	} else if strings.Contains(strings.ToLower(m.message), "error") || strings.Contains(strings.ToLower(m.message), "failed") {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(m.message))
	} else if strings.Contains(strings.ToLower(m.message), "switched") || strings.Contains(strings.ToLower(m.message), "captured") || strings.Contains(strings.ToLower(m.message), "deleted") {
		b.WriteString("\n")
		b.WriteString(successStyle.Render(m.message))
	} else if shouldShowStatus(m.message) {
		b.WriteString("\n")
		b.WriteString(baseStyle.Render(m.message))
	}
	b.WriteString("\n")

	if m.log != "" {
		b.WriteString("\n")
		b.WriteString(panelStyle.Render(strings.TrimSpace(m.log)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(shortcutView())
	view := tea.NewView(b.String())
	view.AltScreen = true
	return view
}

func shortcutView() string {
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("44")).Bold(true)
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	items := []struct {
		key   string
		label string
	}{
		{key: "a", label: "start"},
		{key: "s", label: "capture"},
		{key: "enter", label: "switch"},
		{key: "d", label: "delete"},
		{key: "r", label: "refresh"},
		{key: "q", label: "quit"},
	}

	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, keyStyle.Render(item.key)+textStyle.Render(" "+item.label))
	}
	return strings.Join(parts, textStyle.Render("  |  "))
}

func (m model) inputView() string {
	label := "Input"
	switch m.mode {
	case modeAddID:
		label = "Start Login - profile id"
	case modeAddLabel:
		label = "Start Login - display name"
	case modeSaveID:
		label = "Capture Login - save as profile id"
	case modeSaveLabel:
		label = "Capture Login - display name"
	case modeDeleteConfirm:
		return "Press y to confirm deletion, or esc to cancel.\n\n\n"
	}
	return fmt.Sprintf("%s\n%s\n\n", headerStyle.Render(label), inputStyle.Render(m.input+" "))
}

func (m model) workflowLine() string {
	if m.pendingID != "" && !m.working {
		return sectionStyle.Render("Capture next") + faintStyle.Render("  Finish Riot login for ") + brandStyle.Render(m.pendingID) + faintStyle.Render(", then press s.")
	}
	if len(m.profiles) == 0 && !m.working {
		return sectionStyle.Render("No profiles") + faintStyle.Render("  Press a to start your first login.")
	}
	return ""
}

func (m model) loadProfiles() tea.Msg {
	profiles, err := m.store.List()
	return profilesMsg{profiles: profiles, err: err}
}

func switchProfile(store *account.Store, id string) tea.Cmd {
	return func() tea.Msg {
		var logs bytes.Buffer
		if _, err := store.Get(id); err != nil {
			return actionMsg{err: err, log: logs.String()}
		}
		if err := riot.Switch(store.SnapshotDir(id), riot.Options{Log: logTo(&logs)}); err != nil {
			return actionMsg{err: err, log: logs.String()}
		}
		return actionMsg{message: "Switched to " + id + ".", log: logs.String()}
	}
}

func beginSetup(id, label string) tea.Cmd {
	return func() tea.Msg {
		var logs bytes.Buffer
		if err := riot.BeginSetup(riot.Options{Log: logTo(&logs)}); err != nil {
			return actionMsg{err: err, log: logs.String()}
		}
		return actionMsg{message: fmt.Sprintf("Riot opened for %s. Log in, enable Stay signed in, then click Save.", label), log: logs.String()}
	}
}

func captureProfile(store *account.Store, id, label string) tea.Cmd {
	return func() tea.Msg {
		var logs bytes.Buffer
		if err := riot.Capture(store.SnapshotDir(id), riot.Options{Log: logTo(&logs)}); err != nil {
			return actionMsg{err: err, log: logs.String()}
		}
		if err := store.Upsert(account.Profile{ID: id, Label: label, CapturedAt: time.Now()}); err != nil {
			return actionMsg{err: err, log: logs.String()}
		}
		return actionMsg{message: fmt.Sprintf("Captured %s (%s).", id, label), log: logs.String()}
	}
}

func deleteProfile(store *account.Store, id string) tea.Cmd {
	return func() tea.Msg {
		if err := store.Remove(id); err != nil {
			return actionMsg{err: err}
		}
		return actionMsg{message: "Deleted " + id + "."}
	}
}

func shouldShowStatus(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "selected") ||
		strings.Contains(lower, "cancelled") ||
		strings.Contains(lower, "delete") ||
		strings.Contains(lower, "refresh") ||
		strings.Contains(lower, "no profiles") ||
		strings.Contains(lower, "no action")
}

func logTo(buf *bytes.Buffer) func(string, ...any) {
	return func(format string, args ...any) {
		fmt.Fprintf(buf, format+"\n", args...)
	}
}

func labelFor(profiles []account.Profile, id string) string {
	for _, profile := range profiles {
		if profile.ID == id {
			return profile.Label
		}
	}
	return id
}

func labelOrID(id, label string) string {
	if strings.TrimSpace(label) == "" {
		return id
	}
	return strings.TrimSpace(label)
}

func truncate(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	if maxLen <= 3 {
		return value[:maxLen]
	}
	return value[:maxLen-3] + "..."
}
