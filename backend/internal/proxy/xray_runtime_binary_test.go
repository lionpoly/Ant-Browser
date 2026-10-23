package proxy

import (
	"ant-chrome/backend/internal/config"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveBinaryFindsPackagedNestedXrayBinary(t *testing.T) {
	root := t.TempDir()
	platformDir := filepath.Join(root, "bin", runtime.GOOS+"-"+runtime.GOARCH, "xray")
	if err := os.MkdirAll(platformDir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(platformDir, "xray")
	if err := os.WriteFile(want, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	manager := &XrayManager{Config: &config.Config{}, AppRoot: root}
	got, err := manager.resolveBinary()
	if err != nil {
		t.Fatalf("resolveBinary() error = %v", err)
	}
	if got != want {
		t.Fatalf("resolveBinary() = %q, want %q", got, want)
	}
}

func TestResolveBinaryFallsBackToDarwinAmd64BundleOnArm64(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("only applies to arm64 macOS")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "bin", "darwin-amd64", "xray")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "xray")
	if err := os.WriteFile(want, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	manager := &XrayManager{Config: &config.Config{}, AppRoot: root}
	got, err := manager.resolveBinary()
	if err != nil {
		t.Fatalf("resolveBinary() error = %v", err)
	}
	if got != want {
		t.Fatalf("resolveBinary() = %q, want %q", got, want)
	}
}
