package demo

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/willzys/xdm/internal/cache"
)

const conversationID = "demo-willian"

type Backend struct {
	mu            sync.RWMutex
	conversation  cache.Conversation
	messages      []cache.Message
	nextMessageID int
}

func New() *Backend {
	now := time.Now()
	return &Backend{
		conversation: cache.Conversation{
			ID:          conversationID,
			Title:       "willian",
			Preview:     "send me literally anything, i'm fake anyway xD",
			UpdatedAt:   now.Add(-2 * time.Minute),
			UnreadCount: 2,
		},
		messages: []cache.Message{
			{ID: "demo-message-1", ConversationID: conversationID, SenderID: "willian", SenderName: "willian", Text: "yo, this tui is kinda neat ngl :p", CreatedAt: now.Add(-6 * time.Minute)},
			{ID: "demo-message-2", ConversationID: conversationID, SenderID: "demo-self", SenderName: "you", Text: "lol fr, i'm just poking around", CreatedAt: now.Add(-4 * time.Minute)},
			{ID: "demo-message-3", ConversationID: conversationID, SenderID: "willian", SenderName: "willian", Text: "send me literally anything, i'm fake anyway xD", CreatedAt: now.Add(-2 * time.Minute)},
		},
		nextMessageID: 4,
	}
}

func (b *Backend) Inbox(ctx context.Context, query string) ([]cache.Conversation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	query = strings.ToLower(strings.TrimSpace(query))
	if query != "" && !strings.Contains(b.conversation.Title, query) && !messagesContain(b.messages, query) {
		return nil, nil
	}
	return []cache.Conversation{b.conversation}, nil
}

func (b *Backend) Messages(ctx context.Context, id string) ([]cache.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if id != conversationID {
		return nil, fmt.Errorf("demo conversation %q not found", id)
	}
	return append([]cache.Message(nil), b.messages...), nil
}

func (b *Backend) Search(ctx context.Context, query string) ([]cache.SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil, nil
	}
	var results []cache.SearchResult
	for index := len(b.messages) - 1; index >= 0; index-- {
		message := b.messages[index]
		if !strings.Contains(strings.ToLower(message.Text), query) {
			continue
		}
		results = append(results, cache.SearchResult{
			MessageID:         message.ID,
			ConversationID:    conversationID,
			ConversationTitle: b.conversation.Title,
			SenderName:        message.SenderName,
			Text:              message.Text,
			CreatedAt:         message.CreatedAt,
		})
	}
	return results, nil
}

func (b *Backend) MarkRead(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if id != conversationID {
		return fmt.Errorf("demo conversation %q not found", id)
	}
	b.conversation.UnreadCount = 0
	return nil
}

func (b *Backend) Sync(ctx context.Context) error {
	return ctx.Err()
}

func (b *Backend) Send(ctx context.Context, id, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if id != conversationID {
		return fmt.Errorf("demo conversation %q not found", id)
	}
	now := time.Now()
	b.messages = append(b.messages, cache.Message{
		ID:             fmt.Sprintf("demo-message-%d", b.nextMessageID),
		ConversationID: conversationID,
		SenderID:       "demo-self",
		SenderName:     "you",
		Text:           text,
		CreatedAt:      now,
	})
	b.nextMessageID++
	b.conversation.Preview = text
	b.conversation.UpdatedAt = now
	b.conversation.UnreadCount = 0
	return nil
}

func messagesContain(messages []cache.Message, query string) bool {
	for _, message := range messages {
		if strings.Contains(strings.ToLower(message.Text), query) || strings.Contains(message.SenderName, query) {
			return true
		}
	}
	return false
}
