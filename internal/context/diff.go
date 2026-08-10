package context

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
)

// PacketDiffSchema identifies the comparison of two context packets.
const PacketDiffSchema = "struktly/packet-diff/v1"

// ErrInvalidPacket marks a file that is not a readable context packet, as
// distinct from one that could not be read at all.
var ErrInvalidPacket = errors.New("invalid context packet")

// maxPacketFileBytes bounds a packet file read. A packet holds at most 512 KiB
// of selected content, which JSON escaping can inflate, so the ceiling is well
// clear of a legitimate packet and still refuses an arbitrary large file.
const maxPacketDocumentBytes = 16 << 20

// FieldChange records one field that differs, rendered as text so numbers,
// hashes and identifiers share one shape.
type FieldChange struct {
	Field  string `json:"field"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// ItemSummary describes a selected file without reproducing it. A diff names
// what was selected and why; it never carries content, so comparing two packets
// cannot disclose what reading either of them would not.
type ItemSummary struct {
	Path          string `json:"path"`
	Reason        string `json:"reason"`
	Rendering     string `json:"rendering,omitempty"`
	IncludedBytes int    `json:"included_bytes"`
}

type ItemChange struct {
	Path    string        `json:"path"`
	Changes []FieldChange `json:"changes"`
}

type ItemDiff struct {
	Added     []ItemSummary `json:"added"`
	Removed   []ItemSummary `json:"removed"`
	Changed   []ItemChange  `json:"changed"`
	Unchanged int           `json:"unchanged"`
}

type StringSetDiff struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
}

type DecisionDiff struct {
	Added   []PacketDecision `json:"added"`
	Removed []PacketDecision `json:"removed"`
}

// PacketDiff explains what changed between two context packets.
type PacketDiff struct {
	Schema     string        `json:"schema"`
	Identical  bool          `json:"identical"`
	PacketHash FieldChange   `json:"packet_hash"`
	Repository []FieldChange `json:"repository"`
	Limits     []FieldChange `json:"limits"`
	Items      ItemDiff      `json:"items"`
	// RequiredChecks and SuggestedChecks change independently of selection, and
	// a check that quietly disappeared between two packets is the kind of thing
	// this command exists to surface.
	RequiredChecks  StringSetDiff `json:"required_checks"`
	SuggestedChecks StringSetDiff `json:"suggested_checks"`
	Exclusions      DecisionDiff  `json:"exclusions"`
	Truncations     DecisionDiff  `json:"truncations"`
}

// LoadPacket reads one packet JSON document.
func LoadPacket(path string) (Packet, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Packet{}, fmt.Errorf("read packet %s: %w", path, err)
	}
	if info.Size() > maxPacketDocumentBytes {
		return Packet{}, fmt.Errorf("%w: %s exceeds %d bytes", ErrInvalidPacket, path, maxPacketDocumentBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Packet{}, fmt.Errorf("read packet %s: %w", path, err)
	}
	var packet Packet
	if err := json.Unmarshal(data, &packet); err != nil {
		return Packet{}, fmt.Errorf("%w: %s: %w", ErrInvalidPacket, path, err)
	}
	// One live version of each format, so a document naming another schema is
	// rejected rather than guessed at. See docs/compatibility.md.
	if packet.Schema != PacketSchema {
		return Packet{}, fmt.Errorf("%w: %s declares schema %q, want %q",
			ErrInvalidPacket, path, packet.Schema, PacketSchema)
	}
	return packet, nil
}

// DiffPackets reports what changed from before to after.
//
// Packet identity already answers "is this the same context?" in one
// comparison. What it cannot answer is "what moved?", which is the question
// anyone asks next — after a commit, after a configuration change, or after a
// change to the selector itself.
func DiffPackets(before, after Packet) PacketDiff {
	diff := PacketDiff{
		Schema:     PacketDiffSchema,
		Identical:  before.PacketHash == after.PacketHash,
		PacketHash: FieldChange{Field: "packet_hash", Before: before.PacketHash, After: after.PacketHash},
		Repository: diffFields([]FieldChange{
			{Field: "identity", Before: before.Repository.Identity, After: after.Repository.Identity},
			{Field: "branch", Before: before.Repository.Branch, After: after.Repository.Branch},
			{Field: "head_revision", Before: before.Repository.HeadRevision, After: after.Repository.HeadRevision},
			{Field: "base_revision", Before: before.Repository.BaseRevision, After: after.Repository.BaseRevision},
		}),
		Limits: diffFields([]FieldChange{
			{Field: "max_items", Before: itoa(before.Limits.MaxItems), After: itoa(after.Limits.MaxItems)},
			{Field: "max_file_bytes", Before: itoa(before.Limits.MaxFileBytes), After: itoa(after.Limits.MaxFileBytes)},
			{Field: "max_total_bytes", Before: itoa(before.Limits.MaxTotalBytes), After: itoa(after.Limits.MaxTotalBytes)},
		}),
		Items:           diffItems(before.Items, after.Items),
		RequiredChecks:  diffStringSet(before.RequiredChecks, after.RequiredChecks),
		SuggestedChecks: diffStringSet(before.SuggestedChecks, after.SuggestedChecks),
		Exclusions:      diffDecisions(before.Exclusions, after.Exclusions),
		Truncations:     diffDecisions(before.Truncations, after.Truncations),
	}
	return diff
}

func diffFields(candidates []FieldChange) []FieldChange {
	changes := []FieldChange{}
	for _, candidate := range candidates {
		if candidate.Before != candidate.After {
			changes = append(changes, candidate)
		}
	}
	return changes
}

func diffItems(before, after []PacketItem) ItemDiff {
	diff := ItemDiff{Added: []ItemSummary{}, Removed: []ItemSummary{}, Changed: []ItemChange{}}
	beforeByPath := make(map[string]PacketItem, len(before))
	for _, item := range before {
		beforeByPath[item.Path] = item
	}
	afterByPath := make(map[string]PacketItem, len(after))
	for _, item := range after {
		afterByPath[item.Path] = item
	}

	for _, item := range after {
		previous, existed := beforeByPath[item.Path]
		if !existed {
			diff.Added = append(diff.Added, summarizeItem(item))
			continue
		}
		changes := diffFields([]FieldChange{
			{Field: "reason", Before: previous.Reason, After: item.Reason},
			{Field: "content_hash", Before: previous.ContentHash, After: item.ContentHash},
			{Field: "rendering", Before: previous.Rendering, After: item.Rendering},
			{Field: "included_bytes", Before: itoa(previous.IncludedBytes), After: itoa(item.IncludedBytes)},
			{Field: "original_bytes", Before: itoa64(previous.OriginalBytes), After: itoa64(item.OriginalBytes)},
			{Field: "truncated", Before: btoa(previous.Truncated), After: btoa(item.Truncated)},
			{Field: "provenance.location", Before: previous.Provenance.Location, After: item.Provenance.Location},
		})
		if len(changes) == 0 {
			diff.Unchanged++
			continue
		}
		diff.Changed = append(diff.Changed, ItemChange{Path: item.Path, Changes: changes})
	}
	for _, item := range before {
		if _, survives := afterByPath[item.Path]; !survives {
			diff.Removed = append(diff.Removed, summarizeItem(item))
		}
	}

	sort.Slice(diff.Added, func(i, j int) bool { return diff.Added[i].Path < diff.Added[j].Path })
	sort.Slice(diff.Removed, func(i, j int) bool { return diff.Removed[i].Path < diff.Removed[j].Path })
	sort.Slice(diff.Changed, func(i, j int) bool { return diff.Changed[i].Path < diff.Changed[j].Path })
	return diff
}

func summarizeItem(item PacketItem) ItemSummary {
	return ItemSummary{
		Path:          item.Path,
		Reason:        item.Reason,
		Rendering:     item.Rendering,
		IncludedBytes: item.IncludedBytes,
	}
}

func diffStringSet(before, after []string) StringSetDiff {
	diff := StringSetDiff{Added: []string{}, Removed: []string{}}
	beforeSet := make(map[string]struct{}, len(before))
	for _, value := range before {
		beforeSet[value] = struct{}{}
	}
	afterSet := make(map[string]struct{}, len(after))
	for _, value := range after {
		afterSet[value] = struct{}{}
	}
	for _, value := range after {
		if _, existed := beforeSet[value]; !existed {
			diff.Added = append(diff.Added, value)
		}
	}
	for _, value := range before {
		if _, survives := afterSet[value]; !survives {
			diff.Removed = append(diff.Removed, value)
		}
	}
	sort.Strings(diff.Added)
	sort.Strings(diff.Removed)
	return diff
}

func diffDecisions(before, after []PacketDecision) DecisionDiff {
	diff := DecisionDiff{Added: []PacketDecision{}, Removed: []PacketDecision{}}
	key := func(d PacketDecision) string { return d.Path + "\x00" + d.Reason }
	beforeSet := make(map[string]struct{}, len(before))
	for _, decision := range before {
		beforeSet[key(decision)] = struct{}{}
	}
	afterSet := make(map[string]struct{}, len(after))
	for _, decision := range after {
		afterSet[key(decision)] = struct{}{}
	}
	for _, decision := range after {
		if _, existed := beforeSet[key(decision)]; !existed {
			diff.Added = append(diff.Added, decision)
		}
	}
	for _, decision := range before {
		if _, survives := afterSet[key(decision)]; !survives {
			diff.Removed = append(diff.Removed, decision)
		}
	}
	sortDecisions(diff.Added)
	sortDecisions(diff.Removed)
	return diff
}

func itoa(value int) string     { return strconv.Itoa(value) }
func itoa64(value int64) string { return strconv.FormatInt(value, 10) }
func btoa(value bool) string    { return strconv.FormatBool(value) }
