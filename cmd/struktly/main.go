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
	// A pass-through owns its own exit code and has already said whatever it had
	// to say, so it is settled before this CLI's classification and before its
	// structured error document: neither of those describes another program.
	var exitErr exitCodeError
	if errors.As(err, &exitErr) {
		if exitErr.message != "" {
			_, _ = fmt.Fprintln(stderr, exitErr.message)
		}
		return exitErr.code
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

// errInvalidInvocation marks every failure that is the caller's fault rather
// than the repository's: a bad flag value, a wrong argument count, an
// unsupported command, a pair of flags that cannot be combined.
//
// This used to be decided by searching the error text for markers like
// "accepts " and "unknown flag". Matching on prose is the underlying defect,
// not the missing marker: pflag says `invalid argument "abc" for "--max-items"
// flag`, which matched nothing, so a bad flag value exited 1 as
// `operation_failed` against a contract that promises 2 and
// `invalid_invocation`. Marker lists also misfile the opposite way — a config
// file that cannot be read for permissions is not an invalid config. Errors now
// carry their classification instead of having it guessed back out of them.
var errInvalidInvocation = errors.New("invalid invocation")

// errDoctorFailed reports that doctor ran and something it checked is wrong.
// The report has already been written; this only carries the exit code.
var errDoctorFailed = errors.New("doctor reported a failing check")

// errTasksUnarchived reports that `tasks archive --check` found finished tasks
// misfiled outside archive/. The listing has already been written; this only
// carries the exit code.
var errTasksUnarchived = errors.New("finished tasks remain outside " + repoctx.TaskArchiveDir + "/")

func invalidInvocation(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", errInvalidInvocation, err)
}

// invalidInvocationArgs wraps a cobra positional-argument validator so an
// argument-count failure carries its exit code.
func invalidInvocationArgs(validator cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		return invalidInvocation(validator(cmd, args))
	}
}

func classifyError(err error) (int, string) {
	if errors.Is(err, stdcontext.Canceled) {
		return 130, "canceled"
	}
	if errors.Is(err, errInvalidInvocation) ||
		errors.Is(err, repoctx.ErrInvalidPacketLimit) ||
		errors.Is(err, repoctx.ErrInvalidScope) ||
		errors.Is(err, repoctx.ErrInvalidSeed) {
		return 2, "invalid_invocation"
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
	if errors.Is(err, repoctx.ErrInvalidPacket) {
		return 1, "invalid_packet"
	}
	if errors.Is(err, repoctx.ErrInvalidConfig) {
		return 1, "invalid_config"
	}
	if errors.Is(err, errDoctorFailed) {
		return 1, "diagnostic_failed"
	}
	if errors.Is(err, errVerificationFailed) {
		return 1, "verification_failed"
	}
	if errors.Is(err, errCapabilitiesUnsatisfied) {
		return 1, "capabilities_unsatisfied"
	}
	if errors.Is(err, errTasksUnarchived) {
		return 1, "tasks_unarchived"
	}
	if errors.Is(err, repoctx.ErrTaskNotFound) {
		return 1, "task_not_found"
	}
	if errors.Is(err, repoctx.ErrTaskAlreadyArchived) {
		return 1, "task_already_archived"
	}
	// Cobra reports an unknown subcommand from Find, before any hook this
	// program can install, so this one classification still reads the message.
	if strings.HasPrefix(err.Error(), "unknown command") {
		return 2, "invalid_invocation"
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
	// Inherited by every subcommand, so a flag-parsing failure anywhere in the
	// tree carries its exit-code classification instead of being recognised
	// from its wording.
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return invalidInvocation(err)
	})
	cmd.PersistentFlags().StringVar(&repoRoot, "root", ".", "Repository root to inspect")
	cmd.PersistentFlags().Bool("json-errors", false, "Emit structured errors on stderr")

	cmd.AddCommand(newInitCmd(&repoRoot))
	cmd.AddCommand(newScanCmd(&repoRoot))
	cmd.AddCommand(newBriefCmd(&repoRoot))
	cmd.AddCommand(newTasksCmd(&repoRoot))
	cmd.AddCommand(newSuggestInstructionsCmd(&repoRoot))
	cmd.AddCommand(newStatusCmd(&repoRoot))
	cmd.AddCommand(newExplainCmd(&repoRoot))
	cmd.AddCommand(newDiffCmd())
	cmd.AddCommand(newValidateCmd(&repoRoot))
	cmd.AddCommand(newDoctorCmd(&repoRoot))
	cmd.AddCommand(newMCPCmd(&repoRoot))
	cmd.AddCommand(newIntelCmd())
	cmd.AddCommand(newVerifyCmd())
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newCapabilitiesCmd())

	return cmd
}

