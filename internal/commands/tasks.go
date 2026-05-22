package commands

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/9d4/groupfdn/internal/api"
	"github.com/spf13/cobra"
)

// TasksCmd returns the tasks command
func TasksCmd() *cobra.Command {
	ctx := NewCommandContext()

	tasksCmd := &cobra.Command{
		Use:     "tasks",
		Aliases: []string{"task", "t"},
		Short:   "Task management commands",
		Long:    "Manage tasks, assignments, and action items",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			format, _ := cmd.Flags().GetString("format")
			ctx.SetFormat(format)
		},
	}

	tasksCmd.AddCommand(tasksListCmd(ctx))
	tasksCmd.AddCommand(tasksGetCmd(ctx))
	tasksCmd.AddCommand(tasksCreateCmd(ctx))
	tasksCmd.AddCommand(tasksUpdateCmd(ctx))
	tasksCmd.AddCommand(tasksStatusCmd(ctx))
	tasksCmd.AddCommand(tasksDeleteCmd(ctx))
	tasksCmd.AddCommand(tasksActionCmd(ctx))

	return tasksCmd
}

func tasksListCmd(ctx *CommandContext) *cobra.Command {
	var page, limit int
	var projectID, status string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks",
		RunE: func(c *cobra.Command, args []string) error {
			if !ctx.Config.IsAuthenticated() {
				return errors.New("not authenticated. Please login first")
			}

			client := api.NewClient(ctx.Config)
			result, err := client.GetTasks(page, limit, projectID, status)
			if err != nil {
				return err
			}

			isJSON := ctx.Formatter.IsJSON()
			rows := make([]map[string]interface{}, 0, len(result.Tasks))
			for _, task := range result.Tasks {
				row := map[string]interface{}{
					"status":   task.Status,
					"priority": task.Priority,
				}

				if isJSON {
					row["id"] = task.ID
					row["title"] = task.Title
					row["project"] = taskProjectName(task)
					row["assignee"] = taskAssigneeName(task)
					row["estimatedHours"] = task.EstimatedHours
					row["actualHours"] = task.ActualHours
					row["updatedAt"] = task.UpdatedAt
				} else {
					row["id"] = truncateText(task.ID, 12)
					row["title"] = truncateText(task.Title, 30)
					row["project"] = truncateText(taskProjectName(task), 15)
					row["assignee"] = truncateText(taskAssigneeName(task), 20)
					row["estimatedHours"] = formatHoursOneDecimal(task.EstimatedHours)
					row["actualHours"] = formatHoursOneDecimal(task.ActualHours)
					row["updatedAt"] = relativeTime(task.UpdatedAt)
				}
				rows = append(rows, row)
			}

			headers := []string{"id", "title", "project", "status", "priority", "assignee", "estimatedHours", "actualHours", "updatedAt"}
			ctx.Formatter.Print(rows, headers)
			if !isJSON {
				fmt.Printf("Page: %d | Limit: %d | Total: %d\n", result.Page, result.Limit, result.Total)
			}
			return nil
		},
	}

	cmd.Flags().IntVarP(&page, "page", "p", 1, "Page number")
	cmd.Flags().IntVarP(&limit, "limit", "l", 20, "Items per page")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Filter by project ID")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status")

	return cmd
}

func tasksGetCmd(ctx *CommandContext) *cobra.Command {
	return &cobra.Command{
		Use:   "get [id]",
		Short: "Get task details",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if !ctx.Config.IsAuthenticated() {
				return errors.New("not authenticated. Please login first")
			}

			client := api.NewClient(ctx.Config)
			task, err := client.GetTask(args[0])
			if err != nil {
				return err
			}

			if ctx.Formatter.IsJSON() {
				ctx.Formatter.PrintMap(taskToMap(task))
				return nil
			}

			fmt.Printf("ID:          %s\n", task.ID)
			fmt.Printf("Title:       %s\n", task.Title)
			fmt.Printf("Description: %s\n", task.Description)
			fmt.Printf("Project:     %s\n", taskProjectName(*task))
			fmt.Printf("Status:      %s\n", task.Status)
			fmt.Printf("Priority:    %s\n", task.Priority)
			fmt.Printf("Assignees:   %s\n", taskAssigneeName(*task))
			fmt.Printf("Start Date:  %s\n", task.StartDate)
			fmt.Printf("Due Date:    %s\n", task.DueDate)
			fmt.Printf("Est. Hours:  %.1f\n", task.EstimatedHours)
			fmt.Printf("Act. Hours:  %.1f\n", task.ActualHours)
			fmt.Printf("Comments:    %d\n", task.CommentCount)
			fmt.Printf("Created:     %s\n", relativeTime(task.CreatedAt))
			fmt.Printf("Updated:     %s\n", relativeTime(task.UpdatedAt))

			if len(task.ActionItems) > 0 {
				fmt.Println("\nAction Items:")
				for _, ai := range task.ActionItems {
					doneMark := "[ ]"
					if ai.Done {
						doneMark = "[x]"
					}
					fmt.Printf("  %s %s\n", doneMark, ai.Title)
				}
			}

			return nil
		},
	}
}

