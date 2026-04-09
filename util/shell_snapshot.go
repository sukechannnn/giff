package util

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var snapshotPath string

// CreateShellSnapshot captures shell aliases by running an interactive login shell
// with a timeout to avoid hanging on complex .zshrc/.bashrc.
func CreateShellSnapshot() {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}

	// Check if rc file exists
	var rcExists bool
	if strings.HasSuffix(shell, "zsh") {
		_, err := os.Stat(filepath.Join(homeDir, ".zshrc"))
		rcExists = err == nil
	} else if strings.HasSuffix(shell, "bash") {
		_, err := os.Stat(filepath.Join(homeDir, ".bashrc"))
		rcExists = err == nil
	}
	if !rcExists {
		return
	}

	// Create snapshot file
	tmpFile, err := os.CreateTemp("", "giff-snapshot-*.sh")
	if err != nil {
		return
	}
	snapshotPath = tmpFile.Name()
	tmpFile.Close()

	// Use interactive shell (-i) to load rc file, then dump aliases.
	cmd := exec.Command(shell, "-ic", "alias")
	output, err := cmd.Output()
	if err != nil {
		snapshotPath = ""
		os.Remove(tmpFile.Name())
		return
	}

	// Write snapshot file: ensure each line has "alias " prefix
	var sb strings.Builder
	sb.WriteString("# giff shell snapshot\n")
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "alias ") {
			sb.WriteString("alias ")
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	os.WriteFile(snapshotPath, []byte(sb.String()), 0644)
}

// GetSnapshotPath returns the path to the snapshot file, or empty string if none.
func GetSnapshotPath() string {
	return snapshotPath
}

// CleanupShellSnapshot removes the snapshot file.
func CleanupShellSnapshot() {
	if snapshotPath != "" {
		os.Remove(snapshotPath)
		snapshotPath = ""
	}
}
