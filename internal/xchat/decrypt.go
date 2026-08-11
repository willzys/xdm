package xchat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/willzys/xdm/internal/webapi"
)

type Diagnostics struct {
	Messages int `json:"messages"`
	Events   int `json:"events"`
	Errors   int `json:"errors"`
}

type decryptRequest struct {
	webapi.XChatUnlockMaterial
	PIN []byte `json:"pin"`
}

type decryptResponse struct {
	Diagnostics
	Error string `json:"error"`
}

func Diagnose(ctx context.Context, material webapi.XChatUnlockMaterial, pin []byte) (Diagnostics, error) {
	if len(pin) == 0 {
		return Diagnostics{}, errors.New("XChat PIN is required")
	}
	runtime, err := resolveCryptoRuntime()
	if err != nil {
		return Diagnostics{}, err
	}
	command, err := runtime.commandContext(ctx, "decrypt.mjs")
	if err != nil {
		return Diagnostics{}, err
	}
	payload, err := json.Marshal(decryptRequest{XChatUnlockMaterial: material, PIN: pin})
	if err != nil {
		return Diagnostics{}, err
	}
	defer clear(payload)

	command.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	var response decryptResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		if runErr != nil {
			return Diagnostics{}, fmt.Errorf("running XChat crypto runtime: %w%s", runErr, limitedStderr(stderr.String()))
		}
		return Diagnostics{}, errors.New("XChat crypto runtime returned an invalid response")
	}
	if response.Error != "" {
		return Diagnostics{}, errors.New(response.Error)
	}
	if runErr != nil {
		return Diagnostics{}, fmt.Errorf("running XChat crypto runtime: %w", runErr)
	}
	return response.Diagnostics, nil
}

func runtimeDirectory() (string, error) {
	if override := strings.TrimSpace(os.Getenv("XDM_XCHAT_RUNTIME")); override != "" {
		return filepath.Abs(override)
	}
	if workingDirectory, err := os.Getwd(); err == nil {
		candidate := filepath.Join(workingDirectory, "internal", "xchat", "runtime")
		if _, err := os.Stat(filepath.Join(candidate, "decrypt.mjs")); err == nil {
			return candidate, nil
		}
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("locating XChat crypto runtime")
	}
	return filepath.Join(filepath.Dir(source), "runtime"), nil
}

func limitedStderr(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) > 500 {
		value = value[:500]
	}
	return ": " + value
}
