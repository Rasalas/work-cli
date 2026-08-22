package cli

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestDBPathCommandPrintsConfiguredPath(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "work.sqlite")
	t.Setenv("WORK_DB", dbPath)
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))

	var buf bytes.Buffer
	oldOut := out
	out = &buf
	t.Cleanup(func() {
		out = oldOut
	})

	cmd := rootCmd()
	cmd.SetArgs([]string{"db", "path"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got, want := buf.String(), dbPath+"\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestDBBackupCommandWritesSnapshot(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "work.sqlite")
	t.Setenv("WORK_DB", dbPath)
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))

	backupDir := t.TempDir()

	var buf bytes.Buffer
	oldOut := out
	out = &buf
	t.Cleanup(func() {
		out = oldOut
	})

	cmd := rootCmd()
	cmd.SetArgs([]string{"db", "backup", "--dir", backupDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(backupDir, "work.sqlite.bak-*.sqlite"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("found %d backup files, want 1 (%v)", len(matches), matches)
	}
	if !bytes.Contains(buf.Bytes(), []byte(matches[0])) {
		t.Fatalf("output %q does not mention backup path %q", buf.String(), matches[0])
	}
}
