package bundle

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
)

var ErrUnknownBundle = fmt.Errorf("unknown bundle format")

type Bundle interface {
	afero.Fs

	Layout() Layout
}

type bundle struct {
	afero.Fs
}

func (bundle) Layout() Layout {
	return defaultLayout{}
}

// New creates bundle representation from given path. It supports reading extracted
// bundle from directory or `tar.gz` archive, which is automatically extracted
// to a temporary folder.
func New(path string) (Bundle, error) {
	var fs afero.Fs

	switch {
	// TODO: add a check for file size, if trying to open a large bundle in
	// memory.
	case strings.HasSuffix(path, ".tar.gz"):
		// f, err := os.Open(path)
		// if err != nil {
		// 	return nil, err
		// }
		// gz, err := gzip.NewReader(f)
		// if err != nil {
		// 	return nil, err
		// }
		// // troubleshoot.sh generates support bundle with single directory in it
		// // Walk is not working for tar archive in aferofs:
		// // https://github.com/spf13/afero/issues/281
		// fs = afero.NewBasePathFs(
		// 	tarfs.New(tar.NewReader(gz)),
		// 	filepath.Base(strings.TrimSuffix(path, ".tar.gz")),
		// )

		tmpDir, err := os.MkdirTemp("", "support-bundle-live")
		if err != nil {
			return nil, err
		}

		log.Printf("extracting support bundle from %q to %q", path, tmpDir)
		cmd := exec.CommandContext(
			context.Background(), "tar", "xvzf", path, "-C", tmpDir)
		if err := cmd.Run(); err != nil {
			return nil, err
		}

		// TODO: detect extracted path, assume dir same as archive name
		dirname := filepath.Base(strings.TrimSuffix(path, ".tar.gz"))
		fs = fromDir(filepath.Join(tmpDir, dirname))
	default:
		isDir, err := afero.IsDir(afero.NewOsFs(), path)
		if err != nil {
			return nil, err
		}

		if !isDir {
			break
		}

		fs = fromDir(path)
	}

	if fs == nil {
		return nil, ErrUnknownBundle
	}

	return bundle{fs}, nil
}

func fromDir(path string) afero.Fs {
	return afero.NewReadOnlyFs(afero.NewBasePathFs(afero.NewOsFs(), path))
}
