package context

import (
	stdcontext "context"
	"time"
)

type ScanOptions struct {
	Root    string
	Now     time.Time
	NoWrite bool
}

type ScanResult struct {
	OutputPath   string
	SnapshotPath string
	Snapshot     Snapshot
}

type BriefOptions struct {
	Context              stdcontext.Context
	Root                 string
	Task                 string
	Now                  time.Time
	NoWrite              bool
	ExpectedBaseRevision string
	MaxItems             int
	MaxFileBytes         int
	MaxTotalBytes        int
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
	// Root is the anchored repository root, which is not necessarily the
	// requested one. Callers report paths relative to it.
	Root        string
	OutputPaths []string
}
