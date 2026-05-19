# Plan: `claude-profile diff` Command

## Context

The user wants to compare the current `~/.claude/settings.json` against the effective settings that a profile would produce if applied, without mutating disk. The diff implementation must mirror the existing merge behavior in [`main.go`](/Users/lossend/pro/claude-profile/main.go): start from an empty map, merge `common/` with `mergeIntoExisting`, merge the profile layer directory with `mergeIntoExisting`, then merge `secrets/<name>.json` with `mergeMaps` when present. Do not refer to a nonexistent `mergeProfile` helper.

## Implementation

### Command: `claude-profile diff <name>`

**Flags:**
- `--source` — override settings.json path (default: `~/.claude/settings.json`)
- `--json` — emit structured JSON instead of human-readable output
- `--show-secrets` — opt in to printing raw sensitive values; default output must redact them

**Cobra wiring:**
- Add `newDiffCmd()` and register it in the root command
- Set `ValidArgsFunction: completeProfileNames`
- Default `--source` the same way other commands default their path flags
- Pass both `stdout` and `stderr` into `app.diffProfile(...)` so warnings and output are separated cleanly

### Core Logic (`app.diffProfile`)

1. Verify the repo directories exist and the profile manifest exists.
2. Resolve the source path:
   - If `--source` is empty, use `filepath.Join(app.home, ".claude", "settings.json")`
   - Read the current settings from that path
   - If the path is invalid or unreadable, return the read error
3. Build the merged profile output using the exact `applyProfile` sequence:
   - `mergeIntoExisting(map[string]any{}, filepath.Join(a.repoRoot, "common"), nil)`
   - `mergeIntoExisting(merged, a.profileLayersDir(name), nil)`
   - `readOptionalJSONFile(secretPath)` and then:
     - if it returns an error, return the error
     - if it returns `nil`, write `warning: secret override <path> not found` to `stderr` and continue
     - otherwise merge with `mergeMaps`
4. Compute a symmetric diff between the current settings and the merged profile result.
5. Redact sensitive values by default in both output modes. Only show raw values when `--show-secrets` is set.
6. Write output with `writeDiffHuman(...)` or `writeDiffJSON(...)`. Both functions must return `error`, and `app.diffProfile` must propagate those errors.

## Diff Model

Define a `diffEntry` struct with:
- `Path string`
- `Kind string` (`added`, `removed`, `modified`)
- `Current any`
- `Profile any`

Use JSON Pointer (RFC 6901) for all paths:
- `/env/ANTHROPIC_MODEL`
- `/hooks/preToolUse`
- `/permissions/allow`

Escaping rules:
- `~` becomes `~0`
- `/` becomes `~1`

The root pointer is the empty string `""` if the whole document ever needs to be represented.

## Diff Algorithm

Only recurse when **both** sides at the current path are `map[string]any`. This must match the replacement semantics in `mergeIntoExisting` / `mergeMaps` / `mergeValue`:

- If both sides are maps, walk the sorted union of keys and recurse by child JSON Pointer.
- If the values are equal (`reflect.DeepEqual`), emit nothing.
- Otherwise, emit a **single** entry at the current path.

Important consequences:
- `null` transitions are whole-value changes at the parent path.
- Map-to-scalar changes are whole-value changes at the parent path.
- Scalar-to-map changes are whole-value changes at the parent path.
- Array changes are whole-value changes at the parent path.
- Do **not** flatten nested additions, removals, type changes, or array changes into leaf entries.

That means:
- If `/env` changes from an object to a string, emit one `modified` entry at `/env`
- If `/permissions/allow` changes array contents, emit one `modified` entry at `/permissions/allow`
- If a nested object exists only on one side, emit one `added` or `removed` entry at that object path with the full object value

## Secret Redaction

Default behavior must never print raw secret values in human output or JSON output.

Plan the output layer so it:
- Detects sensitive fields using the same key heuristic already used in [`main.go`](/Users/lossend/pro/claude-profile/main.go) (`TOKEN`, `PASSWORD`, `SECRET`, `*_KEY`)
- Masks sensitive values before writing output unless `--show-secrets` is enabled
- Applies redaction recursively so nested secret-bearing objects and arrays do not leak raw values

Recommended display contract:
- Human output: print a stable masked placeholder such as `"[REDACTED]"`
- JSON output: store the masked placeholder in `current` / `profile` fields unless `--show-secrets` is enabled

## Output Format

Human-readable (default):

```text
Diff: ~/.claude/settings.json ↔ profile "work"

  /env/ANTHROPIC_MODEL:
    - current: "claude-sonnet-4-20250514"
    + profile: "claude-opus-4-20250514"

  /permissions/allow:
    - current: ["Read", "Edit"]
    + profile: ["Read", "Edit", "Bash(npm run build)"]

  /env/OPENAI_API_KEY:
    - current: "[REDACTED]"
    + profile: "[REDACTED]"
```

Do not include element-level array examples such as `permissions.allow[2]`.

JSON (`--json`):

```json
{
  "source": "~/.claude/settings.json",
  "profile": "work",
  "entries": [
    {
      "path": "/env/ANTHROPIC_MODEL",
      "kind": "modified",
      "current": "claude-sonnet-4-20250514",
      "profile": "claude-opus-4-20250514"
    },
    {
      "path": "/env/OPENAI_API_KEY",
      "kind": "modified",
      "current": "[REDACTED]",
      "profile": "[REDACTED]"
    }
  ]
}
```

