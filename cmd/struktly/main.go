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
	if errors.Is(err, repoctx.ErrInvalidPacketLimit) {
		return 2, "invalid_invocation"
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
	cmd.AddCommand(newTasksCmd(&repoRoot))
	cmd.AddCommand(newSuggestInstructionsCmd(&repoRoot))
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
		Commands: []string{"capabilities", "context", "doctor", "explain", "scan", "status", "tasks", "validate"},
		Schemas: []string{
			capabilitiesSchema,
			"struktly/doctor/v1",
			"struktly/error/v1",
			"struktly/explanation/v1",
			repoctx.PacketSchema,
			repoctx.SnapshotSchema,
			repoctx.TasksSchema,
			"struktly/status/v1",
			"struktly/validation/v1",
		},
		Features: []string{
			"context.cancellation",
			"context.limits",
			"context.expect_base_revision",
			"context.no_write",
			"scan.no_write",
			"structured_errors",
			"tasks.partial_results",
		},
	}
}

func newTasksCmd(repoRoot *string) *cobra.Command {
	var toJSON bool
	cmd := &cobra.Command{
		Use:   "tasks",
		Short: "List repository task declarations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			document, err := repoctx.DiscoverTasks(*repoRoot)
			if err != nil {
				return err
			}
			if toJSON {
				return writeJSON(cmd.OutOrStdout(), document)
			}
			for _, task := range document.Tasks {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", task.Status, task.Path, task.Title); err != nil {
					return err
				}
			}
			for _, invalid := range document.Invalid {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "invalid\t%s\t%s\n", invalid.Path, invalid.Reason); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&toJSON, "json", false, "Print the versioned tasks document")
	return cmd
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
	var toJSON bool
	var noWrite bool
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Write .struktly/project-context.md for a repository",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if noWrite && !toJSON {
				return fmt.Errorf("--no-write requires --json")
			}
			result, err := repoctx.Scan(repoctx.ScanOptions{Root: *repoRoot, NoWrite: noWrite})
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
	cmd.Flags().BoolVar(&toJSON, "json", false, "Print the structured snapshot to stdout for piping")
	cmd.Flags().BoolVar(&noWrite, "no-write", false, "Do not write generated files; requires --json")
	return cmd
}

func newBriefCmd(repoRoot *string) *cobra.Command {
	var toStdout bool
	var toJSON bool
	var noWrite bool
	var expectedBaseRevision string
	var maxItems int
	var maxFileBytes int
	var maxTotalBytes int
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
			flags := cmd.Flags()
			maxItemsSet := flags.Lookup("max-items").Changed
			maxFileBytesSet := flags.Lookup("max-file-bytes").Changed
			maxTotalBytesSet := flags.Lookup("max-total-bytes").Changed
			if maxItemsSet && maxItems <= 0 {
				return fmt.Errorf("%w: max_items must be greater than 0", repoctx.ErrInvalidPacketLimit)
			}
			if maxFileBytesSet && maxFileBytes <= 0 {
				return fmt.Errorf("%w: max_file_bytes must be greater than 0", repoctx.ErrInvalidPacketLimit)
			}
			if maxTotalBytesSet && maxTotalBytes <= 0 {
				return fmt.Errorf("%w: max_total_bytes must be greater than 0", repoctx.ErrInvalidPacketLimit)
			}
			if !maxItemsSet {
				maxItems = 0
			}
			if !maxFileBytesSet {
				maxFileBytes = 0
			}
			if !maxTotalBytesSet {
				maxTotalBytes = 0
			}
			result, err := repoctx.Brief(repoctx.BriefOptions{
				Context:              cmd.Context(),
				Root:                 *repoRoot,
				Task:                 args[0],
				NoWrite:              noWrite,
				ExpectedBaseRevision: expectedBaseRevision,
				MaxItems:             maxItems,
				MaxFileBytes:         maxFileBytes,
				MaxTotalBytes:        maxTotalBytes,
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
	cmd.Flags().BoolVar(&toStdout, "stdout", false, "Print the context packet to stdout for piping")
	cmd.Flags().BoolVar(&toJSON, "json", false, "Print the structured packet to stdout for piping")
	cmd.Flags().BoolVar(&noWrite, "no-write", false, "Do not write generated files; requires --json")
	cmd.Flags().StringVar(&expectedBaseRevision, "expect-base-revision", "", "Fail if Git HEAD does not match this revision")
	cmd.Flags().IntVar(&maxItems, "max-items", 0, "Maximum selected files")
	cmd.Flags().IntVar(&maxFileBytes, "max-file-bytes", 0, "Maximum bytes to read from each selected file")
	cmd.Flags().IntVar(&maxTotalBytes, "max-total-bytes", 0, "Maximum total selected content bytes")
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
