package bundle

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/mholt/archives"
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
			if err := unarchiveToDirectory(context.TODO(), path, tmpDir); err != nil {
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

func unarchiveToDirectory(ctx context.Context, archive, destDir string) error {
	sourceArchive, err := os.Open(archive)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer sourceArchive.Close()

	format, reader, err := archives.Identify(ctx, archive, sourceArchive)
	if err != nil {
		return fmt.Errorf("failed to identify archive: %w", err)
	}

	if ex, ok := format.(archives.Extractor); ok {
		return ex.Extract(ctx, reader, func(ctx context.Context, info archives.FileInfo) error {
			if info.IsDir() {
				if err := os.MkdirAll(filepath.Join(destDir, info.Name()), 0o755); err != nil {
					return err
				}
				return nil
			}

			baseDir := filepath.Dir(info.NameInArchive)
			if err := os.MkdirAll(filepath.Join(destDir, baseDir), 0o755); err != nil {
				return err
			}

			src, err := info.Open()
			if err != nil {
				return fmt.Errorf("failed to open file %q: %w", info.Name(), err)
			}
			defer src.Close()

			dstPath := filepath.Join(destDir, baseDir, info.Name())
			dst, err := os.Create(dstPath)
			if err != nil {
				return fmt.Errorf("failed to create file %q: %w", dstPath, err)
			}
			defer dst.Close()

			_, err = io.Copy(dst, src)
			return err
		})
	}

	return fmt.Errorf("unsupported archive format")
}

func fromDir(path string) afero.Fs {
	return afero.NewReadOnlyFs(afero.NewBasePathFs(afero.NewOsFs(), path))
}
