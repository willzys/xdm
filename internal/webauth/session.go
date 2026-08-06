package webauth

import (
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrNotAuthenticated = errors.New("web session not found; run 'xdm auth web'")

type Account struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Username string `json:"username,omitempty"`
}

type Cookie struct {
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	Domain   string    `json:"domain"`
	Path     string    `json:"path"`
	Expires  time.Time `json:"expires,omitempty"`
	Secure   bool      `json:"secure,omitempty"`
	HTTPOnly bool      `json:"http_only,omitempty"`
	HostOnly bool      `json:"host_only,omitempty"`
}

type Session struct {
	Account     Account   `json:"account"`
	Browser     string    `json:"browser"`
	UserAgent   string    `json:"user_agent,omitempty"`
	Cookies     []Cookie  `json:"cookies"`
	CreatedAt   time.Time `json:"created_at"`
	ValidatedAt time.Time `json:"validated_at"`
}

func (s Session) clone() Session {
	s.Cookies = append([]Cookie(nil), s.Cookies...)
	return s
}

func (s Session) Key() string {
	if s.Account.ID != "" {
		return s.Account.ID
	}
	if s.Account.Username != "" {
		return strings.ToLower(strings.TrimPrefix(s.Account.Username, "@"))
	}
	return "default"
}

func (s Session) DisplayName() string {
	if s.Account.Username != "" {
		return "@" + strings.TrimPrefix(s.Account.Username, "@")
	}
	if s.Account.Name != "" {
		return s.Account.Name
	}
	return s.Key()
}

func (s Session) UserID() string {
	if s.Account.ID != "" {
		return s.Account.ID
	}
	for _, cookie := range s.Cookies {
		if cookie.Name != "twid" || cookie.Value == "" || !allowedCookieDomain(cookie.Domain) {
			continue
		}
		value, err := url.QueryUnescape(cookie.Value)
		if err != nil {
			continue
		}
		value = strings.TrimPrefix(value, "u=")
		if _, err := strconv.ParseUint(value, 10, 64); err == nil {
			return value
		}
	}
	return ""
}

func (s Session) Validate(now time.Time) error {
	if len(s.Cookies) == 0 {
		return ErrNotAuthenticated
	}
	hasAuthToken := false
	hasCSRFToken := false
	for _, cookie := range s.Cookies {
		if cookie.Value == "" || !allowedCookieDomain(cookie.Domain) {
			continue
		}
		if !cookie.Expires.IsZero() && !cookie.Expires.After(now) {
			continue
		}
		switch cookie.Name {
		case "auth_token":
			hasAuthToken = true
		case "ct0":
			hasCSRFToken = true
		}
	}
	if !hasAuthToken || !hasCSRFToken {
		return errors.New("web session is expired or incomplete; run 'xdm auth web' again")
	}
	return nil
}

func (s Session) CSRFToken() string {
	for _, cookie := range s.Cookies {
		if cookie.Name == "ct0" && cookie.Value != "" && allowedCookieDomain(cookie.Domain) && (cookie.Expires.IsZero() || cookie.Expires.After(time.Now())) {
			return cookie.Value
		}
	}
	return ""
}

func (s Session) Apply(request *http.Request) {
	if request == nil || request.URL == nil || !allowedCookieDomain(request.URL.Hostname()) {
		return
	}
	for _, cookie := range s.Cookies {
		if cookie.Value == "" || (!cookie.Expires.IsZero() && !cookie.Expires.After(time.Now())) || !cookieApplies(cookie, request.URL) {
			continue
		}
		request.AddCookie(&http.Cookie{
			Name: cookie.Name, Value: cookie.Value, Path: cookie.Path,
			Domain: cookie.Domain, Expires: cookie.Expires, Secure: cookie.Secure, HttpOnly: cookie.HTTPOnly,
		})
	}
	if token := s.CSRFToken(); token != "" {
		request.Header.Set("X-CSRF-Token", token)
	}
	if s.UserAgent != "" {
		request.Header.Set("User-Agent", s.UserAgent)
	}
}

func normalizeCookies(cookies []Cookie) []Cookie {
	byKey := make(map[string]Cookie)
	for _, cookie := range cookies {
		cookie.Domain = strings.ToLower(strings.TrimSpace(cookie.Domain))
		if cookie.Name == "" || cookie.Value == "" || !allowedCookieDomain(cookie.Domain) {
			continue
		}
		if cookie.Path == "" {
			cookie.Path = "/"
		}
		byKey[cookie.Domain+"\x00"+cookie.Path+"\x00"+cookie.Name] = cookie
	}
	result := make([]Cookie, 0, len(byKey))
	for _, cookie := range byKey {
		result = append(result, cookie)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Domain != result[j].Domain {
			return result[i].Domain < result[j].Domain
		}
		if result[i].Path != result[j].Path {
			return result[i].Path < result[j].Path
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func allowedCookieDomain(domain string) bool {
	domain = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), ".")
	return domain == "x.com" || strings.HasSuffix(domain, ".x.com") || domain == "twitter.com" || strings.HasSuffix(domain, ".twitter.com")
}

func cookieApplies(cookie Cookie, target *url.URL) bool {
	if target == nil || !allowedCookieDomain(target.Hostname()) {
		return false
	}
	if cookie.Secure && !strings.EqualFold(target.Scheme, "https") {
		return false
	}
	domain := strings.TrimPrefix(cookie.Domain, ".")
	host := strings.ToLower(target.Hostname())
	if cookie.HostOnly && host != domain {
		return false
	}
	if !cookie.HostOnly && host != domain && !strings.HasSuffix(host, "."+domain) {
		return false
	}
	path := cookie.Path
	if path == "" {
		path = "/"
	}
	targetPath := target.EscapedPath()
	if targetPath == "" {
		targetPath = "/"
	}
	if targetPath == path {
		return true
	}
	if !strings.HasPrefix(targetPath, path) {
		return false
	}
	return strings.HasSuffix(path, "/") || (len(targetPath) > len(path) && targetPath[len(path)] == '/')
}

func domainMatchesHost(domain, host string) bool {
	domain = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), ".")
	host = strings.ToLower(strings.TrimSpace(host))
	return domain != "" && (host == domain || strings.HasSuffix(host, "."+domain))
}
