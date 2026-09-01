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

func TestRealPlatformJudgementsReachHumanAndJSONReports(t *testing.T) {
	path := filepath.Join("testdata", "record-bundle-with-evidence.json")

	stdout, stderr, err := executeTestCommand("verify", path)
	if err != nil {
		t.Fatalf("verify: %v\nstderr:\n%s", err, stderr)
	}
	for _, want := range []string{
		"judgements: carried inside the verified payload — integrity and consistency only; correctness not checked",
		"compliance: violated by machine at 2026-07-13T12:00:02Z",
		"docs/product/PLAN.md@abc123:42",
		"decision dec_escalation [rule_check]: escalation_answered by human at 2026-07-13T12:00:04Z",
		"escalation esc_test [answered]",
		"answer by local-user at 2026-07-13T12:00:04Z: Intended: the check now covers the new section.",
		"not checked: whether the work the Record describes is correct",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("human report omitted %q:\n%s", want, stdout)
		}
	}

	stdout, stderr, err = executeTestCommand("verify", "--json", path)
	if err != nil {
		t.Fatalf("verify --json: %v\nstderr:\n%s", err, stderr)
	}
	var report verificationReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("report is not JSON: %v\n%s", err, stdout)
	}
	judgements := report.Judgements
	if !judgements.Available || judgements.Unavailable != "" {
		t.Fatalf("judgements availability = %#v", judgements)
	}
	if judgements.Compliance == nil || judgements.Compliance.Outcome != "violated" ||
		judgements.Compliance.Actor != "machine" || judgements.Compliance.DecidedAt != "2026-07-13T12:00:02Z" ||
		judgements.Compliance.Reason == "" {
		t.Fatalf("compliance judgement = %#v", judgements.Compliance)
	}
	if len(judgements.Compliance.Verdicts) != 2 {
		t.Fatalf("cited rules = %#v", judgements.Compliance.Verdicts)
	}
	cited := judgements.Compliance.Verdicts[0]
	if cited.StatementID != "stmt_failing" || cited.Verdict != "failed" || cited.Text == "" ||
		cited.Path != "docs/product/PLAN.md" || cited.Revision != "abc123" || cited.Line != 42 {
		t.Fatalf("cited rule = %#v", cited)
	}
	if len(judgements.Decisions) != 3 || judgements.Decisions[0].Actor != "machine" ||
		judgements.Decisions[2].Actor != "human" || judgements.Decisions[2].Outcome != "escalation_answered" {
		t.Fatalf("durable decisions = %#v", judgements.Decisions)
	}
	if len(judgements.Escalations) != 1 || !judgements.Escalations[0].Answered ||
		judgements.Escalations[0].Answer != "Intended: the check now covers the new section." ||
		judgements.Escalations[0].AnsweredBy != "local-user" ||
		judgements.Escalations[0].AnsweredAt != "2026-07-13T12:00:04Z" {
		t.Fatalf("answered escalation = %#v", judgements.Escalations)
	}
}

func TestVerifiedJudgementsKeepKnownEmptyApartFromUnavailable(t *testing.T) {
	knownEmpty := rewriteSealedPayload(t, func(sealed map[string]any) {
		sealed["judgements"] = map[string]any{
			"compliance":  nil,
			"decisions":   []any{},
			"escalations": []any{},
			"limitations": []any{"No compliance verdict has been recorded."},
		}
	})
	known := verifyBytes(t, knownEmpty)
	if !known.Intact || !known.Judgements.Available || known.Judgements.Compliance != nil ||
		known.Judgements.Decisions == nil || len(known.Judgements.Decisions) != 0 ||
		known.Judgements.Escalations == nil || len(known.Judgements.Escalations) != 0 {
		t.Fatalf("known-empty judgements = %#v", known.Judgements)
	}

	oldPayload := rewriteSealedPayload(t, func(sealed map[string]any) {
		delete(sealed, "judgements")
	})
	old := verifyBytes(t, oldPayload)
	if !old.Intact || old.Judgements.Available || old.Judgements.Unavailable == "" ||
		old.Judgements.Decisions != nil || old.Judgements.Escalations != nil {
		t.Fatalf("unavailable judgements = %#v", old.Judgements)
	}

	knownJSON, err := json.Marshal(known)
	if err != nil {
		t.Fatal(err)
	}
	oldJSON, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(knownJSON), `"decisions":[]`) ||
		!strings.Contains(string(oldJSON), `"decisions":null`) {
		t.Fatalf("machine report collapsed known-empty and unavailable:\nknown %s\nold %s", knownJSON, oldJSON)
	}
	// The published schema has to admit both states, or it would flatten them.
	assertDocumentConforms(t, "record-verification.v1.json", knownJSON)
	assertDocumentConforms(t, "record-verification.v1.json", oldJSON)

	var knownHuman, oldHuman strings.Builder
	if err := writeVerificationReport(&knownHuman, known); err != nil {
		t.Fatal(err)
	}
	if err := writeVerificationReport(&oldHuman, old); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(knownHuman.String(), "compliance: no recorded verdict") ||
		!strings.Contains(knownHuman.String(), "decisions: none") ||
		!strings.Contains(knownHuman.String(), "escalations: none") ||
		!strings.Contains(oldHuman.String(), "judgements: unavailable") {
		t.Fatalf("human report collapsed known-empty and unavailable:\nknown:\n%s\nold:\n%s",
			knownHuman.String(), oldHuman.String())
	}
}

