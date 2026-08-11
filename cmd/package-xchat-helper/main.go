package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/willzys/xdm/internal/xchat/helperbundle"
)

func main() {
	var nodeDirectory string
	var runtimeDirectory string
	var outputDirectory string
	flag.StringVar(&nodeDirectory, "node-dir", "", "extracted official Node distribution")
	flag.StringVar(&runtimeDirectory, "runtime", filepath.FromSlash("internal/xchat/runtime"), "installed XChat JavaScript runtime")
	flag.StringVar(&outputDirectory, "output", "", "new helper bundle directory")
	flag.Parse()
	if strings.TrimSpace(nodeDirectory) == "" || strings.TrimSpace(outputDirectory) == "" {
		fmt.Fprintln(os.Stderr, "usage: package-xchat-helper --node-dir <official-node-distribution> --output <new-directory>")
		os.Exit(2)
	}
	nodeBinary := filepath.Join(nodeDirectory, "bin", "node")
	if runtime.GOOS == "windows" {
		nodeBinary = filepath.Join(nodeDirectory, "node.exe")
	}
	output, err := exec.Command(nodeBinary, "--version").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading Node version: %v\n", err)
		os.Exit(1)
	}
	err = helperbundle.Bundle(helperbundle.Options{
		NodeDirectory:    nodeDirectory,
		NodeVersion:      strings.TrimSpace(string(output)),
		RuntimeDirectory: runtimeDirectory,
		OutputDirectory:  outputDirectory,
		TargetOS:         runtime.GOOS,
		TargetArch:       runtime.GOARCH,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("XChat helper bundle created at %s\n", outputDirectory)
}
