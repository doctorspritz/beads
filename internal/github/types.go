// Package github provides client and data types for the GitHub REST API.
//
// This package handles all interactions with GitHub's issue tracking system,
// including fetching, creating, and updating issues. It provides bidirectional
// mapping between GitHub's data model and Beads' internal types.
package github

import (
	"net/http"
	"strings"
	"time"

	"github.com/steveyegge/beads/internal/types"
)

// API configuration constants.
const (
	// DefaultAPIBase is the GitHub REST API base URL.
	DefaultAPIBase = "https://api.github.com"

	// DefaultTimeout is the default HTTP request timeout.
	DefaultTimeout = 30 * time.Second

	// MaxRetries is the maximum number of retries for rate-limited requests.
	MaxRetries = 3

	// RetryDelay is the base delay between retries (exponential backoff).
	RetryDelay = time.Second

	// MaxPageSize is the maximum number of issues to fetch per page.
	MaxPageSize = 100

	// MaxPages is the maximum number of pages to fetch before stopping.
	MaxPages = 1000
)

// Client provides methods to interact with the GitHub REST API.
type Client struct {
	Token      string       // GitHub personal access token
	Owner      string       // Repository owner (user or org)
	Repo       string       // Repository name
	BaseURL    string       // API base URL (defaults to DefaultAPIBase)
	HTTPClient *http.Client // Optional custom HTTP client
}

// Issue represents an issue from the GitHub API.
type Issue struct {
	ID        int        `json:"id"`     // Global issue ID
	Number    int        `json:"number"` // Repository-scoped issue number
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	State     string     `json:"state"` // "open" or "closed"
	Labels    []Label    `json:"labels"`
	Assignee  *User      `json:"assignee,omitempty"`
	Assignees []User     `json:"assignees,omitempty"`
	User      *User      `json:"user,omitempty"` // Author
	Milestone *Milestone `json:"milestone,omitempty"`
	HTMLURL   string     `json:"html_url"`
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`

	// PullRequest is non-nil if this "issue" is actually a pull request.
	// Used to filter PRs out of issue listings.
	PullRequest *PullRequestRef `json:"pull_request,omitempty"`
}

// PullRequestRef is a minimal reference to filter PRs from issue listings.
type PullRequestRef struct {
	URL string `json:"url,omitempty"`
}

// Label represents a GitHub label.
type Label struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description,omitempty"`
}

// User represents a GitHub user.
type User struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name,omitempty"`
	Email     string `json:"email,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	HTMLURL   string `json:"html_url,omitempty"`
}

// Milestone represents a GitHub milestone.
type Milestone struct {
	ID          int        `json:"id"`
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	State       string     `json:"state"` // "open" or "closed"
	DueOn       *time.Time `json:"due_on,omitempty"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
	HTMLURL     string     `json:"html_url,omitempty"`
}

// PriorityMapping maps priority label values to beads priority (0-4).
var PriorityMapping = map[string]int{
	"critical": 0,
	"high":     1,
	"medium":   2,
	"low":      3,
	"none":     4,
}

// StatusMapping maps status label values to beads status strings.
var StatusMapping = map[string]string{
	"open":        "open",
	"in_progress": "in_progress",
	"blocked":     "blocked",
	"deferred":    "deferred",
	"closed":      "closed",
}

// typeMapping maps type label values to beads issue type strings.
var typeMapping = map[string]string{
	"bug":         "bug",
	"feature":     "feature",
	"task":        "task",
	"epic":        "epic",
	"chore":       "chore",
	"enhancement": "feature",
}

// getPriorityFromLabel returns the beads priority for a priority label value.
// Returns -1 if the value is not recognized.
func getPriorityFromLabel(value string) int {
	if p, ok := PriorityMapping[strings.ToLower(value)]; ok {
		return p
	}
	return -1
}

// getStatusFromLabel returns the beads status for a status label value.
func getStatusFromLabel(value string) string {
	return StatusMapping[strings.ToLower(value)]
}

// getTypeFromLabel returns the beads issue type for a type label value.
func getTypeFromLabel(value string) string {
	return typeMapping[strings.ToLower(value)]
}

// LabelNames extracts label name strings from a slice of Label.
func LabelNames(labels []Label) []string {
	names := make([]string, 0, len(labels))
	for _, l := range labels {
		names = append(names, l.Name)
	}
	return names
}

// parseLabelPrefix splits a label into prefix and value.
// Labels like "priority::high" are split into ("priority", "high").
// Labels without "::" return empty prefix and the original label as value.
func parseLabelPrefix(label string) (prefix, value string) {
	parts := strings.SplitN(label, "::", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", label
}

// validStates for GitHub issues.
var validStates = map[string]bool{
	"open":   true,
	"closed": true,
}

// SyncStats tracks statistics for a GitHub sync operation.
type SyncStats struct {
	Pulled    int `json:"pulled"`
	Pushed    int `json:"pushed"`
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Skipped   int `json:"skipped"`
	Errors    int `json:"errors"`
	Conflicts int `json:"conflicts"`
}

// priorityToLabel converts beads priority (0-4) to a label value.
func priorityToLabel(priority int) string {
	switch priority {
	case 0:
		return "critical"
	case 1:
		return "high"
	case 2:
		return "medium"
	case 3:
		return "low"
	case 4:
		return "none"
	default:
		return "medium"
	}
}

// statusFromLabelsAndState determines beads status from GitHub labels and state.
func statusFromLabelsAndState(labels []string, state string) string {
	if state == "closed" {
		return "closed"
	}

	for _, label := range labels {
		prefix, value := parseLabelPrefix(label)
		if prefix == "status" {
			normalized := strings.ToLower(value)
			if _, ok := StatusMapping[normalized]; ok {
				return normalized
			}
		}
	}

	return "open"
}

// typeFromLabels extracts issue type from labels.
func typeFromLabels(labels []string) string {
	for _, label := range labels {
		prefix, value := parseLabelPrefix(label)
		if prefix == "type" {
			if t := getTypeFromLabel(value); t != "" {
				return t
			}
		}
		if prefix == "" {
			if t := getTypeFromLabel(value); t != "" {
				return t
			}
		}
	}
	return "task"
}

// priorityFromLabels extracts priority from labels.
func priorityFromLabels(labels []string) int {
	for _, label := range labels {
		prefix, value := parseLabelPrefix(label)
		if prefix == "priority" {
			if p := getPriorityFromLabel(value); p >= 0 {
				return p
			}
		}
	}
	return 2 // Default: medium

}

// filterNonScopedLabels returns only labels without scoped prefixes.
func filterNonScopedLabels(labels []string) []string {
	var filtered []string
	for _, label := range labels {
		prefix, _ := parseLabelPrefix(label)
		if prefix == "priority" || prefix == "status" || prefix == "type" {
			continue
		}
		filtered = append(filtered, label)
	}
	return filtered
}

// IsIssue returns true if the GitHub Issue is an actual issue (not a PR).
func (i *Issue) IsIssue() bool {
	return i.PullRequest == nil
}

// StateForBeads maps GitHub state to beads-compatible state.
func StateForBeads(state string) types.Status {
	switch strings.ToLower(state) {
	case "closed":
		return types.StatusClosed
	default:
		return types.StatusOpen
	}
}
