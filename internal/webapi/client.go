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
	xchatInboxURL  = "https://api.x.com/graphql/Gl7r1aY59L7jLBjVC98lqg/GetInitialXChatPageQuery"
	xchatKeysURL   = "https://api.x.com/graphql/RQAjOoIX9dIsHoVjuVV0Iw/GetPublicKeys"
	webBearerToken = "AAAAAAAAAAAAAAAAAAAAANRILgAAAAAAnNwIzUejRCOuH5E6I8xnZz4puTs%3D1Zv7ttfk8LF81IUq16cHjhLTvJu4FA33AGWWjCpTnA"
)

type Client struct {
	baseURL      string
	xchatURL     string
	xchatKeysURL string
	httpClient   *http.Client
	self         api.User
	clientUUID   string
}

type InboxDiagnostics struct {
	TopLevelFields         []string
	InitialStateFields     []string
	EntryKinds             map[string]int
	HasInitialState        bool
	ConversationCount      int
	EntryCount             int
	MessageEntryCount      int
	UserCount              int
	XChatItemCount         int
	XChatEventCount        int
	XChatKeyEventCount     int
	XChatErrorCount        int
	XChatMessageCount      int
	XChatEncryptedCount    int
	XChatPlaintextCount    int
	XChatDecodeFailures    int
	XChatPublicKeyVersions int
	XChatJuiceboxRealms    int
	XChatHasJuiceboxConfig bool
	XChatManagedPIN        bool
}

func NewClient(httpClient *http.Client, self api.User) (*Client, error) {
	if httpClient == nil {
		return nil, errors.New("web HTTP client is required")
	}
	if self.ID == "" || self.Username == "" {
		return nil, errors.New("web session account identity is incomplete; run 'xdm auth web' again")
	}
	clientUUID, err := newRequestID()
	if err != nil {
		return nil, fmt.Errorf("generating web client ID: %w", err)
	}
	return &Client{baseURL: defaultBaseURL, xchatURL: xchatInboxURL, xchatKeysURL: xchatKeysURL, httpClient: httpClient, self: self, clientUUID: strings.ToLower(clientUUID)}, nil
}

func newClient(baseURL string, httpClient *http.Client, self api.User) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{baseURL: baseURL, xchatURL: baseURL + "/graphql/GetInitialXChatPageQuery", xchatKeysURL: baseURL + "/graphql/GetPublicKeys", httpClient: httpClient, self: self, clientUUID: "00000000-0000-4000-8000-000000000000"}
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
	xchat, err := c.getInitialXChatPage(ctx)
	if err != nil {
		return InboxDiagnostics{}, fmt.Errorf("checking XChat inbox: %w", err)
	}
	diagnostics.XChatItemCount = len(xchat.Data.Page.Items)
	diagnostics.XChatErrorCount = len(xchat.Errors) + len(xchat.Data.Page.Errors)
	for _, item := range xchat.Data.Page.Items {
		diagnostics.XChatEventCount += len(item.LatestMessageEvents) + len(item.EncodedMessageEvents)
		diagnostics.XChatKeyEventCount += len(item.LatestConversationKeyChangeEvents)
		for _, encoded := range append(append([]string(nil), item.LatestMessageEvents...), item.EncodedMessageEvents...) {
			message, encrypted, decodeErr := classifyXChatEvent(encoded)
			if decodeErr != nil {
				diagnostics.XChatDecodeFailures++
				continue
			}
			if !message {
				continue
			}
			diagnostics.XChatMessageCount++
			if encrypted {
				diagnostics.XChatEncryptedCount++
			} else {
				diagnostics.XChatPlaintextCount++
			}
		}
	}
	keys, err := c.getXChatPublicKeys(ctx, []string{c.self.ID})
	if err != nil {
		return InboxDiagnostics{}, fmt.Errorf("checking XChat public keys: %w", err)
	}
	for _, user := range keys.Data.Users {
		if user.RestID != c.self.ID {
			continue
		}
		diagnostics.XChatManagedPIN = user.Result.PublicKeys.ManagedPIN
		for _, item := range user.Result.PublicKeys.Items {
			diagnostics.XChatPublicKeyVersions++
			diagnostics.XChatJuiceboxRealms += len(item.TokenMap.Tokens)
			if item.TokenMap.ConfigJSON != "" {
				diagnostics.XChatHasJuiceboxConfig = true
			}
		}
	}
	return diagnostics, nil
}

func (c *Client) getXChatPublicKeys(ctx context.Context, userIDs []string) (xchatPublicKeysResponse, error) {
	variables := struct {
		IDs                   []string `json:"ids"`
		IncludeJuiceboxTokens bool     `json:"include_juicebox_tokens"`
	}{userIDs, true}
	encoded, err := json.Marshal(variables)
	if err != nil {
		return xchatPublicKeysResponse{}, err
	}
	values := url.Values{"variables": {string(encoded)}}
	var response xchatPublicKeysResponse
	if err := c.doURL(ctx, http.MethodGet, c.xchatKeysURL+"?"+values.Encode(), "https://x.com/messages", nil, &response); err != nil {
		return xchatPublicKeysResponse{}, err
	}
	return response, nil
}

func (c *Client) getInitialXChatPage(ctx context.Context) (xchatInboxResponse, error) {
	settings := struct {
		InboxConversationEventLimit int `json:"inbox_conversation_event_limit"`
		InboxConversationLimit      int `json:"inbox_conversation_limit"`
		ConversationEventLimit      int `json:"conversation_event_limit"`
		UserEventLimit              int `json:"user_event_limit"`
	}{5, 20, 200, 500}
	variables := struct {
		QuerySettings      any `json:"query_settings"`
		MessagePullVersion int `json:"message_pull_version"`
	}{settings, 1761251295}
	encoded, err := json.Marshal(variables)
	if err != nil {
		return xchatInboxResponse{}, err
	}
	values := url.Values{"variables": {string(encoded)}}
	var response xchatInboxResponse
	if err := c.doURL(ctx, http.MethodGet, c.xchatURL+"?"+values.Encode(), "https://x.com/messages", nil, &response); err != nil {
		return xchatInboxResponse{}, err
	}
	return response, nil
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
	return c.doURL(ctx, method, c.baseURL+path, referer, body, target)
}

func (c *Client) doURL(ctx context.Context, method, requestURL, referer string, body, target any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "*/*")
	request.Header.Set("Authorization", "Bearer "+webBearerToken)
	request.Header.Set("Referer", referer)
	request.Header.Set("X-Twitter-Active-User", "yes")
	request.Header.Set("X-Twitter-Auth-Type", "OAuth2Session")
	request.Header.Set("X-Twitter-Client-Language", "en")
	if c.clientUUID != "" {
		request.Header.Set("X-Client-UUID", c.clientUUID)
	}
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
