package accountdata

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const webCacheIndexName = "web-cache-index.json"

type webCacheIndex struct {
	Accounts map[string]string `json:"accounts"`
}

// RememberWebCache records non-secret account aliases for an existing cache so
// it can still be selected after its authentication session is removed.
func (r *Remover) RememberWebCache(cacheKey string, aliases ...string) error {
	if err := r.validateRoot(); err != nil {
		return err
	}
	cacheKey = sanitizeAccount(cacheKey)
	if cacheKey == "" {
		return errors.New("web cache key is required")
	}
	exists, err := r.databaseExists(filepath.Join(r.root, webCachePrefix+cacheKey+".db"))
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	index, err := r.loadWebCacheIndex()
	if errors.Is(err, os.ErrNotExist) {
		index = webCacheIndex{}
	} else if err != nil {
		return err
	}
	if index.Accounts == nil {
		index.Accounts = make(map[string]string)
	}
	index.Accounts[normalizeAlias(cacheKey)] = cacheKey
	for _, alias := range aliases {
		if alias = normalizeAlias(alias); alias != "" {
			index.Accounts[alias] = cacheKey
		}
	}
	return r.saveWebCacheIndex(index)
}

// ResolveWebCache returns the cache key associated with an account key or
// username, including accounts whose authentication has already been removed.
func (r *Remover) ResolveWebCache(account string) (string, error) {
	if err := r.validateRoot(); err != nil {
		return "", err
	}
	account = normalizeAlias(account)
	if account == "" {
		return "", errors.New("web account is required")
	}
	index, err := r.loadWebCacheIndex()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if key := sanitizeAccount(index.Accounts[account]); key != "" {
		return key, nil
	}
	key := sanitizeAccount(account)
	exists, err := r.databaseExists(filepath.Join(r.root, webCachePrefix+key+".db"))
	if err != nil {
		return "", err
	}
	if exists {
		return key, nil
	}
	return "", fmt.Errorf("web cache for account %q was not found", account)
}

func (r *Remover) forgetWebCache(cacheKey string) error {
	index, err := r.loadWebCacheIndex()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	cacheKey = sanitizeAccount(cacheKey)
	for alias, key := range index.Accounts {
		if sanitizeAccount(key) == cacheKey {
			delete(index.Accounts, alias)
		}
	}
	if len(index.Accounts) == 0 {
		return r.removeWebCacheIndex()
	}
	return r.saveWebCacheIndex(index)
}

func (r *Remover) loadWebCacheIndex() (webCacheIndex, error) {
	data, err := os.ReadFile(filepath.Join(r.root, webCacheIndexName))
	if err != nil {
		return webCacheIndex{}, err
	}
	var index webCacheIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return webCacheIndex{}, fmt.Errorf("decoding web cache index: %w", err)
	}
	return index, nil
}

func (r *Remover) saveWebCacheIndex(index webCacheIndex) error {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(r.root, 0700); err != nil {
		return fmt.Errorf("creating xdm cache directory: %w", err)
	}
	tmp, err := os.CreateTemp(r.root, "web-cache-index-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := replaceIndexFile(tmpPath, filepath.Join(r.root, webCacheIndexName)); err != nil {
		return fmt.Errorf("saving web cache index: %w", err)
	}
	return nil
}

func (r *Remover) removeWebCacheIndex() error {
	var removeErr error
	for _, suffix := range []string{"", ".bak"} {
		if err := os.Remove(filepath.Join(r.root, webCacheIndexName) + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			removeErr = errors.Join(removeErr, fmt.Errorf("removing web cache index%s: %w", suffix, err))
		}
	}
	return removeErr
}

func (r *Remover) databaseExists(path string) (bool, error) {
	for _, suffix := range databaseSuffixes {
		_, err := os.Lstat(path + suffix)
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("inspecting %s: %w", filepath.Base(path+suffix), err)
		}
	}
	return false, nil
}

func replaceIndexFile(source, target string) error {
	if err := os.Rename(source, target); err == nil {
		return nil
	} else if runtime.GOOS != "windows" {
		return err
	}
	backup := target + ".bak"
	_ = os.Remove(backup)
	if err := os.Rename(target, backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(source, target); err != nil {
		_ = os.Rename(backup, target)
		return err
	}
	_ = os.Remove(backup)
	return nil
}

func normalizeAlias(value string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(value), "@"))
}
