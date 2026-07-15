package main

import (
	stdcontext "context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/struktly/struktly/internal/app"
	"github.com/struktly/struktly/internal/buildinfo"
	repoctx "github.com/struktly/struktly/internal/context"
	"github.com/struktly/struktly/internal/evidence"
	"github.com/struktly/struktly/internal/memory"
	"github.com/struktly/struktly/internal/runs"
)

func main() {
	ctx, stop := signal.NotifyContext(stdcontext.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(runCLI(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

type errorDocument struct {
	Schema string      `json:"schema"`
	Error  errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func runCLI(ctx stdcontext.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	root := newRootCmd()
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	exitCode, code := classifyError(err)
	if jsonErrorRequested(args) {
		_ = json.NewEncoder(stderr).Encode(errorDocument{
			Schema: "struktly/error/v1",
			Error:  errorDetail{Code: code, Message: err.Error()},
		})
	} else {
		_, _ = fmt.Fprintln(stderr, err)
	}
	return exitCode
}

func classifyError(err error) (int, string) {
	if errors.Is(err, stdcontext.Canceled) {
		return 130, "canceled"
	}
	if errors.Is(err, repoctx.ErrNotGitRepository) {
		return 1, "not_git_repository"
	}
	if errors.Is(err, repoctx.ErrRepositoryChanged) {
		return 1, "repository_changed"
	}
	if errors.Is(err, repoctx.ErrInvalidTask) {
		return 1, "invalid_task"
	}
	message := err.Error()
	if strings.Contains(message, ".struktly/config.json") {
		return 1, "invalid_config"
	}
	for _, marker := range []string{
		"unknown command", "unknown flag", "required flag", "accepts ", "requires ", "cannot be used", "use either --stdout or --json",
	} {
		if strings.Contains(message, marker) {
			return 2, "invalid_invocation"
		}
	}
	return 1, "operation_failed"
}

func jsonErrorRequested(args []string) bool {
	for _, arg := range args {
		for _, flag := range []string{"--json", "--json-errors"} {
			if arg == flag {
				return true
			}
			if value, ok := strings.CutPrefix(arg, flag+"="); ok {
				enabled, err := strconv.ParseBool(value)
				if err != nil || enabled {
					return true
				}
			}
		}
	}
	return false
}

func newRootCmd() *cobra.Command {
	var repoRoot string

	cmd := &cobra.Command{
		Use:           "struktly",
		Short:         "Build repository context for a coding request",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().StringVar(&repoRoot, "root", ".", "Repository root to inspect")
	cmd.PersistentFlags().Bool("json-errors", false, "Emit structured errors on stderr")

	cmd.AddCommand(newInitCmd(&repoRoot))
	cmd.AddCommand(newScanCmd(&repoRoot))
	cmd.AddCommand(newBriefCmd(&repoRoot))
	cmd.AddCommand(newEvidenceCmd(&repoRoot))
	cmd.AddCommand(newSuggestInstructionsCmd(&repoRoot))
	cmd.AddCommand(newRunCmd(&repoRoot))
	cmd.AddCommand(newMemoryCmd(&repoRoot))
	cmd.AddCommand(newStatusCmd(&repoRoot))
	cmd.AddCommand(newExplainCmd(&repoRoot))
	cmd.AddCommand(newValidateCmd(&repoRoot))
	cmd.AddCommand(newDoctorCmd(&repoRoot))
	cmd.AddCommand(newMCPCmd(&repoRoot))
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newCapabilitiesCmd())

	return cmd
}

const capabilitiesSchema = "struktly/capabilities/v1"

type capabilitiesDocument struct {
	Schema   string         `json:"schema"`
	Build    buildinfo.Info `json:"build"`
	Commands []string       `json:"commands"`
	Schemas  []string       `json:"schemas"`
	Features []string       `json:"features"`
}

func currentCapabilities() capabilitiesDocument {
	return capabilitiesDocument{
		Schema:   capabilitiesSchema,
		Build:    buildinfo.Current(),
		Commands: []string{"capabilities", "context", "doctor", "explain", "scan", "status", "validate"},
		Schemas: []string{
			capabilitiesSchema,
			"struktly/doctor/v1",
			"struktly/error/v1",
			"struktly/explanation/v1",
			repoctx.PacketSchema,
			repoctx.SnapshotSchema,
			"struktly/status/v1",
			"struktly/validation/v1",
		},
		Features: []string{
			"context.cancellation",
			"context.expect_base_revision",
			"context.no_write",
			"scan.no_write",
			"structured_errors",
		},
	}
}

func newCapabilitiesCmd() *cobra.Command {
	var toJSON bool
	cmd := &cobra.Command{
		Use:   "capabilities",
		Short: "Report supported machine interfaces",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			capabilities := currentCapabilities()
			if toJSON {
				return writeJSON(cmd.OutOrStdout(), capabilities)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "struktly %s\n", capabilities.Build.Version)
			for _, feature := range capabilities.Features {
				fmt.Fprintln(cmd.OutOrStdout(), feature)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&toJSON, "json", false, "Print the versioned capabilities document")
	return cmd
}

func newVersionCmd() *cobra.Command {
	var toJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print Struktly version and build metadata",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := buildinfo.Current()
			if toJSON {
				return writeJSON(cmd.OutOrStdout(), info)
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "struktly %s\n", info.Version)
			if err != nil {
				return err
			}
			if info.Revision != "" {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "revision: %s\n", info.Revision); err != nil {
					return err
				}
			}
			if info.Date != "" {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "built: %s\n", info.Date)
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&toJSON, "json", false, "Print build metadata as JSON")
	return cmd
}

func newStatusCmd(repoRoot *string) *cobra.Command {
	var toJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Experimental: inspect repository context status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := app.Status(cmd.Context(), *repoRoot)
			if err != nil {
				return err
			}
			if toJSON {
				return writeJSON(cmd.OutOrStdout(), report)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "repository: %s (%s)\n", report.Repository.Name, report.Repository.HeadRevision)
			fmt.Fprintf(cmd.OutOrStdout(), "branch: %s\n", emptyValue(report.Repository.Branch, "detached HEAD"))
			fmt.Fprintf(cmd.OutOrStdout(), "config: %s\n", declaredValue(report.ConfigDeclared))
			for _, file := range report.PortableFiles {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", file.Path, file.Status)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", report.LatestSnapshot.Path, report.LatestSnapshot.Status)
			return err
		},
	}
	cmd.Flags().BoolVar(&toJSON, "json", false, "Print structured status to stdout")
	return cmd
}

func newExplainCmd(repoRoot *string) *cobra.Command {
	var task string
	var toJSON bool
	cmd := &cobra.Command{
		Use:   "explain <path>",
		Short: "Experimental: explain context inclusion or exclusion",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			explanation, err := repoctx.ExplainSelection(cmd.Context(), *repoRoot, args[0], task)
			if err != nil {
				return err
			}
			if toJSON {
				return writeJSON(cmd.OutOrStdout(), explanation)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s (%s)\n", explanation.Path, explanation.Decision, explanation.Reason)
			return err
		},
	}
	cmd.Flags().StringVar(&task, "task", "", "Optional task used for task-match selection")
	cmd.Flags().BoolVar(&toJSON, "json", false, "Print structured explanation to stdout")
	return cmd
}

func newValidateCmd(repoRoot *string) *cobra.Command {
	var toJSON bool
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Experimental: validate configuration and task files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := app.Validate(cmd.Context(), *repoRoot)
			if err != nil {
				return err
			}
			if toJSON {
				return writeJSON(cmd.OutOrStdout(), report)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "valid repository declarations (%s, %d tasks)\n", declaredValue(report.ConfigDeclared), len(report.Tasks))
			return err
		},
	}
	cmd.Flags().BoolVar(&toJSON, "json", false, "Print structured validation result to stdout")
	return cmd
}