const (
	capabilitiesSchema = "struktly/capabilities/v1"
	versionSchema      = "struktly/version/v1"
)

// versionDocument wraps build metadata so `version --json` names its schema
// like every other machine output. buildinfo.Info is embedded, so the existing
// fields keep their names and positions.
type versionDocument struct {
	Schema string `json:"schema"`
	buildinfo.Info
}

type capabilitiesDocument struct {
	Schema   string         `json:"schema"`
	Build    buildinfo.Info `json:"build"`
	Commands []string       `json:"commands"`
	Schemas  []string       `json:"schemas"`
	Features []string       `json:"features"`
}

func currentCapabilities() capabilitiesDocument {
	return capabilitiesDocument{
		Schema: capabilitiesSchema,
		Build:  buildinfo.Current(),
		Commands: []string{
			"capabilities", "context", "diff", "doctor", "explain", "init", "mcp",
			"scan", "status", "suggest-instructions", "tasks", "tasks archive",
			"tasks complete", "validate", "verify", "version",
		},
		Schemas: []string{
			capabilitiesSchema,
			// Markdown-only presentation identifiers, carried in OKF
			// frontmatter instead of JSON Schema files.
			"struktly/agent-instructions/v1",
			"struktly/project-context/v1",
			"struktly/doctor/v1",
			"struktly/error/v1",
			"struktly/explanation/v1",
			capabilityRequirementsSchema,
			initResultSchema,
			instructionSuggestionsSchema,
			recordBundleSchema,
			recordVerificationSchema,
			repoctx.PacketSchema,
			repoctx.PacketDiffSchema,
			repoctx.SnapshotSchema,
			repoctx.TaskArchiveSchema,
			repoctx.TaskTransitionSchema,
			repoctx.TasksSchema,
			"struktly/status/v1",
			"struktly/validation/v1",
			versionSchema,
		},
		Features: []string{
			"capabilities.require",
			"context.cancellation",
			"context.declaration_rendering",
			"context.scope",
			"context.seeds",
			"context.symbol_matching",
			"context.title_matching",
			"context.import_neighbors",
			"context.limits",
			"context.expect_base_revision",
			"context.no_write",
			"scan.no_write",
			"structured_errors",
			"tasks.archive",
			"tasks.complete",
			"tasks.partial_results",
		},
	}
}

func newTasksCmd(repoRoot *string) *cobra.Command {
	var toJSON bool
	cmd := &cobra.Command{
		Use:   "tasks",
		Short: "List repository task declarations",
		Args:  invalidInvocationArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			document, err := repoctx.DiscoverTasks(*repoRoot)
			if err != nil {
				return err
			}
			if toJSON {
				return writeJSON(cmd.OutOrStdout(), document)
			}
			out := newProse(cmd.OutOrStdout())
			for _, task := range document.Tasks {
				out.printf("%s\t%s\t%s\n", task.Status, task.Path, task.Title)
			}
			for _, invalid := range document.Invalid {
				out.printf("invalid\t%s\t%s\n", invalid.Path, invalid.Reason)
			}
			return out.err
		},
	}
	cmd.Flags().BoolVar(&toJSON, "json", false, "Print the versioned tasks document")
	cmd.AddCommand(newTasksArchiveCmd(repoRoot))
	cmd.AddCommand(newTasksCompleteCmd(repoRoot))
	return cmd
}

