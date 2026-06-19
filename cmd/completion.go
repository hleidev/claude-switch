package cmd

import (
	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:                   "completion <zsh|bash>",
	Short:                 "Generate the shell completion script",
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"zsh", "bash"},
	Args:                  cobra.ExactValidArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "zsh":
			return rootCmd.GenZshCompletion(cmd.OutOrStdout())
		case "bash":
			return rootCmd.GenBashCompletionV2(cmd.OutOrStdout(), true)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
