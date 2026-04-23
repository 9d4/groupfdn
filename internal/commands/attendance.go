package commands

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/9d4/groupfdn/internal/api"
	"github.com/mergestat/timediff"
	"github.com/spf13/cobra"
)

// AttendanceCmd returns the attendance command
func AttendanceCmd() *cobra.Command {
	ctx := NewCommandContext()

	attendanceCmd := &cobra.Command{
		Use:     "attendance",
		Aliases: []string{"attend", "att", "atd"},
		Short:   "Attendance management commands",
		Long:    "Manage attendance, check-in/out, and daily activities",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			format, _ := cmd.Flags().GetString("format")
			ctx.SetFormat(format)
		},
	}

	attendanceCmd.AddCommand(attendanceCheckInCmd(ctx))
	attendanceCmd.AddCommand(attendanceCheckOutCmd(ctx))
	attendanceCmd.AddCommand(attendanceTodayCmd(ctx))
	attendanceCmd.AddCommand(attendanceListCmd(ctx))
	attendanceCmd.AddCommand(attendanceTeamWorkHoursCmd(ctx))
	attendanceCmd.AddCommand(attendanceActivityCmd(ctx))

	return attendanceCmd
}

func attendanceCheckInCmd(ctx *CommandContext) *cobra.Command {
	var latitude, longitude float64
	var notes string

	cmd := &cobra.Command{
		Use:   "checkin",
		Short: "Check in for work",
		RunE: func(c *cobra.Command, args []string) error {
			if !ctx.Config.IsAuthenticated() {
				return errors.New("not authenticated. Please login first")
			}

			client := api.NewClient(ctx.Config)

			body := make(map[string]interface{})
			if c.Flags().Changed("latitude") {
				body["latitude"] = latitude
			}
			if c.Flags().Changed("longitude") {
				body["longitude"] = longitude
			}
			if notes != "" {
				body["notes"] = notes
			}

			resp, err := client.Post("/attendance/checkin", body)
			if err != nil {
				return err
			}

			var result map[string]interface{}
			if err := api.ParseResponse(resp, &result); err != nil {
				return err
			}

			ctx.Formatter.PrintMessage("Check-in successful")
			ctx.Formatter.PrintMap(result)
			return nil
		},
	}

	cmd.Flags().Float64Var(&latitude, "latitude", 0, "GPS latitude")
	cmd.Flags().Float64Var(&longitude, "longitude", 0, "GPS longitude")
	cmd.Flags().StringVar(&notes, "notes", "", "Check-in notes")

	return cmd
}

func attendanceCheckOutCmd(ctx *CommandContext) *cobra.Command {
	return &cobra.Command{
		Use:   "checkout",
		Short: "Check out from work",
		RunE: func(c *cobra.Command, args []string) error {
			if !ctx.Config.IsAuthenticated() {
				return errors.New("not authenticated. Please login first")
			}

			client := api.NewClient(ctx.Config)

			resp, err := client.Post("/attendance/checkout", nil)
			if err != nil {
				return err
			}

			var result map[string]interface{}
			if err := api.ParseResponse(resp, &result); err != nil {
				return err
			}

			ctx.Formatter.PrintMessage("Check-out successful")
			ctx.Formatter.PrintMap(result)
			return nil
		},
	}
}

func attendanceTodayCmd(ctx *CommandContext) *cobra.Command {
	return &cobra.Command{
		Use:   "today",
		Short: "Get today's attendance",
		RunE: func(c *cobra.Command, args []string) error {
			if !ctx.Config.IsAuthenticated() {
				return errors.New("not authenticated. Please login first")
			}

			client := api.NewClient(ctx.Config)

			resp, err := client.Get("/attendance/today")
			if err != nil {
				return err
			}

			var result map[string]interface{}
			if err := api.ParseResponse(resp, &result); err != nil {
				return err
			}

			ctx.Formatter.PrintMap(result)
			return nil
		},
	}
}

