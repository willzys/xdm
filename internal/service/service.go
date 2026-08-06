package service

import (
	"context"
	"sync"
	"time"

	"github.com/willzys/xdm/internal/api"
	"github.com/willzys/xdm/internal/cache"
)

type Service struct {
	api   Client
	cache *cache.Cache
	mu    sync.Mutex
	self  api.User
}

type Client interface {
	Me(context.Context) (api.User, error)
	Events(context.Context, string) (api.EventPage, error)
	Send(context.Context, string, string) (api.SendResult, error)
}

func New(apiClient Client, messageCache *cache.Cache) *Service {
	return &Service{api: apiClient, cache: messageCache}
}

func (s *Service) Inbox(ctx context.Context, query string) ([]cache.Conversation, error) {
	return s.cache.Conversations(ctx, query)
}

func (s *Service) Messages(ctx context.Context, conversationID string) ([]cache.Message, error) {
	return s.cache.Messages(ctx, conversationID)
}

func (s *Service) Search(ctx context.Context, query string) ([]cache.SearchResult, error) {
	return s.cache.Search(ctx, query)
}

func (s *Service) MarkRead(ctx context.Context, conversationID string) error {
	return s.cache.MarkRead(ctx, conversationID)
}

func (s *Service) Sync(ctx context.Context) error {
	self, err := s.currentUser(ctx)
	if err != nil {
		return err
	}
	page, err := s.api.Events(ctx, "")
	if err != nil {
		return err
	}
	return s.cache.SavePage(ctx, page, self.ID)
}

func (s *Service) Send(ctx context.Context, conversationID, text string) error {
	self, err := s.currentUser(ctx)
	if err != nil {
		return err
	}
	result, err := s.api.Send(ctx, conversationID, text)
	if err != nil {
		return err
	}
	return s.cache.SaveSent(ctx, conversationID, result.Data.EventID, self.ID, text, time.Now())
}

func (s *Service) currentUser(ctx context.Context) (api.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.self.ID != "" {
		return s.self, nil
	}
	user, err := s.api.Me(ctx)
	if err != nil {
		return api.User{}, err
	}
	s.self = user
	return user, nil
}
