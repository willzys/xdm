package webauth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zalando/go-keyring"
)

const (
	vaultVersion   = 1
	keyringService = "xdm"
	keyringUser    = "web-session-key"
)

type Vault struct {
	Version  int                `json:"version"`
	Active   string             `json:"active,omitempty"`
	Sessions map[string]Session `json:"sessions"`
}

type Store struct {
	path string
	mu   sync.Mutex
}

func NewStore() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &Store{path: filepath.Join(dir, "xdm", "web-sessions.enc")}, nil
}

func (s *Store) Save(session Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session.Cookies = normalizeCookies(session.Cookies)
	validationTime := time.Now()
	if err := session.Validate(validationTime); err != nil {
		return err
	}
	vault, err := s.loadVault()
	if err != nil && !errors.Is(err, ErrNotAuthenticated) {
		return err
	}
	if vault.Sessions == nil {
		vault = Vault{Version: vaultVersion, Sessions: make(map[string]Session)}
	}
	key := session.Key()
	vault.Sessions[key] = session
	vault.Active = key
	return s.saveVault(vault)
}

func (s *Store) LoadActive() (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault, err := s.loadVault()
	if err != nil {
		return Session{}, err
	}
	session, ok := vault.Sessions[vault.Active]
	if !ok {
		return Session{}, ErrNotAuthenticated
	}
	return session.clone(), nil
}

func (s *Store) List() ([]Session, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault, err := s.loadVault()
	if err != nil {
		return nil, "", err
	}
	keys := make([]string, 0, len(vault.Sessions))
	for key := range vault.Sessions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]Session, 0, len(keys))
	for _, key := range keys {
		result = append(result, vault.Sessions[key].clone())
	}
	return result, vault.Active, nil
}

func (s *Store) SetActive(account string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault, err := s.loadVault()
	if err != nil {
		return err
	}
	account = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(account), "@"))
	for key, session := range vault.Sessions {
		if strings.EqualFold(key, account) || strings.EqualFold(strings.TrimPrefix(session.Account.Username, "@"), account) {
			vault.Active = key
			return s.saveVault(vault)
		}
	}
	return fmt.Errorf("web account %q is not saved", account)
}

func (s *Store) Delete(account string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault, err := s.loadVault()
	if errors.Is(err, ErrNotAuthenticated) {
		if strings.TrimSpace(account) == "" {
			return s.deleteAll()
		}
		return nil
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(account) == "" {
		return s.deleteAll()
	}
	account = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(account), "@"))
	matched := ""
	for key, session := range vault.Sessions {
		if strings.EqualFold(key, account) || strings.EqualFold(strings.TrimPrefix(session.Account.Username, "@"), account) {
			matched = key
			break
		}
	}
	if matched == "" {
		return fmt.Errorf("web account %q is not saved", account)
	}
	delete(vault.Sessions, matched)
	if len(vault.Sessions) == 0 {
		return s.deleteAll()
	}
	if vault.Active == matched {
		keys := make([]string, 0, len(vault.Sessions))
		for key := range vault.Sessions {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		vault.Active = keys[0]
	}
	return s.saveVault(vault)
}

func (s *Store) loadVault() (Vault, error) {
	encrypted, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		encrypted, err = os.ReadFile(s.path + ".bak")
		if errors.Is(err, os.ErrNotExist) {
			return Vault{}, ErrNotAuthenticated
		}
	}
	if err != nil {
		return Vault{}, fmt.Errorf("reading encrypted web sessions: %w", err)
	}
	key, err := loadKey(false)
	if err != nil {
		return Vault{}, err
	}
	plain, err := decrypt(key, encrypted)
	if err != nil {
		return Vault{}, fmt.Errorf("decrypting web sessions: %w", err)
	}
	var vault Vault
	if err := json.Unmarshal(plain, &vault); err != nil {
		return Vault{}, fmt.Errorf("decoding web sessions: %w", err)
	}
	if vault.Version != vaultVersion || vault.Sessions == nil {
		return Vault{}, errors.New("unsupported web session vault format")
	}
	return vault, nil
}

func (s *Store) saveVault(vault Vault) error {
	vault.Version = vaultVersion
	plain, err := json.Marshal(vault)
	if err != nil {
		return err
	}
	key, err := loadKey(true)
	if err != nil {
		return err
	}
	encrypted, err := encrypt(key, plain)
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "web-sessions-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(encrypted); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := replaceFile(tmpPath, s.path); err != nil {
		return fmt.Errorf("saving encrypted web sessions: %w", err)
	}
	return nil
}

func replaceFile(source, target string) error {
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

func (s *Store) deleteAll() error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(s.path + ".bak"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := keyring.Delete(keyringService, keyringUser); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("deleting web session key: %w", err)
	}
	return nil
}

func loadKey(create bool) ([]byte, error) {
	value, err := keyring.Get(keyringService, keyringUser)
	if err == nil {
		key, decodeErr := base64.RawStdEncoding.DecodeString(value)
		if decodeErr != nil || len(key) != 32 {
			return nil, errors.New("invalid web session key in OS keyring")
		}
		return key, nil
	}
	if !errors.Is(err, keyring.ErrNotFound) {
		return nil, fmt.Errorf("reading web session key from OS keyring: %w", err)
	}
	if !create {
		return nil, ErrNotAuthenticated
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	if err := keyring.Set(keyringService, keyringUser, base64.RawStdEncoding.EncodeToString(key)); err != nil {
		return nil, fmt.Errorf("saving web session key to OS keyring: %w", err)
	}
	return key, nil
}

func encrypt(key, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return append(nonce, aead.Seal(nil, nonce, plain, []byte("xdm-web-sessions-v1"))...), nil
}

func decrypt(key, encrypted []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(encrypted) < aead.NonceSize() {
		return nil, errors.New("encrypted web session is truncated")
	}
	nonce := encrypted[:aead.NonceSize()]
	return aead.Open(nil, nonce, encrypted[aead.NonceSize():], []byte("xdm-web-sessions-v1"))
}
