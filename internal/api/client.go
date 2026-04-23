package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/9d4/groupfdn/internal/config"
)

const (
	BaseURL        = "https://group.fdn.my.id/api"
	RequestTimeout = 15 * time.Second
)

// Client handles HTTP requests with authentication
type Client struct {
	httpClient *http.Client
	config     *config.Config
	debug      bool
}

// NewClient creates a new API client
func NewClient(cfg *config.Config) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: RequestTimeout,
		},
		config: cfg,
		debug:  isDebugEnabled(),
	}
}

func isDebugEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("DEBUG")))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// doRequest makes an HTTP request with optional authentication
func (c *Client) doRequest(method, url string, body interface{}, skipAuth bool) (*http.Response, error) {
	var bodyReader io.Reader
	var jsonBody []byte
	if body != nil {
		var err error
		jsonBody, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add auth header if needed
	if !skipAuth && c.config.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.AccessToken)
	}

	if c.debug {
		c.logRequest(req, jsonBody)
	}

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	duration := time.Since(start)
	if err != nil {
		if c.debug {
			fmt.Fprintf(os.Stderr, "[DEBUG] <- error %s %s (%s): %v\n", method, url, duration, err)
		}
		return nil, err
	}

	if c.debug {
		c.logResponse(method, url, resp, duration)
	}

	return resp, nil
}

// Request makes an authenticated HTTP request with auto-refresh on 401
func (c *Client) Request(method, endpoint string, body interface{}) (*http.Response, error) {
	url := BaseURL + endpoint
	skipAuth := c.shouldSkipAuth(endpoint)

	resp, err := c.doRequest(method, url, body, skipAuth)
	if err != nil {
		return nil, err
	}

	// Handle 401 - try to refresh token and retry
	if resp.StatusCode == http.StatusUnauthorized && !skipAuth {
		if c.debug {
			fmt.Fprintf(os.Stderr, "[DEBUG] 401 received for %s %s, attempting refresh\n", method, url)
		}
		resp.Body.Close()

		if err := c.refreshToken(); err != nil {
			if c.debug {
				fmt.Fprintf(os.Stderr, "[DEBUG] refresh failed: %v\n", err)
			}
			return nil, fmt.Errorf("session expired, please login again")
		}
		if c.debug {
			fmt.Fprintf(os.Stderr, "[DEBUG] refresh successful, retrying %s %s\n", method, url)
		}

		// Retry the original request with new token
		resp, err = c.doRequest(method, url, body, false)
		if err != nil {
			return nil, err
		}
	}

	return resp, nil
}

// Get makes a GET request
func (c *Client) Get(endpoint string) (*http.Response, error) {
	return c.Request(http.MethodGet, endpoint, nil)
}

// Post makes a POST request
func (c *Client) Post(endpoint string, body interface{}) (*http.Response, error) {
	return c.Request(http.MethodPost, endpoint, body)
}

// Put makes a PUT request
func (c *Client) Put(endpoint string, body interface{}) (*http.Response, error) {
	return c.Request(http.MethodPut, endpoint, body)
}

// Patch makes a PATCH request
func (c *Client) Patch(endpoint string, body interface{}) (*http.Response, error) {
	return c.Request(http.MethodPatch, endpoint, body)
}

// Delete makes a DELETE request
func (c *Client) Delete(endpoint string) (*http.Response, error) {
	return c.Request(http.MethodDelete, endpoint, nil)
}

// shouldSkipAuth checks if endpoint should skip auth
func (c *Client) shouldSkipAuth(endpoint string) bool {
	skipEndpoints := []string{
		"/auth/login",
		"/auth/refresh",
		"/auth/otp/send-otp",
		"/auth/otp/verify-otp",
	}

	for _, skip := range skipEndpoints {
		if endpoint == skip {
			return true
		}
	}
	return false
}

// refreshToken attempts to refresh the access token
func (c *Client) refreshToken() error {
	if c.config.RefreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	body := map[string]string{
		"refreshToken": c.config.RefreshToken,
	}

	resp, err := c.doRequest(http.MethodPost, BaseURL+"/auth/refresh", body, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Clear tokens on failed refresh
		c.config.Clear()
		return fmt.Errorf("refresh failed")
	}

	var result struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode refresh response: %w", err)
	}

	c.config.AccessToken = result.AccessToken
	if result.RefreshToken != "" {
		c.config.RefreshToken = result.RefreshToken
	}

	return c.config.Save()
}

