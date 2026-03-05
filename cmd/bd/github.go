package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/github"
	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/tracker"
	"github.com/steveyegge/beads/internal/types"
)

// GitHubConfig holds GitHub connection configuration.
type GitHubConfig struct {
	Token string // Personal access token
	Org   string // Repository owner (user or org)
	Repo  string // Repository name
}

// githubCmd is the root command for GitHub operations.
var githubCmd = &cobra.Command{
	Use:   "github",
	Short: "GitHub integration commands",
	Long: `Commands for syncing issues between beads and GitHub.

Configuration can be set via 'bd config' or environment variables:
  github.token / GITHUB_TOKEN  - Personal access token
  github.org / GITHUB_ORG      - Repository owner (user or organization)
  github.repo / GITHUB_REPO    - Repository name`,
}

// githubSyncCmd synchronizes issues between beads and GitHub.
var githubSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync issues with GitHub",
	Long: `Synchronize issues between beads and GitHub.

By default, performs bidirectional sync:
- Pulls new/updated issues from GitHub to beads
- Pushes local beads issues to GitHub

Use --pull-only or --push-only to limit direction.`,
	RunE: runGitHubSync,
}

// githubStatusCmd displays GitHub configuration and sync status.
var githubStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show GitHub sync status",
	Long:  `Display current GitHub configuration and sync status.`,
	RunE:  runGitHubStatus,
}

var (
	githubSyncDryRun   bool
	githubSyncPullOnly bool
	githubSyncPushOnly bool
	githubPreferLocal  bool
	githubPreferGitHub bool
	githubPreferNewer  bool
)

func init() {
	githubCmd.AddCommand(githubSyncCmd)
	githubCmd.AddCommand(githubStatusCmd)

	githubSyncCmd.Flags().BoolVar(&githubSyncDryRun, "dry-run", false, "Show what would be synced without making changes")
	githubSyncCmd.Flags().BoolVar(&githubSyncPullOnly, "pull-only", false, "Only pull issues from GitHub")
	githubSyncCmd.Flags().BoolVar(&githubSyncPushOnly, "push-only", false, "Only push issues to GitHub")

	// Conflict resolution flags (mutually exclusive)
	githubSyncCmd.Flags().BoolVar(&githubPreferLocal, "prefer-local", false, "On conflict, keep local beads version")
	githubSyncCmd.Flags().BoolVar(&githubPreferGitHub, "prefer-github", false, "On conflict, use GitHub version")
	githubSyncCmd.Flags().BoolVar(&githubPreferNewer, "prefer-newer", false, "On conflict, use most recent version (default)")

	rootCmd.AddCommand(githubCmd)
}

// getGitHubConfig returns GitHub configuration from bd config or environment.
func getGitHubConfig() GitHubConfig {
	ctx := context.Background()
	config := GitHubConfig{}

	config.Token = getGitHubConfigValue(ctx, "github.token")
	config.Org = getGitHubConfigValue(ctx, "github.org")
	config.Repo = getGitHubConfigValue(ctx, "github.repo")

	return config
}

// getGitHubConfigValue reads a GitHub configuration value from store or environment.
func getGitHubConfigValue(ctx context.Context, key string) string {
	if store != nil {
		value, _ := store.GetConfig(ctx, key)
		if value != "" {
			return value
		}
	} else if dbPath != "" {
		tempStore, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
		if err == nil {
			defer func() { _ = tempStore.Close() }()
			value, _ := tempStore.GetConfig(ctx, key)
			if value != "" {
				return value
			}
		}
	}

	envKey := githubConfigToEnvVar(key)
	if envKey != "" {
		if value := os.Getenv(envKey); value != "" {
			return value
		}
	}

	return ""
}

// githubConfigToEnvVar maps GitHub config keys to their environment variable names.
func githubConfigToEnvVar(key string) string {
	switch key {
	case "github.token":
		return "GITHUB_TOKEN"
	case "github.org":
		return "GITHUB_ORG"
	case "github.repo":
		return "GITHUB_REPO"
	default:
		return ""
	}
}

// validateGitHubConfig checks that required configuration is present.
func validateGitHubConfig(config GitHubConfig) error {
	if config.Token == "" {
		return fmt.Errorf("github.token is not configured. Set via 'bd config set github.token <token>' or GITHUB_TOKEN environment variable")
	}
	if config.Org == "" {
		return fmt.Errorf("github.org is not configured. Set via 'bd config set github.org <owner>' or GITHUB_ORG environment variable")
	}
	if config.Repo == "" {
		return fmt.Errorf("github.repo is not configured. Set via 'bd config set github.repo <repo>' or GITHUB_REPO environment variable")
	}
	return nil
}