// newTasksArchiveCmd enforces the location invariant the task format states:
// the live tasks directory carries no done or canceled task. Frontmatter is
// the source of truth, so a finished task sitting live is misfiled, and this
// sweep files it — the migration and cleanup case; `tasks complete` is the
// transition that keeps the invariant from being violated in the first place.
func newTasksArchiveCmd(repoRoot *string) *cobra.Command {
	var toJSON bool
	var check bool
	cmd := &cobra.Command{
		Use:   "archive",
		Short: "File finished tasks under " + repoctx.TaskArchiveDir + "/ and repair links",
		Args:  invalidInvocationArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			document, err := repoctx.ArchiveTasks(repoctx.ArchiveTasksOptions{Root: *repoRoot, Check: check})
			if err != nil {
				return err
			}
			if toJSON {
				if err := writeJSON(cmd.OutOrStdout(), document); err != nil {
					return err
				}
			} else if err := writeTaskArchive(cmd.OutOrStdout(), document, check); err != nil {
				return err
			}
			// The report is the payload, so it is always written; the exit
			// code then tells a gate branching on --check whether the live
			// directory violates the invariant.
			if check && !document.Clean {
				return errTasksUnarchived
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&toJSON, "json", false, "Print the versioned task-archive document")
	cmd.Flags().BoolVar(&check, "check", false, "Report misfiled finished tasks without moving them")
	return cmd
}

func writeTaskArchive(w io.Writer, document repoctx.TaskArchiveDocument, check bool) error {
	if document.Clean {
		_, err := fmt.Fprintf(w, "nothing to archive (no finished tasks outside %s/)\n", repoctx.TaskArchiveDir)
		return err
	}
	if check {
		_, err := fmt.Fprintf(w, "%d finished task(s) misfiled outside %s/:\n", len(document.Archived), repoctx.TaskArchiveDir)
		if err != nil {
			return err
		}
		for _, move := range document.Archived {
			_, err := fmt.Fprintf(w, "  %s -> %s\n", move.From, move.To)
			if err != nil {
				return err
			}
		}
		_, err = fmt.Fprintln(w, "run: struktly tasks archive")
		return err
	}
	for _, move := range document.Archived {
		_, err := fmt.Fprintf(w, "archived %s -> %s\n", move.From, move.To)
		if err != nil {
			return err
		}
	}
	for _, rewrite := range document.Rewritten {
		_, err := fmt.Fprintf(w, "rewrote %d link(s) in %s\n", rewrite.Links, rewrite.Path)
		if err != nil {
			return err
		}
	}
	return nil
}

// newTasksCompleteCmd is the atomic done transition: frontmatter status and
// updated date, the move to archive/, and link repair land in one invocation,
// ordered so a failure leaves the original live file intact.
func newTasksCompleteCmd(repoRoot *string) *cobra.Command {
	var toJSON bool
	cmd := &cobra.Command{
		Use:   "complete <id>",
		Short: "Set a task's status to done and file it under " + repoctx.TaskArchiveDir + "/",
		Args:  invalidInvocationArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			document, err := repoctx.CompleteTask(repoctx.CompleteTaskOptions{Root: *repoRoot, ID: args[0]})
			if err != nil {
				return err
			}
			if toJSON {
				return writeJSON(cmd.OutOrStdout(), document)
			}
			_, writeErr := fmt.Fprintf(cmd.OutOrStdout(), "completed %s: %s -> %s\n", document.ID, document.From, document.To)
			if writeErr != nil {
				return writeErr
			}
			for _, rewrite := range document.Rewritten {
				_, writeErr = fmt.Fprintf(cmd.OutOrStdout(), "rewrote %d link(s) in %s\n", rewrite.Links, rewrite.Path)
			}
			return writeErr
		},
	}
	cmd.Flags().BoolVar(&toJSON, "json", false, "Print the versioned task-transition document")
	return cmd
}

