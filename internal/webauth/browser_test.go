package webauth

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestBrowserProfilePathIsPersistentPerBrowser(t *testing.T) {
	cacheDir := t.TempDir()

	got := browserProfilePath(cacheDir, "chrome")
	want := filepath.Join(cacheDir, "xdm", "browser-auth", "chrome")
	if got != want {
		t.Fatalf("browserProfilePath() = %q, want %q", got, want)
	}
	if got == browserProfilePath(cacheDir, "edge") {
		t.Fatal("browserProfilePath() should isolate browser profiles")
	}
}

func TestBootstrapArgumentsDoNotEnableRemoteDebugging(t *testing.T) {
	profile := filepath.Join("testdata", "profile")
	arguments := bootstrapArguments(profile)

	if slices.Contains(arguments, "--remote-debugging-port=0") {
		t.Fatal("bootstrapArguments() enabled remote debugging during login")
	}
	if !slices.Contains(arguments, "--user-data-dir="+profile) {
		t.Fatal("bootstrapArguments() did not use the dedicated profile")
	}
	if arguments[len(arguments)-1] != loginURL {
		t.Fatalf("bootstrapArguments() opens %q, want %q", arguments[len(arguments)-1], loginURL)
	}
}

func TestCaptureArgumentsEnableRemoteDebuggingAfterLogin(t *testing.T) {
	profile := filepath.Join("testdata", "profile")
	arguments := captureArguments(profile)

	if !slices.Contains(arguments, "--remote-debugging-port=0") {
		t.Fatal("captureArguments() did not enable remote debugging")
	}
	if !slices.Contains(arguments, "--user-data-dir="+profile) {
		t.Fatal("captureArguments() did not reuse the dedicated profile")
	}
	if arguments[len(arguments)-1] != homeURL {
		t.Fatalf("captureArguments() opens %q, want %q", arguments[len(arguments)-1], homeURL)
	}
	for _, argument := range arguments {
		if strings.Contains(argument, "/i/flow/login") {
			t.Fatalf("captureArguments() opens the login flow under remote debugging: %q", argument)
		}
	}
}

func TestClearDevToolsActivePortRemovesStaleMarker(t *testing.T) {
	profile := t.TempDir()
	path := filepath.Join(profile, "DevToolsActivePort")
	if err := os.WriteFile(path, []byte("52520\n/devtools/browser/stale\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := clearDevToolsActivePort(profile); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("DevToolsActivePort still exists: %v", err)
	}
	if err := clearDevToolsActivePort(profile); err != nil {
		t.Fatalf("clearing an absent marker: %v", err)
	}
}

func TestSessionUserIDReadsTWIDCookie(t *testing.T) {
	session := Session{Cookies: []Cookie{{
		Name: "twid", Value: "u%3D123456789", Domain: ".x.com", Path: "/",
	}}}

	if got := session.UserID(); got != "123456789" {
		t.Fatalf("UserID() = %q, want %q", got, "123456789")
	}
}

func TestSessionApplySkipsCookieValuesRejectedByNetHTTP(t *testing.T) {
	session := Session{Cookies: []Cookie{
		{Name: "auth_token", Value: "auth", Domain: ".x.com", Path: "/", Secure: true},
		{Name: "ct0", Value: "csrf", Domain: ".x.com", Path: "/", Secure: true},
		{Name: "auxiliary", Value: `"quoted"`, Domain: ".x.com", Path: "/", Secure: true},
	}}
	request, err := http.NewRequest(http.MethodGet, "https://x.com/i/api/1.1/dm/inbox_initial_state.json", nil)
	if err != nil {
		t.Fatal(err)
	}

	session.Apply(request)
	header := request.Header.Get("Cookie")
	if !strings.Contains(header, "auth_token=auth") || !strings.Contains(header, "ct0=csrf") {
		t.Fatalf("required cookies missing from %q", header)
	}
	if strings.Contains(header, "auxiliary") {
		t.Fatalf("invalid auxiliary cookie was applied: %q", header)
	}
}
