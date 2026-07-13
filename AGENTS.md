# Struktly CLI working guidance

This repository is the independently useful open-source Struktly CLI.

Keep the CLI independently installable, deterministic, local-first, and useful
without an account, a model provider, or another repository. Other products may
use the CLI executable and stable file formats; do not add imports of their internals.

Before substantial work, read `docs/roadmap.md`, `docs/compatibility.md`, and the
accepted ADRs under `docs/adr/`. Also inspect `.struktly/tasks/` for a matching
ready or in-progress task; its frontmatter contains the assigned agent and
resumable session details when present.
