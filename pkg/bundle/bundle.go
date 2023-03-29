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

// ErrUnknownBundleFormat is returned when bundle cannot be loaded.
var ErrUnknownBundleFormat = fmt.Errorf("unknown bundle format")

// Bundle is representing support bundle data.
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

		baseDir := filepath.Join(os.TempDir(), "troubleshoot-live")
		fi, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		tmpDir := filepath.Join(baseDir, fmt.Sprintf("%s_%d", filepath.Base(path), fi.Size()))
		ok, err := afero.DirExists(afero.NewOsFs(), tmpDir)
		if err != nil {
			return nil, err
		}
		// Directory for extracting bundle doesn't exist yet
		if !ok {
			if err := os.MkdirAll(tmpDir, 0o755); err != nil {
				return nil, fmt.Errorf("failed to create dir %q for extracting bundle", baseDir)
			}
		}

		existingDirItems, err := os.ReadDir(tmpDir)
		if err != nil {
			return nil, err
		}

		if len(existingDirItems) == 0 {
			log.Printf("Extracting support bundle from %q to %q ...", path, tmpDir)
			cmd := exec.CommandContext(
				context.Background(), "tar", "xvzf", path, "-C", tmpDir)
			if err := cmd.Run(); err != nil {
				return nil, err
			}
		} else {
			log.Printf("Using already extracted support bundle in %q ...", tmpDir)
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
		return nil, ErrUnknownBundleFormat
	}

	return bundle{fs}, nil
}

func fromDir(path string) afero.Fs {
	return afero.NewReadOnlyFs(afero.NewBasePathFs(afero.NewOsFs(), path))
}
