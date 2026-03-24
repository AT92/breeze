package cmd

import (
	"fmt"

	"breeze/internal/version"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of breeze",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("breeze %s\n", version.Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
