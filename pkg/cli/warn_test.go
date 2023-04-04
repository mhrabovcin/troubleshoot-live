package cli

import (
	"bytes"
	"testing"

	"github.com/mesosphere/dkp-cli-runtime/core/output"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhrabovcin/troubleshoot-live/pkg/bundle"
)

func TestWarnOnErrorsFilePresence(t *testing.T) {
	stdOut, stdErr, out := newOutput(0)
	assert.Empty(t, stdOut.String())
	assert.Empty(t, stdErr.String())
	fs, err := newMemFs(map[string][]byte{
		"cluster-resources/pods.json":        nil,
		"cluster-resources/pods-errors.json": nil,
	})
	require.NoError(t, err)
	b := bundle.FromFs(fs)

	WarnOnErrorsFilePresence(b, out, "cluster-resources/pods.json")
	assert.Empty(t, stdOut.String())
	expectedErr := `WRN The file "cluster-resources/pods-errors.json" suggests that there were ` +
		`some error when collecting data for "cluster-resources/pods.json"`
	assert.Contains(t, stdErr.String(), expectedErr)
}

func newOutput(verbose int) (*bytes.Buffer, *bytes.Buffer, output.Output) {
	stdOut := &bytes.Buffer{}
	stdErr := &bytes.Buffer{}
	return stdOut, stdErr, output.NewNonInteractiveShell(stdOut, stdErr, verbose)
}

func newMemFs(files map[string][]byte) (afero.Fs, error) {
	fs := afero.NewMemMapFs()
	for path, data := range files {
		if err := afero.WriteReader(fs, path, bytes.NewReader(data)); err != nil {
			return nil, err
		}
	}
	return fs, nil
}