func newDoctorCmd(repoRoot *string) *cobra.Command {
	var toJSON bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Experimental: diagnose repository and installation problems",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := app.Doctor(cmd.Context(), *repoRoot)
			if err != nil {
				return err
			}
			if toJSON {
				return writeJSON(cmd.OutOrStdout(), report)
			}
			for _, check := range report.Checks {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s", check.Status, check.Name); err != nil {
					return err
				}
				if check.Message != "" {
					fmt.Fprintf(cmd.OutOrStdout(), ": %s", check.Message)
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&toJSON, "json", false, "Print structured diagnostics to stdout")
	return cmd
}

func declaredValue(declared bool) string {
	if declared {
		return "declared"
	}
	return "built-in defaults"
}

func emptyValue(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func newInitCmd(repoRoot *string) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create repository configuration and write project context",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInit(cmd, *repoRoot)
		},
	}
}

func runInit(cmd *cobra.Command, repoRoot string) error {
	result, err := app.Init(app.InitOptions{Root: repoRoot})
	if err != nil {
		return err
	}

	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}

	for _, path := range result.CreatedPaths {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "created %s\n", relToRoot(root, path)); err != nil {
			return err
		}
	}
	for _, path := range result.SkippedPaths {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "kept %s (already exists)\n", relToRoot(root, path)); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", relToRoot(root, result.ScanOutputPath))
	return err
}

