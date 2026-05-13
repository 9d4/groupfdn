package commands

import (
	"github.com/spf13/cobra"
)

// ActivityCmd returns a top-level activity command that mirrors
// "attendance activity" for convenience.
func ActivityCmd() *cobra.Command {
	ctx := NewCommandContext()

	activityCmd := &cobra.Command{
		Use:     "activity",
		Aliases: []string{"act"},
		Short:   "Manage daily activities",
		Long:    "Shortcut for 'attendance activity'. Create, list, update, and delete daily activities.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			format, _ := cmd.Flags().GetString("format")
			ctx.SetFormat(format)
		},
	}

	activityCmd.AddCommand(attendanceActivityCreateCmd(ctx))
	activityCmd.AddCommand(attendanceActivityListCmd(ctx))
	activityCmd.AddCommand(attendanceActivityUpdateCmd(ctx))
	activityCmd.AddCommand(attendanceActivityDeleteCmd(ctx))

	return activityCmd
}