func attendanceListCmd(ctx *CommandContext) *cobra.Command {
	var page, limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List attendance records",
		RunE: func(c *cobra.Command, args []string) error {
			if !ctx.Config.IsAuthenticated() {
				return errors.New("not authenticated. Please login first")
			}

			client := api.NewClient(ctx.Config)

			endpoint := fmt.Sprintf("/attendance?page=%d&limit=%d", page, limit)
			resp, err := client.Get(endpoint)
			if err != nil {
				return err
			}

			var result struct {
				Page    int                      `json:"page"`
				Limit   int                      `json:"limit"`
				Records []map[string]interface{} `json:"records"`
			}
			if err := api.ParseResponse(resp, &result); err != nil {
				return err
			}

			rows := make([]map[string]interface{}, 0, len(result.Records))
			for _, record := range result.Records {
				isJSON := ctx.Formatter.IsJSON()
				row := map[string]interface{}{
					"status":   record["status"],
					"workType": record["workType"],
				}

				if isJSON {
					row["id"] = record["_id"]
					row["date"] = record["date"]
					row["checkInTime"] = record["checkInTime"]
					row["checkOutTime"] = record["checkOutTime"]
					row["totalWorkHours"] = record["totalWorkHours"]
				} else {
					row["id"] = truncateText(asString(record["_id"]), 12)
					row["date"] = relativeTime(record["date"])
					row["checkInTime"] = relativeTime(record["checkInTime"])
					row["checkOutTime"] = relativeTime(record["checkOutTime"])
					row["totalWorkHours"] = formatHoursOneDecimal(record["totalWorkHours"])
				}

				if userRaw, ok := record["userId"]; ok {
					if user, ok := userRaw.(map[string]interface{}); ok {
						if isJSON {
							row["userName"] = user["name"]
							row["userEmail"] = user["email"]
							row["department"] = user["department"]
						} else {
							row["userName"] = truncateText(asString(user["name"]), 20)
							row["userEmail"] = truncateText(asString(user["email"]), 24)
							row["department"] = user["department"]
						}
					}
				}

				rows = append(rows, row)
			}

			headers := []string{"id", "userName", "userEmail", "department", "date", "checkInTime", "checkOutTime", "status", "workType", "totalWorkHours"}
			ctx.Formatter.Print(rows, headers)
			if !ctx.Formatter.IsJSON() {
				fmt.Printf("Page: %d | Limit: %d | Records: %d\n", result.Page, result.Limit, len(result.Records))
			}

			return nil
		},
	}

	cmd.Flags().IntVarP(&page, "page", "p", 1, "Page number")
	cmd.Flags().IntVarP(&limit, "limit", "l", 20, "Items per page")

	return cmd
}

func asString(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func truncateText(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}

func relativeTime(v interface{}) string {
	if v == nil {
		return "-"
	}
	s := asString(v)
	if s == "" || s == "<nil>" {
		return "-"
	}

	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02", "2006-01-02 15:04:05"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return timediff.TimeDiff(t)
		}
	}
	return s
}

func formatHoursOneDecimal(v interface{}) string {
	if v == nil {
		return "0.0"
	}

	s := asString(v)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		if n, ok := v.(float64); ok {
			f = n
		} else {
			return s
		}
	}

	return fmt.Sprintf("%.1f", f)
}

