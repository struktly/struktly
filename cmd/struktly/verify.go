package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/struktly/struktly/internal/schema"
	"github.com/struktly/struktly/schemas"
)

// Verifying a Struktly Record without Struktly.
//
// A record of what an agent did is worth what its reader can check. This
// command re-derives the digest of the sealed bytes and compares it with the
// digest recorded when they were sealed, so a person who received a bundle
// trusts arithmetic rather than whoever sent it.
//
// What it proves is exact and narrow: that the bundle is intact and internally
// consistent — the bytes are the ones the digest describes. It does not, and
// cannot, say the work was correct, that the checks were the right checks, or
// that the machine which produced it was honest about anything outside the
// bundle. Saying more than this would make the verification itself a claim
// requiring trust, which is the thing it exists to remove.

const (
	recordBundleSchema       = "struktly/record-bundle/v1"
	recordVerificationSchema = "struktly/record-verification/v1"
)

// errVerificationFailed reports that the bundle did not verify. The report is
// written first; this only carries the exit code.
var errVerificationFailed = errors.New("record bundle did not verify")

type recordBundle struct {
	Schema   string `json:"schema"`
	Manifest struct {
		ExecutionID      string `json:"execution_id"`
		ProvenanceID     string `json:"provenance_id"`
		Revision         int    `json:"revision"`
		Disposition      string `json:"disposition"`
		PayloadSchema    string `json:"payload_schema"`
		PayloadSHA256    string `json:"payload_sha256"`
		SealedAt         string `json:"sealed_at"`
		EvidenceSHA256   string `json:"evidence_sha256"`
		EvidenceEmbedded bool   `json:"evidence_embedded"`
	} `json:"manifest"`
	Sealed json.RawMessage `json:"sealed"`
}

// verificationCheck is one thing that was checked and what it said.
type verificationCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type verificationReport struct {
	Schema string              `json:"schema"`
	Path   string              `json:"path"`
	Intact bool                `json:"intact"`
	Checks []verificationCheck `json:"checks"`
	// Record identifies what was verified, so a reader can tell two bundles
	// apart without opening them.
	Record struct {
		ExecutionID string `json:"execution_id"`
		Revision    int    `json:"revision"`
		Disposition string `json:"disposition"`
		SealedAt    string `json:"sealed_at,omitempty"`
	} `json:"record"`
	// Unverifiable states what this bundle does not let anybody check, so an
	// intact result is not read as a broader guarantee than it is.
	Unverifiable []string `json:"unverifiable"`
}

func (r verificationReport) failed() bool {
	for _, check := range r.Checks {
		if check.Status == "fail" {
			return true
		}
	}
	return false
}

