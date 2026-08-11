package accountdata

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRememberWebCacheResolvesUsernameWithoutSession(t *testing.T) {
	remover := testRemover(t)
	writeFiles(t, remover.root, "messages-web-123.db")

	if err := remover.RememberWebCache("123", "@Example"); err != nil {
		t.Fatal(err)
	}
	key, err := remover.ResolveWebCache("example")
	if err != nil {
		t.Fatal(err)
	}
	if key != "123" {
		t.Fatalf("ResolveWebCache() = %q, want %q", key, "123")
	}
}

func TestRememberWebCacheSkipsMissingCache(t *testing.T) {
	remover := testRemover(t)
	if err := remover.RememberWebCache("123", "example"); err != nil {
		t.Fatal(err)
	}
	assertMissing(t, remover.root, webCacheIndexName)
}

func TestRemoveWebCacheForgetsEveryAlias(t *testing.T) {
	remover := testRemover(t)
	writeFiles(t, remover.root, "messages-web-123.db")
	if err := remover.RememberWebCache("123", "example"); err != nil {
		t.Fatal(err)
	}

	if err := remover.RemoveWebCache("123"); err != nil {
		t.Fatal(err)
	}
	if _, err := remover.ResolveWebCache("example"); err == nil {
		t.Fatal("ResolveWebCache() found a removed cache alias")
	}
	assertMissing(t, remover.root, webCacheIndexName)
}

func TestRemoveWebCachePreservesOtherAccountAliases(t *testing.T) {
	remover := testRemover(t)
	writeFiles(t, remover.root, "messages-web-123.db", "messages-web-456.db")
	if err := remover.RememberWebCache("123", "first"); err != nil {
		t.Fatal(err)
	}
	if err := remover.RememberWebCache("456", "second"); err != nil {
		t.Fatal(err)
	}

	if err := remover.RemoveWebCache("123"); err != nil {
		t.Fatal(err)
	}
	key, err := remover.ResolveWebCache("second")
	if err != nil {
		t.Fatal(err)
	}
	if key != "456" {
		t.Fatalf("ResolveWebCache() = %q, want %q", key, "456")
	}
	assertMissing(t, remover.root, "messages-web-123.db")
	assertExists(t, filepath.Join(remover.root, "messages-web-456.db"))
}

func TestResolveWebCacheFallsBackToCacheKey(t *testing.T) {
	remover := testRemover(t)
	writeFiles(t, remover.root, "messages-web-123.db-journal")

	key, err := remover.ResolveWebCache("123")
	if err != nil {
		t.Fatal(err)
	}
	if key != "123" {
		t.Fatalf("ResolveWebCache() = %q, want %q", key, "123")
	}
}

func TestRemoveAllWebCachesDeletesIndex(t *testing.T) {
	remover := testRemover(t)
	writeFiles(t, remover.root, "messages-web-123.db")
	if err := remover.RememberWebCache("123", "example"); err != nil {
		t.Fatal(err)
	}

	if err := remover.RemoveAllWebCaches(); err != nil {
		t.Fatal(err)
	}
	assertMissing(t, remover.root, "messages-web-123.db", webCacheIndexName)
}

func TestWebCacheIndexUsesPrivatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports synthesized permission bits")
	}
	remover := testRemover(t)
	writeFiles(t, remover.root, "messages-web-123.db")
	if err := remover.RememberWebCache("123", "example"); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(filepath.Join(remover.root, webCacheIndexName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatalf("web cache index permissions = %o, want no group or other access", info.Mode().Perm())
	}
}