func newScanCmd(repoRoot *string) *cobra.Command {
	var runID string
	var toJSON bool
	var noWrite bool
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Write .struktly/project-context.md for a repository",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if noWrite && !toJSON {
				return fmt.Errorf("--no-write requires --json")
			}
			if noWrite && runID != "" {
				return fmt.Errorf("--no-write cannot be used with --run")
			}
			result, err := repoctx.Scan(repoctx.ScanOptions{Root: *repoRoot, RunID: runID, NoWrite: noWrite})
			if err != nil {
				return err
			}
			confirmation := cmd.OutOrStdout()
			if toJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(result.Snapshot); err != nil {
					return fmt.Errorf("encode snapshot json: %w", err)
				}
				confirmation = cmd.ErrOrStderr()
			}
			if !noWrite {
				_, err = fmt.Fprintf(confirmation, "wrote %s\n", result.OutputPath)
			}
			return err
		},
	}
	cmd.Flags().StringVar(&runID, "run", "", "Attach scan output to a run id")
	cmd.Flags().BoolVar(&toJSON, "json", false, "Print the structured snapshot to stdout for piping")
	cmd.Flags().BoolVar(&noWrite, "no-write", false, "Do not write generated files; requires --json")
	return cmd
}

func newBriefCmd(repoRoot *string) *cobra.Command {
	var runID string
	var toStdout bool
	var toJSON bool
	var noWrite bool
	var expectedBaseRevision string
	cmd := &cobra.Command{
		Use:     "context <request>",
		Aliases: []string{"brief"},
		Short:   "Build a context packet for one coding request",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if toStdout && toJSON {
				return fmt.Errorf("use either --stdout or --json, not both")
			}
			if noWrite && !toJSON {
				return fmt.Errorf("--no-write requires --json")
			}
			if noWrite && runID != "" {
				return fmt.Errorf("--no-write cannot be used with --run")
			}
			result, err := repoctx.Brief(repoctx.BriefOptions{
				Context:              cmd.Context(),
				Root:                 *repoRoot,
				Task:                 args[0],
				RunID:                runID,
				NoWrite:              noWrite,
				ExpectedBaseRevision: expectedBaseRevision,
			})
			if err != nil {
				return err
			}
			confirmation := cmd.OutOrStdout()
			switch {
			case toStdout:
				data, err := os.ReadFile(result.OutputPath)
				if err != nil {
					return fmt.Errorf("read context packet: %w", err)
				}
				if _, err := cmd.OutOrStdout().Write(data); err != nil {
					return err
				}
				confirmation = cmd.ErrOrStderr()
			case toJSON:
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(result.Packet); err != nil {
					return fmt.Errorf("encode context packet json: %w", err)
				}
				confirmation = cmd.ErrOrStderr()
			}
			if !noWrite {
				_, err = fmt.Fprintf(confirmation, "wrote %s\n", result.OutputPath)
			}
			return err
		},
	}
	cmd.Flags().StringVar(&runID, "run", "", "Attach context packet to a run id")
	cmd.Flags().BoolVar(&toStdout, "stdout", false, "Print the context packet to stdout for piping")
	cmd.Flags().BoolVar(&toJSON, "json", false, "Print the structured packet to stdout for piping")
	cmd.Flags().BoolVar(&noWrite, "no-write", false, "Do not write generated files; requires --json")
	cmd.Flags().StringVar(&expectedBaseRevision, "expect-base-revision", "", "Fail if Git HEAD does not match this revision")
	return cmd
}

func newSuggestInstructionsCmd(repoRoot *string) *cobra.Command {
	return &cobra.Command{
		Use:   "suggest-instructions",
		Short: "Write suggested agent instruction drafts under .struktly/agent-instructions/",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSuggestInstructions(cmd, *repoRoot)
		},
	}
}

