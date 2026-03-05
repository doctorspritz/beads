package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// NewClient creates a new GitHub client with the given token, owner, and repo.
func NewClient(token, owner, repo string) *Client {
	return &Client{
		Token:   token,
		Owner:   owner,
		Repo:    repo,
		BaseURL: DefaultAPIBase,
		HTTPClient: &http.Client{
			Timeout: DefaultTimeout,
		},
	}
}

// WithHTTPClient returns a new client configured to use the specified HTTP client.
func (c *Client) WithHTTPClient(httpClient *http.Client) *Client {
	return &Client{
		Token:      c.Token,
		Owner:      c.Owner,
		Repo:       c.Repo,
		BaseURL:    c.BaseURL,
		HTTPClient: httpClient,
	}
}

// WithBaseURL returns a new client configured to use a custom API base URL.
func (c *Client) WithBaseURL(baseURL string) *Client {
	return &Client{
		Token:      c.Token,
		Owner:      c.Owner,
		Repo:       c.Repo,
		BaseURL:    baseURL,
		HTTPClient: c.HTTPClient,
	}
}

// repoPath returns the "owner/repo" path for API calls.
func (c *Client) repoPath() string {
	return c.Owner + "/" + c.Repo
}

// buildURL constructs a full API URL from path and optional query parameters.
func (c *Client) buildURL(path string, params map[string]string) string {
	u := c.BaseURL + path

	if len(params) > 0 {
		values := url.Values{}
		for k, v := range params {
			values.Set(k, v)
		}
		u += "?" + values.Encode()
	}

	return u
}

// doRequest performs an HTTP request with authentication and retry logic.
func (c *Client) doRequest(ctx context.Context, method, urlStr string, body interface{}) ([]byte, http.Header, error) {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	var lastErr error
	for attempt := 0; attempt <= MaxRetries; attempt++ {
		var reqBody io.Reader
		if bodyBytes != nil {
			reqBody = bytes.NewReader(bodyBytes)
		}

		req, err := http.NewRequestWithContext(ctx, method, urlStr, reqBody)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed (attempt %d/%d): %w", attempt+1, MaxRetries+1, err)
			continue
		}

		const maxResponseSize = 50 * 1024 * 1024
		respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		_ = resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("failed to read response (attempt %d/%d): %w", attempt+1, MaxRetries+1, err)
			continue
		}

		// Handle rate limiting
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			// Check for rate limit headers
			if resetStr := resp.Header.Get("X-RateLimit-Reset"); resetStr != "" {
				if resetUnix, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
					waitDuration := time.Until(time.Unix(resetUnix, 0))
					if waitDuration > 0 && waitDuration < 2*time.Minute {
						select {
						case <-ctx.Done():
							return nil, nil, ctx.Err()
						case <-time.After(waitDuration):
							continue
						}
					}
				}
			}

			delay := RetryDelay * time.Duration(1<<attempt)
			lastErr = fmt.Errorf("rate limited (attempt %d/%d)", attempt+1, MaxRetries+1)
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(delay):
				continue
			}
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, nil, fmt.Errorf("API error: %s (status %d)", string(respBody), resp.StatusCode)
		}

		return respBody, resp.Header, nil
	}

	return nil, nil, fmt.Errorf("max retries (%d) exceeded: %w", MaxRetries+1, lastErr)
}

// FetchIssues retrieves issues from GitHub with optional filtering by state.
// state can be: "open", "closed", or "all".
func (c *Client) FetchIssues(ctx context.Context, state string) ([]Issue, error) {
	var allIssues []Issue
	page := 1

	for {
		select {
		case <-ctx.Done():
			return allIssues, ctx.Err()
		default:
		}

		params := map[string]string{
			"per_page": strconv.Itoa(MaxPageSize),
			"page":     strconv.Itoa(page),
		}
		if state != "" && state != "all" {
			params["state"] = state
		} else {
			params["state"] = "all"
		}

		urlStr := c.buildURL("/repos/"+c.repoPath()+"/issues", params)
		respBody, _, err := c.doRequest(ctx, http.MethodGet, urlStr, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch issues: %w", err)
		}

		var issues []Issue
		if err := json.Unmarshal(respBody, &issues); err != nil {
			return nil, fmt.Errorf("failed to parse issues response: %w", err)
		}

		// Filter out pull requests (GitHub returns PRs in issue listings)
		for _, issue := range issues {
			if issue.IsIssue() {
				allIssues = append(allIssues, issue)
			}
		}

		// No more results
		if len(issues) < MaxPageSize {
			break
		}

		page++
		if page > MaxPages {
			return nil, fmt.Errorf("pagination limit exceeded: stopped after %d pages", MaxPages)
		}
	}

	return allIssues, nil
}

