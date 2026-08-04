package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/willzys/xdm/internal/cache"
)

const syncInterval = 65 * time.Second

type Backend interface {
	Inbox(context.Context, string) ([]cache.Conversation, error)
	Messages(context.Context, string) ([]cache.Message, error)
	Search(context.Context, string) ([]cache.SearchResult, error)
	MarkRead(context.Context, string) error
	Sync(context.Context) error
	Send(context.Context, string, string) error
}

type focus int

const (
	focusInbox focus = iota
	focusChat
	focusComposer
	focusSearch
	focusSearchResults
)

type Model struct {
	ctx              context.Context
	backend          Backend
	width, height    int
	focus            focus
	inbox            []cache.Conversation
	selected         int
	messages         []cache.Message
	searchResults    []cache.SearchResult
	selectedSearch   int
	composer         textarea.Model
	search           textinput.Model
	spinner          spinner.Model
	loading, sending bool
	query, status    string
	highlightMessage string
	highlightQuery   string
	err              error
}

type inboxMsg struct {
	items []cache.Conversation
	err   error
}
type messagesMsg struct {
	conversationID string
	items          []cache.Message
	err            error
}
type searchResultsMsg struct {
	items []cache.SearchResult
	err   error
}
type syncMsg struct{ err error }
type sendMsg struct{ err error }
type syncTick time.Time

func New(ctx context.Context, backend Backend) Model {
	composer := textarea.New()
	composer.Placeholder = "Write a message"
	composer.Prompt = ""
	composer.ShowLineNumbers = false
	composer.SetHeight(3)
	composer.CharLimit = 10000
	composer.KeyMap.InsertNewline = key.NewBinding(
		key.WithKeys("ctrl+enter", "alt+enter"),
		key.WithHelp("ctrl+enter", "insert newline"),
	)
	search := textinput.New()
	search.Placeholder = "Search conversations and messages"
	search.CharLimit = 256
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	return Model{ctx: ctx, backend: backend, width: 100, height: 30, focus: focusInbox, composer: composer, search: search, spinner: spin, loading: true}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, syncNow(m.ctx, m.backend), tickSync())
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil
	case spinner.TickMsg:
		if m.loading || m.sending {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	case syncTick:
		if !m.loading {
			m.loading = true
			return m, tea.Batch(m.spinner.Tick, syncNow(m.ctx, m.backend), tickSync())
		}
		return m, tickSync()
	case syncMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.status = "Inbox updated"
		return m, loadInbox(m.ctx, m.backend, "")
	case inboxMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.inbox = msg.items
		if len(m.inbox) == 0 {
			m.selected = 0
			m.messages = nil
			return m, nil
		}
		if m.selected >= len(m.inbox) {
			m.selected = len(m.inbox) - 1
		}
		return m, m.openCurrent()
	case messagesMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		if current, ok := m.current(); ok && current.ID == msg.conversationID {
			m.messages = msg.items
		}
		return m, nil
	case searchResultsMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.searchResults = msg.items
		m.selectedSearch = 0
		m.focus = focusSearchResults
		return m, nil
	case sendMsg:
		m.sending = false
		if msg.err != nil {
			m.err = msg.err
			m.composer.Focus()
			return m, nil
		}
		m.composer.Reset()
		m.focus = focusChat
		m.status = "Message sent"
		return m, tea.Batch(loadInbox(m.ctx, m.backend, ""), m.loadCurrentMessages())
	}
	key, ok := message.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	if key.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.focus == focusComposer {
		return m.updateComposer(key)
	}
	if m.focus == focusSearch {
		return m.updateSearch(key)
	}
	if m.focus == focusSearchResults {
		return m.updateSearchResults(key)
	}
	return m.updateNavigation(key)
}

func (m Model) updateNavigation(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "q":
		return m, tea.Quit
	case "tab":
		if m.focus == focusInbox {
			m.focus = focusChat
		} else {
			m.focus = focusInbox
		}
	case "j", "down":
		if m.focus == focusInbox && m.selected < len(m.inbox)-1 {
			m.selected++
			m.clearHighlight()
			return m, m.openCurrent()
		}
	case "k", "up":
		if m.focus == focusInbox && m.selected > 0 {
			m.selected--
			m.clearHighlight()
			return m, m.openCurrent()
		}
	case "g":
		if len(m.inbox) > 0 {
			m.selected = 0
			m.clearHighlight()
			return m, m.openCurrent()
		}
	case "G":
		if len(m.inbox) > 0 {
			m.selected = len(m.inbox) - 1
			m.clearHighlight()
			return m, m.openCurrent()
		}
	case "enter", "i":
		if _, ok := m.current(); ok {
			m.focus = focusComposer
			return m, m.composer.Focus()
		}
	case "/":
		m.focus = focusSearch
		m.search.SetValue(m.query)
		m.search.CursorEnd()
		return m, m.search.Focus()
	case "R":
		if !m.loading {
			m.loading = true
			m.status = "Syncing"
			return m, tea.Batch(m.spinner.Tick, syncNow(m.ctx, m.backend))
		}
	}
	return m, nil
}

