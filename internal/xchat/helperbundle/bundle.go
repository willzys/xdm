package helperbundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const runtimeDirectoryName = "xchat-runtime"

type Options struct {
	NodeDirectory    string
	NodeVersion      string
	RuntimeDirectory string
	OutputDirectory  string
	TargetOS         string
	TargetArch       string
}

type manifest struct {
	SchemaVersion  int    `json:"schemaVersion"`
	NodeVersion    string `json:"nodeVersion"`
	TargetOS       string `json:"targetOS"`
	TargetArch     string `json:"targetArch"`
	Helper         string `json:"helper"`
	Runtime        string `json:"runtime"`
	PackageLockSHA string `json:"packageLockSha256"`
}

func Bundle(options Options) error {
	if err := validateOptions(&options); err != nil {
		return err
	}
	if _, err := os.Stat(options.OutputDirectory); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return fmt.Errorf("output directory already exists: %s", options.OutputDirectory)
		}
		return err
	}

	parent := filepath.Dir(options.OutputDirectory)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, ".xdm-xchat-helper-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)

	helper := helperFilename(options.TargetOS)
	nodeBinary := filepath.Join(options.NodeDirectory, nodeFilename(options.TargetOS))
	if err := copyFile(nodeBinary, filepath.Join(staging, helper)); err != nil {
		return fmt.Errorf("copying Node executable: %w", err)
	}
	if err := copyFile(filepath.Join(options.NodeDirectory, "LICENSE"), filepath.Join(staging, "THIRD_PARTY_NOTICES", "NODE-LICENSE.txt")); err != nil {
		return fmt.Errorf("copying Node license: %w", err)
	}
	if err := copyRuntime(options.RuntimeDirectory, filepath.Join(staging, runtimeDirectoryName)); err != nil {
		return err
	}

	lockHash, err := fileSHA256(filepath.Join(options.RuntimeDirectory, "package-lock.json"))
	if err != nil {
		return err
	}
	metadata := manifest{
		SchemaVersion:  1,
		NodeVersion:    options.NodeVersion,
		TargetOS:       options.TargetOS,
		TargetArch:     options.TargetArch,
		Helper:         helper,
		Runtime:        runtimeDirectoryName,
		PackageLockSHA: lockHash,
	}
	encoded, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(filepath.Join(staging, "xchat-helper.json"), encoded, 0o644); err != nil {
		return err
	}
	if err := os.Rename(staging, options.OutputDirectory); err != nil {
		return fmt.Errorf("publishing helper bundle: %w", err)
	}
	return nil
}

func validateOptions(options *Options) error {
	options.NodeDirectory = filepath.Clean(strings.TrimSpace(options.NodeDirectory))
	options.RuntimeDirectory = filepath.Clean(strings.TrimSpace(options.RuntimeDirectory))
	options.OutputDirectory = filepath.Clean(strings.TrimSpace(options.OutputDirectory))
	options.NodeVersion = strings.TrimSpace(options.NodeVersion)
	options.TargetOS = strings.ToLower(strings.TrimSpace(options.TargetOS))
	options.TargetArch = strings.ToLower(strings.TrimSpace(options.TargetArch))
	if options.NodeDirectory == "." || options.RuntimeDirectory == "." || options.OutputDirectory == "." {
		return errors.New("node-dir, runtime and output must be set")
	}
	if options.NodeVersion == "" {
		return errors.New("node version must be set")
	}
	majorText := strings.SplitN(strings.TrimPrefix(options.NodeVersion, "v"), ".", 2)[0]
	major, err := strconv.Atoi(majorText)
	if err != nil || major < 18 {
		return fmt.Errorf("Node 18 or newer is required, got %q", options.NodeVersion)
	}
	if options.TargetOS != "windows" && options.TargetOS != "linux" && options.TargetOS != "darwin" {
		return fmt.Errorf("unsupported target OS %q", options.TargetOS)
	}
	if options.TargetArch == "" {
		return errors.New("target architecture must be set")
	}
	required := []string{
		"session.mjs",
		"decrypt.mjs",
		"package.json",
		"package-lock.json",
		filepath.Join("node_modules", "@xdevplatform", "chat-xdk", "package.json"),
		filepath.Join("node_modules", "@xdevplatform", "chat-xdk", "pkg", "chat_xdk_wasm_bg.wasm"),
		filepath.Join("node_modules", "juicebox-sdk", "package.json"),
		filepath.Join("node_modules", "juicebox-sdk", "juicebox-sdk_bg.wasm"),
	}
	for _, name := range required {
		if info, err := os.Stat(filepath.Join(options.RuntimeDirectory, name)); err != nil || info.IsDir() {
			return fmt.Errorf("runtime dependency is missing: %s", name)
		}
	}
	for _, name := range []string{nodeFilename(options.TargetOS), "LICENSE"} {
		if info, err := os.Stat(filepath.Join(options.NodeDirectory, name)); err != nil || info.IsDir() {
			return fmt.Errorf("official Node distribution file is missing: %s", name)
		}
	}
	return nil
}

func copyRuntime(source, destination string) error {
	for _, name := range []string{"session.mjs", "decrypt.mjs", "package.json", "package-lock.json"} {
		if err := copyFile(filepath.Join(source, name), filepath.Join(destination, name)); err != nil {
			return fmt.Errorf("copying runtime file %s: %w", name, err)
		}
	}
	if err := copyTree(filepath.Join(source, "node_modules"), filepath.Join(destination, "node_modules")); err != nil {
		return fmt.Errorf("copying runtime dependencies: %w", err)
	}
	return nil
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are not supported: %s", path)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		return copyFile(path, target)
	})
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func helperFilename(targetOS string) string {
	if targetOS == "windows" {
		return "xdm-xchat-helper.exe"
	}
	return "xdm-xchat-helper"
}

func nodeFilename(targetOS string) string {
	if targetOS == "windows" {
		return "node.exe"
	}
	return "bin/node"
}
