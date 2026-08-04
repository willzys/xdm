package cmd

import (
	"github.com/spf13/cobra"
	"github.com/willzys/xdm/internal/demo"
	"github.com/willzys/xdm/internal/tui"
)

var demoCmd = &cobra.Command{
	Use:   "demo",
	Short: "Try xdm with a local sample conversation",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.Run(cmd.Context(), demo.New())
	},
}

func init() {
	rootCmd.AddCommand(demoCmd)
}