func (m Model) updateComposer(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.focus = focusChat
		m.composer.Blur()
		return m, nil
	case "enter":
		text := strings.TrimSpace(m.composer.Value())
		conversation, ok := m.current()
		if !ok || text == "" || m.sending {
			return m, nil
		}
		m.sending = true
		m.err = nil
		m.composer.Blur()
		return m, tea.Batch(m.spinner.Tick, send(m.ctx, m.backend, conversation.ID, text))
	}
	var cmd tea.Cmd
	m.composer, cmd = m.composer.Update(key)
	return m, cmd
}

func (m Model) updateSearch(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.search.Blur()
		m.focus = focusInbox
		return m, nil
	case "enter":
		m.query = strings.TrimSpace(m.search.Value())
		m.search.Blur()
		if m.query == "" {
			m.searchResults = nil
			m.focus = focusInbox
			return m, nil
		}
		return m, loadSearch(m.ctx, m.backend, m.query)
	}
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(key)
	return m, cmd
}

func (m Model) updateSearchResults(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.focus = focusInbox
		return m, nil
	case "/":
		m.focus = focusSearch
		m.search.SetValue(m.query)
		m.search.CursorEnd()
		return m, m.search.Focus()
	case "j", "down":
		if m.selectedSearch < len(m.searchResults)-1 {
			m.selectedSearch++
		}
	case "k", "up":
		if m.selectedSearch > 0 {
			m.selectedSearch--
		}
	case "g":
		m.selectedSearch = 0
	case "G":
		if len(m.searchResults) > 0 {
			m.selectedSearch = len(m.searchResults) - 1
		}
	case "enter":
		if m.selectedSearch < 0 || m.selectedSearch >= len(m.searchResults) {
			return m, nil
		}
		result := m.searchResults[m.selectedSearch]
		for index, conversation := range m.inbox {
			if conversation.ID != result.ConversationID {
				continue
			}
			m.selected = index
			m.focus = focusChat
			m.highlightMessage = result.MessageID
			m.highlightQuery = m.query
			return m, m.openCurrent()
		}
		m.err = fmt.Errorf("conversation for search result is no longer available")
	}
	return m, nil
}

func (m *Model) clearHighlight() {
	m.highlightMessage = ""
	m.highlightQuery = ""
}

func (m *Model) resize() {
	width := m.width - 38
	if m.width < 72 {
		width = m.width - 4
	}
	m.composer.SetWidth(max(20, width))
	m.search.SetWidth(max(20, min(60, m.width-8)))
}

func (m Model) current() (cache.Conversation, bool) {
	if m.selected < 0 || m.selected >= len(m.inbox) {
		return cache.Conversation{}, false
	}
	return m.inbox[m.selected], true
}

func (m Model) openCurrent() tea.Cmd {
	conversation, ok := m.current()
	if !ok {
		return nil
	}
	return tea.Batch(markRead(m.ctx, m.backend, conversation.ID), loadMessages(m.ctx, m.backend, conversation.ID))
}
func (m Model) loadCurrentMessages() tea.Cmd {
	conversation, ok := m.current()
	if !ok {
		return nil
	}
	return loadMessages(m.ctx, m.backend, conversation.ID)
}
func loadInbox(ctx context.Context, b Backend, q string) tea.Cmd {
	return func() tea.Msg { items, err := b.Inbox(ctx, q); return inboxMsg{items, err} }
}
func loadMessages(ctx context.Context, b Backend, id string) tea.Cmd {
	return func() tea.Msg { items, err := b.Messages(ctx, id); return messagesMsg{id, items, err} }
}
func loadSearch(ctx context.Context, b Backend, query string) tea.Cmd {
	return func() tea.Msg { items, err := b.Search(ctx, query); return searchResultsMsg{items, err} }
}
func markRead(ctx context.Context, b Backend, id string) tea.Cmd {
	return func() tea.Msg { _ = b.MarkRead(ctx, id); return nil }
}
func syncNow(ctx context.Context, b Backend) tea.Cmd {
	return func() tea.Msg { return syncMsg{b.Sync(ctx)} }
}
func send(ctx context.Context, b Backend, id, text string) tea.Cmd {
	return func() tea.Msg { return sendMsg{b.Send(ctx, id, text)} }
}
func tickSync() tea.Cmd {
	return tea.Tick(syncInterval, func(t time.Time) tea.Msg { return syncTick(t) })
}
