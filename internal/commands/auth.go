package commands

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/9d4/groupfdn/internal/api"
	"github.com/9d4/groupfdn/internal/config"
	"github.com/9d4/groupfdn/internal/output"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// CommandContext holds shared dependencies
type CommandContext struct {
	Config    *config.Config
	Formatter *output.Formatter
}

// NewCommandContext creates a context from globals
func NewCommandContext() *CommandContext {
	cfg, _ := config.Load()
	return &CommandContext{
		Config:    cfg,
		Formatter: output.NewFormatter(output.Format("table")),
	}
}

// SetFormat sets the output format
func (ctx *CommandContext) SetFormat(format string) {
	ctx.Formatter = output.NewFormatter(output.Format(format))
}

// AuthCmd returns the auth command
func AuthCmd() *cobra.Command {
	ctx := NewCommandContext()

	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication commands",
		Long:  "Manage authentication and user profile",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			format, _ := cmd.Flags().GetString("format")
			ctx.SetFormat(format)
		},
	}

	authCmd.AddCommand(authLoginCmd(ctx))
	authCmd.AddCommand(authLogoutCmd(ctx))
	authCmd.AddCommand(authProfileCmd(ctx))
	authCmd.AddCommand(authRefreshCmd(ctx))

	return authCmd
}

func authLoginCmd(ctx *CommandContext) *cobra.Command {
	var email, password string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Login with email and password",
		RunE: func(c *cobra.Command, args []string) error {
			reader := bufio.NewReader(os.Stdin)

			// Interactive prompts if flags not provided
			if email == "" {
				fmt.Print("Email: ")
				var err error
				email, err = reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("failed to read email: %w", err)
				}
				email = strings.TrimSpace(email)
				if email == "" {
					return errors.New("email is required")
				}
			}

			if password == "" {
				// Check if we're running in a terminal
				if term.IsTerminal(int(os.Stdin.Fd())) {
					fmt.Print("Password: ")
					bytePassword, err := term.ReadPassword(int(os.Stdin.Fd()))
					if err != nil {
						return fmt.Errorf("failed to read password: %w", err)
					}
					password = string(bytePassword)
					fmt.Println() // New line after password input
				} else {
					// Not a terminal, read normally (for piping)
					fmt.Print("Password: ")
					var err error
					password, err = reader.ReadString('\n')
					if err != nil {
						return fmt.Errorf("failed to read password: %w", err)
					}
					password = strings.TrimSpace(password)
				}
				if password == "" {
					return errors.New("password is required")
				}
			}

			client := api.NewClient(ctx.Config)

			// Try login first (it will indicate OTP is required)
			_, err := client.Login(email, password)
			if err != nil {
				// Check if error is about OTP required
				errStr := err.Error()
				if !strings.Contains(errStr, "OTP") && !strings.Contains(errStr, "otp") {
					return err
				}
				// OTP is required, continue with OTP flow
			}

			// Send OTP request
			fmt.Println("Requesting OTP...")
			if err := client.SendOTP(email); err != nil {
				return fmt.Errorf("failed to send OTP: %w", err)
			}
			fmt.Println("OTP code has been sent to your email")

			// OTP verification loop with retry
			maxRetries := 3
			for attempt := 1; attempt <= maxRetries; attempt++ {
				fmt.Print("OTP code: ")
				otp, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("failed to read OTP: %w", err)
				}
				otp = strings.TrimSpace(otp)

				if otp == "" {
					fmt.Println("OTP cannot be empty")
					continue
				}

				// Verify OTP
				resp, err := client.VerifyOTP(email, otp)
				if err != nil {
					// Check for rate limiting
					if strings.Contains(err.Error(), "Terlalu banyak") || strings.Contains(err.Error(), "429") {
						return fmt.Errorf("rate limited: %w", err)
					}

					if attempt < maxRetries {
						fmt.Printf("Invalid OTP. Try again (%d/%d attempts remaining)\n", maxRetries-attempt, maxRetries)
						continue
					}
					return errors.New("invalid OTP. Maximum attempts exceeded")
				}

				// Success! Save tokens
				ctx.Config.AccessToken = resp.AccessToken
				ctx.Config.RefreshToken = resp.RefreshToken
				ctx.Config.Email = resp.User.Email

				if err := ctx.Config.Save(); err != nil {
					return err
				}

				ctx.Formatter.PrintMessage("Login successful")
				return nil
			}

			return errors.New("login failed after maximum attempts")
		},
	}

	cmd.Flags().StringVarP(&email, "email", "e", "", "Email address (optional, will prompt if not provided)")
	cmd.Flags().StringVarP(&password, "password", "p", "", "Password (optional, will prompt if not provided)")

	return cmd
}

func authLogoutCmd(ctx *CommandContext) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Logout and clear stored credentials",
		RunE: func(c *cobra.Command, args []string) error {
			client := api.NewClient(ctx.Config)

			// Try to logout on server (ignore errors)
			_ = client.Logout()

			// Clear local config
			if err := ctx.Config.Clear(); err != nil {
				return err
			}

			ctx.Formatter.PrintMessage("Logout successful")
			return nil
		},
	}
}

func authProfileCmd(ctx *CommandContext) *cobra.Command {
	return &cobra.Command{
		Use:   "profile",
		Short: "Get current user profile",
		RunE: func(c *cobra.Command, args []string) error {
			if !ctx.Config.IsAuthenticated() {
				return errors.New("not authenticated. Please login first")
			}

			client := api.NewClient(ctx.Config)
			claims, err := decodeJWTClaims(ctx.Config.AccessToken)
			if err != nil {
				return errors.New("invalid access token. Please login again")
			}

			stats, err := client.GetProjectsStats()
			if err != nil {
				return err
			}

			profile := map[string]interface{}{
				"email":  ctx.Config.Email,
				"claims": claims,
				"projectsStats": map[string]interface{}{
					"total":        stats.Total,
					"ended":        stats.Ended,
					"running":      stats.Running,
					"pending":      stats.Pending,
					"done":         stats.Done,
					"commentCount": stats.CommentCount,
				},
			}

			ctx.Formatter.PrintMap(profile)
			return nil
		},
	}
}

func authRefreshCmd(ctx *CommandContext) *cobra.Command {
	return &cobra.Command{
		Use:   "refresh",
		Short: "Manually refresh access token",
		RunE: func(c *cobra.Command, args []string) error {
			if !ctx.Config.IsAuthenticated() {
				return errors.New("not authenticated. Please login first")
			}

			client := api.NewClient(ctx.Config)

			// Trigger an authenticated request; the client refreshes on 401.
			_, err := client.GetProjectsStats()
			if err != nil {
				return err
			}

			ctx.Formatter.PrintMessage("Token refreshed successfully")
			return nil
		},
	}
}

func decodeJWTClaims(token string) (map[string]interface{}, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}

	if expRaw, ok := claims["exp"]; ok {
		expUnix, ok := numericToInt64(expRaw)
		if ok {
			claims["expUnix"] = expUnix
			claims["expTime"] = time.Unix(expUnix, 0).UTC().Format(time.RFC3339)
		}
	}

	return claims, nil
}

func numericToInt64(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case float32:
		return int64(n), true
	case int:
		return int64(n), true
	case int64:
		return n, true
	case json.Number:
		i, err := n.Int64()
		if err == nil {
			return i, true
		}
	case string:
		i, err := strconv.ParseInt(n, 10, 64)
		if err == nil {
			return i, true
		}
	}
	return 0, false
}
