package cli

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallCommandRunsInstaller(t *testing.T) {
	var calls []string
	oldRunInstaller := runInstaller
	runInstaller = func(action, dir, version string) error {
		calls = append(calls, action, dir, version)
		return nil
	}
	t.Cleanup(func() {
		runInstaller = oldRunInstaller
	})

	cmd := rootCmd()
	cmd.SetArgs([]string{"install", "--dir", "/tmp/bin", "--version", "v0.1.0"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := []string{"install", "/tmp/bin", "v0.1.0"}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("calls = %v, want %v", calls, want)
		}
	}
}

func TestUpdateCommandRunsInstaller(t *testing.T) {
	var calls []string
	oldRunInstaller := runInstaller
	runInstaller = func(action, dir, version string) error {
		calls = append(calls, action, dir, version)
		return nil
	}
	t.Cleanup(func() {
		runInstaller = oldRunInstaller
	})

	cmd := rootCmd()
	cmd.SetArgs([]string{"update", "--dir", "/tmp/bin", "--version", "v0.1.0"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := []string{"update", "/tmp/bin", "v0.1.0"}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("calls = %v, want %v", calls, want)
		}
	}
}

func TestUninstallCommandRunsInstaller(t *testing.T) {
	var calls []string
	oldRunInstaller := runInstaller
	runInstaller = func(action, dir, version string) error {
		calls = append(calls, action, dir, version)
		return nil
	}
	t.Cleanup(func() {
		runInstaller = oldRunInstaller
	})

	cmd := rootCmd()
	cmd.SetArgs([]string{"uninstall", "--dir", "/tmp/bin"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := []string{"uninstall", "/tmp/bin", ""}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("calls = %v, want %v", calls, want)
		}
	}
}

func TestVersionCommandPrintsVersion(t *testing.T) {
	oldOut := out
	var buf bytes.Buffer
	out = &buf
	t.Cleanup(func() {
		out = oldOut
	})

	cmd := rootCmd()
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got, want := buf.String(), "work dev\ncommit unknown\nbuilt unknown\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestVerifyChecksumAcceptsMatchingAndRejectsTamperedScript(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "install.sh")
	content := []byte("#!/usr/bin/env bash\necho hi\n")
	if err := os.WriteFile(script, content, 0o700); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	sum := sha256.Sum256(content)
	checksums := fmt.Sprintf("%x  install.sh\n%s  work-cli_v1.0.0_darwin_arm64.tar.gz\n",
		sum, "0000000000000000000000000000000000000000000000000000000000000000")

	if err := verifyChecksum(script, []byte(checksums), "install.sh"); err != nil {
		t.Fatalf("verifyChecksum() error = %v", err)
	}

	if err := os.WriteFile(script, []byte("#!/usr/bin/env bash\nrm -rf /\n"), 0o700); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := verifyChecksum(script, []byte(checksums), "install.sh"); err == nil {
		t.Fatal("verifyChecksum() should reject a tampered script")
	}
}

func TestVerifyChecksumFailsWithoutChecksumEntry(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "install.sh")
	if err := os.WriteFile(script, []byte("echo hi"), 0o700); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := verifyChecksum(script, []byte("deadbeef  other-file.txt\n"), "install.sh"); err == nil {
		t.Fatal("verifyChecksum() without matching entry should fail")
	}
}
