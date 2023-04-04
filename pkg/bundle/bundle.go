package bundle

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/mholt/archiver/v3"
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
// bundle from a directory or a `tar.gz` archive, which is automatically extracted
// to a temporary folder.
func New(path string) (Bundle, error) {
	switch {
	case strings.HasSuffix(path, ".tar.gz"):
		fi, err := os.Stat(path)
		if err != nil {
			return nil, err
		}

		baseDir := filepath.Join(os.TempDir(), "troubleshoot-live")
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
			if err := unarchiveToDirectory(path, tmpDir); err != nil {
				return nil, err
			}
		} else {
			log.Printf("Using already extracted support bundle in %q ...", tmpDir)
		}

		entries, err := os.ReadDir(tmpDir)
		if err != nil {
			return nil, fmt.Errorf("failed to locate bundle directory form archive: %w", err)
		}

		if len(entries) != 1 {
			return nil, fmt.Errorf("more than 1 directory in archive, cannot infer bundle directory")
		}

		return FromFs(fromDir(filepath.Join(tmpDir, entries[0].Name()))), nil
	default:
		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}

		isDir, err := afero.IsDir(afero.NewOsFs(), absPath)
		if err != nil {
			return nil, err
		}

		if !isDir {
			break
		}

		return FromFs(fromDir(absPath)), nil
	}

	return nil, ErrUnknownBundleFormat
}

// FromFs allows to create bundle form provided afero.Fs.
func FromFs(fs afero.Fs) Bundle {
	return bundle{fs}
}

func unarchiveToDirectory(archive, destDir string) error {
	archiverByExtension, err := archiver.ByExtension(archive)
	if err != nil {
		return fmt.Errorf("failed to identify archive format: %w", err)
	}

	unarc, ok := archiverByExtension.(archiver.Unarchiver)
	if !ok {
		return fmt.Errorf("not an valid archive extension")
	}

	switch t := unarc.(type) {
	case *archiver.TarGz:
		t.OverwriteExisting = true
	case *archiver.Tar:
		t.OverwriteExisting = true
	}

	if err := unarc.Unarchive(archive, destDir); err != nil {
		return fmt.Errorf("failed to unarchive bundle: %w", err)
	}

	return nil
}

func fromDir(path string) afero.Fs {
	return afero.NewReadOnlyFs(afero.NewBasePathFs(afero.NewOsFs(), path))
}
