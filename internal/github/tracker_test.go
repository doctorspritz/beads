package github

import (
	"testing"
)

func TestIsExternalRef(t *testing.T) {
	tr := &Tracker{}

	tests := []struct {
		ref  string
		want bool
	}{
		{"gh-1", true},
		{"gh-123", true},
		{"gh-0", true},
		{"", false},
		{"jira-ABC", false},
		{"gitlab:123", false},
		{"gh", false},
		{"gh-", true}, // prefix matches, ExtractIdentifier will reject
	}

	for _, tt := range tests {
		got := tr.IsExternalRef(tt.ref)
		if got != tt.want {
			t.Errorf("IsExternalRef(%q) = %v, want %v", tt.ref, got, tt.want)
		}
	}
}

func TestExtractIdentifier(t *testing.T) {
	tr := &Tracker{}

	tests := []struct {
		ref  string
		want string
	}{
		{"gh-1", "1"},
		{"gh-123", "123"},
		{"gh-42", "42"},
		{"gh-", ""},      // no number
		{"gh-abc", ""},   // not a number
		{"jira-123", ""}, // wrong prefix
		{"", ""},         // empty
		{"gh-0", "0"},    // zero is valid
		{"gh--1", ""},    // negative
	}

	for _, tt := range tests {
		got := tr.ExtractIdentifier(tt.ref)
		if got != tt.want {
			t.Errorf("ExtractIdentifier(%q) = %q, want %q", tt.ref, got, tt.want)
		}
	}
}

func TestBuildExternalRef(t *testing.T) {
	tr := &Tracker{}

	tests := []struct {
		identifier string
		want       string
	}{
		{"1", "gh-1"},
		{"123", "gh-123"},
		{"42", "gh-42"},
	}

	for _, tt := range tests {
		ti := trackerIssueWithIdentifier(tt.identifier)
		got := tr.BuildExternalRef(&ti)
		if got != tt.want {
			t.Errorf("BuildExternalRef(identifier=%q) = %q, want %q", tt.identifier, got, tt.want)
		}
	}
}

func TestGithubToTrackerIssue(t *testing.T) {
	now := mustParseTime("2024-01-15T10:00:00Z")
	gh := &Issue{
		ID:     12345,
		Number: 42,
		Title:  "Fix the bug",
		Body:   "There is a bug that needs fixing",
		State:  "open",
		Labels: []Label{
			{Name: "bug"},
			{Name: "priority::high"},
		},
		Assignee: &User{
			ID:    1,
			Login: "octocat",
		},
		HTMLURL:   "https://github.com/owner/repo/issues/42",
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	ti := githubToTrackerIssue(gh)

	if ti.ID != "12345" {
		t.Errorf("ID = %q, want %q", ti.ID, "12345")
	}
	if ti.Identifier != "42" {
		t.Errorf("Identifier = %q, want %q", ti.Identifier, "42")
	}
	if ti.Title != "Fix the bug" {
		t.Errorf("Title = %q, want %q", ti.Title, "Fix the bug")
	}
	if ti.Assignee != "octocat" {
		t.Errorf("Assignee = %q, want %q", ti.Assignee, "octocat")
	}
	if ti.URL != "https://github.com/owner/repo/issues/42" {
		t.Errorf("URL = %q, want %q", ti.URL, "https://github.com/owner/repo/issues/42")
	}
	if len(ti.Labels) != 2 {
		t.Errorf("Labels = %v, want 2 labels", ti.Labels)
	}
}

func TestIsIssue(t *testing.T) {
	issue := &Issue{Number: 1}
	if !issue.IsIssue() {
		t.Error("Issue without PullRequest should be IsIssue()=true")
	}

	pr := &Issue{Number: 2, PullRequest: &PullRequestRef{URL: "https://example.com"}}
	if pr.IsIssue() {
		t.Error("Issue with PullRequest should be IsIssue()=false")
	}
}
