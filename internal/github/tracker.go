package github

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/tracker"
	"github.com/steveyegge/beads/internal/types"
)

func init() {
	tracker.Register("github", func() tracker.IssueTracker {
		return &Tracker{}
	})
}

// Tracker implements tracker.IssueTracker for GitHub.
type Tracker struct {
	client *Client
	config *MappingConfig
	store  storage.Storage
}

func (t *Tracker) Name() string         { return "github" }
func (t *Tracker) DisplayName() string  { return "GitHub" }
func (t *Tracker) ConfigPrefix() string { return "github" }

func (t *Tracker) Init(ctx context.Context, store storage.Storage) error {
	t.store = store

	token, err := t.getConfig(ctx, "github.token", "GITHUB_TOKEN")
	if err != nil || token == "" {
		return fmt.Errorf("GitHub token not configured (set github.token or GITHUB_TOKEN)")
	}

	org, err := t.getConfig(ctx, "github.org", "GITHUB_ORG")
	if err != nil || org == "" {
		return fmt.Errorf("GitHub org not configured (set github.org or GITHUB_ORG)")
	}

	repo, err := t.getConfig(ctx, "github.repo", "GITHUB_REPO")
	if err != nil || repo == "" {
		return fmt.Errorf("GitHub repo not configured (set github.repo or GITHUB_REPO)")
	}

	t.client = NewClient(token, org, repo)
	t.config = DefaultMappingConfig()
	return nil
}

func (t *Tracker) Validate() error {
	if t.client == nil {
		return fmt.Errorf("GitHub tracker not initialized")
	}
	return nil
}

func (t *Tracker) Close() error { return nil }

func (t *Tracker) FetchIssues(ctx context.Context, opts tracker.FetchOptions) ([]tracker.TrackerIssue, error) {
	var issues []Issue
	var err error

	state := opts.State
	if state == "" {
		state = "all"
	}

	if opts.Since != nil {
		issues, err = t.client.FetchIssuesSince(ctx, state, *opts.Since)
	} else {
		issues, err = t.client.FetchIssues(ctx, state)
	}
	if err != nil {
		return nil, err
	}

	result := make([]tracker.TrackerIssue, 0, len(issues))
	for _, gh := range issues {
		result = append(result, githubToTrackerIssue(&gh))
	}
	return result, nil
}

func (t *Tracker) FetchIssue(ctx context.Context, identifier string) (*tracker.TrackerIssue, error) {
	number, err := strconv.Atoi(identifier)
	if err != nil {
		return nil, fmt.Errorf("invalid GitHub issue number %q: %w", identifier, err)
	}

	gh, err := t.client.FetchIssueByNumber(ctx, number)
	if err != nil {
		return nil, err
	}
	if gh == nil {
		return nil, nil
	}

	ti := githubToTrackerIssue(gh)
	return &ti, nil
}

func (t *Tracker) CreateIssue(ctx context.Context, issue *types.Issue) (*tracker.TrackerIssue, error) {
	fields := BeadsIssueToGitHubFields(issue, t.config)
	var labels []string
	if l, ok := fields["labels"].([]string); ok {
		labels = l
	}

	created, err := t.client.CreateIssue(ctx, issue.Title, issue.Description, labels)
	if err != nil {
		return nil, err
	}

	ti := githubToTrackerIssue(created)
	return &ti, nil
}

func (t *Tracker) UpdateIssue(ctx context.Context, externalID string, issue *types.Issue) (*tracker.TrackerIssue, error) {
	number, err := strconv.Atoi(externalID)
	if err != nil {
		return nil, fmt.Errorf("invalid GitHub issue number %q: %w", externalID, err)
	}

	updates := BeadsIssueToGitHubFields(issue, t.config)
	updated, err := t.client.UpdateIssue(ctx, number, updates)
	if err != nil {
		return nil, err
	}

	ti := githubToTrackerIssue(updated)
	return &ti, nil
}

func (t *Tracker) FieldMapper() tracker.FieldMapper {
	return &githubFieldMapper{config: t.config}
}

func (t *Tracker) IsExternalRef(ref string) bool {
	return strings.HasPrefix(ref, "gh-")
}

func (t *Tracker) ExtractIdentifier(ref string) string {
	if !strings.HasPrefix(ref, "gh-") {
		return ""
	}
	numStr := ref[3:]
	n, err := strconv.Atoi(numStr)
	if err != nil || n < 0 {
		return ""
	}
	return numStr
}

func (t *Tracker) BuildExternalRef(issue *tracker.TrackerIssue) string {
	return fmt.Sprintf("gh-%s", issue.Identifier)
}

// getConfig reads a config value from storage, falling back to env var.
func (t *Tracker) getConfig(ctx context.Context, key, envVar string) (string, error) {
	val, err := t.store.GetConfig(ctx, key)
	if err == nil && val != "" {
		return val, nil
	}
	if envVar != "" {
		if envVal := os.Getenv(envVar); envVal != "" {
			return envVal, nil
		}
	}
	return "", nil
}

// githubToTrackerIssue converts a github.Issue to a tracker.TrackerIssue.
func githubToTrackerIssue(gh *Issue) tracker.TrackerIssue {
	ti := tracker.TrackerIssue{
		ID:          strconv.Itoa(gh.ID),
		Identifier:  strconv.Itoa(gh.Number),
		URL:         gh.HTMLURL,
		Title:       gh.Title,
		Description: gh.Body,
		Labels:      LabelNames(gh.Labels),
		Raw:         gh,
	}

	if gh.State != "" {
		ti.State = gh.State
	}

	if gh.Assignee != nil {
		ti.Assignee = gh.Assignee.Login
		ti.AssigneeID = strconv.Itoa(gh.Assignee.ID)
	}

	if gh.CreatedAt != nil {
		ti.CreatedAt = *gh.CreatedAt
	}
	if gh.UpdatedAt != nil {
		ti.UpdatedAt = *gh.UpdatedAt
	}
	if gh.ClosedAt != nil {
		ti.CompletedAt = gh.ClosedAt
	}

	return ti
}