Empty diff contract in JSON mode:
- Still emit a valid JSON object
- `entries` must be `[]`, not `null`
- Do not print a human-only "No differences" line when `--json` is set

## Files to Modify

- [`main.go`](/Users/lossend/pro/claude-profile/main.go) — add `newDiffCmd()`, `app.diffProfile(...)`, JSON Pointer diff helpers, redaction, and output writers
- [`main_test.go`](/Users/lossend/pro/claude-profile/main_test.go) — add command and diff behavior coverage
- [`README.md`](/Users/lossend/pro/claude-profile/README.md) — document `diff`, `--source`, `--json`, `--show-secrets`, JSON Pointer paths, and missing-secret warning behavior

## New / Changed Functions

| Function | Purpose |
|----------|---------|
| `newDiffCmd()` | Cobra command setup with `ValidArgsFunction: completeProfileNames` |
| `app.diffProfile(stdout, stderr io.Writer, name, sourcePath string, jsonOutput, showSecrets bool) error` | Orchestrates merge, source read, diff, redaction, warnings, and output |
| `computeDiffEntries(current, profile any, path string) []diffEntry` | Recurses only when both sides are maps; otherwise emits a whole-value entry at the current path |
| `joinJSONPointer(base, token string) string` | Appends and escapes JSON Pointer tokens per RFC 6901 |
| `writeDiffHuman(w io.Writer, sourcePath, profileName string, entries []diffEntry) error` | Writes human-readable diff and propagates write failures |
| `writeDiffJSON(w io.Writer, sourcePath, profileName string, entries []diffEntry) error` | Encodes JSON diff and propagates encoder/write failures |
| `redactDiffEntries(entries []diffEntry, showSecrets bool) []diffEntry` | Masks sensitive `current` / `profile` values unless opt-in is enabled |
| `formatValue(v any) string` | Marshals a display-safe value for human output |
| `mergedKeySet(a, b map[string]any) []string` | Sorted union of keys for deterministic map recursion |

Remove the old `flattenValue(...)` idea from the plan. It conflicts with the required whole-value replacement semantics.

## Design Decisions

- Reuse the real merge helpers: `mergeIntoExisting`, `mergeMaps`, and `mergeValue`
- Recurse only for map-vs-map comparisons; everything else is a whole-value replacement at the current JSON Pointer
- Arrays are compared as whole values only
- Secrets are redacted by default; `--show-secrets` is an explicit opt-in escape hatch
- Missing secret files warn on `stderr` and do not fail the command, matching `applyProfile`
- Output is deterministic via sorted key traversal
- Output writers return `error` so write failures cannot be silently dropped

## Test Plan

1. No differences in human mode:
   - source settings exactly match merged profile
   - command prints a no-diff message and returns success

2. Empty diff in JSON mode:
   - source settings exactly match merged profile
   - output is valid JSON
   - `entries` is `[]`
   - no extra human text is printed

3. Added key:
   - key exists only in profile result
   - emits one `added` entry at the JSON Pointer for that key

4. Removed key:
   - key exists only in source settings
   - emits one `removed` entry at the JSON Pointer for that key

5. Modified scalar value:
   - same key exists on both sides with different scalar values
   - emits one `modified` entry at the JSON Pointer for that key

6. Nested map recursion:
   - both sides contain nested objects
   - emitted paths use JSON Pointer, not dot notation

7. Array change:
   - same array key exists on both sides with different contents
   - emits one `modified` entry at the array path
   - entry carries full old/new arrays
   - no element-level paths appear

8. Null handling:
   - `null` vs scalar
   - `null` vs object
   - object vs `null`
   - each emits a single whole-value entry at the parent path

9. Map-to-scalar type change:
   - source has object, profile has scalar
   - emits one `modified` entry at the parent path

10. Scalar-to-map type change:
   - source has scalar, profile has object
   - emits one `modified` entry at the parent path

11. Nested object added or removed:
   - a full object exists only on one side
   - emits one `added` or `removed` entry at that object path
   - does not flatten to child leaves

12. Secret redaction by default:
   - secret values differ
   - human and JSON outputs contain masked placeholders, not raw secrets

13. `--show-secrets` opt-in:
   - same fixture as secret-redaction test
   - raw values are shown only when the flag is enabled

14. Missing secret file warning:
   - profile has no `secrets/<name>.json`
   - command succeeds
   - warning is written to `stderr`
   - output diff still completes

15. Invalid `--source` path:
   - pass a nonexistent or unreadable file path
   - command returns an error

16. Nonexistent profile:
   - command returns `profile "<name>" not found`

17. Writer error propagation:
   - use a failing writer for human output and JSON output
   - verify `writeDiffHuman`, `writeDiffJSON`, and `app.diffProfile` return the write error

18. Shell completion wiring:
   - `newDiffCmd()` exposes `ValidArgsFunction: completeProfileNames`

## Verification

1. `go build ./...` passes
2. `go test ./...` passes
3. Manual test: `claude-profile diff <name>` shows JSON Pointer paths and whole-array replacements
4. Manual test: `claude-profile diff <name> --json | jq .` produces valid JSON
5. Manual test: secret values are masked by default
6. Manual test: `claude-profile diff <name> --show-secrets` reveals raw secret values
7. Manual test: removing `secrets/<name>.json` prints a warning to `stderr` and still succeeds
