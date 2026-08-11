package xchat

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveCryptoRuntimeUsesConfiguredHelper(t *testing.T) {
	directory := t.TempDir()
	helper := filepath.Join(directory, helperFilename())
	writeRuntimeFixture(t, helper, directory)
	t.Setenv(helperEnvironment, helper)
	t.Setenv(runtimeEnvironment, directory)

	resolved, err := resolveCryptoRuntime()
	if err != nil {
		t.Fatal(err)
	}
	if resolved.executable != helper || resolved.directory != directory {
		t.Fatalf("resolveCryptoRuntime() = %#v", resolved)
	}
	command, err := resolved.command("session.mjs")
	if err != nil {
		t.Fatal(err)
	}
	if command.Path != helper || len(command.Args) != 2 || command.Args[1] != filepath.Join(directory, "session.mjs") {
		t.Fatalf("command = %#v", command)
	}
	for _, entry := range command.Env {
		if strings.HasPrefix(strings.ToUpper(entry), "NODE_OPTIONS=") || strings.HasPrefix(strings.ToUpper(entry), "NODE_PATH=") {
			t.Fatalf("unsafe Node environment was preserved: %q", entry)
		}
	}
}

func TestResolveCryptoRuntimeRejectsIncompleteBundle(t *testing.T) {
	directory := t.TempDir()
	helper := filepath.Join(directory, helperFilename())
	if err := os.WriteFile(helper, []byte("helper"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(helperEnvironment, helper)
	t.Setenv(runtimeEnvironment, directory)

	_, err := resolveCryptoRuntime()
	if err == nil || !strings.Contains(err.Error(), "runtime is incomplete") {
		t.Fatalf("resolveCryptoRuntime() error = %v", err)
	}
}

func TestBundledCryptoRuntimeUsesFilesBesideExecutable(t *testing.T) {
	directory := t.TempDir()
	helper := filepath.Join(directory, helperFilename())
	runtimeDirectory := filepath.Join(directory, "xchat-runtime")
	writeRuntimeFixture(t, helper, runtimeDirectory)
	t.Setenv(runtimeEnvironment, "")

	resolved, found, err := bundledCryptoRuntime(filepath.Join(directory, "xdm-test"))
	if err != nil {
		t.Fatal(err)
	}
	if !found || resolved.executable != helper || resolved.directory != runtimeDirectory {
		t.Fatalf("bundledCryptoRuntime() = %#v, %t", resolved, found)
	}
}

func TestHelperFilenameMatchesPlatform(t *testing.T) {
	want := "xdm-xchat-helper"
	if runtime.GOOS == "windows" {
		want += ".exe"
	}
	if got := helperFilename(); got != want {
		t.Fatalf("helperFilename() = %q, want %q", got, want)
	}
}

func TestCryptoEnvironmentRemovesNodeInjectionSettings(t *testing.T) {
	environment := cryptoEnvironment([]string{"PATH=/bin", "NODE_OPTIONS=--require=inject.js", "node_path=elsewhere", "XDM_TEST=1"})
	want := []string{"PATH=/bin", "XDM_TEST=1"}
	if strings.Join(environment, "\n") != strings.Join(want, "\n") {
		t.Fatalf("cryptoEnvironment() = %#v, want %#v", environment, want)
	}
}

func writeRuntimeFixture(t *testing.T, helper, directory string) {
	t.Helper()
	files := []string{
		helper,
		filepath.Join(directory, "session.mjs"),
		filepath.Join(directory, "decrypt.mjs"),
		filepath.Join(directory, "node_modules", "@xdevplatform", "chat-xdk", "package.json"),
		filepath.Join(directory, "node_modules", "@xdevplatform", "chat-xdk", "pkg", "chat_xdk_wasm_bg.wasm"),
		filepath.Join(directory, "node_modules", "juicebox-sdk", "package.json"),
		filepath.Join(directory, "node_modules", "juicebox-sdk", "juicebox-sdk_bg.wasm"),
	}
	for _, name := range files {
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte("fixture"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}
