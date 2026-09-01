# Constraints

- Generated context is deterministic. Nothing may depend on wall-clock time, map
  iteration order, or the order a filesystem returns entries in.
- Exclusions are load-bearing rather than tuning. Detected secrets, sensitive
  names, binaries, symlinks, Git-ignored files, dependencies, build output, and
  oversized content stay out of a packet; loosening one is a security change and
  needs to be argued as one.
- JSON is the stable machine surface and Markdown is presentation. A consumer
  negotiates through `capabilities` and ignores unknown fields.
- What `capabilities` advertises describes this binary. An advertised command
  resolves and an advertised schema is a published file, held by test; a
  feature identifier is added only with the proof that establishes it.
- Repository writes stay under `.struktly/`, and `--no-write` writes nothing.
- A person's text is data at every boundary. A request, a task id, or a path
  must never be able to become a flag.
- Pre-1.0 there is one live version of each schema. Breaking changes are made in
  place and recorded in the changelog, and that rule expires at 1.0 or at the
  first external consumer.
