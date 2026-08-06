package xchat

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/willzys/xdm/internal/api"
	"github.com/willzys/xdm/internal/webapi"
)

type Session struct {
	command *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	stderr  *limitedBuffer
	mu      sync.Mutex
}

type sessionRequest struct {
	Operation      string                      `json:"op"`
	Material       *webapi.XChatUnlockMaterial `json:"material,omitempty"`
	PIN            []byte                      `json:"pin,omitempty"`
	ConversationID string                      `json:"conversationId,omitempty"`
	Text           string                      `json:"text,omitempty"`
}

type sessionResponse struct {
	Events []struct {
		ID             string `json:"id"`
		SenderID       string `json:"senderId"`
		ConversationID string `json:"conversationId"`
		CreatedAtMsec  int64  `json:"createdAtMsec"`
		Text           string `json:"text"`
		Verified       bool   `json:"verified"`
	} `json:"events"`
	MessageEvents int                          `json:"messageEvents"`
	Errors        int                          `json:"errors"`
	Error         string                       `json:"error"`
	Send          webapi.XChatEncryptedMessage `json:"send"`
}

func (s *Session) EncryptMessage(ctx context.Context, conversationID, text string) (webapi.XChatEncryptedMessage, error) {
	select {
	case <-ctx.Done():
		return webapi.XChatEncryptedMessage{}, ctx.Err()
	default:
	}
	response, err := s.exchange(sessionRequest{Operation: "encrypt", ConversationID: conversationID, Text: text})
	if err != nil {
		return webapi.XChatEncryptedMessage{}, err
	}
	if response.Send.MessageID == "" || response.Send.EncodedMessageCreateEvent == "" || response.Send.EncodedMessageEventSignature == "" {
		return webapi.XChatEncryptedMessage{}, errors.New("XChat crypto session returned an incomplete send payload")
	}
	return response.Send, nil
}

func NewSession(ctx context.Context, material webapi.XChatUnlockMaterial, pin []byte) (*Session, error) {
	runtimeDir, err := runtimeDirectory()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(runtimeDir, "node_modules", "@xdevplatform", "chat-xdk", "package.json")); err != nil {
		return nil, fmt.Errorf("XChat crypto runtime is not installed; run 'npm.cmd install --prefix \"%s\"'", runtimeDir)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		return nil, errors.New("Node.js 18 or newer is required for XChat decryption on Windows")
	}
	command := exec.CommandContext(ctx, node, filepath.Join(runtimeDir, "session.mjs"))
	command.Dir = runtimeDir
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdoutPipe, err := command.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, err
	}
	stderr := &limitedBuffer{limit: 1000}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		stdin.Close()
		return nil, err
	}
	session := &Session{command: command, stdin: stdin, stdout: bufio.NewReader(stdoutPipe), stderr: stderr}
	if _, err := session.exchange(sessionRequest{Operation: "unlock", Material: &material, PIN: pin}); err != nil {
		session.Close()
		return nil, err
	}
	return session, nil
}

func (s *Session) Decrypt(ctx context.Context, material webapi.XChatUnlockMaterial) (api.EventPage, error) {
	select {
	case <-ctx.Done():
		return api.EventPage{}, ctx.Err()
	default:
	}
	response, err := s.exchange(sessionRequest{Operation: "decrypt", Material: &material})
	if err != nil {
		return api.EventPage{}, err
	}
	if response.Errors > 0 {
		return api.EventPage{}, fmt.Errorf("XChat could not decrypt %d inbox events", response.Errors)
	}
	if len(response.Events) == 0 && len(material.Events) > 0 {
		return api.EventPage{}, fmt.Errorf("XChat returned no text messages after decoding %d raw events (%d message events)", len(material.Events), response.MessageEvents)
	}
	var page api.EventPage
	page.Includes.Users = append(page.Includes.Users, material.Users...)
	for _, item := range response.Events {
		if item.ID == "" || item.ConversationID == "" {
			continue
		}
		page.Data = append(page.Data, api.Event{
			ID: item.ID, EventType: "MessageCreate", Text: item.Text,
			SenderID: item.SenderID, ConversationID: item.ConversationID,
			ParticipantIDs: append([]string(nil), material.Participants[item.ConversationID]...),
			CreatedAt:      time.UnixMilli(item.CreatedAtMsec).UTC(),
		})
	}
	page.Meta.ResultCount = len(page.Data)
	return page, nil
}

func (s *Session) exchange(request sessionRequest) (sessionResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.command == nil {
		return sessionResponse{}, errors.New("XChat crypto session is closed")
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return sessionResponse{}, err
	}
	defer clear(encoded)
	if _, err := s.stdin.Write(encoded); err != nil {
		return sessionResponse{}, fmt.Errorf("writing to XChat crypto session: %w", err)
	}
	if _, err := s.stdin.Write([]byte{'\n'}); err != nil {
		return sessionResponse{}, fmt.Errorf("writing to XChat crypto session: %w", err)
	}
	line, err := s.stdout.ReadBytes('\n')
	if err != nil {
		return sessionResponse{}, fmt.Errorf("reading XChat crypto session: %w%s", err, limitedStderr(s.stderr.String()))
	}
	defer clear(line)
	var response sessionResponse
	if err := json.Unmarshal(line, &response); err != nil {
		return sessionResponse{}, errors.New("XChat crypto session returned an invalid response")
	}
	if response.Error != "" {
		return sessionResponse{}, errors.New(response.Error)
	}
	return response, nil
}

func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.command == nil {
		return nil
	}
	encoded, _ := json.Marshal(sessionRequest{Operation: "close"})
	_, _ = s.stdin.Write(append(encoded, '\n'))
	_ = s.stdin.Close()
	err := s.command.Wait()
	s.command = nil
	return err
}

type limitedBuffer struct {
	mu    sync.Mutex
	data  []byte
	limit int
}

func (b *limitedBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.limit - len(b.data)
	if remaining > 0 {
		b.data = append(b.data, value[:min(remaining, len(value))]...)
	}
	return len(value), nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}
