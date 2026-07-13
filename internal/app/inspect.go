package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

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

func Doctor(ctx context.Context, root string) (DoctorReport, error) {
	repository, err := repoctx.ResolveRepository(ctx, root)
	if err != nil {
		return DoctorReport{}, err
	}

	checks := []DoctorCheck{{Name: "git_repository", Status: "pass"}}
	_, declared, configErr := repoctx.LoadConfig(repository.AbsoluteRoot())
	switch {
	case configErr != nil:
		checks = append(checks, DoctorCheck{Name: "config", Status: "fail", Message: configErr.Error()})
	case declared:
		checks = append(checks, DoctorCheck{Name: "config", Status: "pass"})
	default:
		checks = append(checks, DoctorCheck{Name: "config", Status: "pass", Message: "using built-in defaults"})
	}

	for _, path := range []string{".struktly/runs", ".struktly/memory/candidates"} {
		check := DoctorCheck{Name: path, Status: "pass"}
		if _, err := os.Stat(filepath.Join(repository.AbsoluteRoot(), filepath.FromSlash(path))); err == nil {
			check.Status = "warning"
			check.Message = "legacy runtime state is stored inside the repository"
		} else if !os.IsNotExist(err) {
			check.Status = "fail"
			check.Message = fmt.Sprintf("inspect %s: %v", path, err)
		}
		checks = append(checks, check)
	}
	sort.Slice(checks, func(i, j int) bool { return checks[i].Name < checks[j].Name })

	return DoctorReport{Schema: doctorSchema, Repository: repository, Checks: checks}, nil
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