// maskGitHubToken masks a token for safe display.
func maskGitHubToken(token string) string {
	if token == "" {
		return "(not set)"
	}
	if len(token) <= 4 {
		return "****"
	}
	return token[:4] + "****"
}

// runGitHubStatus implements the github status command.
func runGitHubStatus(cmd *cobra.Command, args []string) error {
	config := getGitHubConfig()

	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintln(out, "GitHub Configuration")
	_, _ = fmt.Fprintln(out, "====================")
	_, _ = fmt.Fprintf(out, "Org:   %s\n", config.Org)
	_, _ = fmt.Fprintf(out, "Repo:  %s\n", config.Repo)
	_, _ = fmt.Fprintf(out, "Token: %s\n", maskGitHubToken(config.Token))

	if err := validateGitHubConfig(config); err != nil {
		_, _ = fmt.Fprintf(out, "\nStatus: ❌ Not configured\n")
		_, _ = fmt.Fprintf(out, "Error: %v\n", err)
		return nil
	}

	_, _ = fmt.Fprintf(out, "\nStatus: ✓ Configured\n")
	return nil
}

// runGitHubSync implements the github sync command.
func runGitHubSync(cmd *cobra.Command, args []string) error {
	config := getGitHubConfig()
	if err := validateGitHubConfig(config); err != nil {
		return err
	}

	if !githubSyncDryRun {
		CheckReadonly("github sync")
	}

	if githubSyncPullOnly && githubSyncPushOnly {
		return fmt.Errorf("cannot use both --pull-only and --push-only")
	}

	// Validate conflict flags
	flagsSet := 0
	if githubPreferLocal {
		flagsSet++
	}
	if githubPreferGitHub {
		flagsSet++
	}
	if githubPreferNewer {
		flagsSet++
	}
	if flagsSet > 1 {
		return fmt.Errorf("cannot use multiple conflict resolution flags (--prefer-local, --prefer-github, --prefer-newer)")
	}

	if err := ensureStoreActive(); err != nil {
		return fmt.Errorf("database not available: %w", err)
	}

	out := cmd.OutOrStdout()
	ctx := context.Background()

	// Create and initialize the GitHub tracker
	gt := &github.Tracker{}
	if err := gt.Init(ctx, store); err != nil {
		return fmt.Errorf("initializing GitHub tracker: %w", err)
	}

	// Create the sync engine
	engine := tracker.NewEngine(gt, store, actor)
	engine.OnMessage = func(msg string) { _, _ = fmt.Fprintln(out, "  "+msg) }
	engine.OnWarning = func(msg string) { _, _ = fmt.Fprintf(os.Stderr, "Warning: %s\n", msg) }

	// Set up pull hooks
	engine.PullHooks = buildGitHubPullHooks(ctx)

	// Build sync options from CLI flags
	pull := !githubSyncPushOnly
	push := !githubSyncPullOnly

	opts := tracker.SyncOptions{
		Pull:   pull,
		Push:   push,
		DryRun: githubSyncDryRun,
	}

	// Map conflict resolution
	if githubPreferLocal {
		opts.ConflictResolution = tracker.ConflictLocal
	} else if githubPreferGitHub {
		opts.ConflictResolution = tracker.ConflictExternal
	} else {
		opts.ConflictResolution = tracker.ConflictTimestamp
	}

	if githubSyncDryRun {
		_, _ = fmt.Fprintln(out, "Dry run mode - no changes will be made")
		_, _ = fmt.Fprintln(out)
	}

	// Run sync
	result, err := engine.Sync(ctx, opts)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return err
	}

	// Output results
	if !githubSyncDryRun {
		if result.Stats.Pulled > 0 {
			_, _ = fmt.Fprintf(out, "✓ Pulled %d issues (%d created, %d updated)\n",
				result.Stats.Pulled, result.Stats.Created, result.Stats.Updated)
		}
		if result.Stats.Pushed > 0 {
			_, _ = fmt.Fprintf(out, "✓ Pushed %d issues\n", result.Stats.Pushed)
		}
		if result.Stats.Conflicts > 0 {
			_, _ = fmt.Fprintf(out, "→ Resolved %d conflicts\n", result.Stats.Conflicts)
		}
	}

	if githubSyncDryRun {
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, "Run without --dry-run to apply changes")
	}

	return nil
}

// buildGitHubPullHooks creates PullHooks for GitHub-specific pull behavior.
func buildGitHubPullHooks(ctx context.Context) *tracker.PullHooks {
	prefix := "bd"
	if store != nil {
		if p, err := store.GetConfig(ctx, "issue_prefix"); err == nil && p != "" {
			prefix = p
		}
	}

	return &tracker.PullHooks{
		GenerateID: func(_ context.Context, issue *types.Issue) error {
			if issue.ID == "" {
				issue.ID = generateIssueID(prefix)
			}
			return nil
		},
	}
}
