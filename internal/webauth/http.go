package webauth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Transport struct {
	base    http.RoundTripper
	store   *Store
	mu      sync.Mutex
	session Session
}

func NewHTTPClient(session Session, store *Store) (*http.Client, error) {
	if store == nil {
		return nil, errors.New("web session store is required")
	}
	if err := session.Validate(time.Now()); err != nil {
		return nil, err
	}
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &Transport{
			base: http.DefaultTransport, store: store, session: session,
		},
	}, nil
}

func (t *Transport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil || !allowedCookieDomain(request.URL.Hostname()) {
		return nil, errors.New("refusing to send an X web session to a non-X host")
	}
	if !strings.EqualFold(request.URL.Scheme, "https") {
		return nil, errors.New("refusing to send an X web session without HTTPS")
	}
	t.mu.Lock()
	session := t.session.clone()
	t.mu.Unlock()
	if err := session.Validate(time.Now()); err != nil {
		return nil, err
	}
	cloned := request.Clone(request.Context())
	cloned.Header = request.Header.Clone()
	cloned.Header.Del("Cookie")
	session.Apply(cloned)
	response, err := t.base.RoundTrip(cloned)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	previous := t.session.clone()
	if updateSessionCookies(&t.session, response, cloned.URL.Hostname(), time.Now().UTC()) {
		if err := t.store.Save(t.session); err != nil {
			t.session = previous
			response.Body.Close()
			return nil, fmt.Errorf("saving rotated X web session: %w", err)
		}
	}
	return response, nil
}

func (t *Transport) Session() Session {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.session.clone()
}

func updateSessionCookies(session *Session, response *http.Response, responseHost string, now time.Time) bool {
	if session == nil || response == nil || !allowedCookieDomain(responseHost) {
		return false
	}
	updates := response.Cookies()
	if len(updates) == 0 {
		return false
	}
	byKey := make(map[string]Cookie, len(session.Cookies)+len(updates))
	for _, cookie := range session.Cookies {
		key := cookieKey(cookie.Domain, cookie.Path, cookie.Name)
		byKey[key] = cookie
	}
	changed := false
	for _, cookie := range updates {
		domain := strings.ToLower(cookie.Domain)
		hostOnly := domain == ""
		if domain == "" {
			domain = strings.ToLower(responseHost)
		}
		if !allowedCookieDomain(domain) || !domainMatchesHost(domain, responseHost) {
			continue
		}
		path := cookie.Path
		if path == "" {
			path = "/"
		}
		key := cookieKey(domain, path, cookie.Name)
		if cookie.MaxAge < 0 || (!cookie.Expires.IsZero() && !cookie.Expires.After(now)) {
			if _, ok := byKey[key]; ok {
				delete(byKey, key)
				changed = true
			}
			continue
		}
		value := Cookie{
			Name: cookie.Name, Value: cookie.Value, Domain: domain, Path: path,
			Expires: cookie.Expires, Secure: cookie.Secure, HTTPOnly: cookie.HttpOnly, HostOnly: hostOnly,
		}
		if previous, ok := byKey[key]; !ok || previous != value {
			byKey[key] = value
			changed = true
		}
	}
	if !changed {
		return false
	}
	cookies := make([]Cookie, 0, len(byKey))
	for _, cookie := range byKey {
		cookies = append(cookies, cookie)
	}
	session.Cookies = normalizeCookies(cookies)
	session.ValidatedAt = now
	return true
}

func cookieKey(domain, path, name string) string {
	return strings.ToLower(domain) + "\x00" + path + "\x00" + name
}
