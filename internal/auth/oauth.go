package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	authorizeURL = "https://x.com/i/oauth2/authorize"
	tokenURL = "https://api.x.com/2/oauth2/token"
)

var scopes = []string{"dm.read", "dm.write", "tweet.read", "users.read", "offline.access"}

type Token struct {
	AccessToken string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType string `json:"token_type"`
	Scope string `json:"scope"`
	ExpiresAt time.Time `json:"expires_at"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType string `json:"token_type"`
	Scope string `json:"scope"`
	ExpiresIn int `json:"expires_in"`
}

type Manager struct {
	clientID string
	redirectURI string
	store Store
	httpClient *http.Client
	mu sync.Mutex
}

func NewManager(clientID, redirectURI string, store Store) *Manager {
	return &Manager{clientID: clientID, redirectURI: redirectURI, store: store, httpClient: &http.Client{Timeout: 30 * time.Second}}
}

func (m *Manager) AccessToken(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	token, err := m.store.Load()
	if err != nil { return "", err }
	if token.AccessToken != "" && time.Until(token.ExpiresAt) > time.Minute { return token.AccessToken, nil }
	if token.RefreshToken == "" { return "", ErrNotAuthenticated }
	refreshed, err := m.exchange(ctx, url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {token.RefreshToken}, "client_id": {m.clientID},
	})
	if err != nil { return "", fmt.Errorf("refreshing OAuth token: %w", err) }
	if refreshed.RefreshToken == "" { refreshed.RefreshToken = token.RefreshToken }
	if err := m.store.Save(refreshed); err != nil { return "", err }
	return refreshed.AccessToken, nil
}

func (m *Manager) Authorize(ctx context.Context, openBrowser bool) error {
	if strings.TrimSpace(m.clientID) == "" { return errors.New("OAuth client ID is required") }
	redirect, err := url.Parse(m.redirectURI)
	if err != nil || redirect.Scheme != "http" || redirect.Hostname() != "127.0.0.1" {
		return errors.New("redirect URI must be an http://127.0.0.1 callback URL")
	}
	state, err := randomString(32)
	if err != nil { return err }
	verifier, err := randomString(64)
	if err != nil { return err }
	digest := sha256.Sum256([]byte(verifier))
	params := url.Values{
		"response_type": {"code"}, "client_id": {m.clientID}, "redirect_uri": {m.redirectURI},
		"scope": {strings.Join(scopes, " ")}, "state": {state},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(digest[:])}, "code_challenge_method": {"S256"},
	}
	listener, err := net.Listen("tcp", redirect.Host)
	if err != nil { return fmt.Errorf("starting OAuth callback listener: %w", err) }
	defer listener.Close()
	type callback struct{ code, state, oauthErr string }
	result := make(chan callback, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(redirect.Path, func(w http.ResponseWriter, r *http.Request) {
		select {
		case result <- callback{code: r.URL.Query().Get("code"), state: r.URL.Query().Get("state"), oauthErr: r.URL.Query().Get("error")}:
		default:
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("Authorization received. You can close this window and return to xdm."))
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	defer server.Shutdown(context.Background())
	authURL := authorizeURL + "?" + params.Encode()
	fmt.Println("Open this URL to authorize xdm:")
	fmt.Println(authURL)
	if openBrowser { _ = openURL(authURL) }
	select {
	case <-ctx.Done(): return ctx.Err()
	case received := <-result:
		if received.oauthErr != "" { return fmt.Errorf("authorization rejected: %s", received.oauthErr) }
		if received.state != state || received.code == "" { return errors.New("invalid OAuth callback") }
		token, err := m.exchange(ctx, url.Values{
			"code": {received.code}, "grant_type": {"authorization_code"}, "client_id": {m.clientID},
			"redirect_uri": {m.redirectURI}, "code_verifier": {verifier},
		})
		if err != nil { return err }
		return m.store.Save(token)
	}
}

func (m *Manager) exchange(ctx context.Context, values url.Values) (Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(values.Encode()))
	if err != nil { return Token{}, err }
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := m.httpClient.Do(req)
	if err != nil { return Token{}, err }
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 { return Token{}, fmt.Errorf("token endpoint returned %s", response.Status) }
	var result tokenResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil { return Token{}, err }
	if result.AccessToken == "" { return Token{}, errors.New("token endpoint returned no access token") }
	return Token{AccessToken: result.AccessToken, RefreshToken: result.RefreshToken, TokenType: result.TokenType, Scope: result.Scope, ExpiresAt: time.Now().Add(time.Duration(result.ExpiresIn)*time.Second)}, nil
}

func randomString(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil { return "", err }
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func openURL(value string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows": command = exec.Command("rundll32", "url.dll,FileProtocolHandler", value)
	case "darwin": command = exec.Command("open", value)
	default: command = exec.Command("xdg-open", value)
	}
	return command.Start()
}
