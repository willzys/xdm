package config

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWebCachePathSeparatesAndSanitizesAccounts(t *testing.T) {
	cacheRoot := configureUserCacheDir(t)

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

func configureUserCacheDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LocalAppData", root)
		return root
	case "darwin":
		t.Setenv("HOME", root)
		return filepath.Join(root, "Library", "Caches")
	default:
		t.Setenv("XDG_CACHE_HOME", root)
		return root
	}
}
