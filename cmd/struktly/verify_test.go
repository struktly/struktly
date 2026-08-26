package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sha256Hex(payload string) string {
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// bundleFor builds a bundle the way Platform exports one: the sealed bytes
// verbatim, and the digest that was taken of exactly those bytes.
func bundleFor(t *testing.T, sealed string, digest string) string {
	t.Helper()
	document := map[string]any{
		"schema": "struktly/record-bundle/v1",
		"manifest": map[string]any{
			"execution_id":   "run_abc",
			"provenance_id":  "provenance_1",
			"revision":       2,
			"disposition":    "completed",
			"payload_schema": "struktly/provenance/v1",
			"payload_sha256": digest,
			"sealed_at":      "2026-08-13T10:00:00Z",
			// A real evidence digest, not a stub: schemas/record-bundle.v1.json
			// requires 64 lowercase hex characters because that is what a
			// sha256 of the evidence snapshot is, and a fixture shorter than
			// the contract tests a bundle the platform cannot produce.
			"evidence_sha256":   "e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0",
			"evidence_embedded": false,
		},
		"sealed":      json.RawMessage(sealed),
		"exported_at": "2026-08-13T11:00:00Z",
	}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "bundle.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVerifyAcceptsAnIntactBundle(t *testing.T) {
	sealed := `{"schema":"struktly/provenance/v1","execution_id":"run_abc","revision":2}`
	path := bundleFor(t, sealed, sha256Hex(sealed))

	stdout, stderr, err := executeTestCommand("verify", path)
	if err != nil {
		t.Fatalf("verify: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "[pass] payload") || !strings.Contains(stdout, "intact") {
		t.Fatalf("report did not confirm the payload:\n%s", stdout)
	}
	// An intact bundle must still say what it does not prove.
	if !strings.Contains(stdout, "not checked:") {
		t.Fatalf("report claimed more than it checked:\n%s", stdout)
	}
}

// The property the whole export exists for: one flipped byte stops verifying.
func TestVerifyRefusesATamperedPayload(t *testing.T) {
	sealed := `{"schema":"struktly/provenance/v1","execution_id":"run_abc","revision":2}`
	digest := sha256Hex(sealed)
	tampered := strings.Replace(sealed, `"revision":2`, `"revision":3`, 1)
	path := bundleFor(t, tampered, digest)

	stdout, _, err := executeTestCommand("verify", path)
	if err == nil {
		t.Fatalf("verify accepted a tampered payload:\n%s", stdout)
	}
	if !strings.Contains(stdout, "[fail] payload") || !strings.Contains(stdout, "NOT intact") {
		t.Fatalf("report did not name the tampering:\n%s", stdout)
	}
}

// Bytes with no digest cannot be checked at all, which is worse than a
// mismatch: a mismatch is an answer.
func TestVerifyRefusesABundleWithNoDigest(t *testing.T) {
	sealed := `{"schema":"struktly/provenance/v1","execution_id":"run_abc","revision":2}`
	path := bundleFor(t, sealed, "")

	stdout, _, err := executeTestCommand("verify", path)
	if err == nil {
		t.Fatalf("verify accepted a bundle carrying no digest:\n%s", stdout)
	}
	if !strings.Contains(stdout, "records no digest") {
		t.Fatalf("report did not explain why it could not check:\n%s", stdout)
	}
}

func TestVerifyJSONReportIsVersioned(t *testing.T) {
	sealed := `{"schema":"struktly/provenance/v1","execution_id":"run_abc","revision":2}`
	path := bundleFor(t, sealed, sha256Hex(sealed))

	stdout, _, err := executeTestCommand("verify", path, "--json")
	if err != nil {
		t.Fatalf("verify --json: %v", err)
	}
	var report struct {
		Schema       string   `json:"schema"`
		Intact       bool     `json:"intact"`
		Unverifiable []string `json:"unverifiable"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("report is not JSON: %v\n%s", err, stdout)
	}
	if report.Schema != "struktly/record-verification/v1" || !report.Intact {
		t.Fatalf("report = %+v", report)
	}
	if len(report.Unverifiable) == 0 {
		t.Fatal("a machine reader must also be told what was not checked")
	}
}