func newCapabilitiesCmd() *cobra.Command {
	var toJSON bool
	var requirePath string
	cmd := &cobra.Command{
		Use:   "capabilities",
		Short: "Report supported machine interfaces",
		Args:  invalidInvocationArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			capabilities := currentCapabilities()

			// Read the requirements before writing anything: a malformed file
			// must not come back looking like an answer. Keyed on whether the
			// flag was given, so `--require=""` is a failed check rather than
			// a silently skipped one.
			var missing []string
			if cmd.Flags().Changed("require") {
				required, err := loadCapabilityRequirements(requirePath)
				if err != nil {
					return err
				}
				missing = unsatisfiedCapabilities(required, capabilities)
			}

			if toJSON {
				if err := writeJSON(cmd.OutOrStdout(), capabilities); err != nil {
					return err
				}
			} else {
				out := newProse(cmd.OutOrStdout())
				out.printf("struktly %s\n", capabilities.Build.Version)
				for _, feature := range capabilities.Features {
					out.printf("%s\n", feature)
				}
				if out.err != nil {
					return out.err
				}
			}

			// The document is the payload and is always written, as doctor
			// writes its report; the exit code then tells a caller branching on
			// it whether the build it holds is one it can drive.
			if len(missing) > 0 {
				return fmt.Errorf("%w: %s", errCapabilitiesUnsatisfied, strings.Join(missing, ", "))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&toJSON, "json", false, "Print the versioned capabilities document")
	cmd.Flags().StringVar(&requirePath, "require", "", "Fail unless this build satisfies the "+capabilityRequirementsSchema+" document at this path")
	return cmd
}

func newVersionCmd() *cobra.Command {
	var toJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print Struktly version and build metadata",
		Args:  invalidInvocationArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := buildinfo.Current()
			if toJSON {
				return writeJSON(cmd.OutOrStdout(), versionDocument{Schema: versionSchema, Info: info})
			}
			out := newProse(cmd.OutOrStdout())
			out.printf("struktly %s\n", info.Version)
			if info.Revision != "" {
				out.printf("revision: %s\n", info.Revision)
			}
			if info.Date != "" {
				out.printf("built: %s\n", info.Date)
			}
			return out.err
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
		Args:  invalidInvocationArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := app.Status(cmd.Context(), *repoRoot)
			if err != nil {
				return err
			}
			if toJSON {
				return writeJSON(cmd.OutOrStdout(), report)
			}
			out := newProse(cmd.OutOrStdout())
			out.printf("repository: %s (%s)\n", report.Repository.Name, report.Repository.HeadRevision)
			out.printf("branch: %s\n", emptyValue(report.Repository.Branch, "detached HEAD"))
			out.printf("config: %s\n", declaredValue(report.ConfigDeclared))
			for _, file := range report.PortableFiles {
				out.printf("%s: %s\n", file.Path, file.Status)
			}
			out.printf("%s: %s\n", report.LatestSnapshot.Path, report.LatestSnapshot.Status)
			return out.err
		},
	}
	cmd.Flags().BoolVar(&toJSON, "json", false, "Print structured status to stdout")
	return cmd
}