func tasksCreateCmd(ctx *CommandContext) *cobra.Command {
	var title, description, priority, status, projectID, assignee, startDate, dueDate string
	var estimatedHours float64

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new task",
		RunE: func(c *cobra.Command, args []string) error {
			if !ctx.Config.IsAuthenticated() {
				return errors.New("not authenticated. Please login first")
			}

			// If title not provided via flag, open editor like git commit
			if !c.Flags().Changed("title") {
				content, err := openEditor(editorTemplate())
				if err != nil {
					return err
				}
				parsedTitle, parsedDesc, err := parseEditorContent(content)
				if err != nil {
					return err
				}
				title = parsedTitle
				description = parsedDesc
			}

			client := api.NewClient(ctx.Config)

			// Interactive project picker if not provided
			if projectID == "" {
				projects, err := client.GetProjects()
				if err != nil {
					return fmt.Errorf("failed to fetch projects: %w", err)
				}
				if len(projects) == 0 {
					return errors.New("no projects available")
				}

				items := make([]string, len(projects))
				for i, p := range projects {
					items[i] = fmt.Sprintf("%s (ID: %s)", p.Name, truncateText(p.ID, 12))
				}

				idx, err := selectFromList(items, "Select a project:")
				if err != nil {
					return err
				}
				projectID = projects[idx].ID
				fmt.Printf("Selected project: %s\n", projects[idx].Name)
			}

			// Interactive assignee picker if not provided
			if assignee == "" {
				usersResp, err := client.GetUsers(100)
				if err != nil {
					return fmt.Errorf("failed to fetch users: %w", err)
				}
				if len(usersResp.Users) == 0 {
					return errors.New("no users available")
				}

				items := make([]string, len(usersResp.Users))
				for i, u := range usersResp.Users {
					items[i] = fmt.Sprintf("%s <%s>", u.Name, u.Email)
				}

				idx, err := selectFromList(items, "Select an assignee:")
				if err != nil {
					return err
				}
				assignee = usersResp.Users[idx].ID
				fmt.Printf("Selected assignee: %s\n", usersResp.Users[idx].Name)
			}

			req := &api.CreateTaskRequest{
				Title:          title,
				Description:    description,
				Priority:       priority,
				Status:         status,
				ProjectID:      projectID,
				Assignees:      []string{assignee},
				AssignedTeam:   nil,
				StartDate:      startDate,
				DueDate:        dueDate,
				EstimatedHours: estimatedHours,
			}

			if status == "" {
				req.Status = "todo"
			}
			if priority == "" {
				req.Priority = "medium"
			}

			task, err := client.CreateTask(req)
			if err != nil {
				return err
			}

			ctx.Formatter.PrintMessage("Task created successfully")
			if ctx.Formatter.IsJSON() {
				ctx.Formatter.PrintMap(taskToMap(task))
			} else {
				fmt.Printf("ID:     %s\n", task.ID)
				fmt.Printf("Title:  %s\n", task.Title)
				fmt.Printf("Status: %s\n", task.Status)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Task title")
	cmd.Flags().StringVar(&description, "description", "", "Task description")
	cmd.Flags().StringVar(&priority, "priority", "", "Priority (low, medium, high)")
	cmd.Flags().StringVar(&status, "status", "", "Status (backlog, todo, in-progress, review, done, blocked)")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project ID")
	cmd.Flags().StringVar(&assignee, "assignee", "", "Assignee user ID")
	cmd.Flags().StringVar(&startDate, "start-date", "", "Start date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&dueDate, "due-date", "", "Due date (YYYY-MM-DD)")
	cmd.Flags().Float64Var(&estimatedHours, "estimated-hours", 0, "Estimated hours")

	return cmd
}

func tasksUpdateCmd(ctx *CommandContext) *cobra.Command {
	var title, description, priority, status, projectID, assignee, startDate, dueDate string
	var estimatedHours float64
	var removeAssignee bool

	cmd := &cobra.Command{
		Use:   "update [id]",
		Short: "Update a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if !ctx.Config.IsAuthenticated() {
				return errors.New("not authenticated. Please login first")
			}

			client := api.NewClient(ctx.Config)
			taskID := args[0]

			req := &api.UpdateTaskRequest{}
			if c.Flags().Changed("title") {
				req.Title = title
			}
			if c.Flags().Changed("description") {
				req.Description = description
			}
			if c.Flags().Changed("priority") {
				req.Priority = priority
			}
			if c.Flags().Changed("status") {
				req.Status = status
			}
			if c.Flags().Changed("project-id") {
				req.ProjectID = projectID
			}
			if c.Flags().Changed("assignee") {
				assignees := []string{assignee}
				req.Assignees = &assignees
			}
			if removeAssignee {
				assignees := []string{}
				req.Assignees = &assignees
			}
			if c.Flags().Changed("start-date") {
				req.StartDate = startDate
			}
			if c.Flags().Changed("due-date") {
				req.DueDate = dueDate
			}
			if c.Flags().Changed("estimated-hours") {
				req.EstimatedHours = estimatedHours
			}

			task, err := client.UpdateTask(taskID, req)
			if err != nil {
				return err
			}

			ctx.Formatter.PrintMessage("Task updated successfully")
			if ctx.Formatter.IsJSON() {
				ctx.Formatter.PrintMap(taskToMap(task))
			} else {
				fmt.Printf("ID:     %s\n", task.ID)
				fmt.Printf("Title:  %s\n", task.Title)
				fmt.Printf("Status: %s\n", task.Status)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Task title")
	cmd.Flags().StringVar(&description, "description", "", "Task description")
	cmd.Flags().StringVar(&priority, "priority", "", "Priority (low, medium, high)")
	cmd.Flags().StringVar(&status, "status", "", "Status (backlog, todo, in-progress, review, done, blocked)")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project ID")
	cmd.Flags().StringVar(&assignee, "assignee", "", "Assignee user ID")
	cmd.Flags().BoolVar(&removeAssignee, "remove-assignee", false, "Remove all assignees")
	cmd.Flags().StringVar(&startDate, "start-date", "", "Start date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&dueDate, "due-date", "", "Due date (YYYY-MM-DD)")
	cmd.Flags().Float64Var(&estimatedHours, "estimated-hours", 0, "Estimated hours")

	return cmd
}

func tasksStatusCmd(ctx *CommandContext) *cobra.Command {
	var status string

	cmd := &cobra.Command{
		Use:   "status [id]",
		Short: "Quickly update task status",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if !ctx.Config.IsAuthenticated() {
				return errors.New("not authenticated. Please login first")
			}

			client := api.NewClient(ctx.Config)
			task, err := client.UpdateTaskStatus(args[0], status)
			if err != nil {
				return err
			}

			ctx.Formatter.PrintMessage("Status updated successfully")
			if ctx.Formatter.IsJSON() {
				ctx.Formatter.PrintMap(taskToMap(task))
			} else {
				fmt.Printf("ID:     %s\n", task.ID)
				fmt.Printf("Title:  %s\n", task.Title)
				fmt.Printf("Status: %s\n", task.Status)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "New status (required)")
	cmd.MarkFlagRequired("status")

	return cmd
}

func tasksDeleteCmd(ctx *CommandContext) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete [id]",
		Short: "Delete a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if !ctx.Config.IsAuthenticated() {
				return errors.New("not authenticated. Please login first")
			}

			taskID := args[0]

			if !force {
				fmt.Printf("Are you sure you want to delete task %s? [y/N]: ", truncateText(taskID, 12))
				reader := bufio.NewReader(os.Stdin)
				input, err := reader.ReadString('\n')
				if err != nil {
					return err
				}
				input = strings.TrimSpace(strings.ToLower(input))
				if input != "y" && input != "yes" {
					fmt.Println("Cancelled")
					return nil
				}
			}

			client := api.NewClient(ctx.Config)
			if err := client.DeleteTask(taskID); err != nil {
				return err
			}

			ctx.Formatter.PrintMessage("Task deleted successfully")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")

	return cmd
}

func tasksActionCmd(ctx *CommandContext) *cobra.Command {
	actionCmd := &cobra.Command{
		Use:     "action",
		Aliases: []string{"actions", "act", "ai"},
		Short:   "Manage task action items",
		Long:    "List and toggle action items for a task",
	}

	actionCmd.AddCommand(tasksActionListCmd(ctx))
	actionCmd.AddCommand(tasksActionToggleCmd(ctx))

	return actionCmd
}

func tasksActionListCmd(ctx *CommandContext) *cobra.Command {
	return &cobra.Command{
		Use:   "list [task-id]",
		Short: "List action items for a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if !ctx.Config.IsAuthenticated() {
				return errors.New("not authenticated. Please login first")
			}

			client := api.NewClient(ctx.Config)
			task, err := client.GetTask(args[0])
			if err != nil {
				return err
			}

			if ctx.Formatter.IsJSON() {
				rows := make([]map[string]interface{}, len(task.ActionItems))
				for i, ai := range task.ActionItems {
					rows[i] = map[string]interface{}{
						"id":    ai.ID,
						"title": ai.Title,
						"done":  ai.Done,
					}
				}
				headers := []string{"id", "title", "done"}
				ctx.Formatter.Print(rows, headers)
				return nil
			}

			if len(task.ActionItems) == 0 {
				fmt.Println("No action items")
				return nil
			}

			fmt.Printf("Action items for: %s\n", task.Title)
			for _, ai := range task.ActionItems {
				doneMark := "[ ]"
				if ai.Done {
					doneMark = "[x]"
				}
				fmt.Printf("  %s %s\n", doneMark, ai.Title)
			}
			return nil
		},
	}
}

func tasksActionToggleCmd(ctx *CommandContext) *cobra.Command {
	return &cobra.Command{
		Use:   "toggle [task-id]",
		Short: "Toggle an action item status",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if !ctx.Config.IsAuthenticated() {
				return errors.New("not authenticated. Please login first")
			}

			client := api.NewClient(ctx.Config)
			taskID := args[0]

			task, err := client.GetTask(taskID)
			if err != nil {
				return err
			}

			if len(task.ActionItems) == 0 {
				return errors.New("no action items to toggle")
			}

			items := make([]string, len(task.ActionItems))
			for i, ai := range task.ActionItems {
				doneMark := "[ ]"
				if ai.Done {
					doneMark = "[x]"
				}
				items[i] = fmt.Sprintf("%s %s", doneMark, ai.Title)
			}

			idx, err := selectFromList(items, fmt.Sprintf("Select action item to toggle (task: %s):", task.Title))
			if err != nil {
				return err
			}

			// Toggle the selected action item
			task.ActionItems[idx].Done = !task.ActionItems[idx].Done

			// Build update body with action items
			actionItemsPayload := make([]map[string]interface{}, len(task.ActionItems))
			for i, ai := range task.ActionItems {
				actionItemsPayload[i] = map[string]interface{}{
					"_id":   ai.ID,
					"title": ai.Title,
					"done":  ai.Done,
				}
			}

			body := map[string]interface{}{
				"actionItems": actionItemsPayload,
			}

			resp, err := client.Put("/tasks/"+taskID, body)
			if err != nil {
				return err
			}

			var updatedTask api.Task
			if err := api.ParseResponse(resp, &updatedTask); err != nil {
				return err
			}

			ctx.Formatter.PrintMessage("Action item toggled successfully")
			if ctx.Formatter.IsJSON() {
				ctx.Formatter.PrintMap(taskToMap(&updatedTask))
			} else {
				doneMark := "[ ]"
				if updatedTask.ActionItems[idx].Done {
					doneMark = "[x]"
				}
				fmt.Printf("%s %s\n", doneMark, updatedTask.ActionItems[idx].Title)
			}
			return nil
		},
	}
}

// Helper functions

func taskProjectName(task api.Task) string {
	if task.ProjectID != nil {
		return task.ProjectID.Name
	}
	return "-"
}

func taskAssigneeName(task api.Task) string {
	if len(task.Assignees) > 0 {
		return task.Assignees[0].Name
	}
	if task.AssignedTo != nil {
		return task.AssignedTo.Name
	}
	return "-"
}

func taskToMap(task *api.Task) map[string]interface{} {
	return map[string]interface{}{
		"_id":            task.ID,
		"title":          task.Title,
		"description":    task.Description,
		"projectId":      task.ProjectID,
		"assignees":      task.Assignees,
		"assignedTo":     task.AssignedTo,
		"priority":       task.Priority,
		"status":         task.Status,
		"startDate":      task.StartDate,
		"dueDate":        task.DueDate,
		"estimatedHours": task.EstimatedHours,
		"actualHours":    task.ActualHours,
		"actionItems":    task.ActionItems,
		"commentCount":   task.CommentCount,
		"createdAt":      task.CreatedAt,
		"updatedAt":      task.UpdatedAt,
	}
}
