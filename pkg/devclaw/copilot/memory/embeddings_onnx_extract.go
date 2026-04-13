package memory

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"
)

// extractLibFromTgz extracts the libonnxruntime shared library from the
// ONNX Runtime release .tgz archive. The library is located at
// onnxruntime-*/lib/libonnxruntime.so.* (or .dylib on macOS).
func extractLibFromTgz(tgzPath, destPath string) error {
	f, err := os.Open(tgzPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}

		// Look for lib/libonnxruntime.so.* or lib/libonnxruntime.dylib
		name := hdr.Name
		if !strings.Contains(name, "/lib/libonnxruntime") {
			continue
		}
		// Skip symlinks, only want the actual file.
		if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		// Extract to destPath.
		tmpPath := destPath + ".extracting"
		out, err := os.Create(tmpPath)
		if err != nil {
			return err
		}
		// Limit copy to 200MB to prevent decompression bombs.
		_, err = io.Copy(out, io.LimitReader(tr, 200*1024*1024))
		out.Close()
		if err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("extract lib: %w", err)
		}
		if err := os.Chmod(tmpPath, 0o755); err != nil {
			os.Remove(tmpPath)
			return err
		}
		return os.Rename(tmpPath, destPath)
	}

	return fmt.Errorf("libonnxruntime not found in archive")
}
