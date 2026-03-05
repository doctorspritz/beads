package github

import (
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/types"
)

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestGitHubIssueToBeads(t *testing.T) {
	now := mustParseTime("2024-01-15T10:00:00Z")
	config := DefaultMappingConfig()

	gh := &Issue{
		ID:     100,
		Number: 7,
		Title:  "Add feature X",
		Body:   "We need feature X for reasons",
		State:  "open",
		Labels: []Label{
			{Name: "type::feature"},
			{Name: "priority::high"},
			{Name: "chum-canary"},
		},
		Assignee:  &User{Login: "alice"},
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	issue := GitHubIssueToBeads(gh, config)
	if issue == nil {
		t.Fatal("GitHubIssueToBeads returned nil")
	}

	if issue.Title != "Add feature X" {
		t.Errorf("Title = %q, want %q", issue.Title, "Add feature X")
	}
	if issue.IssueType != types.IssueType("feature") {
		t.Errorf("IssueType = %q, want %q", issue.IssueType, "feature")
	}
	if issue.Priority != 1 {
		t.Errorf("Priority = %d, want %d (high)", issue.Priority, 1)
	}
	if issue.Status != types.StatusOpen {
		t.Errorf("Status = %q, want %q", issue.Status, types.StatusOpen)
	}
	if issue.Assignee != "alice" {
		t.Errorf("Assignee = %q, want %q", issue.Assignee, "alice")
	}
	if issue.ExternalRef == nil || *issue.ExternalRef != "gh-7" {
		t.Errorf("ExternalRef = %v, want %q", issue.ExternalRef, "gh-7")
	}
	// chum-canary should remain in labels (non-scoped)
	if len(issue.Labels) != 1 || issue.Labels[0] != "chum-canary" {
		t.Errorf("Labels = %v, want [chum-canary]", issue.Labels)
	}
}

func TestGitHubIssueToBeads_ClosedState(t *testing.T) {
	now := mustParseTime("2024-01-15T10:00:00Z")
	config := DefaultMappingConfig()

	gh := &Issue{
		Number:    1,
		Title:     "Done task",
		State:     "closed",
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	issue := GitHubIssueToBeads(gh, config)
	if issue.Status != types.StatusClosed {
		t.Errorf("Status = %q, want %q", issue.Status, types.StatusClosed)
	}
}

func TestGitHubIssueToBeads_StatusLabel(t *testing.T) {
	now := mustParseTime("2024-01-15T10:00:00Z")
	config := DefaultMappingConfig()

	gh := &Issue{
		Number: 1,
		Title:  "Blocked task",
		State:  "open",
		Labels: []Label{
			{Name: "status::blocked"},
		},
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	issue := GitHubIssueToBeads(gh, config)
	if issue.Status != types.Status("blocked") {
		t.Errorf("Status = %q, want %q", issue.Status, "blocked")
	}
}

func TestBeadsIssueToGitHubFields(t *testing.T) {
	config := DefaultMappingConfig()

	issue := &types.Issue{
		Title:       "Fix bug",
		Description: "The bug is bad",
		IssueType:   types.TypeTask,
		Priority:    0, // critical
		Status:      types.StatusClosed,
		Labels:      []string{"chum-canary"},
	}

	fields := BeadsIssueToGitHubFields(issue, config)

	if fields["title"] != "Fix bug" {
		t.Errorf("title = %q, want %q", fields["title"], "Fix bug")
	}
	if fields["state"] != "closed" {
		t.Errorf("state = %q, want %q", fields["state"], "closed")
	}

	labels, ok := fields["labels"].([]string)
	if !ok {
		t.Fatalf("labels not []string: %T", fields["labels"])
	}

	// Should contain type::task, priority::critical, and chum-canary
	hasType := false
	hasPriority := false
	hasCanary := false
	for _, l := range labels {
		switch l {
		case "type::task":
			hasType = true
		case "priority::critical":
			hasPriority = true
		case "chum-canary":
			hasCanary = true
		}
	}
	if !hasType {
		t.Error("missing type::task label")
	}
	if !hasPriority {
		t.Error("missing priority::critical label")
	}
	if !hasCanary {
		t.Error("missing chum-canary label")
	}
}

func TestBeadsIssueToGitHubFields_InProgressStatus(t *testing.T) {
	config := DefaultMappingConfig()

	issue := &types.Issue{
		Title:  "WIP",
		Status: types.StatusInProgress,
	}

	fields := BeadsIssueToGitHubFields(issue, config)

	if fields["state"] != "open" {
		t.Errorf("state = %q, want %q", fields["state"], "open")
	}

	labels := fields["labels"].([]string)
	found := false
	for _, l := range labels {
		if l == "status::in_progress" {
			found = true
		}
	}
	if !found {
		t.Errorf("missing status::in_progress label in %v", labels)
	}
}

func TestParseLabelPrefix(t *testing.T) {
	tests := []struct {
		label      string
		wantPrefix string
		wantValue  string
	}{
		{"priority::high", "priority", "high"},
		{"status::in_progress", "status", "in_progress"},
		{"type::bug", "type", "bug"},
		{"chum-canary", "", "chum-canary"},
		{"", "", ""},
		{"a::b::c", "a", "b::c"},
	}

	for _, tt := range tests {
		prefix, value := parseLabelPrefix(tt.label)
		if prefix != tt.wantPrefix || value != tt.wantValue {
			t.Errorf("parseLabelPrefix(%q) = (%q, %q), want (%q, %q)",
				tt.label, prefix, value, tt.wantPrefix, tt.wantValue)
		}
	}
}

func TestFilterNonScopedLabels(t *testing.T) {
	labels := []string{
		"priority::high",
		"status::in_progress",
		"type::bug",
		"chum-canary",
		"help-wanted",
	}

	filtered := filterNonScopedLabels(labels)
	if len(filtered) != 2 {
		t.Fatalf("got %d labels, want 2: %v", len(filtered), filtered)
	}
	if filtered[0] != "chum-canary" || filtered[1] != "help-wanted" {
		t.Errorf("filtered = %v, want [chum-canary help-wanted]", filtered)
	}
}
