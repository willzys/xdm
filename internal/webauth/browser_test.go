package webauth

import (
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
