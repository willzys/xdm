package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWebCachePathSeparatesAndSanitizesAccounts(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("LocalAppData", cacheRoot)

	first, err := WebCachePath("Example/User")
	if err != nil {
		t.Fatal(err)
	}
	second, err := WebCachePath("another")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("WebCachePath() did not separate accounts")
	}
	if filepath.Dir(first) != filepath.Join(cacheRoot, "xdm") {
		t.Fatalf("cache directory = %q", filepath.Dir(first))
	}
	if strings.Contains(filepath.Base(first), "/") || !strings.Contains(filepath.Base(first), "example_user") {
		t.Fatalf("cache filename was not sanitized: %q", filepath.Base(first))
	}
}

func TestWebCachePathRequiresAccount(t *testing.T) {
	if _, err := WebCachePath(" "); err == nil {
		t.Fatal("WebCachePath() accepted an empty account")
	}
}