// ParseResponse parses JSON response into the target
func ParseResponse(resp *http.Response, target interface{}) error {
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errResp struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil {
			if errResp.Message != "" {
				return fmt.Errorf("%s", errResp.Message)
			}
			if errResp.Error != "" {
				return fmt.Errorf("%s", errResp.Error)
			}
		}

		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	if target == nil {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

// LoginResponse represents auth response
type LoginResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	User         struct {
		ID    string `json:"_id"`
		Email string `json:"email"`
		Name  string `json:"name"`
		Role  string `json:"role"`
	} `json:"user"`
}

// ProjectsStats represents /projects/stats response.
type ProjectsStats struct {
	Total        int `json:"total"`
	Ended        int `json:"ended"`
	Running      int `json:"running"`
	Pending      int `json:"pending"`
	Done         int `json:"done"`
	CommentCount int `json:"commentCount"`
}

// Login performs authentication
func (c *Client) Login(email, password string) (*LoginResponse, error) {
	body := map[string]string{
		"email":    email,
		"password": password,
	}

	resp, err := c.Post("/auth/login", body)
	if err != nil {
		return nil, err
	}

	var result LoginResponse
	if err := ParseResponse(resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// Logout performs logout
func (c *Client) Logout() error {
	resp, err := c.Post("/auth/logout", nil)
	if err != nil {
		return err
	}
	return ParseResponse(resp, nil)
}

// GetProfile fetches user profile
func (c *Client) GetProfile() (map[string]interface{}, error) {
	resp, err := c.Get("/auth/me")
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := ParseResponse(resp, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// SendOTP requests OTP to be sent to email
func (c *Client) SendOTP(email string) error {
	body := map[string]string{
		"email": email,
	}

	resp, err := c.Post("/auth/otp/send-otp", body)
	if err != nil {
		return err
	}

	return ParseResponse(resp, nil)
}

// VerifyOTP verifies OTP and returns tokens
func (c *Client) VerifyOTP(email, otp string) (*LoginResponse, error) {
	body := map[string]string{
		"email": email,
		"otp":   otp,
	}

	resp, err := c.Post("/auth/otp/verify-otp", body)
	if err != nil {
		return nil, err
	}

	var result LoginResponse
	if err := ParseResponse(resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetProjectsStats fetches project stats for the current user.
func (c *Client) GetProjectsStats() (*ProjectsStats, error) {
	resp, err := c.Get("/projects/stats")
	if err != nil {
		return nil, err
	}

	var result ProjectsStats
	if err := ParseResponse(resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// Project represents a project from the API
type Project struct {
	ID          string `json:"_id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Description string `json:"description"`
}

// GetProjects fetches list of projects
func (c *Client) GetProjects() ([]Project, error) {
	resp, err := c.Get("/projects?limit=100")
	if err != nil {
		return nil, err
	}

	var result struct {
		Projects []Project `json:"projects"`
		Total    int       `json:"total"`
	}
	if err := ParseResponse(resp, &result); err != nil {
		return nil, err
	}

	return result.Projects, nil
}

func (c *Client) logRequest(req *http.Request, body []byte) {
	headers := sanitizeHeaders(req.Header)
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Fprintf(os.Stderr, "[DEBUG] -> %s %s\n", req.Method, req.URL.String())
	for _, k := range keys {
		fmt.Fprintf(os.Stderr, "[DEBUG]    %s: %s\n", k, strings.Join(headers[k], ","))
	}
	if len(body) == 0 {
		fmt.Fprintln(os.Stderr, "[DEBUG]    body: (empty)")
		return
	}
	fmt.Fprintf(os.Stderr, "[DEBUG]    body: %s\n", truncate(sanitizeJSONBytes(body), 2048))
}

func (c *Client) logResponse(method, url string, resp *http.Response, duration time.Duration) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[DEBUG] <- %d %s %s (%s), failed to read body: %v\n", resp.StatusCode, method, url, duration, err)
		return
	}
	resp.Body = io.NopCloser(bytes.NewBuffer(body))

	fmt.Fprintf(os.Stderr, "[DEBUG] <- %d %s %s (%s)\n", resp.StatusCode, method, url, duration)
	if len(body) == 0 {
		fmt.Fprintln(os.Stderr, "[DEBUG]    body: (empty)")
		return
	}
	fmt.Fprintf(os.Stderr, "[DEBUG]    body: %s\n", truncate(sanitizeJSONBytes(body), 2048))
}

func sanitizeHeaders(h http.Header) http.Header {
	cloned := make(http.Header, len(h))
	for k, vals := range h {
		lower := strings.ToLower(k)
		if lower == "authorization" {
			cloned[k] = []string{"[REDACTED]"}
			continue
		}
		copied := make([]string, len(vals))
		copy(copied, vals)
		cloned[k] = copied
	}
	return cloned
}

func sanitizeJSONBytes(b []byte) string {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return string(b)
	}
	v = redactSensitive(v)
	clean, err := json.Marshal(v)
	if err != nil {
		return string(b)
	}
	return string(clean)
}

func redactSensitive(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, val := range t {
			if isSensitiveKey(k) {
				t[k] = "[REDACTED]"
				continue
			}
			t[k] = redactSensitive(val)
		}
		return t
	case []interface{}:
		for i, val := range t {
			t[i] = redactSensitive(val)
		}
		return t
	default:
		return v
	}
}

func isSensitiveKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	switch k {
	case "authorization", "accesstoken", "refreshtoken", "password", "otp":
		return true
	default:
		return false
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...<truncated>"
}
