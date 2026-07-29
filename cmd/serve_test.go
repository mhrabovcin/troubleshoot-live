package cmd

import (
	"io"
	"strings"
	"testing"

	"github.com/mesosphere/dkp-cli-runtime/core/output"
)

func TestImportWorkersFlag(t *testing.T) {
	cmd := NewServeCommand(output.NewNonInteractiveShell(io.Discard, io.Discard, 0))
	flag := cmd.Flags().Lookup("import-workers")
	if flag == nil || flag.DefValue != "8" {
		t.Fatalf("expected --import-workers default 8")
	}

	for _, workers := range []string{"0", "-1"} {
		cmd := NewServeCommand(output.NewNonInteractiveShell(io.Discard, io.Discard, 0))
		cmd.SetArgs([]string{"bundle", "--import-workers=" + workers})
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "positive integer") {
			t.Fatalf("expected positive integer error for %s workers, got %v", workers, err)
		}
	}
}
