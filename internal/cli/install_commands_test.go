package cli

import (
	"bytes"
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
