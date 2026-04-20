package cmd

import (
	"fmt"

	"github.com/lisiting01/auralens-cli/internal/output"
	"github.com/spf13/cobra"
)

// These variables are injected at build time by GoReleaser via -ldflags.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print auralens version information",
	Run: func(cmd *cobra.Command, args []string) {
		if outputJSON {
			_ = output.JSON(map[string]string{
				"version": Version,
				"commit":  Commit,
				"date":    Date,
			})
			return
		}
		fmt.Printf("auralens %s (%s) built %s\n", Version, Commit, Date)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