func attendanceTeamWorkHoursCmd(ctx *CommandContext) *cobra.Command {
	var startDate, endDate string
	var page int

	cmd := &cobra.Command{
		Use:   "team-work-hours",
		Short: "Get team work hours report",
		RunE: func(c *cobra.Command, args []string) error {
			if !ctx.Config.IsAuthenticated() {
				return errors.New("not authenticated. Please login first")
			}

			client := api.NewClient(ctx.Config)

			endpoint := fmt.Sprintf("/attendance/team-work-hours?startDate=%s&endDate=%s&page=%d", startDate, endDate, page)
			resp, err := client.Get(endpoint)
			if err != nil {
				return err
			}

			var result struct {
				Data []map[string]interface{} `json:"data"`
				Meta struct {
					Total      int `json:"total"`
					Page       int `json:"page"`
					TotalPages int `json:"totalPages"`
				} `json:"meta"`
			}
			if err := api.ParseResponse(resp, &result); err != nil {
				return err
			}

			headers := []string{"user", "totalWorkHours", "daysPresent", "daysAbsent"}
			ctx.Formatter.Print(result.Data, headers)

			return nil
		},
	}

	cmd.Flags().StringVar(&startDate, "start-date", "", "Start date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&endDate, "end-date", "", "End date (YYYY-MM-DD)")
	cmd.Flags().IntVarP(&page, "page", "p", 1, "Page number")
	cmd.MarkFlagRequired("start-date")
	cmd.MarkFlagRequired("end-date")

	return cmd
}

func attendanceActivityCmd(ctx *CommandContext) *cobra.Command {
	activityCmd := &cobra.Command{
		Use:     "activity",
		Aliases: []string{"act"},
		Short:   "Manage daily activities",
		Long:    "Create, list, update, and delete daily activities",
	}

	activityCmd.AddCommand(attendanceActivityCreateCmd(ctx))
	activityCmd.AddCommand(attendanceActivityListCmd(ctx))
	activityCmd.AddCommand(attendanceActivityUpdateCmd(ctx))
	activityCmd.AddCommand(attendanceActivityDeleteCmd(ctx))

	return activityCmd
}

func attendanceActivityCreateCmd(ctx *CommandContext) *cobra.Command {
	var date, title, description, activityType, projectID string
	var duration float64

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a daily activity",
		RunE: func(c *cobra.Command, args []string) error {
			if !ctx.Config.IsAuthenticated() {
				return errors.New("not authenticated. Please login first")
			}

			date = defaultDateToday(date)
			// Convert date to ISO format (YYYY-MM-DDT00:00:00.000Z)
			isoDate := date + "T00:00:00.000Z"

			client := api.NewClient(ctx.Config)

			// If activity type is task and no project ID provided, show interactive picker
			if activityType == "task" && projectID == "" {
				projects, err := client.GetProjects()
				if err != nil {
					return fmt.Errorf("failed to fetch projects: %w", err)
				}

				if len(projects) == 0 {
					return errors.New("no projects available. Please create a project first")
				}

				// Display project list
				fmt.Println("Available projects:")
				for i, p := range projects {
					fmt.Printf("  [%d] %s (ID: %s)\n", i+1, p.Name, truncateText(p.ID, 12))
				}

				// Interactive selection
				reader := bufio.NewReader(os.Stdin)
				for {
					fmt.Print("Select project number: ")
					input, err := reader.ReadString('\n')
					if err != nil {
						return fmt.Errorf("failed to read input: %w", err)
					}
					input = strings.TrimSpace(input)

					idx, err := strconv.Atoi(input)
					if err != nil || idx < 1 || idx > len(projects) {
						fmt.Printf("Invalid selection. Please enter a number between 1 and %d\n", len(projects))
						continue
					}

					projectID = projects[idx-1].ID
					fmt.Printf("Selected: %s\n", projects[idx-1].Name)
					break
				}
			}

			// Build request body matching API format
			body := map[string]interface{}{
				"title":        title,
				"description":  description,
				"duration":     duration,
				"activityType": activityType,
				"date":         isoDate,
			}

			// Only include projectId if provided (task type)
			if projectID != "" {
				body["projectId"] = projectID
			}

			resp, err := client.Post("/attendance/daily-activity", body)
			if err != nil {
				return err
			}

			var result map[string]interface{}
			if err := api.ParseResponse(resp, &result); err != nil {
				return err
			}

			ctx.Formatter.PrintMessage("Activity created successfully")
			ctx.Formatter.PrintMap(result)
			return nil
		},
	}

	cmd.Flags().StringVar(&date, "date", "", "Activity date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&title, "title", "", "Activity title (required)")
	cmd.Flags().StringVar(&description, "description", "", "Activity description")
	cmd.Flags().Float64Var(&duration, "duration", 0, "Duration in hours")
	cmd.Flags().StringVar(&activityType, "activity-type", "task", "Activity type (task, meeting, etc.)")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project ID (optional, will prompt if not provided for tasks)")
	cmd.MarkFlagRequired("title")

	return cmd
}

func attendanceActivityListCmd(ctx *CommandContext) *cobra.Command {
	var date string
	var verbose bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List daily activities",
		RunE: func(c *cobra.Command, args []string) error {
			if !ctx.Config.IsAuthenticated() {
				return errors.New("not authenticated. Please login first")
			}

			date = defaultDateToday(date)

			client := api.NewClient(ctx.Config)

			endpoint := fmt.Sprintf("/attendance/daily-activity?date=%s", date)
			resp, err := client.Get(endpoint)
			if err != nil {
				return err
			}

			var result []map[string]interface{}
			if err := api.ParseResponse(resp, &result); err != nil {
				return err
			}

			rows := make([]map[string]interface{}, 0, len(result))
			isJSON := ctx.Formatter.IsJSON()
			for _, item := range result {
				row := map[string]interface{}{
					"type": asString(item["activityType"]),
				}

				if isJSON {
					row["id"] = item["_id"]
					row["title"] = activityTitle(item)
					row["project"] = nestedMapString(item, "projectId", "name")
					row["duration"] = item["duration"]
					row["user"] = nestedMapString(item, "userId", "name")
					row["date"] = item["date"]
					row["updated"] = item["updatedAt"]
					if verbose {
						row["description"] = asString(item["description"])
						row["created"] = item["createdAt"]
					}
				} else {
					row["id"] = truncateText(asString(item["_id"]), 12)
					row["title"] = truncateText(activityTitle(item), 28)
					row["project"] = truncateText(nestedMapString(item, "projectId", "name"), 16)
					row["duration"] = formatDurationHours(item["duration"])
					row["user"] = truncateText(nestedMapString(item, "userId", "name"), 18)
					row["date"] = relativeTime(item["date"])
					row["updated"] = relativeTime(item["updatedAt"])
					if verbose {
						row["description"] = asString(item["description"])
						row["created"] = relativeTime(item["createdAt"])
					}
				}
				rows = append(rows, row)
			}

			headers := []string{"id", "title", "project", "type", "duration", "user", "date", "updated"}
			if verbose {
				headers = []string{"id", "title", "description", "project", "type", "duration", "user", "date", "created", "updated"}
			}
			ctx.Formatter.Print(rows, headers)
			if !isJSON {
				fmt.Printf("Records: %d\n", len(rows))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&date, "date", "", "Activity date (YYYY-MM-DD)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show description and extra columns")

	return cmd
}

func defaultDateToday(date string) string {
	if strings.TrimSpace(date) != "" {
		return date
	}
	return time.Now().Format("2006-01-02")
}

func nestedMapString(item map[string]interface{}, mapKey, field string) string {
	v, ok := item[mapKey]
	if !ok {
		return ""
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return ""
	}
	return asString(m[field])
}

func activityTitle(item map[string]interface{}) string {
	if title := strings.TrimSpace(asString(item["title"])); title != "" {
		return title
	}
	if activity := strings.TrimSpace(asString(item["activity"])); activity != "" {
		return activity
	}
	if desc := strings.TrimSpace(asString(item["description"])); desc != "" {
		return desc
	}
	return "-"
}

func formatDurationHours(v interface{}) string {
	if v == nil {
		return "0.0h"
	}
	s := asString(v)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		if n, ok := v.(float64); ok {
			f = n
		} else {
			return s
		}
	}
	return fmt.Sprintf("%.1fh", f)
}

func attendanceActivityUpdateCmd(ctx *CommandContext) *cobra.Command {
	var activity string

	cmd := &cobra.Command{
		Use:   "update [id]",
		Short: "Update a daily activity",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if !ctx.Config.IsAuthenticated() {
				return errors.New("not authenticated. Please login first")
			}

			client := api.NewClient(ctx.Config)
			activityID := args[0]

			body := map[string]string{
				"activity": activity,
			}

			endpoint := fmt.Sprintf("/attendance/daily-activity/%s", activityID)
			resp, err := client.Put(endpoint, body)
			if err != nil {
				return err
			}

			var result map[string]interface{}
			if err := api.ParseResponse(resp, &result); err != nil {
				return err
			}

			ctx.Formatter.PrintMessage("Activity updated successfully")
			ctx.Formatter.PrintMap(result)
			return nil
		},
	}

	cmd.Flags().StringVar(&activity, "activity", "", "Activity description")
	cmd.MarkFlagRequired("activity")

	return cmd
}

func attendanceActivityDeleteCmd(ctx *CommandContext) *cobra.Command {
	return &cobra.Command{
		Use:   "delete [id]",
		Short: "Delete a daily activity",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if !ctx.Config.IsAuthenticated() {
				return errors.New("not authenticated. Please login first")
			}

			client := api.NewClient(ctx.Config)
			activityID := args[0]

			endpoint := fmt.Sprintf("/attendance/daily-activity/%s", activityID)
			resp, err := client.Delete(endpoint)
			if err != nil {
				return err
			}

			if err := api.ParseResponse(resp, nil); err != nil {
				return err
			}

			ctx.Formatter.PrintMessage("Activity deleted successfully")
			return nil
		},
	}
}
