package util

import (
	"os"
	"path/filepath"
	"strings"
)

// LoadShellHistory loads command history from the user's shell history file.
// Returns commands in reverse chronological order (most recent first).
func LoadShellHistory(maxEntries int) []string {
	shell := os.Getenv("SHELL")
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	var histFile string
	var isZsh bool
	if strings.HasSuffix(shell, "zsh") {
		histFile = filepath.Join(homeDir, ".zsh_history")
		isZsh = true
	} else if strings.HasSuffix(shell, "bash") {
		histFile = filepath.Join(homeDir, ".bash_history")
	} else {
		return nil
	}

	data, err := os.ReadFile(histFile)
	if err != nil {
		return nil
	}

	lines := strings.Split(string(data), "\n")

	// Parse and collect commands (reverse order)
	var commands []string
	seen := make(map[string]bool)
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		var cmd string
		if isZsh {
			// zsh format: ": timestamp:0;command" or continuation with "\"
			if idx := strings.Index(line, ";"); idx >= 0 {
				cmd = line[idx+1:]
			} else {
				continue
			}
		} else {
			cmd = line
		}

		cmd = strings.TrimSpace(cmd)
		if cmd == "" || seen[cmd] {
			continue
		}
		seen[cmd] = true
		commands = append(commands, cmd)
		if len(commands) >= maxEntries {
			break
		}
	}

	return commands
}
