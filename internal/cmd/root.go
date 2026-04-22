package cmd

import (
	"github.com/9d4/groupfdn/internal/commands"
	"github.com/9d4/groupfdn/internal/config"
	"github.com/9d4/groupfdn/internal/output"
	"github.com/spf13/cobra"
)

var (
	// Format is the global output format flag
	Format string

	// Cfg is the loaded configuration
	Cfg *config.Config

	// Formatter is the global output formatter
	Formatter *output.Formatter

	// RootCmd is the root command
	RootCmd = &cobra.Command{
		Use:   "groupfdn",
		Short: "CLI for Group Foundation API",
		Long:  `A command-line interface for interacting with the Group Foundation API.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			var err error
			Cfg, err = config.Load()
			if err != nil {
				return err
			}
			Formatter = output.NewFormatter(output.Format(Format))
			return nil
		},
	}
)

func init() {
	RootCmd.PersistentFlags().StringVarP(&Format, "format", "f", "table", "Output format (table, simple, json)")

	// Add subcommands
	RootCmd.AddCommand(commands.AuthCmd())
	RootCmd.AddCommand(commands.AttendanceCmd())

	// Add alias
	RootCmd.Aliases = []string{"fdn"}
}

// Execute runs the root command
func Execute() error {
	return RootCmd.Execute()
}