func newExplainCmd(repoRoot *string) *cobra.Command {
	var task string
	var scope string
	var toJSON bool
	cmd := &cobra.Command{
		Use:   "explain <path>",
		Short: "Experimental: explain context inclusion or exclusion",
		Args:  invalidInvocationArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			explanation, err := repoctx.ExplainSelection(cmd.Context(), *repoRoot, args[0], task, scope)
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
	cmd.Flags().StringVar(&scope, "scope", "", "Narrow selection to a repository-relative directory")
	cmd.Flags().BoolVar(&toJSON, "json", false, "Print structured explanation to stdout")
	return cmd
}

// newDiffCmd compares two packet files. It is the only context command that
// needs no repository: a packet is self-describing, so the comparison is a pure
// function of the two documents and works anywhere they can be read.
func newDiffCmd() *cobra.Command {
	var toJSON bool
	cmd := &cobra.Command{
		Use:   "diff <before.json> <after.json>",
		Short: "Compare two context packets",
		Args:  invalidInvocationArgs(cobra.ExactArgs(2)),
		RunE: func(cmd *cobra.Command, args []string) error {
			before, err := repoctx.LoadPacket(args[0])
			if err != nil {
				return err
			}
			after, err := repoctx.LoadPacket(args[1])
			if err != nil {
				return err
			}
			diff := repoctx.DiffPackets(before, after)
			if toJSON {
				return writeJSON(cmd.OutOrStdout(), diff)
			}
			return writePacketDiff(cmd.OutOrStdout(), diff)
		},
	}
	cmd.Flags().BoolVar(&toJSON, "json", false, "Print the versioned packet diff to stdout")
	return cmd
}

func writePacketDiff(w io.Writer, diff repoctx.PacketDiff) error {
	out := newProse(w)
	if diff.Identical {
		out.printf("identical packets (%s)\n", diff.PacketHash.Before)
		return out.err
	}
	out.printf("packet hash: %s -> %s\n", diff.PacketHash.Before, diff.PacketHash.After)
	for _, group := range []struct {
		label   string
		changes []repoctx.FieldChange
	}{
		{label: "repository", changes: diff.Repository},
		{label: "limits", changes: diff.Limits},
	} {
		for _, change := range group.changes {
			out.printf("%s %s: %s -> %s\n", group.label, change.Field,
				emptyValue(change.Before, "(none)"), emptyValue(change.After, "(none)"))
		}
	}

	out.printf("\nitems: %d unchanged, %d added, %d removed, %d changed\n",
		diff.Items.Unchanged, len(diff.Items.Added), len(diff.Items.Removed), len(diff.Items.Changed))
	for _, item := range diff.Items.Added {
		out.printf("  + %s\t%s%s\t%d bytes\n", item.Path, item.Reason, renderingSuffix(item.Rendering), item.IncludedBytes)
	}
	for _, item := range diff.Items.Removed {
		out.printf("  - %s\t%s\n", item.Path, item.Reason)
	}
	for _, item := range diff.Items.Changed {
		out.printf("  ~ %s\n", item.Path)
		for _, change := range item.Changes {
			out.printf("      %s: %s -> %s\n", change.Field,
				emptyValue(change.Before, "(none)"), emptyValue(change.After, "(none)"))
		}
	}

	for _, group := range []struct {
		label string
		set   repoctx.StringSetDiff
	}{
		{label: "required checks", set: diff.RequiredChecks},
		{label: "suggested checks", set: diff.SuggestedChecks},
	} {
		for _, value := range group.set.Added {
			out.printf("%s + %s\n", group.label, value)
		}
		for _, value := range group.set.Removed {
			out.printf("%s - %s\n", group.label, value)
		}
	}

	for _, group := range []struct {
		label string
		set   repoctx.DecisionDiff
	}{
		{label: "exclusion", set: diff.Exclusions},
		{label: "truncation", set: diff.Truncations},
	} {
		for _, decision := range group.set.Added {
			out.printf("%s + %s\t%s\n", group.label, decision.Path, decision.Reason)
		}
		for _, decision := range group.set.Removed {
			out.printf("%s - %s\t%s\n", group.label, decision.Path, decision.Reason)
		}
	}
	return out.err
}

func renderingSuffix(rendering string) string {
	if rendering == "" {
		return ""
	}
	return " (" + rendering + ")"
}

func newValidateCmd(repoRoot *string) *cobra.Command {
	var toJSON bool
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Experimental: validate configuration and task files",
		Args:  invalidInvocationArgs(cobra.NoArgs),
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
		Args:  invalidInvocationArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := app.Doctor(cmd.Context(), *repoRoot)
			if err != nil {
				return err
			}
			if toJSON {
				if err := writeJSON(cmd.OutOrStdout(), report); err != nil {
					return err
				}
			} else {
				out := newProse(cmd.OutOrStdout())
				for _, check := range report.Checks {
					out.printf("[%s] %s", check.Status, check.Name)
					if check.Message != "" {
						out.printf(": %s", check.Message)
					}
					out.newline()
				}
				if out.err != nil {
					return out.err
				}
			}
			// The report is the payload, so it is always written; the exit code
			// then tells a caller branching on it whether anything failed.
			if report.HasFailure() {
				return errDoctorFailed
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
	var toJSON bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create repository configuration and write project context",
		Args:  invalidInvocationArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInit(cmd, *repoRoot, toJSON)
		},
	}
	cmd.Flags().BoolVar(&toJSON, "json", false, "Print the versioned init result to stdout")
	return cmd
}

// initDocument is the machine contract for init: what was created, what was
// deliberately left alone, and where the scan snapshot went, all repository-
// relative. Prose output stays for people; a caller that must not parse prose
// — Platform is one — reads this instead.
type initDocument struct {
	Schema   string   `json:"schema"`
	Root     string   `json:"root"`
	Created  []string `json:"created"`
	Skipped  []string `json:"skipped"`
	Snapshot string   `json:"snapshot"`
}

const initResultSchema = "struktly/init-result/v1"

func runInit(cmd *cobra.Command, repoRoot string, toJSON bool) error {
	result, err := app.Init(app.InitOptions{Root: repoRoot})
	if err != nil {
		return err
	}

	root := result.Root
	if toJSON {
		document := initDocument{Schema: initResultSchema, Root: portableRoot, Created: []string{}, Skipped: []string{}}
		for _, path := range result.CreatedPaths {
			document.Created = append(document.Created, relToRoot(root, path))
		}
		for _, path := range result.SkippedPaths {
			document.Skipped = append(document.Skipped, relToRoot(root, path))
		}
		document.Snapshot = relToRoot(root, result.ScanOutputPath)
		return writeJSON(cmd.OutOrStdout(), document)
	}

	out := newProse(cmd.OutOrStdout())
	for _, path := range result.CreatedPaths {
		out.printf("created %s\n", relToRoot(root, path))
	}
	for _, path := range result.SkippedPaths {
		out.printf("kept %s (already exists)\n", relToRoot(root, path))
	}
	out.printf("wrote %s\n", relToRoot(root, result.ScanOutputPath))
	return out.err
}

