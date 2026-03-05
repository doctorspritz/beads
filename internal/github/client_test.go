package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/tracker"
)

// trackerIssueWithIdentifier is a test helper.
func trackerIssueWithIdentifier(id string) tracker.TrackerIssue {
	return tracker.TrackerIssue{Identifier: id}
}

func TestFetchIssues(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	issues := []Issue{
		{
			ID:        1,
			Number:    1,
			Title:     "First issue",
			Body:      "Body 1",
			State:     "open",
			HTMLURL:   "https://github.com/test/repo/issues/1",
			CreatedAt: &now,
			UpdatedAt: &now,
		},
		{
			ID:        2,
			Number:    2,
			Title:     "Second issue",
			Body:      "Body 2",
			State:     "closed",
			HTMLURL:   "https://github.com/test/repo/issues/2",
			CreatedAt: &now,
			UpdatedAt: &now,
		},
		// This is a PR — should be filtered out
		{
			ID:          3,
			Number:      3,
			Title:       "A pull request",
			State:       "open",
			PullRequest: &PullRequestRef{URL: "https://api.github.com/repos/test/repo/pulls/3"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing or wrong authorization header")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(issues)
	}))
	defer server.Close()

	client := NewClient("test-token", "test", "repo").WithBaseURL(server.URL)
	ctx := context.Background()

	result, err := client.FetchIssues(ctx, "all")
	if err != nil {
		t.Fatalf("FetchIssues: %v", err)
	}

	// Should have filtered out the PR
	if len(result) != 2 {
		t.Fatalf("got %d issues, want 2", len(result))
	}

	if result[0].Title != "First issue" {
		t.Errorf("first issue title = %q, want %q", result[0].Title, "First issue")
	}
	if result[1].State != "closed" {
		t.Errorf("second issue state = %q, want %q", result[1].State, "closed")
	}
}

func TestFetchIssueByNumber(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	issue := Issue{
		ID:        42,
		Number:    7,
		Title:     "The issue",
		State:     "open",
		HTMLURL:   "https://github.com/test/repo/issues/7",
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/test/repo/issues/7" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(issue)
	}))
	defer server.Close()

	client := NewClient("test-token", "test", "repo").WithBaseURL(server.URL)
	ctx := context.Background()

	result, err := client.FetchIssueByNumber(ctx, 7)
	if err != nil {
		t.Fatalf("FetchIssueByNumber: %v", err)
	}

	if result.Number != 7 {
		t.Errorf("Number = %d, want 7", result.Number)
	}
	if result.Title != "The issue" {
		t.Errorf("Title = %q, want %q", result.Title, "The issue")
	}
}

func TestCreateIssue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/repos/test/repo/issues" {
			t.Errorf("path = %s, want /repos/test/repo/issues", r.URL.Path)
		}

		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)

		if body["title"] != "New issue" {
			t.Errorf("title = %v, want %q", body["title"], "New issue")
		}

		now := time.Now().UTC().Truncate(time.Second)
		resp := Issue{
			ID:        99,
			Number:    10,
			Title:     "New issue",
			State:     "open",
			HTMLURL:   "https://github.com/test/repo/issues/10",
			CreatedAt: &now,
			UpdatedAt: &now,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-token", "test", "repo").WithBaseURL(server.URL)
	ctx := context.Background()

	result, err := client.CreateIssue(ctx, "New issue", "description", []string{"bug"})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	if result.Number != 10 {
		t.Errorf("Number = %d, want 10", result.Number)
	}
}

func TestUpdateIssue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		if r.URL.Path != "/repos/test/repo/issues/5" {
			t.Errorf("path = %s, want /repos/test/repo/issues/5", r.URL.Path)
		}

		now := time.Now().UTC().Truncate(time.Second)
		resp := Issue{
			ID:        50,
			Number:    5,
			Title:     "Updated",
			State:     "closed",
			HTMLURL:   "https://github.com/test/repo/issues/5",
			CreatedAt: &now,
			UpdatedAt: &now,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-token", "test", "repo").WithBaseURL(server.URL)
	ctx := context.Background()

	result, err := client.UpdateIssue(ctx, 5, map[string]interface{}{"state": "closed"})
	if err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}

	if result.State != "closed" {
		t.Errorf("State = %q, want %q", result.State, "closed")
	}
}

func TestAddComment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/repos/test/repo/issues/3/comments" {
			t.Errorf("path = %s, want /repos/test/repo/issues/3/comments", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": 1})
	}))
	defer server.Close()

	client := NewClient("test-token", "test", "repo").WithBaseURL(server.URL)
	ctx := context.Background()

	err := client.AddComment(ctx, 3, "This is a comment")
	if err != nil {
		t.Fatalf("AddComment: %v", err)
	}
}

func TestAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer server.Close()

	client := NewClient("test-token", "test", "repo").WithBaseURL(server.URL)
	ctx := context.Background()

	_, err := client.FetchIssueByNumber(ctx, 999)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}
