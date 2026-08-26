# Where these bundles came from

Neither file is hand-authored. Both are the exact bytes returned by
`governance.ProvenanceService.Export` in `struktly-platform`, serialized with
`json.Marshal` — compact, as the platform's HTTP boundary encodes them.

| file | evidence_sha256 | produced from |
| --- | --- | --- |
| `record-bundle-with-evidence.json` | present, 64 hex | a sealed Record whose execution has a readable evidence document |
| `record-bundle-no-evidence.json` | **key absent** | the same, with the execution's evidence row deleted so `loadEvidence` takes its no-rows path |

Produced at `struktly-platform` revision:

    009b396f198cc8c1d762e076a05d127d681f80a0

Consumed by this repository at:

    8de299447973345e6c10dd2d06422a950d91dddd

**Do not reformat these files.** `sealed` is raw JSON whose SHA-256 is recorded
in the manifest, so the digest covers those exact bytes. Running them through a
pretty-printer — `jq .`, an editor's format-on-save, `json.MarshalIndent` —
invalidates an otherwise untouched bundle and the failure looks identical to
tampering.

## Regenerating

Regeneration is a `struktly-platform` concern and is not automated from here:
this repository is the consumer of the contract, and a generator living on the
consumer's side would let the consumer decide what the producer emits. The
platform-side generator is recorded as a dependency in
`.struktly/tasks/record-bundle-cross-repository-fixture.md` there.
