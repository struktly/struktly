package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	repoctx "github.com/struktly/struktly/internal/context"
)

const (
	statusSchema     = "struktly/status/v1"
	validationSchema = "struktly/validation/v1"
	doctorSchema     = "struktly/doctor/v1"
	configPath       = ".struktly/config.json"
)

type FileStatus struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type StatusReport struct {
	Schema         string             `json:"schema"`
	Repository     repoctx.Repository `json:"repository"`
	ConfigDeclared bool               `json:"config_declared"`
	ConfigPath     string             `json:"config_path"`
	PortableFiles  []FileStatus       `json:"portable_files"`
	LatestSnapshot FileStatus         `json:"latest_snapshot"`
	Warnings       []string           `json:"warnings"`
}

type ValidationReport struct {
	Schema         string             `json:"schema"`
	Valid          bool               `json:"valid"`
	Repository     repoctx.Repository `json:"repository"`
	ConfigDeclared bool               `json:"config_declared"`
	Config         repoctx.Config     `json:"config"`
	Tasks          []repoctx.Task     `json:"tasks"`
}

type DoctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type DoctorReport struct {
	Schema     string             `json:"schema"`
	Repository repoctx.Repository `json:"repository"`
	Checks     []DoctorCheck      `json:"checks"`
}

func Status(ctx context.Context, root string) (StatusReport, error) {
	repository, err := repoctx.ResolveRepository(ctx, root)
	if err != nil {
		return StatusReport{}, err
	}
	_, declared, err := repoctx.LoadConfig(repository.AbsoluteRoot())
	if err != nil {
		return StatusReport{}, err
	}

	paths := []string{
		configPath,
		".struktly/direction.md",
		".struktly/constraints.md",
		".struktly/decisions.md",
	}
	portable := make([]FileStatus, 0, len(paths))
	for _, path := range paths {
		status, err := inspectFile(repository.AbsoluteRoot(), path)
		if err != nil {
			return StatusReport{}, err
		}
		portable = append(portable, status)
	}
	latest, err := inspectFile(repository.AbsoluteRoot(), ".struktly/scans/latest.json")
	if err != nil {
		return StatusReport{}, err
	}

	return StatusReport{
		Schema:         statusSchema,
		Repository:     repository,
		ConfigDeclared: declared,
		ConfigPath:     configPath,
		PortableFiles:  portable,
		LatestSnapshot: latest,
		Warnings:       []string{},
	}, nil
}

func Validate(ctx context.Context, root string) (ValidationReport, error) {
	repository, err := repoctx.ResolveRepository(ctx, root)
	if err != nil {
		return ValidationReport{}, err
	}
	config, declared, err := repoctx.LoadConfig(repository.AbsoluteRoot())
	if err != nil {
		return ValidationReport{}, err
	}
	tasks, err := repoctx.LoadTasks(repository.AbsoluteRoot())
	if err != nil {
		return ValidationReport{}, err
	}
	return ValidationReport{
		Schema:         validationSchema,
		Valid:          true,
		Repository:     repository,
		ConfigDeclared: declared,
		Config:         config,
		Tasks:          tasks,
	}, nil
}

// Doctor diagnoses the repository and reports each check.
//
// It used to return early when the repository would not resolve, so
// `git_repository` could only ever report "pass" — the one outcome that told a
// reader nothing the command's own success had not already told them. A
// diagnostic that refuses to run when something is wrong cannot diagnose it, so
// the failure is now a check result. Callers keep the exit code they had:
// HasFailure reports whether any check failed, and the command exits 1 when it
// does, which also fixes a failing config check exiting 0.
func Doctor(ctx context.Context, root string) (DoctorReport, error) {
	report := DoctorReport{Schema: doctorSchema}

	repository, err := repoctx.ResolveRepository(ctx, root)
	if err != nil {
		if ctx.Err() != nil {
			return DoctorReport{}, ctx.Err()
		}
		report.Checks = append(report.Checks, DoctorCheck{
			Name: "git_repository", Status: "fail", Message: err.Error(),
		})
		return report, nil
	}
	report.Repository = repository
	report.Checks = append(report.Checks, DoctorCheck{
		Name: "git_repository", Status: "pass", Message: repository.Identity,
	})

	_, declared, configErr := repoctx.LoadConfig(repository.AbsoluteRoot())
	switch {
	case configErr != nil:
		report.Checks = append(report.Checks, DoctorCheck{Name: "config", Status: "fail", Message: configErr.Error()})
	case declared:
		report.Checks = append(report.Checks, DoctorCheck{Name: "config", Status: "pass"})
	default:
		report.Checks = append(report.Checks, DoctorCheck{Name: "config", Status: "pass", Message: "using built-in defaults"})
	}

	return report, nil
}

// HasFailure reports whether any diagnostic check failed.
func (r DoctorReport) HasFailure() bool {
	for _, check := range r.Checks {
		if check.Status == "fail" {
			return true
		}
	}
	return false
}

func inspectFile(root, path string) (FileStatus, error) {
	status := FileStatus{Path: path, Status: "missing"}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err == nil {
		status.Status = "present"
	} else if !os.IsNotExist(err) {
		return FileStatus{}, fmt.Errorf("inspect %s: %w", path, err)
	}
	return status, nil
}
