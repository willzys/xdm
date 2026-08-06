package webauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	loginURL = "https://x.com/i/flow/login"
	homeURL  = "https://x.com/home"
)

type LoginOptions struct {
	Browser string
	Timeout time.Duration
	Output  io.Writer
}

type browserSpec struct {
	name string
	path string
}

type devtoolsTarget struct {
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

type browserCookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	Secure   bool    `json:"secure"`
	HTTPOnly bool    `json:"httpOnly"`
}

func Login(ctx context.Context, options LoginOptions) (Session, error) {
	if options.Timeout <= 0 {
		options.Timeout = 5 * time.Minute
	}
	if options.Output == nil {
		options.Output = io.Discard
	}
	browser, err := findBrowser(options.Browser)
	if err != nil {
		return Session{}, err
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return Session{}, err
	}
	profile := browserProfilePath(cacheDir, browser.name)
	if err := os.MkdirAll(profile, 0700); err != nil {
		return Session{}, err
	}

	loginCtx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()
	if err := bootstrapLogin(loginCtx, browser, profile, options.Output); err != nil {
		return Session{}, err
	}
	if err := loginCtx.Err(); err != nil {
		return Session{}, err
	}
	if err := clearDevToolsActivePort(profile); err != nil {
		return Session{}, err
	}

	command := exec.CommandContext(loginCtx, browser.path, captureArguments(profile)...)
	if err := command.Start(); err != nil {
		return Session{}, fmt.Errorf("starting %s: %w", browser.name, err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()

	fmt.Fprintf(options.Output, "Reopened the dedicated %s profile to capture the authenticated X session.\n", browser.name)
	port, browserPath, err := waitForDevTools(loginCtx, profile, done)
	if err != nil {
		terminateBrowser(command, done)
		return Session{}, err
	}
	browserSocket := "ws://127.0.0.1:" + strconv.Itoa(port) + browserPath
	client, err := dialCDP(loginCtx, browserSocket)
	if err != nil {
		terminateBrowser(command, done)
		return Session{}, fmt.Errorf("connecting to %s: %w", browser.name, err)
	}
	defer client.Close()
	defer closeBrowser(command, done, client)

	var version struct {
		UserAgent string `json:"userAgent"`
	}
	_ = client.Call(loginCtx, "Browser.getVersion", nil, &version)
	fmt.Fprintln(options.Output, "Waiting for the authenticated X session...")
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		cookies, cookieErr := readCookies(loginCtx, client)
		if cookieErr != nil {
			lastErr = fmt.Errorf("reading browser cookies: %w", cookieErr)
		} else if hasRequiredCookies(cookies) {
			account, accountErr := readAccount(loginCtx, port)
			if accountErr == nil && account.Username != "" {
				account.ID = (Session{Cookies: cookies}).UserID()
				now := time.Now().UTC()
				session := Session{
					Account: account, Browser: browser.name, UserAgent: version.UserAgent,
					Cookies: cookies, CreatedAt: now, ValidatedAt: now,
				}
				if err := session.Validate(now); err != nil {
					return Session{}, err
				}
				return session, nil
			}
			lastErr = accountErr
		}
		select {
		case <-loginCtx.Done():
			if errors.Is(loginCtx.Err(), context.DeadlineExceeded) {
				if lastErr != nil {
					return Session{}, fmt.Errorf("timed out waiting for X login: %w", lastErr)
				}
				return Session{}, errors.New("timed out waiting for X login")
			}
			return Session{}, loginCtx.Err()
		case err := <-done:
			if err == nil {
				return Session{}, errors.New("browser closed before X login completed")
			}
			return Session{}, fmt.Errorf("browser exited before X login completed: %w", err)
		case <-ticker.C:
		}
	}
}

func browserProfilePath(cacheDir, browser string) string {
	return filepath.Join(cacheDir, "xdm", "browser-auth", browser)
}

func commonBrowserArguments(profile string) []string {
	return []string{
		"--user-data-dir=" + profile,
		"--no-first-run",
		"--no-default-browser-check",
		"--window-size=1280,900",
	}
}

func bootstrapArguments(profile string) []string {
	return append(commonBrowserArguments(profile), loginURL)
}

func captureArguments(profile string) []string {
	arguments := commonBrowserArguments(profile)
	arguments = append(arguments, "--remote-debugging-port=0")
	return append(arguments, homeURL)
}

func bootstrapLogin(ctx context.Context, browser browserSpec, profile string, output io.Writer) error {
	command := exec.CommandContext(ctx, browser.path, bootstrapArguments(profile)...)
	if err := command.Start(); err != nil {
		return fmt.Errorf("starting %s login window: %w", browser.name, err)
	}
	fmt.Fprintf(output, "Opened a dedicated %s profile without remote debugging.\n", browser.name)
	fmt.Fprintln(output, "Sign in to X, confirm the home timeline loads, then close that browser window to continue.")
	if err := command.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("waiting for %s login window: %w", browser.name, err)
	}
	return nil
}

