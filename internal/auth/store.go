package auth

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "xdm"
	keyringUser    = "oauth2-token"
)

var ErrNotAuthenticated = errors.New("not authenticated; run 'xdm auth'")

type Store interface {
	Load() (Token, error)
	Save(Token) error
	Delete() error
}

type KeyringStore struct{}

func (KeyringStore) Load() (Token, error) {
	value, err := keyring.Get(keyringService, keyringUser)
	if errors.Is(err, keyring.ErrNotFound) {
		return Token{}, ErrNotAuthenticated
	}
	if err != nil {
		return Token{}, fmt.Errorf("reading OAuth token from OS keyring: %w", err)
	}
	var token Token
	if err := json.Unmarshal([]byte(value), &token); err != nil {
		return Token{}, fmt.Errorf("decoding OAuth token: %w", err)
	}
	return token, nil
}

func (KeyringStore) Save(token Token) error {
	value, err := json.Marshal(token)
	if err != nil {
		return err
	}
	if err := keyring.Set(keyringService, keyringUser, string(value)); err != nil {
		return fmt.Errorf("saving OAuth token to OS keyring: %w", err)
	}
	return nil
}

func (KeyringStore) Delete() error {
	err := keyring.Delete(keyringService, keyringUser)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