func newVerifyCmd() *cobra.Command {
	var toJSON bool
	cmd := &cobra.Command{
		Use:   "verify <bundle.json>",
		Short: "Check that an exported Struktly Record is intact",
		Args:  invalidInvocationArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := verifyRecordBundle(args[0])
			if err != nil {
				return err
			}
			if toJSON {
				if err := writeJSON(cmd.OutOrStdout(), report); err != nil {
					return err
				}
			} else if err := writeVerificationReport(cmd.OutOrStdout(), report); err != nil {
				return err
			}
			// The report is the payload and is always written; the exit code
			// then tells a caller branching on it what the report said.
			if !report.Intact {
				return errVerificationFailed
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&toJSON, "json", false, "Print the versioned verification report to stdout")
	return cmd
}

func verifyRecordBundle(path string) (verificationReport, error) {
	report := verificationReport{Schema: recordVerificationSchema, Path: path, Checks: []verificationCheck{}}
	file, err := os.Open(path)
	if err != nil {
		return verificationReport{}, invalidInvocation(fmt.Errorf("read bundle: %w", err))
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(file)
	if err != nil {
		return verificationReport{}, fmt.Errorf("read bundle: %w", err)
	}

	var bundle recordBundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return verificationReport{}, invalidInvocation(fmt.Errorf("bundle is not JSON: %w", err))
	}
	report.Record.ExecutionID = bundle.Manifest.ExecutionID
	report.Record.Revision = bundle.Manifest.Revision
	report.Record.Disposition = bundle.Manifest.Disposition
	report.Record.SealedAt = bundle.Manifest.SealedAt

	if bundle.Schema == recordBundleSchema {
		report.Checks = append(report.Checks, verificationCheck{Name: "schema", Status: "pass", Message: bundle.Schema})
	} else {
		report.Checks = append(report.Checks, verificationCheck{
			Name: "schema", Status: "fail",
			Message: fmt.Sprintf("bundle declares %q, this build verifies %q", bundle.Schema, recordBundleSchema),
		})
	}

	// The shape, against the published contract rather than against a second
	// description of it in this file. schemas/record-bundle.v1.json is what a
	// producer is held to and what a third party reads; validating the document
	// against that exact file is what stops the two from drifting.
	//
	// Separate from the digest check below, and deliberately so: "this is not a
	// Record bundle" and "this is a Record bundle somebody altered" are
	// different answers, and a reader who cannot tell them apart cannot act on
	// either. A malformed document fails here and never reaches the arithmetic.
	if contract, err := schemas.Bytes("record-bundle.v1.json"); err != nil {
		report.Checks = append(report.Checks, verificationCheck{
			Name: "contract", Status: "fail",
			Message: "this build carries no published schema to check the bundle against: " + err.Error(),
		})
	} else if err := schema.ValidateJSON(contract, raw); err != nil {
		report.Checks = append(report.Checks, verificationCheck{
			Name: "contract", Status: "fail", Message: err.Error(),
		})
	} else {
		report.Checks = append(report.Checks, verificationCheck{
			Name: "contract", Status: "pass", Message: "struktly/record-bundle/v1",
		})
	}

	switch {
	case len(bundle.Sealed) == 0:
		report.Checks = append(report.Checks, verificationCheck{
			Name: "payload", Status: "fail", Message: "the bundle carries no sealed payload",
		})
	case bundle.Manifest.PayloadSHA256 == "":
		// Bytes with no digest cannot be checked at all, which is worse than a
		// mismatch: a mismatch is an answer.
		report.Checks = append(report.Checks, verificationCheck{
			Name: "payload", Status: "fail", Message: "the manifest records no digest for the sealed payload",
		})
	default:
		derived := sha256.Sum256(bundle.Sealed)
		hexDigest := hex.EncodeToString(derived[:])
		if hexDigest == bundle.Manifest.PayloadSHA256 {
			report.Checks = append(report.Checks, verificationCheck{
				Name: "payload", Status: "pass", Message: hexDigest,
			})
		} else {
			report.Checks = append(report.Checks, verificationCheck{
				Name: "payload", Status: "fail",
				Message: fmt.Sprintf("sealed as %s, these bytes are %s", bundle.Manifest.PayloadSHA256, hexDigest),
			})
		}
	}

	// The payload declares its own schema and identity; a bundle whose manifest
	// disagrees with the document it wraps is inconsistent even when both
	// halves are individually well formed.
	var payload struct {
		Schema      string `json:"schema"`
		ExecutionID string `json:"execution_id"`
		Revision    int    `json:"revision"`
	}
	if len(bundle.Sealed) > 0 && json.Unmarshal(bundle.Sealed, &payload) == nil {
		switch {
		case payload.ExecutionID != "" && payload.ExecutionID != bundle.Manifest.ExecutionID,
			payload.Revision != 0 && payload.Revision != bundle.Manifest.Revision:
			report.Checks = append(report.Checks, verificationCheck{
				Name: "consistency", Status: "fail",
				Message: fmt.Sprintf("manifest describes %s revision %d, the sealed Record says %s revision %d",
					bundle.Manifest.ExecutionID, bundle.Manifest.Revision, payload.ExecutionID, payload.Revision),
			})
		default:
			report.Checks = append(report.Checks, verificationCheck{
				Name: "consistency", Status: "pass",
				Message: "the manifest and the sealed Record describe the same revision",
			})
		}
	}

	report.Unverifiable = []string{
		"whether the work the Record describes is correct",
		"anything the Record itself states it could not capture",
	}
	if !bundle.Manifest.EvidenceEmbedded {
		note := "the evidence snapshot is not in this bundle"
		if bundle.Manifest.EvidenceSHA256 != "" {
			note += ", only the digest it was sealed with"
		}
		report.Unverifiable = append(report.Unverifiable, note)
	}
	report.Intact = !report.failed()
	return report, nil
}

func writeVerificationReport(w io.Writer, report verificationReport) error {
	for _, check := range report.Checks {
		line := fmt.Sprintf("[%s] %s", check.Status, check.Name)
		if check.Message != "" {
			line += ": " + check.Message
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	verdict := "intact"
	if !report.Intact {
		verdict = "NOT intact"
	}
	if _, err := fmt.Fprintf(w, "\n%s — execution %s, revision %d\n",
		verdict, report.Record.ExecutionID, report.Record.Revision); err != nil {
		return err
	}
	for _, note := range report.Unverifiable {
		if _, err := fmt.Fprintf(w, "not checked: %s\n", note); err != nil {
			return err
		}
	}
	return nil
}
