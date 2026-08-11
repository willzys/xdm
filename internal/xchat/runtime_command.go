package xchat

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	helperEnvironment  = "XDM_XCHAT_HELPER"
	runtimeEnvironment = "XDM_XCHAT_RUNTIME"
)

type cryptoRuntime struct {
	executable string
	directory  string
}

func (r cryptoRuntime) command(script string) (*exec.Cmd, error) {
	return r.commandContext(context.Background(), script)
}

func (r cryptoRuntime) commandContext(ctx context.Context, script string) (*exec.Cmd, error) {
	scriptPath := filepath.Join(r.directory, script)
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("XChat crypto runtime is incomplete: %s is missing", scriptPath)
	}
	command := exec.CommandContext(ctx, r.executable, scriptPath)
	command.Dir = r.directory
	command.Env = cryptoEnvironment(os.Environ())
	return command, nil
}

func cryptoEnvironment(environment []string) []string {
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		name, _, found := strings.Cut(entry, "=")
		if found && (strings.EqualFold(name, "NODE_OPTIONS") || strings.EqualFold(name, "NODE_PATH")) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func resolveCryptoRuntime() (cryptoRuntime, error) {
	if helper := strings.TrimSpace(os.Getenv(helperEnvironment)); helper != "" {
		helperPath, err := filepath.Abs(helper)
		if err != nil {
			return cryptoRuntime{}, err
		}
		runtimeDir, err := configuredRuntimeDirectory(filepath.Dir(helperPath))
		if err != nil {
			return cryptoRuntime{}, err
		}
		return validateCryptoRuntime(helperPath, runtimeDir)
	}

	if executable, err := os.Executable(); err == nil {
		if bundled, found, bundleErr := bundledCryptoRuntime(executable); found || bundleErr != nil {
			return bundled, bundleErr
		}
	}

	runtimeDir, err := runtimeDirectory()
	if err != nil {
		return cryptoRuntime{}, err
	}
	if err := validateSDK(runtimeDir); err != nil {
		return cryptoRuntime{}, fmt.Errorf("XChat crypto helper was not found; development runtime unavailable: %w", err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		return cryptoRuntime{}, errors.New("XChat crypto helper was not found; developers can install Node.js 18 or newer and run npm ci in internal/xchat/runtime")
	}
	return cryptoRuntime{executable: node, directory: runtimeDir}, nil
}

func bundledCryptoRuntime(executable string) (cryptoRuntime, bool, error) {
	executableDir := filepath.Dir(executable)
	helperPath := filepath.Join(executableDir, helperFilename())
	if _, err := os.Stat(helperPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cryptoRuntime{}, false, nil
		}
		return cryptoRuntime{}, true, fmt.Errorf("locating bundled XChat crypto helper: %w", err)
	}
	runtimeDir, err := configuredRuntimeDirectory(executableDir)
	if err != nil {
		return cryptoRuntime{}, true, err
	}
	resolved, err := validateCryptoRuntime(helperPath, runtimeDir)
	return resolved, true, err
}

func configuredRuntimeDirectory(helperDirectory string) (string, error) {
	if override := strings.TrimSpace(os.Getenv(runtimeEnvironment)); override != "" {
		return filepath.Abs(override)
	}
	return filepath.Join(helperDirectory, "xchat-runtime"), nil
}

func validateCryptoRuntime(helperPath, runtimeDir string) (cryptoRuntime, error) {
	info, err := os.Stat(helperPath)
	if err != nil {
		return cryptoRuntime{}, fmt.Errorf("locating XChat crypto helper: %w", err)
	}
	if info.IsDir() {
		return cryptoRuntime{}, fmt.Errorf("XChat crypto helper is a directory: %s", helperPath)
	}
	if err := validateSDK(runtimeDir); err != nil {
		return cryptoRuntime{}, err
	}
	return cryptoRuntime{executable: helperPath, directory: runtimeDir}, nil
}

func validateSDK(runtimeDir string) error {
	required := []string{
		"session.mjs",
		"decrypt.mjs",
		filepath.Join("node_modules", "@xdevplatform", "chat-xdk", "package.json"),
		filepath.Join("node_modules", "@xdevplatform", "chat-xdk", "pkg", "chat_xdk_wasm_bg.wasm"),
		filepath.Join("node_modules", "juicebox-sdk", "package.json"),
		filepath.Join("node_modules", "juicebox-sdk", "juicebox-sdk_bg.wasm"),
	}
	for _, name := range required {
		path := filepath.Join(runtimeDir, name)
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			return fmt.Errorf("XChat crypto runtime is incomplete: %s is missing", path)
		}
	}
	return nil
}

func helperFilename() string {
	if runtime.GOOS == "windows" {
		return "xdm-xchat-helper.exe"
	}
	return "xdm-xchat-helper"
}