func findBrowser(requested string) (browserSpec, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested == "" {
		requested = "auto"
	}
	candidates := browserCandidates()
	for _, candidate := range candidates {
		if requested != "auto" && requested != candidate.name {
			continue
		}
		if candidate.path == "" {
			continue
		}
		if strings.ContainsRune(candidate.path, filepath.Separator) || filepath.IsAbs(candidate.path) {
			if info, err := os.Stat(candidate.path); err == nil && !info.IsDir() {
				return candidate, nil
			}
			continue
		}
		if path, err := exec.LookPath(candidate.path); err == nil {
			candidate.path = path
			return candidate, nil
		}
	}
	if requested == "firefox" {
		return browserSpec{}, errors.New("Firefox browser login is not supported yet; use chrome, edge, or chromium")
	}
	if requested != "auto" && requested != "chrome" && requested != "edge" && requested != "chromium" {
		return browserSpec{}, fmt.Errorf("unsupported browser %q; use auto, chrome, edge, or chromium", requested)
	}
	return browserSpec{}, fmt.Errorf("could not find %s; install a Chromium-based browser or choose one with --browser", requested)
}

func browserCandidates() []browserSpec {
	switch runtime.GOOS {
	case "windows":
		local := os.Getenv("LOCALAPPDATA")
		programFiles := os.Getenv("ProgramFiles")
		programFilesX86 := os.Getenv("ProgramFiles(x86)")
		return []browserSpec{
			{name: "chrome", path: filepath.Join(programFiles, "Google", "Chrome", "Application", "chrome.exe")},
			{name: "chrome", path: filepath.Join(programFilesX86, "Google", "Chrome", "Application", "chrome.exe")},
			{name: "chrome", path: filepath.Join(local, "Google", "Chrome", "Application", "chrome.exe")},
			{name: "edge", path: filepath.Join(programFiles, "Microsoft", "Edge", "Application", "msedge.exe")},
			{name: "edge", path: filepath.Join(programFilesX86, "Microsoft", "Edge", "Application", "msedge.exe")},
			{name: "chromium", path: "chromium.exe"},
		}
	case "darwin":
		return []browserSpec{
			{name: "chrome", path: "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"},
			{name: "edge", path: "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"},
			{name: "chromium", path: "/Applications/Chromium.app/Contents/MacOS/Chromium"},
		}
	default:
		return []browserSpec{
			{name: "chrome", path: "google-chrome"},
			{name: "chrome", path: "google-chrome-stable"},
			{name: "edge", path: "microsoft-edge"},
			{name: "edge", path: "microsoft-edge-stable"},
			{name: "chromium", path: "chromium"},
			{name: "chromium", path: "chromium-browser"},
		}
	}
}

func waitForDevTools(ctx context.Context, profile string, done <-chan error) (int, string, error) {
	path := filepath.Join(profile, "DevToolsActivePort")
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) >= 2 {
				port, parseErr := strconv.Atoi(strings.TrimSpace(lines[0]))
				browserPath := strings.TrimSpace(lines[1])
				if parseErr == nil && port > 0 && strings.HasPrefix(browserPath, "/devtools/browser/") {
					connection, dialErr := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 100*time.Millisecond)
					if dialErr == nil {
						_ = connection.Close()
						return port, browserPath, nil
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return 0, "", ctx.Err()
		case err := <-done:
			if err == nil {
				return 0, "", errors.New("browser closed before debugging became available")
			}
			return 0, "", fmt.Errorf("browser failed to start: %w", err)
		case <-ticker.C:
		}
	}
}

func clearDevToolsActivePort(profile string) error {
	path := filepath.Join(profile, "DevToolsActivePort")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing stale browser debugging state: %w", err)
	}
	return nil
}

