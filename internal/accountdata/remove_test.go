package accountdata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveOfficialDeletesSQLiteFilesOnly(t *testing.T) {
	remover := testRemover(t)
	writeFiles(t, remover.root, "messages.db", "messages.db-wal", "messages.db-shm", "keep.txt")

	if err := remover.RemoveOfficial(); err != nil {
		t.Fatal(err)
	}
	assertMissing(t, remover.root, "messages.db", "messages.db-wal", "messages.db-shm")
	assertExists(t, filepath.Join(remover.root, "keep.txt"))
}

func TestRemoveWebDeletesOneAccountAndSharedBrowserProfiles(t *testing.T) {
	remover := testRemover(t)
	writeFiles(t, remover.root,
		"messages-web-account_1.db", "messages-web-account_1.db-wal", "messages-web-other.db", "messages.db")
	profile := filepath.Join(remover.root, "browser-auth", "chrome")
	if err := os.MkdirAll(profile, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "Cookies"), []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := remover.RemoveWeb("Account/1"); err != nil {
		t.Fatal(err)
	}
	assertMissing(t, remover.root, "messages-web-account_1.db", "messages-web-account_1.db-wal", "browser-auth")
	assertExists(t, filepath.Join(remover.root, "messages-web-other.db"))
	assertExists(t, filepath.Join(remover.root, "messages.db"))
}

func TestRemoveAllWebPreservesOfficialAndUnrelatedFiles(t *testing.T) {
	remover := testRemover(t)
	writeFiles(t, remover.root,
		"messages-web-first.db", "messages-web-first.db-shm", "messages-web-orphan.db-wal", "messages-web-second.db", "messages.db", "keep.txt")
	if err := os.MkdirAll(filepath.Join(remover.root, "browser-auth", "edge"), 0700); err != nil {
		t.Fatal(err)
	}

	if err := remover.RemoveAllWeb(); err != nil {
		t.Fatal(err)
	}
	assertMissing(t, remover.root,
		"messages-web-first.db", "messages-web-first.db-shm", "messages-web-orphan.db-wal", "messages-web-second.db", "browser-auth")
	assertExists(t, filepath.Join(remover.root, "messages.db"))
	assertExists(t, filepath.Join(remover.root, "keep.txt"))
}

func TestRemoveWebCachePreservesBrowserProfiles(t *testing.T) {
	remover := testRemover(t)
	writeFiles(t, remover.root, "messages-web-account.db")
	profile := filepath.Join(remover.root, "browser-auth", "chrome")
	if err := os.MkdirAll(profile, 0700); err != nil {
		t.Fatal(err)
	}

	if err := remover.RemoveWebCache("account"); err != nil {
		t.Fatal(err)
	}
	assertMissing(t, remover.root, "messages-web-account.db")
	assertExists(t, profile)
}

func TestRemoveAllIsIdempotent(t *testing.T) {
	remover := testRemover(t)
	writeFiles(t, remover.root, "messages.db", "messages-web-account.db")

	if err := remover.RemoveAll(); err != nil {
		t.Fatal(err)
	}
	if err := remover.RemoveAll(); err != nil {
		t.Fatalf("second RemoveAll() call: %v", err)
	}
	assertMissing(t, remover.root, "messages.db", "messages-web-account.db")
}

func TestRemoveWebRequiresAccount(t *testing.T) {
	remover := testRemover(t)
	profile := filepath.Join(remover.root, "browser-auth", "chrome")
	if err := os.MkdirAll(profile, 0700); err != nil {
		t.Fatal(err)
	}
	if err := remover.RemoveWeb(" "); err == nil {
		t.Fatal("RemoveWeb() accepted an empty account")
	}
	assertExists(t, profile)
}

func TestRemoveOfficialRejectsNonDirectoryCacheRoot(t *testing.T) {
	cacheDir := t.TempDir()
	root := filepath.Join(cacheDir, "xdm")
	if err := os.WriteFile(root, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	remover, err := newRemover(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := remover.RemoveOfficial(); err == nil {
		t.Fatal("RemoveOfficial() accepted a non-directory cache root")
	}
}

func testRemover(t *testing.T) *Remover {
	t.Helper()
	remover, err := newRemover(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(remover.root, 0700); err != nil {
		t.Fatal(err)
	}
	return remover
}

func writeFiles(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(root, name), []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func assertMissing(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("%s still exists: %v", name, err)
		}
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}
