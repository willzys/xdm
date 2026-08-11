package helperbundle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBundleCreatesSelfContainedLayout(t *testing.T) {
	root := t.TempDir()
	nodeDirectory := filepath.Join(root, "node")
	runtimeDirectory := filepath.Join(root, "runtime")
	outputDirectory := filepath.Join(root, "bundle")
	writeFixture(t, filepath.Join(nodeDirectory, "node.exe"), "node")
	writeFixture(t, filepath.Join(nodeDirectory, "LICENSE"), "node license")
	writeRuntime(t, runtimeDirectory)

	err := Bundle(Options{
		NodeDirectory:    nodeDirectory,
		NodeVersion:      "v24.16.0",
		RuntimeDirectory: runtimeDirectory,
		OutputDirectory:  outputDirectory,
		TargetOS:         "windows",
		TargetArch:       "amd64",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"xdm-xchat-helper.exe",
		filepath.Join("THIRD_PARTY_NOTICES", "NODE-LICENSE.txt"),
		filepath.Join("xchat-runtime", "session.mjs"),
		filepath.Join("xchat-runtime", "node_modules", "@xdevplatform", "chat-xdk", "pkg", "chat_xdk_wasm_bg.wasm"),
		filepath.Join("xchat-runtime", "node_modules", "juicebox-sdk", "juicebox-sdk_bg.wasm"),
	} {
		if _, err := os.Stat(filepath.Join(outputDirectory, name)); err != nil {
			t.Errorf("bundle file %s: %v", name, err)
		}
	}
	encoded, err := os.ReadFile(filepath.Join(outputDirectory, "xchat-helper.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata manifest
	if err := json.Unmarshal(encoded, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.NodeVersion != "v24.16.0" || metadata.TargetArch != "amd64" || metadata.Helper != "xdm-xchat-helper.exe" || len(metadata.PackageLockSHA) != 64 {
		t.Fatalf("manifest = %#v", metadata)
	}
}

func TestBundleDoesNotOverwriteOutput(t *testing.T) {
	root := t.TempDir()
	nodeDirectory := filepath.Join(root, "node")
	runtimeDirectory := filepath.Join(root, "runtime")
	outputDirectory := filepath.Join(root, "bundle")
	writeFixture(t, filepath.Join(nodeDirectory, "node.exe"), "node")
	writeFixture(t, filepath.Join(nodeDirectory, "LICENSE"), "node license")
	writeRuntime(t, runtimeDirectory)
	if err := os.Mkdir(outputDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	err := Bundle(Options{
		NodeDirectory:    nodeDirectory,
		NodeVersion:      "v24.16.0",
		RuntimeDirectory: runtimeDirectory,
		OutputDirectory:  outputDirectory,
		TargetOS:         "windows",
		TargetArch:       "amd64",
	})
	if err == nil {
		t.Fatal("Bundle() returned no error")
	}
}

func TestBundleRejectsUnsupportedNode(t *testing.T) {
	err := Bundle(Options{
		NodeDirectory:    "node",
		NodeVersion:      "v16.20.2",
		RuntimeDirectory: "runtime",
		OutputDirectory:  "output",
		TargetOS:         "windows",
		TargetArch:       "amd64",
	})
	if err == nil {
		t.Fatal("Bundle() returned no error")
	}
}

func writeRuntime(t *testing.T, directory string) {
	t.Helper()
	files := map[string]string{
		"session.mjs":       "session",
		"decrypt.mjs":       "decrypt",
		"package.json":      "{}",
		"package-lock.json": "{\"lockfileVersion\":3}",
		filepath.Join("node_modules", "@xdevplatform", "chat-xdk", "package.json"):                 "{}",
		filepath.Join("node_modules", "@xdevplatform", "chat-xdk", "pkg", "chat_xdk_wasm_bg.wasm"): "chat wasm",
		filepath.Join("node_modules", "juicebox-sdk", "package.json"):                              "{}",
		filepath.Join("node_modules", "juicebox-sdk", "juicebox-sdk_bg.wasm"):                      "juicebox wasm",
	}
	for name, contents := range files {
		writeFixture(t, filepath.Join(directory, name), contents)
	}
}

func writeFixture(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
}