func readCookies(ctx context.Context, client *cdpClient) ([]Cookie, error) {
	var result struct {
		Cookies []browserCookie `json:"cookies"`
	}
	if err := client.Call(ctx, "Storage.getCookies", map[string]any{}, &result); err != nil {
		return nil, err
	}
	cookies := make([]Cookie, 0, len(result.Cookies))
	for _, cookie := range result.Cookies {
		if !allowedCookieDomain(cookie.Domain) {
			continue
		}
		var expires time.Time
		if cookie.Expires > 0 {
			seconds, fraction := mathModf(cookie.Expires)
			expires = time.Unix(seconds, int64(fraction*float64(time.Second))).UTC()
		}
		cookies = append(cookies, Cookie{
			Name: cookie.Name, Value: cookie.Value, Domain: cookie.Domain, Path: cookie.Path,
			Expires: expires, Secure: cookie.Secure, HTTPOnly: cookie.HTTPOnly,
			HostOnly: !strings.HasPrefix(cookie.Domain, "."),
		})
	}
	return normalizeCookies(cookies), nil
}

func hasRequiredCookies(cookies []Cookie) bool {
	hasAuthToken := false
	hasCSRFToken := false
	for _, cookie := range cookies {
		switch cookie.Name {
		case "auth_token":
			hasAuthToken = cookie.Value != ""
		case "ct0":
			hasCSRFToken = cookie.Value != ""
		}
	}
	return hasAuthToken && hasCSRFToken
}

func readAccount(ctx context.Context, port int) (Account, error) {
	targets, err := listTargets(ctx, port)
	if err != nil {
		return Account{}, err
	}
	for _, target := range targets {
		if target.Type != "page" || target.WebSocketDebuggerURL == "" {
			continue
		}
		parsed, err := url.Parse(target.URL)
		if err != nil || !allowedCookieDomain(parsed.Hostname()) {
			continue
		}
		client, err := dialCDP(ctx, target.WebSocketDebuggerURL)
		if err != nil {
			continue
		}
		account, evaluateErr := evaluateAccount(ctx, client)
		client.Close()
		if evaluateErr == nil && account.Username != "" {
			return account, nil
		}
	}
	return Account{}, errors.New("authenticated X account is not ready")
}

func listTargets(ctx context.Context, port int) ([]devtoolsTarget, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(port)+"/json/list", nil)
	if err != nil {
		return nil, err
	}
	response, err := (&http.Client{Timeout: 2 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("browser target list returned %s", response.Status)
	}
	var targets []devtoolsTarget
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&targets); err != nil {
		return nil, err
	}
	return targets, nil
}

func evaluateAccount(ctx context.Context, client *cdpClient) (Account, error) {
	const expression = `(() => {
  const profile = document.querySelector('a[data-testid="AppTabBar_Profile_Link"]');
  const account = document.querySelector('[data-testid="SideNav_AccountSwitcher_Button"]');
  const values = (account?.innerText || '').split('\n').map(value => value.trim()).filter(Boolean);
  const handle = values.find(value => value.startsWith('@')) || '';
  const path = profile?.getAttribute('href') || '';
  const username = (handle || path.split('/').filter(Boolean)[0] || '').replace(/^@/, '');
  const name = values.find(value => !value.startsWith('@')) || '';
  return {username, name};
})()`
	var result struct {
		Result struct {
			Value struct {
				Name     string `json:"name"`
				Username string `json:"username"`
			} `json:"value"`
		} `json:"result"`
	}
	if err := client.Call(ctx, "Runtime.evaluate", map[string]any{
		"expression": expression, "awaitPromise": true, "returnByValue": true,
	}, &result); err != nil {
		return Account{}, err
	}
	if result.Result.Value.Username == "" {
		return Account{}, errors.New("X session validation returned no username")
	}
	return Account{Name: result.Result.Value.Name, Username: result.Result.Value.Username}, nil
}

func closeBrowser(command *exec.Cmd, done <-chan error, client *cdpClient) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = client.Call(ctx, "Browser.close", nil, nil)
	select {
	case <-done:
	case <-ctx.Done():
		if command.Process != nil {
			_ = command.Process.Kill()
		}
	}
}

func terminateBrowser(command *exec.Cmd, done <-chan error) {
	if command.Process != nil {
		if err := command.Process.Kill(); errors.Is(err, os.ErrProcessDone) {
			return
		}
	}
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
}

func mathModf(value float64) (int64, float64) {
	seconds := int64(value)
	return seconds, value - float64(seconds)
}
