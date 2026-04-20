//go:build windows

package daemon

import (
	"os"
	"path/filepath"
)

// resolveGitBashPath returns the path to git-bash on Windows for injection
// into CLAUDE_CODE_GIT_BASH_PATH before spawning Claude Code.
func resolveGitBashPath() string {
	if os.Getenv("CLAUDE_CODE_GIT_BASH_PATH") != "" {
		return ""
	}

	candidates := []string{
		`C:\Program Files\Git\bin\bash.exe`,
		`C:\Program Files\AI-LaunchPad\Git\bin\bash.exe`,
		`C:\Program Files (x86)\Git\bin\bash.exe`,
	}

	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		candidates = append(candidates, filepath.Join(localAppData, `Programs\Git\bin\bash.exe`))
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
