package work

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