func runSuggestInstructions(cmd *cobra.Command, repoRoot string) error {
	result, err := repoctx.SuggestInstructions(repoctx.SuggestInstructionsOptions{
		Root: repoRoot,
	})
	if err != nil {
		return err
	}

	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}

	for _, path := range result.OutputPaths {
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", relToRoot(root, path))
		if err != nil {
			return err
		}
	}
	return nil
}

func newEvidenceCmd(repoRoot *string) *cobra.Command {
	opts := &evidenceOptions{}
	cmd := &cobra.Command{
		Use:   "evidence",
		Short: "Experimental: record completed work and checks",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEvidence(cmd, *repoRoot, opts)
		},
	}
	cmd.Flags().StringVar(&opts.task, "task", "", "Task or work summary")
	cmd.Flags().StringVar(&opts.agent, "agent", "", "Agent or tool name")
	cmd.Flags().StringVar(&opts.outcome, "outcome", "", "Outcome summary")
	cmd.Flags().StringVar(&opts.contextPacket, "context-packet", "", "Path to context packet used for the work")
	cmd.Flags().StringSliceVar(&opts.checks, "checks", nil, "Verification command that was run")
	cmd.Flags().StringVar(&opts.checkResult, "result", "", "Result summary for checks run")
	cmd.Flags().BoolVar(&opts.runChecks, "run-checks", false, "Execute the declared checks and record real exit codes")
	cmd.Flags().StringSliceVar(&opts.filesTouched, "files", nil, "Files changed during the work")
	cmd.Flags().StringVar(&opts.reviewer, "reviewer", "", "Reviewer name or review status")
	cmd.Flags().StringVar(&opts.runID, "run", "", "Attach evidence ledger to a run id")
	_ = cmd.MarkFlagRequired("task")
	_ = cmd.MarkFlagRequired("agent")
	_ = cmd.MarkFlagRequired("outcome")
	return cmd
}

type evidenceOptions struct {
	task          string
	agent         string
	outcome       string
	contextPacket string
	checks        []string
	checkResult   string
	runChecks     bool
	filesTouched  []string
	reviewer      string
	runID         string
}

func runEvidence(cmd *cobra.Command, repoRoot string, opts *evidenceOptions) error {
	result, err := evidence.RecordEvidence(evidence.EvidenceOptions{
		Root:          repoRoot,
		Task:          opts.task,
		Agent:         opts.agent,
		Outcome:       opts.outcome,
		ContextPacket: opts.contextPacket,
		Checks:        opts.checks,
		CheckResult:   opts.checkResult,
		RunChecks:     opts.runChecks,
		FilesTouched:  opts.filesTouched,
		Reviewer:      opts.reviewer,
		RunID:         opts.runID,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "appended to %s\n", result.OutputPath)
	return err
}

func newRunCmd(repoRoot *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Experimental: create and inspect local work records",
	}
	cmd.AddCommand(newRunCreateCmd(repoRoot))
	cmd.AddCommand(newRunListCmd(repoRoot))
	cmd.AddCommand(newRunShowCmd(repoRoot))
	cmd.AddCommand(newRunEventCmd(repoRoot))
	cmd.AddCommand(newRunCompleteCmd(repoRoot))
	cmd.AddCommand(newRunFailCmd(repoRoot))
	return cmd
}

type runCreateOptions struct {
	goal     string
	repoPath string
}

func newRunCreateCmd(repoRoot *string) *cobra.Command {
	opts := &runCreateOptions{}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a work record",
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := runs.CreateRun(runs.CreateRunOptions{
				Root:          *repoRoot,
				Goal:          opts.goal,
				RepoPath:      opts.repoPath,
				SourceCommand: "struktly run create --goal " + opts.goal,
			})
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), result.Run)
		},
	}
	cmd.Flags().StringVar(&opts.goal, "goal", "", "Run goal")
	cmd.Flags().StringVar(&opts.repoPath, "repo", "", "Repository path for the run (defaults to --root)")
	_ = cmd.MarkFlagRequired("goal")
	return cmd
}

func newRunListCmd(repoRoot *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List local run records",
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := runs.ListRuns(runs.ListRunsOptions{Root: *repoRoot})
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), result)
		},
	}
}

func newRunShowCmd(repoRoot *string) *cobra.Command {
	return &cobra.Command{
		Use:   "show <run-id>",
		Short: "Show a run record and events",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := runs.ShowRun(runs.ShowRunOptions{Root: *repoRoot, RunID: args[0]})
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), result)
		},
	}
}

type runEventOptions struct {
	eventType string
	message   string
}

