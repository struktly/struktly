package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"

	"github.com/struktly/struktly/internal/schema"
	"github.com/struktly/struktly/schemas"
)

// Asking a build whether it is good enough, instead of being told.
//
// `capabilities --json` has always reported what a build supports, and a
// consumer gating on this CLI has always had to hold its own copy of what it
// needs and compare the two itself. That copy is the problem. It sits in
// somebody else's repository with no test behind it, it is the thing that
// decides whether a version bump ships, and it is written in whatever language
// that gate happens to be in.
//
// `--require` moves the comparison to the side that knows the answer. The
// consumer still states what it needs — that is its statement to make, and this
// repository must never guess it — but it states it once, as data, and the
// binary answers with an exit code.
//
// The requirements document is validated against
// schemas/capability-requirements.v1.json rather than against a second
// description of it here, for the reason schemas/schemas.go gives: two
// expressions of one contract, and the second drifts.

const capabilityRequirementsSchema = "struktly/capability-requirements/v1"

// errCapabilitiesUnsatisfied reports that a build was asked whether it meets a
// caller's requirements and does not.
//
// It is exit 1 rather than 2 because the invocation was correct and the answer
// is a fact about this binary: a caller gating on it wants "no, and here is
// what is missing", which is an outcome, not a usage error.
var errCapabilitiesUnsatisfied = errors.New("this build does not satisfy the required capabilities")

// capabilityRequirements is what a consumer needs, in the same three
// categories capabilities reports. Every category is optional; requiring
// nothing at all is not.
type capabilityRequirements struct {
	Schema   string   `json:"schema"`
	Commands []string `json:"commands,omitempty"`
	Schemas  []string `json:"schemas,omitempty"`
	Features []string `json:"features,omitempty"`
}

// loadCapabilityRequirements reads and checks a caller-supplied requirements
// file.
//
// Every failure here is the caller's: a path that is not there, a document that
// is not JSON, a key that is not in the contract, a file that asks for nothing.
// So they classify as an invalid invocation and exit 2, and none of them
// produce a capabilities document — answering a question that was not asked
// properly is how a gate comes to believe it checked something.
func loadCapabilityRequirements(path string) (capabilityRequirements, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return capabilityRequirements{}, invalidInvocation(fmt.Errorf("read --require: %w", err))
	}
	contract, err := schemas.Bytes("capability-requirements.v1.json")
	if err != nil {
		return capabilityRequirements{}, fmt.Errorf("this build carries no published schema for --require: %w", err)
	}
	if err := schema.ValidateJSON(contract, raw); err != nil {
		return capabilityRequirements{}, invalidInvocation(fmt.Errorf("invalid %s: %w", path, err))
	}
	var required capabilityRequirements
	if err := json.Unmarshal(raw, &required); err != nil {
		return capabilityRequirements{}, invalidInvocation(fmt.Errorf("invalid %s: %w", path, err))
	}
	if len(required.Commands)+len(required.Schemas)+len(required.Features) == 0 {
		return capabilityRequirements{}, invalidInvocation(fmt.Errorf(
			"%s requires no command, schema or feature, so it would pass against any build", path))
	}
	return required, nil
}

// unsatisfiedCapabilities returns every required entry this build does not
// advertise.
//
// All of them, in the order the document lists them, because a gate that
// reported only the first would be answered one bump at a time — and the
// question a consumer is really asking is whether it can move, not which
// single thing to fix next.
func unsatisfiedCapabilities(required capabilityRequirements, advertised capabilitiesDocument) []string {
	var missing []string
	for _, group := range []struct {
		kind       string
		need, have []string
	}{
		{kind: "command", need: required.Commands, have: advertised.Commands},
		{kind: "schema", need: required.Schemas, have: advertised.Schemas},
		{kind: "feature", need: required.Features, have: advertised.Features},
	} {
		for _, name := range group.need {
			if !slices.Contains(group.have, name) {
				missing = append(missing, group.kind+" "+strconv.Quote(name))
			}
		}
	}
	return missing
}
