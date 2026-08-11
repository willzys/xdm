package xchat

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestJavaScriptRuntimeLoadsPinnedSDK(t *testing.T) {
	t.Setenv(runtimeEnvironment, "")
	directory, err := runtimeDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSDK(directory); err != nil {
		t.Skipf("pinned JavaScript dependencies are not installed: %v", err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	command := exec.Command(node, filepath.Join(directory, "session.mjs"))
	command.Dir = directory
	command.Stdin = strings.NewReader("{\"op\":\"integration-test\"}\n{\"op\":\"close\"}\n")
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := string(output); !strings.Contains(got, "unsupported XChat operation integration-test") {
		t.Fatalf("runtime output = %q", got)
	}
}

func TestResolveCryptoRuntimeFallsBackForSourceDevelopment(t *testing.T) {
	t.Setenv(runtimeEnvironment, "")
	directory, err := runtimeDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSDK(directory); err != nil {
		t.Skipf("pinned JavaScript dependencies are not installed: %v", err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	t.Setenv(helperEnvironment, "")
	t.Setenv(runtimeEnvironment, directory)

	resolved, err := resolveCryptoRuntime()
	if err != nil {
		t.Fatal(err)
	}
	if resolved.executable != node || resolved.directory != directory {
		t.Fatalf("resolveCryptoRuntime() = %#v", resolved)
	}
}
