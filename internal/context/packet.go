package context

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// PacketSchema identifies the machine-readable brief result format; see
// docs/compatibility.md.
const PacketSchema = "struktly/packet/v1"

type PacketMemoryItem struct {
	Content        string   `json:"content"`
	Scope          string   `json:"scope,omitempty"`
	SourceRunID    string   `json:"source_run_id,omitempty"`
	SourceArtifact string   `json:"source_artifact,omitempty"`
	Tags           []string `json:"tags,omitempty"`
}

// Packet is the structured counterpart to the context-packet markdown Brief
// writes. Consumers that need to act on a brief programmatically (the GUI,
// MCP) should read this rather than parsing markdown.
type Packet struct {
	Schema               string             `json:"schema"`
	GeneratedAt          time.Time          `json:"generated_at"`
	Metadata             PacketMetadata     `json:"metadata"`
	Repository           Repository         `json:"repository"`
	Items                []PacketItem       `json:"items"`
	InstructionFiles     []string           `json:"instruction_files,omitempty"`
	RequiredChecks       []string           `json:"required_checks"`
	SuggestedChecks      []string           `json:"suggested_checks"`
	Exclusions           []PacketDecision   `json:"exclusions"`
	Truncations          []PacketDecision   `json:"truncations"`
	Limits               PacketLimits       `json:"limits"`
	PacketHash           string             `json:"packet_hash"`
	Task                 string             `json:"task"`
	Direction            string             `json:"direction,omitempty"`
	Constraints          string             `json:"constraints,omitempty"`
	Decisions            string             `json:"decisions,omitempty"`
	Evidence             string             `json:"evidence,omitempty"`
	ApprovedMemory       []PacketMemoryItem `json:"approved_memory,omitempty"`
	VerificationCommands []string           `json:"verification_commands"`
	Docs                 []string           `json:"docs,omitempty"`
	SuggestedFiles       []string           `json:"suggested_files"`
	MissingContext       []string           `json:"missing_context,omitempty"`
	ReadWarnings         []string           `json:"read_warnings,omitempty"`
	SourceRefs           []string           `json:"source_refs"`
}

type packetHashInput struct {
	Schema               string               `json:"schema"`
	Repository           packetHashRepository `json:"repository"`
	Items                []PacketItem         `json:"items"`
	InstructionFiles     []string             `json:"instruction_files"`
	RequiredChecks       []string             `json:"required_checks"`
	SuggestedChecks      []string             `json:"suggested_checks"`
	Exclusions           []PacketDecision     `json:"exclusions"`
	Truncations          []PacketDecision     `json:"truncations"`
	Limits               PacketLimits         `json:"limits"`
	Task                 string               `json:"task"`
	Direction            string               `json:"direction,omitempty"`
	Constraints          string               `json:"constraints,omitempty"`
	Decisions            string               `json:"decisions,omitempty"`
	Evidence             string               `json:"evidence,omitempty"`
	ApprovedMemory       []PacketMemoryItem   `json:"approved_memory,omitempty"`
	VerificationCommands []string             `json:"verification_commands"`
	Docs                 []string             `json:"docs,omitempty"`
	SuggestedFiles       []string             `json:"suggested_files"`
	MissingContext       []string             `json:"missing_context,omitempty"`
	ReadWarnings         []string             `json:"read_warnings,omitempty"`
	SourceRefs           []string             `json:"source_refs"`
}

type packetHashRepository struct {
	Identity     string `json:"identity"`
	VCS          string `json:"vcs"`
	Root         string `json:"root"`
	Branch       string `json:"branch,omitempty"`
	HeadRevision string `json:"head_revision"`
	BaseRevision string `json:"base_revision,omitempty"`
}

func (p *Packet) setHash() error {
	canonical := packetHashInput{
		Schema: p.Schema,
		Repository: packetHashRepository{
			Identity: p.Repository.Identity, VCS: p.Repository.VCS, Root: p.Repository.Root,
			Branch: p.Repository.Branch, HeadRevision: p.Repository.HeadRevision, BaseRevision: p.Repository.BaseRevision,
		},
		Items:                p.Items,
		InstructionFiles:     p.InstructionFiles,
		RequiredChecks:       p.RequiredChecks,
		SuggestedChecks:      p.SuggestedChecks,
		Exclusions:           p.Exclusions,
		Truncations:          p.Truncations,
		Limits:               p.Limits,
		Task:                 p.Task,
		Direction:            p.Direction,
		Constraints:          p.Constraints,
		Decisions:            p.Decisions,
		Evidence:             p.Evidence,
		ApprovedMemory:       p.ApprovedMemory,
		VerificationCommands: p.VerificationCommands,
		Docs:                 p.Docs,
		SuggestedFiles:       p.SuggestedFiles,
		MissingContext:       p.MissingContext,
		ReadWarnings:         p.ReadWarnings,
		SourceRefs:           p.SourceRefs,
	}
	data, err := json.Marshal(canonical)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	p.PacketHash = "sha256:" + hex.EncodeToString(digest[:])
	return nil
}
