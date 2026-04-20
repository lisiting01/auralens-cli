//go:build !windows

package daemon

// resolveGitBashPath is a no-op on non-Windows platforms.
func resolveGitBashPath() string { return "" }