func newRunEventCmd(repoRoot *string) *cobra.Command {
	opts := &runEventOptions{}
	cmd := &cobra.Command{
		Use:   "event <run-id>",
		Short: "Append an event to a run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := runs.AppendRunEvent(runs.RunEventOptions{
				Root:    *repoRoot,
				RunID:   args[0],
				Type:    opts.eventType,
				Message: opts.message,
			})
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), result.Event)
		},
	}
	cmd.Flags().StringVar(&opts.eventType, "type", "", "Event type")
	cmd.Flags().StringVar(&opts.message, "message", "", "Event message")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("message")
	return cmd
}

func newRunCompleteCmd(repoRoot *string) *cobra.Command {
	return &cobra.Command{
		Use:   "complete <run-id>",
		Short: "Mark a run completed",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := runs.CompleteRun(runs.UpdateRunStatusOptions{Root: *repoRoot, RunID: args[0]})
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), result.Run)
		},
	}
}

func newRunFailCmd(repoRoot *string) *cobra.Command {
	return &cobra.Command{
		Use:   "fail <run-id>",
		Short: "Mark a run failed",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := runs.FailRun(runs.UpdateRunStatusOptions{Root: *repoRoot, RunID: args[0]})
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), result.Run)
		},
	}
}

func newMemoryCmd(repoRoot *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Experimental: review candidate and approved repository notes",
	}
	cmd.AddCommand(newMemoryCandidateCmd(repoRoot))
	cmd.AddCommand(newMemoryCandidatesCmd(repoRoot))
	cmd.AddCommand(newMemoryApproveCmd(repoRoot))
	cmd.AddCommand(newMemoryRejectCmd(repoRoot))
	cmd.AddCommand(newMemoryListCmd(repoRoot))
	return cmd
}

type memoryCandidateOptions struct {
	scope          string
	content        string
	tags           []string
	sourceRunID    string
	sourceArtifact string
}

func newMemoryCandidateCmd(repoRoot *string) *cobra.Command {
	opts := &memoryCandidateOptions{}
	cmd := &cobra.Command{
		Use:   "candidate",
		Short: "Create a human-reviewed memory candidate",
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := memory.CreateMemoryCandidate(memory.MemoryCandidateOptions{
				Root:           *repoRoot,
				Scope:          opts.scope,
				Content:        opts.content,
				Tags:           opts.tags,
				SourceRunID:    opts.sourceRunID,
				SourceArtifact: opts.sourceArtifact,
			})
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), result.Candidate)
		},
	}
	cmd.Flags().StringVar(&opts.scope, "scope", "repository", "Memory scope: global, project, or repository")
	cmd.Flags().StringVar(&opts.content, "content", "", "Candidate memory content")
	cmd.Flags().StringSliceVar(&opts.tags, "tags", nil, "Comma-separated memory tags")
	cmd.Flags().StringVar(&opts.sourceRunID, "source-run-id", "", "Source run id")
	cmd.Flags().StringVar(&opts.sourceArtifact, "source-artifact", "", "Source artifact path")
	_ = cmd.MarkFlagRequired("content")
	return cmd
}

func newMemoryCandidatesCmd(repoRoot *string) *cobra.Command {
	return &cobra.Command{
		Use:   "candidates",
		Short: "List memory candidates",
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := memory.ListMemoryCandidates(memory.ListMemoryOptions{Root: *repoRoot})
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), result)
		},
	}
}

func newMemoryApproveCmd(repoRoot *string) *cobra.Command {
	return &cobra.Command{
		Use:   "approve <candidate-id>",
		Short: "Approve a memory candidate",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := memory.ApproveMemoryCandidate(memory.MemoryResolutionOptions{Root: *repoRoot, CandidateID: args[0]})
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), result.Memory)
		},
	}
}

func newMemoryRejectCmd(repoRoot *string) *cobra.Command {
	return &cobra.Command{
		Use:   "reject <candidate-id>",
		Short: "Reject a memory candidate",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := memory.RejectMemoryCandidate(memory.MemoryResolutionOptions{Root: *repoRoot, CandidateID: args[0]})
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), result.Candidate)
		},
	}
}

func newMemoryListCmd(repoRoot *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List approved memory",
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := memory.ListApprovedMemory(memory.ListMemoryOptions{Root: *repoRoot})
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), result)
		},
	}
}

func relToRoot(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func writeJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}
