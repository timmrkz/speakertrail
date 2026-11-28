package fetch

import (
	"archive/tar"
	"compress/bzip2"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// UnpackTarBz2 writes an archive into a folder, refusing anything that
// would land outside it. wrote, when given, hears every piece written.
//
// What arrives over a network is untrusted. A tar can name ../../../ and
// a link can point anywhere, and both are how an archive writes over
// something it was never given. Every name goes through SafeChild, and no
// link of either kind is written at all.
//
// The speech models and the speech library the build carries both
// arrive this way, see cmd/speakertrail-release/carry.go.
//
// A model is mostly one file of several hundred megabytes, so a stop is
// heard between the pieces of a file, not only between files.
func UnpackTarBz2(ctx context.Context, archive, into string, wrote func(int64)) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := os.MkdirAll(into, 0o755); err != nil {
		return err
	}

	reader := tar.NewReader(bzip2.NewReader(file))
	for {
		if err := ctx.Err(); err != nil {
			return ErrCancelled
		}
		head, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("the archive could not be read: %w", err)
		}
		switch head.Typeflag {
		case tar.TypeDir, tar.TypeReg:
		default:
			// Links, devices, anything else. Skipped rather than refused,
			// so one odd entry does not throw away a good model.
			continue
		}
		target, err := SafeChild(into, head.Name)
		if err != nil {
			return err
		}
		if head.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		// Bounded by what the header says, so a stream that claims one
		// size and keeps going cannot fill the disk.
		if _, err := io.Copy(out, io.LimitReader(heeding{ctx, reader, wrote}, head.Size)); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
	}
}

// heeding reads on until ctx is done, and tells wrote how much each read
// brought.
type heeding struct {
	ctx   context.Context
	r     io.Reader
	wrote func(int64)
}

func (h heeding) Read(p []byte) (int, error) {
	if h.ctx.Err() != nil {
		return 0, ErrCancelled
	}
	n, err := h.r.Read(p)
	if n > 0 && h.wrote != nil {
		h.wrote(int64(n))
	}
	return n, err
}
