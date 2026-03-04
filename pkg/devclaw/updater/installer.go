package updater

import (
	"archive/zip"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Installer handles downloading and installing new versions.
type Installer struct {
	assetsURL string
	logger    *slog.Logger
	client    *http.Client
}

// NewInstaller creates a new installer.
func NewInstaller(assetsURL string, logger *slog.Logger) *Installer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Installer{
		assetsURL: strings.TrimRight(assetsURL, "/"),
		logger:    logger.With("component", "updater-installer"),
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

// Install downloads the latest release and replaces the current binary.
// It performs a safe install with backup/rollback.
func (inst *Installer) Install() error {
	inst.logger.Info("starting update installation")

	// 1. Create temp dir.
	tmpDir, err := os.MkdirTemp("", "devclaw-update-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// 2. Download latest.zip.
	zipPath := filepath.Join(tmpDir, "latest.zip")
	if err := inst.downloadFile(inst.assetsURL+"/latest.zip", zipPath); err != nil {
		return fmt.Errorf("download latest.zip: %w", err)
	}
	inst.logger.Info("download complete", "path", zipPath)

	// 3. Extract zip.
	extractDir := filepath.Join(tmpDir, "extracted")
	if err := inst.extractZip(zipPath, extractDir); err != nil {
		return fmt.Errorf("extract zip: %w", err)
	}

	// 4. Find the devclaw binary in the extracted files.
	newBinary, err := inst.findBinary(extractDir)
	if err != nil {
		return fmt.Errorf("find binary: %w", err)
	}
	inst.logger.Info("found new binary", "path", newBinary)

	// 5. Get current binary path.
	currentBinary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get current executable: %w", err)
	}
	currentBinary, err = filepath.EvalSymlinks(currentBinary)
	if err != nil {
		return fmt.Errorf("resolve symlinks: %w", err)
	}

	// 6. Backup: rename current -> .bak
	backupPath := currentBinary + ".bak"
	if err := os.Rename(currentBinary, backupPath); err != nil {
		return fmt.Errorf("backup current binary: %w", err)
	}
	inst.logger.Info("backed up current binary", "backup", backupPath)

	// 7. Copy new binary to the original location.
	if err := inst.copyFile(newBinary, currentBinary); err != nil {
		// Rollback: restore backup.
		inst.logger.Error("copy failed, rolling back", "error", err)
		if rbErr := os.Rename(backupPath, currentBinary); rbErr != nil {
			inst.logger.Error("rollback failed", "error", rbErr)
			return fmt.Errorf("copy new binary failed (%w) AND rollback failed (%v)", err, rbErr)
		}
		return fmt.Errorf("copy new binary (rolled back): %w", err)
	}

	// 8. Set permissions.
	if err := os.Chmod(currentBinary, 0755); err != nil {
		// Rollback.
		inst.logger.Error("chmod failed, rolling back", "error", err)
		os.Remove(currentBinary)
		if rbErr := os.Rename(backupPath, currentBinary); rbErr != nil {
			inst.logger.Error("rollback failed", "error", rbErr)
		}
		return fmt.Errorf("chmod new binary: %w", err)
	}

	inst.logger.Info("binary replaced successfully", "path", currentBinary)

	// 9. Clean up backup (best-effort).
	os.Remove(backupPath)

	return nil
}

// InstallAndRestart performs Install followed by a PM2 restart.
func (inst *Installer) InstallAndRestart() error {
	if err := inst.Install(); err != nil {
		return err
	}

	inst.logger.Info("restarting via pm2")
	cmd := exec.Command("pm2", "restart", "devclaw")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		inst.logger.Warn("pm2 restart failed, process may need manual restart", "error", err)
		return fmt.Errorf("pm2 restart: %w", err)
	}

	return nil
}

func (inst *Installer) downloadFile(url, dest string) error {
	resp, err := inst.client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func (inst *Installer) extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target := filepath.Join(destDir, f.Name)

		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path in zip: %s", f.Name)
		}

		// Reject symlink entries.
		if f.Mode()&os.ModeSymlink != 0 {
			continue
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		// Race-safe write: check for symlinks before and after open.
		if info, lErr := os.Lstat(target); lErr == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				os.Remove(target)
			}
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		outFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, f.Mode())
		if os.IsExist(err) {
			if err := os.Remove(target); err != nil {
				rc.Close()
				return fmt.Errorf("removing existing file before overwrite: %w", err)
			}
			outFile, err = os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, f.Mode())
		}
		if err != nil {
			rc.Close()
			return err
		}

		if fi, statErr := outFile.Stat(); statErr != nil || !fi.Mode().IsRegular() {
			outFile.Close()
			rc.Close()
			if statErr != nil {
				return statErr
			}
			return fmt.Errorf("refusing to write to non-regular file: %s", target)
		}
		if err := outFile.Truncate(0); err != nil {
			outFile.Close()
			rc.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func (inst *Installer) findBinary(dir string) (string, error) {
	// Look for "devclaw" binary (exact name).
	candidates := []string{
		filepath.Join(dir, "devclaw"),
		filepath.Join(dir, "devclaw-linux-amd64"),
	}

	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, nil
		}
	}

	// Walk directory looking for any executable named devclaw*.
	var found string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if strings.HasPrefix(base, "devclaw") && !strings.HasSuffix(base, ".zip") {
			found = path
			return filepath.SkipAll
		}
		return nil
	})

	if found != "" {
		return found, nil
	}

	return "", fmt.Errorf("devclaw binary not found in extracted files")
}

func (inst *Installer) copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