func TestVerifiedJudgementsPreserveAnOpenEscalation(t *testing.T) {
	raw := rewriteSealedPayload(t, func(sealed map[string]any) {
		judgements := sealed["judgements"].(map[string]any)
		judgements["escalations"] = []any{map[string]any{
			"id": "esc_open", "kind": "missing_authority",
			"question": "Who may approve this exception?", "answered": false,
			"raised_by": "scheduler", "raised_at": "2026-07-13T12:00:09Z",
			"evidence_embedded": false,
		}}
	})
	report := verifyBytes(t, raw)
	if !report.Intact || len(report.Judgements.Escalations) != 1 {
		t.Fatalf("open escalation report = %#v", report)
	}
	open := report.Judgements.Escalations[0]
	if open.Answered || open.Answer != "" || open.AnsweredBy != "" || open.AnsweredAt != "" {
		t.Fatalf("open escalation became answered: %#v", open)
	}
	var human strings.Builder
	if err := writeVerificationReport(&human, report); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(human.String(), "escalation esc_open [open]") ||
		strings.Contains(human.String(), "answer by") {
		t.Fatalf("open escalation rendering =\n%s", human.String())
	}
}

func TestAlteringAJudgementByteFailsBeforeItIsReported(t *testing.T) {
	raw := realBundle(t, "record-bundle-with-evidence.json")
	text := string(raw)
	answer := "Intended: the check now covers the new section."
	at := strings.Index(text, answer)
	if at < 0 {
		t.Fatalf("fixture carries no judgement answer %q", answer)
	}
	altered := text[:at] + "X" + text[at+1:]

	report := verifyBytes(t, []byte(altered))
	if report.Intact {
		t.Fatal("an altered judgement was reported intact")
	}
	if got := check(t, report, "contract"); got.Status != "pass" {
		t.Fatalf("altering one judgement byte changed the shape: %s", got.Message)
	}
	if got := check(t, report, "payload"); got.Status != "fail" || !strings.Contains(got.Message, "sealed as") {
		t.Fatalf("payload = %s: %s", got.Status, got.Message)
	}
	if report.Judgements.Available {
		t.Fatalf("the verifier reported a judgement before its bytes verified: %#v", report.Judgements)
	}
}

func rewriteSealedPayload(t *testing.T, mutate func(map[string]any)) []byte {
	t.Helper()
	var document map[string]json.RawMessage
	if err := json.Unmarshal(realBundle(t, "record-bundle-with-evidence.json"), &document); err != nil {
		t.Fatal(err)
	}
	var sealed map[string]any
	if err := json.Unmarshal(document["sealed"], &sealed); err != nil {
		t.Fatal(err)
	}
	mutate(sealed)
	encodedSealed, err := json.Marshal(sealed)
	if err != nil {
		t.Fatal(err)
	}
	document["sealed"] = encodedSealed

	var manifest map[string]any
	if err := json.Unmarshal(document["manifest"], &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["payload_sha256"] = sha256Hex(string(encodedSealed))
	encodedManifest, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	document["manifest"] = encodedManifest

	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
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
