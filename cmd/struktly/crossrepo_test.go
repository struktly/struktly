package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The producer-consumer contract, against documents the platform actually
// produced.
//
// `struktly/record-bundle/v1` is published here and honoured there, which makes
// it the one contract in this repository that a test written here can get
// wrong in a way no test here would notice: a fixture authored to match the
// schema proves the schema matches itself. So these bundles are not authored.
// They are the exact bytes `governance.ProvenanceService.Export` returned, with
// the producing revision recorded in testdata/PROVENANCE.md.
//
// What this repository owns is the consuming half. Regenerating the fixtures is
// the platform's, and deliberately not automated from here — a generator on the
// consumer's side would let the consumer decide what the producer emits, which
// is the failure the whole contract exists to prevent.
func realBundle(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return raw
}

func verifyBytes(t *testing.T, raw []byte) verificationReport {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bundle.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := verifyRecordBundle(path)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	return report
}

func check(t *testing.T, report verificationReport, name string) verificationCheck {
	t.Helper()
	for _, c := range report.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no %q check in report: %+v", name, report.Checks)
	return verificationCheck{}
}

// An untouched platform export verifies, in both observed states of the one
// optional field.
func TestRealPlatformBundlesVerify(t *testing.T) {
	for _, fixture := range []struct {
		file           string
		evidenceDigest bool
	}{
		{"record-bundle-with-evidence.json", true},
		{"record-bundle-no-evidence.json", false},
	} {
		t.Run(fixture.file, func(t *testing.T) {
			raw := realBundle(t, fixture.file)
			var document map[string]any
			if err := json.Unmarshal(raw, &document); err != nil {
				t.Fatal(err)
			}
			manifest, _ := document["manifest"].(map[string]any)
			_, present := manifest["evidence_sha256"]
			if present != fixture.evidenceDigest {
				t.Fatalf("evidence_sha256 present = %v, want %v — the fixture no longer "+
					"exercises the state it was retained for", present, fixture.evidenceDigest)
			}

			report := verifyBytes(t, raw)
			if !report.Intact {
				t.Fatalf("a real untouched export did not verify: %+v", report.Checks)
			}
			for _, name := range []string{"schema", "contract", "payload", "consistency"} {
				if got := check(t, report, name); got.Status != "pass" {
					t.Fatalf("%s = %s: %s", name, got.Status, got.Message)
				}
			}
		})
	}
}

// A malformed bundle fails the contract, and fails it before the arithmetic.
//
// Derived from a real export rather than authored, so the only difference
// between this document and one that verifies is the defect under test.
func TestAMalformedManifestFailsTheContract(t *testing.T) {
	var document map[string]any
	if err := json.Unmarshal(realBundle(t, "record-bundle-with-evidence.json"), &document); err != nil {
		t.Fatal(err)
	}
	manifest := document["manifest"].(map[string]any)
	delete(manifest, "payload_sha256")
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}

	report := verifyBytes(t, raw)
	if report.Intact {
		t.Fatal("a manifest with no payload digest was reported intact")
	}
	contract := check(t, report, "contract")
	if contract.Status != "fail" {
		t.Fatalf("contract = %s, want fail: %s", contract.Status, contract.Message)
	}
	if !strings.Contains(contract.Message, "payload_sha256") {
		t.Fatalf("the contract failure does not name the missing field: %q", contract.Message)
	}
}

// A tampered bundle keeps its shape and loses its digest.
//
// This is the distinction the two checks exist for: a producer's bug and
// evidence of alteration are different findings, and a reader who cannot tell
// them apart cannot act on either.
func TestATamperedPayloadKeepsItsShapeAndFailsTheDigest(t *testing.T) {
	raw := realBundle(t, "record-bundle-with-evidence.json")
	// Alter one character inside the sealed bytes, leaving valid JSON and a
	// valid bundle shape.
	text := string(raw)
	at := strings.Index(text, `"sealed":`)
	if at < 0 {
		t.Fatal("fixture has no sealed member")
	}
	key := strings.Index(text[at:], `"schema"`)
	if key < 0 {
		t.Fatal("could not find a byte to alter inside the sealed payload")
	}
	position := at + key + 2
	altered := text[:position] + string(rune('z')) + text[position+1:]

	report := verifyBytes(t, []byte(altered))
	if report.Intact {
		t.Fatal("an altered payload was reported intact")
	}
	if got := check(t, report, "contract"); got.Status != "pass" {
		t.Fatalf("tampering changed the shape; this no longer isolates the digest: %s", got.Message)
	}
	payload := check(t, report, "payload")
	if payload.Status != "fail" {
		t.Fatalf("payload = %s, want fail", payload.Status)
	}
	if !strings.Contains(payload.Message, "sealed as") {
		t.Fatalf("the digest failure does not report both digests: %q", payload.Message)
	}
}

// Re-encoding an untouched bundle breaks it, and the report must not call that
// tampering without evidence.
//
// Retained because it is the failure most likely to be met in the wild and
// least likely to be understood: `jq .`, an editor formatting on save, or a
// producer using MarshalIndent all invalidate a Record nobody altered.
func TestReencodingAnUntouchedBundleBreaksTheDigest(t *testing.T) {
	var document map[string]any
	if err := json.Unmarshal(realBundle(t, "record-bundle-with-evidence.json"), &document); err != nil {
		t.Fatal(err)
	}
	indented, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	report := verifyBytes(t, indented)
	if report.Intact {
		t.Fatal("a re-indented bundle verified; the digest cannot cover the sealed bytes")
	}
	if got := check(t, report, "contract"); got.Status != "pass" {
		t.Fatalf("re-indenting changed the shape as well: %s", got.Message)
	}
}