// FetchIssuesSince retrieves issues that have been updated since the given time.
func (c *Client) FetchIssuesSince(ctx context.Context, state string, since time.Time) ([]Issue, error) {
	var allIssues []Issue
	page := 1

	sinceStr := since.UTC().Format(time.RFC3339)

	for {
		select {
		case <-ctx.Done():
			return allIssues, ctx.Err()
		default:
		}

		params := map[string]string{
			"per_page": strconv.Itoa(MaxPageSize),
			"page":     strconv.Itoa(page),
			"since":    sinceStr,
		}
		if state != "" && state != "all" {
			params["state"] = state
		} else {
			params["state"] = "all"
		}

		urlStr := c.buildURL("/repos/"+c.repoPath()+"/issues", params)
		respBody, _, err := c.doRequest(ctx, http.MethodGet, urlStr, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch issues since %s: %w", sinceStr, err)
		}

		var issues []Issue
		if err := json.Unmarshal(respBody, &issues); err != nil {
			return nil, fmt.Errorf("failed to parse issues response: %w", err)
		}

		for _, issue := range issues {
			if issue.IsIssue() {
				allIssues = append(allIssues, issue)
			}
		}

		if len(issues) < MaxPageSize {
			break
		}

		page++
		if page > MaxPages {
			return nil, fmt.Errorf("pagination limit exceeded: stopped after %d pages", MaxPages)
		}
	}

	return allIssues, nil
}

// FetchIssueByNumber retrieves a single issue by its number.
func (c *Client) FetchIssueByNumber(ctx context.Context, number int) (*Issue, error) {
	urlStr := c.buildURL("/repos/"+c.repoPath()+"/issues/"+strconv.Itoa(number), nil)
	respBody, _, err := c.doRequest(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch issue #%d: %w", number, err)
	}

	var issue Issue
	if err := json.Unmarshal(respBody, &issue); err != nil {
		return nil, fmt.Errorf("failed to parse issue response: %w", err)
	}

	return &issue, nil
}

// CreateIssue creates a new issue in the repository.
func (c *Client) CreateIssue(ctx context.Context, title, body string, labels []string) (*Issue, error) {
	reqBody := map[string]interface{}{
		"title": title,
		"body":  body,
	}
	if len(labels) > 0 {
		reqBody["labels"] = labels
	}

	urlStr := c.buildURL("/repos/"+c.repoPath()+"/issues", nil)
	respBody, _, err := c.doRequest(ctx, http.MethodPost, urlStr, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create issue: %w", err)
	}

	var issue Issue
	if err := json.Unmarshal(respBody, &issue); err != nil {
		return nil, fmt.Errorf("failed to parse create response: %w", err)
	}

	return &issue, nil
}

// UpdateIssue updates an existing issue in the repository.
func (c *Client) UpdateIssue(ctx context.Context, number int, updates map[string]interface{}) (*Issue, error) {
	urlStr := c.buildURL("/repos/"+c.repoPath()+"/issues/"+strconv.Itoa(number), nil)
	respBody, _, err := c.doRequest(ctx, http.MethodPatch, urlStr, updates)
	if err != nil {
		return nil, fmt.Errorf("failed to update issue #%d: %w", number, err)
	}

	var issue Issue
	if err := json.Unmarshal(respBody, &issue); err != nil {
		return nil, fmt.Errorf("failed to parse update response: %w", err)
	}

	return &issue, nil
}

// AddComment adds a comment to an issue.
func (c *Client) AddComment(ctx context.Context, number int, body string) error {
	reqBody := map[string]interface{}{
		"body": body,
	}
	urlStr := c.buildURL("/repos/"+c.repoPath()+"/issues/"+strconv.Itoa(number)+"/comments", nil)
	_, _, err := c.doRequest(ctx, http.MethodPost, urlStr, reqBody)
	if err != nil {
		return fmt.Errorf("failed to comment on issue #%d: %w", number, err)
	}
	return nil
}
