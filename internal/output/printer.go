package output

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/fatih/color"
)

var (
	green  = color.New(color.FgGreen).SprintFunc()
	yellow = color.New(color.FgYellow).SprintFunc()
	red    = color.New(color.FgRed).SprintFunc()
	cyan   = color.New(color.FgCyan).SprintFunc()
	faint  = color.New(color.Faint).SprintFunc()
	bold   = color.New(color.Bold).SprintFunc()
)

// Green returns text formatted in green.
func Green(s string) string { return green(s) }

// Yellow returns text formatted in yellow.
func Yellow(s string) string { return yellow(s) }

// Red returns text formatted in red.
func Red(s string) string { return red(s) }

// Cyan returns text formatted in cyan.
func Cyan(s string) string { return cyan(s) }

// Faint returns text formatted in faint/dim style.
func Faint(s string) string { return faint(s) }

// Bold returns text formatted in bold.
func Bold(s string) string { return bold(s) }

// Success prints a green "✓" prefixed message to stdout.
func Success(msg string) {
	fmt.Println(green("✓ " + msg))
}

// Error prints a red "✗" prefixed message to stderr.
func Error(msg string) {
	fmt.Fprintln(os.Stderr, red("✗ "+msg))
}

// Warn prints a yellow "!" prefixed message to stderr.
func Warn(msg string) {
	fmt.Fprintln(os.Stderr, yellow("! "+msg))
}

// JSON marshals v and prints it as indented JSON to stdout.
func JSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	fmt.Println(string(data))
	return nil
}
