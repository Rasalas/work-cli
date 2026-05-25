package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestInternalDemoStatusCommandPrintsDeterministicStatus(t *testing.T) {
	var buf bytes.Buffer
	oldOut := out
	out = &buf
	t.Cleanup(func() {
		out = oldOut
	})

	cmd := rootCmd()
	cmd.SetArgs([]string{"internal", "demo-status"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := buf.String()
	for _, want := range []string{
		"running",
		"4h 20m",
		"today",
		"9h 32m",
		"huntreport",
		"4h 20m",
		"thk",
		"5h 12m",
		"paused",
		"57m",
		"notes",
		"check merge requests, test divekit members alias functionality",
		"make aliases clear in other commands like members list, overview and gui",
		"test more and tag to create a release",
		"feature done, writing docs while waiting for ci to finish",
		"Release-Workflow verifizieren",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}
