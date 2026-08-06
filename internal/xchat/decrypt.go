package xchat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	runtimeDir, err := runtimeDirectory()
	if err != nil {
		return Diagnostics{}, err
	}
	if _, err := os.Stat(filepath.Join(runtimeDir, "node_modules", "@xdevplatform", "chat-xdk", "package.json")); err != nil {
		return Diagnostics{}, fmt.Errorf("XChat crypto runtime is not installed; run 'npm.cmd install --prefix \"%s\"'", runtimeDir)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		return Diagnostics{}, errors.New("Node.js 18 or newer is required for XChat decryption on Windows")
	}
	payload, err := json.Marshal(decryptRequest{XChatUnlockMaterial: material, PIN: pin})
	if err != nil {
		return Diagnostics{}, err
	}
	defer clear(payload)

	command := exec.CommandContext(ctx, node, filepath.Join(runtimeDir, "decrypt.mjs"))
	command.Dir = runtimeDir
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
