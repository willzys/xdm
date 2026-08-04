package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var (
	accent = lipgloss.Color("#7AA2F7")
	muted  = lipgloss.Color("#565F89")
	bright = lipgloss.Color("#C0CAF5")
	alert  = lipgloss.Color("#F7768E")
	panel  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted)
)

func (m Model) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true
	return view
}

func (m Model) render() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if m.focus == focusSearch {
		box := panel.BorderForeground(accent).Padding(1, 2).Render("Search messages\n\n" + m.search.View() + "\n\nenter search  esc cancel")
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	if m.focus == focusSearchResults {
		return m.renderSearchResults()
	}
	header := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("xdm") + "  " + lipgloss.NewStyle().Foreground(muted).Render("Direct Messages")
	bodyHeight := max(8, m.height-4)
	var body string
	if m.width < 72 {
		if m.focus == focusInbox {
			body = m.renderInbox(m.width-2, bodyHeight)
		} else {
			body = m.renderChat(m.width-2, bodyHeight)
		}
	} else {
		inboxWidth := min(34, max(26, m.width/3))
		body = lipgloss.JoinHorizontal(lipgloss.Top, m.renderInbox(inboxWidth, bodyHeight), m.renderChat(m.width-inboxWidth-1, bodyHeight))
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, body, m.footer())
}

func (m Model) renderInbox(width, height int) string {
	lines := []string{lipgloss.NewStyle().Foreground(accent).Bold(true).Render("Inbox"), ""}
	if len(m.inbox) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(muted).Render("No cached conversations"))
	}
	available := max(1, height-4)
	start := 0
	if m.selected >= available {
		start = m.selected - available + 1
	}
	for index := start; index < len(m.inbox) && index < start+available; index++ {
		item := m.inbox[index]
		marker := "  "
		style := lipgloss.NewStyle().Foreground(bright)
		if index == m.selected {
			marker = "> "
			style = style.Foreground(accent).Bold(true)
		}
		unread := ""
		if item.UnreadCount > 0 {
			unread = fmt.Sprintf(" [%d]", item.UnreadCount)
		}
		lines = append(lines, style.Render(marker+truncate(item.Title+unread, width-5)))
		lines = append(lines, lipgloss.NewStyle().Foreground(muted).Render("  "+truncate(item.Preview, width-5)))
	}
	border := muted
	if m.focus == focusInbox {
		border = accent
	}
	return panel.Copy().BorderForeground(border).Width(width-2).Height(height-2).Padding(0, 1).Render(strings.Join(lines, "\n"))
}

func (m Model) renderChat(width, height int) string {
	conversation, ok := m.current()
	if !ok {
		return panel.Width(width - 2).Height(height - 2).Render(lipgloss.Place(width-4, height-4, lipgloss.Center, lipgloss.Center, "Sync to load your inbox"))
	}
	lines := []string{lipgloss.NewStyle().Foreground(accent).Bold(true).Render(conversation.Title), ""}
	composerRows := 0
	if m.focus == focusComposer || m.sending {
		composerRows = 6
	}
	available := max(2, height-5-composerRows)
	var messageBlocks []string
	highlightIndex := -1
	for index, message := range m.messages {
		name := message.SenderName
		if name == "" {
			name = message.SenderID
		}
		text := message.Text
		if message.ID == m.highlightMessage {
			highlightIndex = index
			text = highlightTerm(text, m.highlightQuery)
		}
		header := lipgloss.NewStyle().Foreground(accent).Render(name) + lipgloss.NewStyle().Foreground(muted).Render("  "+message.CreatedAt.Local().Format("15:04"))
		body := lipgloss.NewStyle().Foreground(bright).Width(max(20, width-6)).Render(text)
		messageBlocks = append(messageBlocks, header+"\n"+body+"\n")
	}
	visible := max(1, available/3)
	start := max(0, len(messageBlocks)-visible)
	if highlightIndex >= 0 {
		start = max(0, min(highlightIndex-visible/2, len(messageBlocks)-visible))
	}
	end := min(len(messageBlocks), start+visible)
	lines = append(lines, messageBlocks[start:end]...)
	if composerRows > 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(muted).Render(strings.Repeat("-", max(1, width-6))))
		if m.sending {
			lines = append(lines, m.spinner.View()+" sending")
		} else {
			lines = append(lines, m.composer.View(), lipgloss.NewStyle().Foreground(muted).Render("enter send  ctrl+enter newline  esc cancel"))
		}
	}
	border := muted
	if m.focus == focusChat || m.focus == focusComposer {
		border = accent
	}
	return panel.Copy().BorderForeground(border).Width(width-2).Height(height-2).Padding(0, 1).Render(strings.Join(lines, "\n"))
}

func (m Model) renderSearchResults() string {
	width := min(84, max(36, m.width-6))
	visible := max(1, (m.height-10)/2)
	start := 0
	if m.selectedSearch >= visible {
		start = m.selectedSearch - visible + 1
	}
	title := fmt.Sprintf("Search results: %s (%d)", truncate(m.query, width-24), len(m.searchResults))
	lines := []string{lipgloss.NewStyle().Foreground(accent).Bold(true).Render(title), ""}
	if len(m.searchResults) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(muted).Render("No matching messages"))
	}
	for index := start; index < len(m.searchResults) && index < start+visible; index++ {
		result := m.searchResults[index]
		marker := "  "
		style := lipgloss.NewStyle().Foreground(bright)
		if index == m.selectedSearch {
			marker = "> "
			style = style.Foreground(accent).Bold(true)
		}
		heading := marker + result.ConversationTitle + "  " + result.CreatedAt.Local().Format("Jan 02 15:04")
		sender := result.SenderName
		if sender == "" {
			sender = "unknown"
		}
		lines = append(lines, style.Render(truncate(heading, width-6)))
		lines = append(lines, "  "+lipgloss.NewStyle().Foreground(muted).Render(truncate(sender+": "+result.Text, width-8)))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(muted).Render("j/k move  enter open  / edit search  esc close"))
	box := panel.Copy().BorderForeground(accent).Width(width-2).Padding(1, 2).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) footer() string {
	left := "j/k move  tab pane  enter compose  / search  R sync  q quit"
	if m.width < 72 {
		left = "j/k move  tab view  enter compose  / search  q quit"
	}
	right := m.status
	if m.loading {
		right = m.spinner.View() + " syncing"
	}
	if m.err != nil {
		right = lipgloss.NewStyle().Foreground(alert).Render(truncate(m.err.Error(), max(20, m.width/2)))
	}
	space := max(1, m.width-lipgloss.Width(left)-lipgloss.Width(right))
	return lipgloss.NewStyle().Foreground(muted).Render(left) + strings.Repeat(" ", space) + right
}

func truncate(value string, width int) string {
	value = strings.ReplaceAll(value, "\n", " ")
	if width <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

func highlightTerm(value, term string) string {
	term = strings.TrimSpace(term)
	if term == "" {
		return value
	}
	lowerTerm := strings.ToLower(term)
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#16161E")).Background(accent).Bold(true)
	var result strings.Builder
	for len(value) > 0 {
		index := strings.Index(strings.ToLower(value), lowerTerm)
		if index < 0 {
			result.WriteString(value)
			break
		}
		result.WriteString(value[:index])
		end := index + len(term)
		if end > len(value) {
			result.WriteString(value[index:])
			break
		}
		result.WriteString(style.Render(value[index:end]))
		value = value[end:]
	}
	return result.String()
}
