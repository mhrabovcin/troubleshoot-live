package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mesosphere/dkp-cli-runtime/core/output"
	"github.com/spf13/afero"

	"github.com/mhrabovcin/troubleshoot-live/pkg/bundle"
)

// WarnOnErrorsFilePresence is a helper function that will issue a CLI warn
// message if for given file from bundle exists a file with `-errors` suffix.
// This is `troubleshoot`s way of recording errors in the bundle.
// E.g. `cluster-resources/pods.json` => `cluster-resources/pods-errors.json`.
func WarnOnErrorsFilePresence(b bundle.Bundle, out output.Output, path string) {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(filepath.Base(path), ext)
	baseDir := filepath.Dir(path)
	errorsPath := filepath.Join(baseDir, fmt.Sprintf("%s-errors%s", base, ext))

	if ok, _ := afero.Exists(b, errorsPath); ok {
		out.Warnf(
			"The file %q suggests that there were some error when collecting data for %q. The import may not be complete.",
			errorsPath, path,
		)
	}
}
