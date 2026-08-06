package webapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/willzys/xdm/internal/api"
)

const (
	defaultBaseURL = "https://x.com/i/api"
	webBearerToken = "AAAAAAAAAAAAAAAAAAAAANRILgAAAAAAnNwIzUejRCOuH5E6I8xnZz4puTs%3D1Zv7ttfk8LF81IUq16cHjhLTvJu4FA33AGWWjCpTnA"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	self       api.User
}

type InboxDiagnostics struct {
	TopLevelFields     []string
	InitialStateFields []string
	EntryKinds         map[string]int
	HasInitialState    bool
	ConversationCount  int
	EntryCount         int
	MessageEntryCount  int
	UserCount          int
}

func NewClient(httpClient *http.Client, self api.User) (*Client, error) {
	if httpClient == nil {
		return nil, errors.New("web HTTP client is required")
	}
	if self.ID == "" || self.Username == "" {
		return nil, errors.New("web session account identity is incomplete; run 'xdm auth web' again")
	}
	return &Client{baseURL: defaultBaseURL, httpClient: httpClient, self: self}, nil
}

func newClient(baseURL string, httpClient *http.Client, self api.User) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient, self: self}
}

func (c *Client) Me(ctx context.Context) (api.User, error) {
	return c.self, nil
}

func (c *Client) Events(ctx context.Context, paginationToken string) (api.EventPage, error) {
	if paginationToken != "" {
		return api.EventPage{}, errors.New("X web DM pagination is not supported yet")
	}
	values := inboxParameters()
	var response inboxResponse
	if err := c.do(ctx, http.MethodGet, "/1.1/dm/inbox_initial_state.json?"+values.Encode(), "https://x.com/messages", nil, &response); err != nil {
		return api.EventPage{}, err
	}
	return response.eventPage()
}

func (c *Client) DiagnoseInbox(ctx context.Context) (InboxDiagnostics, error) {
	var topLevel map[string]json.RawMessage
	path := "/1.1/dm/inbox_initial_state.json?" + inboxParameters().Encode()
	if err := c.do(ctx, http.MethodGet, path, "https://x.com/messages", nil, &topLevel); err != nil {
		return InboxDiagnostics{}, err
	}
	diagnostics := InboxDiagnostics{
		TopLevelFields: sortedKeys(topLevel),
		EntryKinds:     make(map[string]int),
	}
	rawState, ok := topLevel["inbox_initial_state"]
	if !ok {
		return diagnostics, nil
	}
	diagnostics.HasInitialState = true
	var state map[string]json.RawMessage
	if err := json.Unmarshal(rawState, &state); err != nil {
		return InboxDiagnostics{}, errors.New("decoding X web inbox structure")
	}
	diagnostics.InitialStateFields = sortedKeys(state)
	var conversations map[string]json.RawMessage
	_ = json.Unmarshal(state["conversations"], &conversations)
	diagnostics.ConversationCount = len(conversations)
	var users map[string]json.RawMessage
	_ = json.Unmarshal(state["users"], &users)
	diagnostics.UserCount = len(users)
	var entries []map[string]json.RawMessage
	_ = json.Unmarshal(state["entries"], &entries)
	diagnostics.EntryCount = len(entries)
	for _, entry := range entries {
		for kind := range entry {
			diagnostics.EntryKinds[kind]++
		}
		if _, ok := entry["message"]; ok {
			diagnostics.MessageEntryCount++
		}
	}
	return diagnostics, nil
}

func (c *Client) Send(ctx context.Context, conversationID, text string) (api.SendResult, error) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return api.SendResult{}, errors.New("conversation ID is required")
	}
	if strings.TrimSpace(text) == "" {
		return api.SendResult{}, errors.New("message text is required")
	}
	requestID, err := newRequestID()
	if err != nil {
		return api.SendResult{}, fmt.Errorf("generating web DM request ID: %w", err)
	}
	payload := struct {
		CardsPlatform     string `json:"cards_platform"`
		ConversationID    string `json:"conversation_id"`
		DMUsers           bool   `json:"dm_users"`
		IncludeCards      int    `json:"include_cards"`
		IncludeQuoteCount bool   `json:"include_quote_count"`
		RecipientIDs      bool   `json:"recipient_ids"`
		RequestID         string `json:"request_id"`
		Text              string `json:"text"`
	}{
		CardsPlatform: "Web-12", ConversationID: conversationID, DMUsers: false,
		IncludeCards: 1, IncludeQuoteCount: true, RecipientIDs: false,
		RequestID: requestID, Text: text,
	}
	var response sendResponse
	referer := "https://x.com/messages/" + url.PathEscape(conversationID)
	if err := c.do(ctx, http.MethodPost, "/1.1/dm/new2.json", referer, payload, &response); err != nil {
		return api.SendResult{}, err
	}
	return response.result(conversationID)
}

func (c *Client) do(ctx context.Context, method, path, referer string, body, target any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "*/*")
	request.Header.Set("Authorization", "Bearer "+webBearerToken)
	request.Header.Set("Referer", referer)
	request.Header.Set("X-Twitter-Active-User", "yes")
	request.Header.Set("X-Twitter-Auth-Type", "OAuth2Session")
	request.Header.Set("X-Twitter-Client-Language", "en")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", "https://x.com")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("calling X web API: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %w", method, request.URL.Path, decodeWebError(response))
	}
	if target == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 32<<20)).Decode(target); err != nil {
		return fmt.Errorf("decoding X web response: %w", err)
	}
	return nil
}

func inboxParameters() url.Values {
	return url.Values{
		"cards_platform":                    {"Web-12"},
		"dm_users":                          {"false"},
		"ext":                               {"mediaColor,altText,mediaStats,highlightedLabel,cameraMoment"},
		"filter_low_quality":                {"false"},
		"include_blocked_by":                {"1"},
		"include_blocking":                  {"1"},
		"include_can_dm":                    {"1"},
		"include_can_media_tag":             {"1"},
		"include_cards":                     {"1"},
		"include_composer_source":           {"true"},
		"include_ext_alt_text":              {"true"},
		"include_ext_media_color":           {"true"},
		"include_followed_by":               {"1"},
		"include_groups":                    {"true"},
		"include_inbox_timelines":           {"true"},
		"include_mute_edge":                 {"1"},
		"include_profile_interstitial_type": {"1"},
		"include_reply_count":               {"1"},
		"include_want_retweets":             {"1"},
		"skip_status":                       {"1"},
		"supports_reactions":                {"true"},
		"tweet_mode":                        {"extended"},
	}
}

type webErrorResponse struct {
	Errors []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

func decodeWebError(response *http.Response) error {
	limited, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	var details webErrorResponse
	if json.Unmarshal(limited, &details) == nil && len(details.Errors) > 0 {
		parts := make([]string, 0, len(details.Errors))
		for _, item := range details.Errors {
			parts = append(parts, fmt.Sprintf("%d: %s", item.Code, item.Message))
		}
		return fmt.Errorf("X web API returned %s (%s)", response.Status, strings.Join(parts, "; "))
	}
	return fmt.Errorf("X web API returned %s", response.Status)
}

func parseWebTime(value string) time.Time {
	if milliseconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		return time.UnixMilli(milliseconds).UTC()
	}
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}

func sortedKeys(values map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
