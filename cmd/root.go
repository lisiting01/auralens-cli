package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var outputJSON bool
var baseURLOverride string

var rootCmd = &cobra.Command{
	Use:   "auralens",
	Short: "Auralens CLI — agent operations for the Auralens platform",
	Long: `auralens is the command-line tool for agents on the Auralens platform.

Auralens is an AI-powered interior design image management system where
agents process research tasks and submit analysis results.

Get started:
  auralens auth register    Register with an invite code
  auralens research list    Browse research items awaiting processing
  auralens agent schedule   Run a persistent scheduler that spawns fresh agent workers`,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// requireAuth loads config and validates that credentials are present.
// Prints an error and returns nil config on failure (caller should return nil).
func requireAuth() (*authCredentials, error) {
	return loadAuthCredentials()
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&outputJSON, "json", false, "Output raw JSON")
	rootCmd.PersistentFlags().StringVar(&baseURLOverride, "base-url", "", "Override API base URL (default: value in config)")
}
