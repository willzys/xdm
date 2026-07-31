package cache

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/willzys/xdm/internal/api"
	_ "modernc.org/sqlite"
)

type Conversation struct {
	ID          string
	Title       string
	Preview     string
	UpdatedAt   time.Time
	UnreadCount int
}

type Message struct {
	ID             string
	ConversationID string
	SenderID       string
	SenderName     string
	Text           string
	CreatedAt      time.Time
}

type Cache struct{ db *sql.DB }

func Open(path string) (*Cache, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	cache := &Cache{db: db}
	if err := cache.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return cache, nil
}

func (c *Cache) Close() error { return c.db.Close() }

func (c *Cache) migrate(ctx context.Context) error {
	const schema = `
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;
CREATE TABLE IF NOT EXISTS users (id TEXT PRIMARY KEY, name TEXT NOT NULL, username TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS conversations (
  id TEXT PRIMARY KEY, title TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL, last_read_at TEXT
);
CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  sender_id TEXT NOT NULL, text TEXT NOT NULL, created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS messages_conversation_created ON messages(conversation_id, created_at);
CREATE INDEX IF NOT EXISTS messages_text ON messages(text);`
	_, err := c.db.ExecContext(ctx, schema)
	return err
}

func (c *Cache) SavePage(ctx context.Context, page api.EventPage, selfID string) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	users := make(map[string]api.User, len(page.Includes.Users))
	for _, user := range page.Includes.Users {
		users[user.ID] = user
		if _, err := tx.ExecContext(ctx, `INSERT INTO users(id,name,username) VALUES(?,?,?)
ON CONFLICT(id) DO UPDATE SET name=excluded.name, username=excluded.username`, user.ID, user.Name, user.Username); err != nil {
			return err
		}
	}
	for _, event := range page.Data {
		if event.EventType != "MessageCreate" || event.ConversationID == "" {
			continue
		}
		title := conversationTitle(event.ConversationID, event.ParticipantIDs, selfID, users)
		created := event.CreatedAt.UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `INSERT INTO conversations(id,title,updated_at) VALUES(?,?,?)
ON CONFLICT(id) DO UPDATE SET
 title=CASE WHEN excluded.title<>'' THEN excluded.title ELSE conversations.title END,
 updated_at=MAX(conversations.updated_at,excluded.updated_at)`, event.ConversationID, title, created); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO messages(id,conversation_id,sender_id,text,created_at) VALUES(?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET text=excluded.text`, event.ID, event.ConversationID, event.SenderID, event.Text, created); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func conversationTitle(id string, participants []string, selfID string, users map[string]api.User) string {
	ids := append([]string(nil), participants...)
	if len(ids) == 0 && strings.Contains(id, "-") {
		ids = strings.Split(id, "-")
	}
	var names []string
	for _, participantID := range ids {
		if participantID == selfID {
			continue
		}
		if user, ok := users[participantID]; ok {
			name := user.Name
			if name == "" {
				name = "@" + user.Username
			}
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) > 0 {
		return strings.Join(names, ", ")
	}
	if strings.Contains(id, "-") {
		return "Direct message"
	}
	return "Group conversation"
}

func (c *Cache) Conversations(ctx context.Context, query string) ([]Conversation, error) {
	like := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	rows, err := c.db.QueryContext(ctx, `SELECT c.id,c.title,COALESCE(latest.text,''),c.updated_at,
COUNT(CASE WHEN m.created_at>COALESCE(c.last_read_at,'') THEN 1 END)
FROM conversations c
LEFT JOIN messages m ON m.conversation_id=c.id
LEFT JOIN messages latest ON latest.id=(SELECT id FROM messages WHERE conversation_id=c.id ORDER BY created_at DESC LIMIT 1)
WHERE ?='%%' OR LOWER(c.title) LIKE ? OR EXISTS (
 SELECT 1 FROM messages searched WHERE searched.conversation_id=c.id AND LOWER(searched.text) LIKE ?)
GROUP BY c.id,c.title,latest.text,c.updated_at ORDER BY c.updated_at DESC`, like, like, like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var conversations []Conversation
	for rows.Next() {
		var item Conversation
		var updated string
		if err := rows.Scan(&item.ID, &item.Title, &item.Preview, &updated, &item.UnreadCount); err != nil {
			return nil, err
		}
		item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		conversations = append(conversations, item)
	}
	return conversations, rows.Err()
}

func (c *Cache) Messages(ctx context.Context, conversationID string) ([]Message, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT m.id,m.conversation_id,m.sender_id,
COALESCE(NULLIF(u.name,''),'@'||u.username,''),m.text,m.created_at
FROM messages m LEFT JOIN users u ON u.id=m.sender_id
WHERE m.conversation_id=? ORDER BY m.created_at`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []Message
	for rows.Next() {
		var item Message
		var created string
		if err := rows.Scan(&item.ID, &item.ConversationID, &item.SenderID, &item.SenderName, &item.Text, &created); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		messages = append(messages, item)
	}
	return messages, rows.Err()
}

func (c *Cache) MarkRead(ctx context.Context, conversationID string) error {
	_, err := c.db.ExecContext(ctx, `UPDATE conversations SET last_read_at=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339Nano), conversationID)
	return err
}

func (c *Cache) SaveSent(ctx context.Context, conversationID, eventID, senderID, text string, sentAt time.Time) error {
	page := api.EventPage{Data: []api.Event{{ID: eventID, EventType: "MessageCreate", ConversationID: conversationID, SenderID: senderID, Text: text, CreatedAt: sentAt}}}
	if err := c.SavePage(ctx, page, senderID); err != nil {
		return fmt.Errorf("saving sent message: %w", err)
	}
	return c.MarkRead(ctx, conversationID)
}
