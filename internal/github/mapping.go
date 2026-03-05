package github

import (
	"fmt"

	"github.com/steveyegge/beads/internal/types"
)

// MappingConfig configures how GitHub fields map to beads fields.
type MappingConfig struct {
	PriorityMap  map[string]int    // priority label value → beads priority (0-4)
	StateMap     map[string]string // GitHub state → beads status
	LabelTypeMap map[string]string // type label value → beads issue type
}

// DefaultMappingConfig returns the default mapping configuration.
func DefaultMappingConfig() *MappingConfig {
	priorityMap := make(map[string]int, len(PriorityMapping))
	for k, v := range PriorityMapping {
		priorityMap[k] = v
	}

	labelTypeMap := make(map[string]string, len(typeMapping))
	for k, v := range typeMapping {
		labelTypeMap[k] = v
	}

	return &MappingConfig{
		PriorityMap: priorityMap,
		StateMap: map[string]string{
			"open":   "open",
			"closed": "closed",
		},
		LabelTypeMap: labelTypeMap,
	}
}

// GitHubIssueToBeads converts a GitHub Issue to a beads Issue.
func GitHubIssueToBeads(gh *Issue, config *MappingConfig) *types.Issue {
	labelNames := LabelNames(gh.Labels)

	issue := &types.Issue{
		Title:       gh.Title,
		Description: gh.Body,
		IssueType:   types.IssueType(typeFromLabels(labelNames)),
		Priority:    priorityFromLabels(labelNames),
		Status:      types.Status(statusFromLabelsAndState(labelNames, gh.State)),
		Labels:      filterNonScopedLabels(labelNames),
	}

	ref := fmt.Sprintf("gh-%d", gh.Number)
	issue.ExternalRef = &ref
	issue.SourceSystem = fmt.Sprintf("github:%s:%d", "", gh.Number)

	if gh.Assignee != nil {
		issue.Assignee = gh.Assignee.Login
	}

	if gh.CreatedAt != nil {
		issue.CreatedAt = *gh.CreatedAt
	}
	if gh.UpdatedAt != nil {
		issue.UpdatedAt = *gh.UpdatedAt
	}

	return issue
}

// BeadsIssueToGitHubFields converts a beads Issue to GitHub API update fields.
func BeadsIssueToGitHubFields(issue *types.Issue, config *MappingConfig) map[string]interface{} {
	fields := map[string]interface{}{
		"title": issue.Title,
		"body":  issue.Description,
	}

	// Build labels from type, priority, and status
	var labels []string

	if issue.IssueType != "" {
		labels = append(labels, "type::"+string(issue.IssueType))
	}

	priorityLabel := priorityToLabel(issue.Priority)
	if priorityLabel != "" {
		labels = append(labels, "priority::"+priorityLabel)
	}

	// Add status label (if not open or closed — those are handled by state)
	switch issue.Status {
	case types.StatusInProgress:
		labels = append(labels, "status::in_progress")
	case types.StatusBlocked:
		labels = append(labels, "status::blocked")
	case types.StatusDeferred:
		labels = append(labels, "status::deferred")
	}

	// Add existing non-scoped labels
	labels = append(labels, issue.Labels...)

	fields["labels"] = labels

	// Set state for closed issues
	if issue.Status == types.StatusClosed {
		fields["state"] = "closed"
	} else {
		fields["state"] = "open"
	}

	return fields
}
