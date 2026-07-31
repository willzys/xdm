package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.x.com"

type TokenSource interface {
	AccessToken(context.Context) (string, error)
}

type Client struct {
	baseURL    string
	httpClient *http.Client
	tokens     TokenSource
}

func NewClient(tokens TokenSource) *Client {
	return &Client{
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		tokens:     tokens,
	}
}

func newClient(baseURL string, httpClient *http.Client, tokens TokenSource) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient, tokens: tokens}
}

func (c *Client) Me(ctx context.Context) (User, error) {
	var response struct {
		Data User `json:"data"`
	}
	err := c.do(ctx, http.MethodGet, "/2/users/me?user.fields=name,username", nil, &response)
	return response.Data, err
}

func (c *Client) Events(ctx context.Context, paginationToken string) (EventPage, error) {
	values := url.Values{}
	values.Set("max_results", "100")
	values.Set("event_types", "MessageCreate")
	values.Set("dm_event.fields", "id,event_type,text,sender_id,dm_conversation_id,participant_ids,created_at")
	values.Set("expansions", "sender_id,participant_ids")
	values.Set("user.fields", "id,name,username")
	if paginationToken != "" {
		values.Set("pagination_token", paginationToken)
	}
	var page EventPage
	err := c.do(ctx, http.MethodGet, "/2/dm_events?"+values.Encode(), nil, &page)
	return page, err
}

func (c *Client) Send(ctx context.Context, conversationID, text string) (SendResult, error) {
	if strings.TrimSpace(conversationID) == "" {
		return SendResult{}, errors.New("conversation ID is required")
	}
	if strings.TrimSpace(text) == "" {
		return SendResult{}, errors.New("message text is required")
	}
	body := struct {
		Text string `json:"text"`
	}{Text: text}
	var result SendResult
	path := "/2/dm_conversations/" + url.PathEscape(conversationID) + "/messages"
	err := c.do(ctx, http.MethodPost, path, body, &result)
	return result, err
}

func (c *Client) do(ctx context.Context, method, path string, body, target any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	token, err := c.tokens.AccessToken(ctx)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		limited, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("X API returned %s: %s", response.Status, strings.TrimSpace(string(limited)))
	}
	if target == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("decoding X API response: %w", err)
	}
	return nil
}
