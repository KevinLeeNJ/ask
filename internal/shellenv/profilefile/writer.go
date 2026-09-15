package profilefile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/KevinLeeNJ/ask/internal/application/ports"
	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/platform/filesystem"
	"github.com/KevinLeeNJ/ask/internal/shellenv/adapter"
	"github.com/KevinLeeNJ/ask/internal/shellenv/managedblock"
)

type Writer struct{}

func (Writer) Defaults() (ports.ShellProfileDefaults, error) {
	shellPath := os.Getenv("SHELL")
	home, err := os.UserHomeDir()
	if err != nil {
		return ports.ShellProfileDefaults{}, failure.Wrap(failure.KindConfig, "config.user_dir_unavailable", nil, err)
	}
	shellAdapter, err := adapter.Detect(shellPath, home, runtime.GOOS)
	if err != nil {
		return ports.ShellProfileDefaults{}, failure.New(
			failure.KindConfig,
			"shell.detection_failed",
			map[string]string{"shell": shellPath},
		)
	}
	profile, _ := shellAdapter.DefaultProfile(home, runtime.GOOS)
	return ports.ShellProfileDefaults{
		Shell:       shellAdapter.Name(),
		Syntax:      shellAdapter.Syntax(),
		ProfileFile: filepath.ToSlash(profile),
	}, nil
}

func (Writer) Preview(change ports.ShellProfileChange) (ports.ShellProfilePreview, error) {
	shellAdapter, err := adapter.ByName(change.Shell)
	if err != nil {
		return ports.ShellProfilePreview{}, failure.Wrap(failure.KindConfig, "shell.unsupported", nil, err)
	}
	profile, err := expandPath(change.ProfileFile)
	if err != nil {
		return ports.ShellProfilePreview{}, err
	}
	content, err := readProfile(profile)
	if err != nil {
		return ports.ShellProfilePreview{}, err
	}
	line, err := shellAdapter.Render(change.Variable, change.Value)
	if err != nil {
		return ports.ShellProfilePreview{}, failure.Wrap(failure.KindConfig, "shell.value_invalid", nil, err)
	}
	conflict := conflictOutsideManagedBlock(content, change.Variable, shellAdapter.Variable)
	next, _, err := managedblock.Upsert(content, change.Variable, line, shellAdapter.Variable)
	if err != nil {
		return ports.ShellProfilePreview{}, failure.Wrap(failure.KindConfig, "shell.block_invalid", nil, err)
	}
	backup := backupPath(profile, time.Now())
	return ports.ShellProfilePreview{
		Shell:       shellAdapter.Name(),
		Syntax:      shellAdapter.Syntax(),
		ProfileFile: profile,
		BackupFile:  backup,
		Content:     next,
		Conflict:    conflict,
		Source:      shellAdapter.SourceCommand(profile),
	}, nil
}

func (w Writer) Write(change ports.ShellProfileChange) (ports.ShellProfilePreview, error) {
	preview, err := w.Preview(change)
	if err != nil {
		return ports.ShellProfilePreview{}, err
	}
	if preview.Conflict && !change.Overwrite {
		return ports.ShellProfilePreview{}, failure.New(
			failure.KindConfig,
			"shell.variable_conflict",
			map[string]string{
				"variable": change.Variable,
				"path":     preview.ProfileFile,
			},
		)
	}
	original, err := readProfile(preview.ProfileFile)
	if err != nil {
		return ports.ShellProfilePreview{}, err
	}
	if original != "" {
		if err := filesystem.AtomicWriteFile(preview.BackupFile, []byte(original), 0o600, 0o700); err != nil {
			return ports.ShellProfilePreview{}, failure.Wrap(
				failure.KindConfig,
				"shell.backup_failed",
				map[string]string{"path": preview.BackupFile, "reason": err.Error()},
				err,
			)
		}
	}
	if err := filesystem.AtomicWriteFile(preview.ProfileFile, []byte(preview.Content), 0o600, 0o700); err != nil {
		return ports.ShellProfilePreview{}, failure.Wrap(
			failure.KindConfig,
			"shell.write_failed",
			map[string]string{"path": preview.ProfileFile, "reason": err.Error()},
			err,
		)
	}
	return preview, nil
}

func (w Writer) Remove(change ports.ShellProfileChange) (ports.ShellProfilePreview, error) {
	shellAdapter, err := adapter.ByName(change.Shell)
	if err != nil {
		return ports.ShellProfilePreview{}, failure.Wrap(failure.KindConfig, "shell.unsupported", nil, err)
	}
	profile, err := expandPath(change.ProfileFile)
	if err != nil {
		return ports.ShellProfilePreview{}, err
	}
	original, err := readProfile(profile)
	if err != nil {
		return ports.ShellProfilePreview{}, err
	}
	next, err := managedblock.Remove(original, change.Variable, shellAdapter.Variable)
	if err != nil {
		return ports.ShellProfilePreview{}, failure.Wrap(failure.KindConfig, "shell.block_invalid", nil, err)
	}
	backup := backupPath(profile, time.Now())
	if original != "" {
		if err := filesystem.AtomicWriteFile(backup, []byte(original), 0o600, 0o700); err != nil {
			return ports.ShellProfilePreview{}, failure.Wrap(
				failure.KindConfig,
				"shell.backup_failed",
				map[string]string{"path": backup, "reason": err.Error()},
				err,
			)
		}
	}
	if err := filesystem.AtomicWriteFile(profile, []byte(next), 0o600, 0o700); err != nil {
		return ports.ShellProfilePreview{}, failure.Wrap(
			failure.KindConfig,
			"shell.write_failed",
			map[string]string{"path": profile, "reason": err.Error()},
			err,
		)
	}
	return ports.ShellProfilePreview{
		Shell:       shellAdapter.Name(),
		Syntax:      shellAdapter.Syntax(),
		ProfileFile: profile,
		BackupFile:  backup,
		Content:     next,
		Source:      shellAdapter.SourceCommand(profile),
	}, nil
}

func readProfile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", failure.Wrap(
			failure.KindConfig,
			"shell.read_failed",
			map[string]string{"path": path, "reason": err.Error()},
			err,
		)
	}
	return string(content), nil
}

func conflictOutsideManagedBlock(
	content string,
	variable string,
	adapterVariable func(string) string,
) bool {
	start := strings.Index(content, managedblock.StartMarker)
	end := strings.Index(content, managedblock.EndMarker)
	visible := content
	if start >= 0 && end > start {
		visible = content[:start] + content[end+len(managedblock.EndMarker):]
	}
	for _, line := range strings.Split(visible, "\n") {
		if adapterVariable(line) == variable {
			return true
		}
	}
	return false
}

func expandPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", failure.New(failure.KindConfig, "shell.profile_missing", nil)
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", failure.Wrap(failure.KindConfig, "config.user_dir_unavailable", nil, err)
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", failure.Wrap(failure.KindConfig, "shell.profile_invalid", nil, err)
	}
	return absolute, nil
}

func backupPath(profile string, now time.Time) string {
	return fmt.Sprintf("%s.ask-backup-%s", profile, now.Format("20060102T150405"))
}