func newScanCmd(repoRoot *string) *cobra.Command {
	var toJSON bool
	var noWrite bool
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Write .struktly/project-context.md for a repository",
		Args:  invalidInvocationArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if noWrite && !toJSON {
				return invalidInvocation(errors.New("--no-write requires --json"))
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
	var scope string
	var seeds []string
	var maxItems int
	var maxFileBytes int
	var maxTotalBytes int
	cmd := &cobra.Command{
		Use:     "context <request>",
		Aliases: []string{"brief"},
		Short:   "Build a context packet for one coding request",
		Args:    invalidInvocationArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			if toStdout && toJSON {
				return invalidInvocation(errors.New("use --stdout for Markdown or --json for the structured packet, not both"))
			}
			if noWrite && !toJSON {
				return invalidInvocation(errors.New("--no-write requires --json"))
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
				Scope:                scope,
				Seeds:                seeds,
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
	cmd.Flags().StringVar(&scope, "scope", "", "Narrow selection to a repository-relative directory")
	cmd.Flags().StringArrayVar(&seeds, "seed", nil, "Include a known-relevant file; repeatable")
	cmd.Flags().IntVar(&maxItems, "max-items", 0, "Maximum selected files")
	cmd.Flags().IntVar(&maxFileBytes, "max-file-bytes", 0, "Maximum bytes to read from each selected file")
	cmd.Flags().IntVar(&maxTotalBytes, "max-total-bytes", 0, "Maximum total selected content bytes")
	return cmd
}

func newSuggestInstructionsCmd(repoRoot *string) *cobra.Command {
	var toJSON bool
	cmd := &cobra.Command{
		Use:   "suggest-instructions",
		Short: "Write suggested agent instruction drafts under .struktly/agent-instructions/",
		Args:  invalidInvocationArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSuggestInstructions(cmd, *repoRoot, toJSON)
		},
	}
	cmd.Flags().BoolVar(&toJSON, "json", false, "Print the versioned suggestion result to stdout")
	return cmd
}

// suggestionsDocument is the machine contract for suggest-instructions: the
// draft files written, repository-relative. The drafts themselves stay
// Markdown on disk where a person edits them; the document says where they
// are, not what they say.
type suggestionsDocument struct {
	Schema  string   `json:"schema"`
	Root    string   `json:"root"`
	Written []string `json:"written"`
}

const instructionSuggestionsSchema = "struktly/instruction-suggestions/v1"

func runSuggestInstructions(cmd *cobra.Command, repoRoot string, toJSON bool) error {
	result, err := repoctx.SuggestInstructions(repoctx.SuggestInstructionsOptions{
		Root: repoRoot,
	})
	if err != nil {
		return err
	}

	root := result.Root
	if toJSON {
		document := suggestionsDocument{Schema: instructionSuggestionsSchema, Root: portableRoot, Written: []string{}}
		for _, path := range result.OutputPaths {
			document.Written = append(document.Written, relToRoot(root, path))
		}
		return writeJSON(cmd.OutOrStdout(), document)
	}

	out := newProse(cmd.OutOrStdout())
	for _, path := range result.OutputPaths {
		out.printf("wrote %s\n", relToRoot(root, path))
	}
	return out.err
}

// portableRoot is how a machine document names the root its paths are relative
// to. The absolute root is workstation-specific, so it is never emitted; every
// other repository document uses this same value.
const portableRoot = "."

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

// prose writes the human-readable half of a command. It keeps the first write
// error and skips every write after it, so a command checks once at the end.
type prose struct {
	w   io.Writer
	err error
}

func newProse(w io.Writer) *prose { return &prose{w: w} }

func (p *prose) printf(format string, args ...any) {
	if p.err != nil {
		return
	}
	_, p.err = fmt.Fprintf(p.w, format, args...)
}

func (p *prose) newline() { p.printf("\n") }
