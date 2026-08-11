package accountdata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	officialCache  = "messages.db"
	webCachePrefix = "messages-web-"
)

// Remover deletes local message caches and dedicated browser state created by xdm.
type Remover struct {
	root string
}

func NewRemover() (*Remover, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	return newRemover(cacheDir)
}

func newRemover(cacheDir string) (*Remover, error) {
	root, err := filepath.Abs(filepath.Join(cacheDir, "xdm"))
	if err != nil {
		return nil, fmt.Errorf("resolving xdm cache directory: %w", err)
	}
	cacheDir, err = filepath.Abs(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("resolving user cache directory: %w", err)
	}
	if filepath.Dir(root) != cacheDir || filepath.Base(root) != "xdm" {
		return nil, errors.New("refusing to use an unsafe xdm cache directory")
	}
	return &Remover{root: root}, nil
}

func (r *Remover) RemoveOfficial() error {
	if err := r.validateRoot(); err != nil {
		return err
	}
	return r.removeDatabase(filepath.Join(r.root, officialCache))
}

func (r *Remover) RemoveWeb(account string) error {
	if err := r.validateRoot(); err != nil {
		return err
	}
	account = sanitizeAccount(account)
	if account == "" {
		return errors.New("web account is required for local data removal")
	}
	return errors.Join(r.removeWebCache(account), r.removeBrowserProfiles())
}

func (r *Remover) RemoveWebCache(account string) error {
	if err := r.validateRoot(); err != nil {
		return err
	}
	account = sanitizeAccount(account)
	if account == "" {
		return errors.New("web account is required for local data removal")
	}
	return r.removeWebCache(account)
}

func (r *Remover) removeWebCache(account string) error {
	return r.removeDatabase(filepath.Join(r.root, webCachePrefix+account+".db"))
}

func (r *Remover) RemoveAllWeb() error {
	if err := r.validateRoot(); err != nil {
		return err
	}
	return errors.Join(r.RemoveAllWebCaches(), r.removeBrowserProfiles())
}

func (r *Remover) RemoveAllWebCaches() error {
	if err := r.validateRoot(); err != nil {
		return err
	}
	entries, err := os.ReadDir(r.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading xdm cache directory: %w", err)
	}
	databases := make(map[string]struct{})
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, webCachePrefix) {
			continue
		}
		base := strings.TrimSuffix(strings.TrimSuffix(name, "-wal"), "-shm")
		if strings.HasSuffix(base, ".db") {
			databases[base] = struct{}{}
		}
	}
	var removeErr error
	for name := range databases {
		removeErr = errors.Join(removeErr, r.removeDatabase(filepath.Join(r.root, name)))
	}
	return removeErr
}

func (r *Remover) RemoveAll() error {
	return errors.Join(r.RemoveOfficial(), r.RemoveAllWeb())
}

func (r *Remover) validateRoot() error {
	info, err := os.Lstat(r.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspecting xdm cache directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("refusing to remove data through an unsafe xdm cache path")
	}
	return nil
}

func (r *Remover) removeDatabase(path string) error {
	if filepath.Dir(path) != r.root {
		return errors.New("refusing to remove a database outside the xdm cache directory")
	}
	var removeErr error
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			removeErr = errors.Join(removeErr, fmt.Errorf("removing %s: %w", filepath.Base(path+suffix), err))
		}
	}
	return removeErr
}

func (r *Remover) removeBrowserProfiles() error {
	path := filepath.Join(r.root, "browser-auth")
	if filepath.Dir(path) != r.root || filepath.Base(path) != "browser-auth" {
		return errors.New("refusing to remove an unsafe browser profile directory")
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("removing dedicated browser profiles: %w", err)
	}
	return nil
}

func sanitizeAccount(account string) string {
	account = strings.TrimSpace(account)
	return strings.Map(func(value rune) rune {
		if unicode.IsLetter(value) || unicode.IsDigit(value) || value == '-' || value == '_' {
			return unicode.ToLower(value)
		}
		return '_'
	}, account)
}
