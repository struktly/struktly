package context

import (
	stdcontext "context"
	"time"
)

type ScanOptions struct {
	Root  string
	RunID string
	Now   time.Time
}

type ScanResult struct {
	OutputPath   string
	SnapshotPath string
	Snapshot     Snapshot
}

type BriefOptions struct {
	Context stdcontext.Context
	Root    string
	Task    string
	RunID   string
	Now     time.Time
}

type BriefResult struct {
	OutputPath string
	PacketPath string
	Packet     Packet
}

type SuggestInstructionsOptions struct {
	Root string
	Now  time.Time
}

type SuggestInstructionsResult struct {
	OutputPaths []string
}
