# Profile Layout Migration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Move profiles to `manifest.json` + `layers/*.json`, add a first-class `migrate` command, and fix `apply` output plus positional profile completion for shell users.

**Architecture:** Keep the repo-level layout unchanged except for profile internals. Introduce helpers that resolve profile paths in one place, so `create`, `apply`, `list`, `delete`, and `migrate` all use the same manifest/layers conventions. Add explicit completion hooks for profile-taking commands so Cobra can surface profile names in fish and other shells.

**Tech Stack:** Go, Cobra, Go test

---

### Task 1: Lock the new profile layout and apply output with tests

**Files:**
- Modify: `main_test.go`
- Test: `main_test.go`

**Step 1: Write the failing tests**

- Add a create test that expects:
  - `profiles/<name>/manifest.json`
  - `profiles/<name>/layers/010-config.json`
- Add an apply test that expects success output on stdout after writing settings.
- Add a list test that expects file names from the `layers/` directory only.

**Step 2: Run test to verify it fails**

Run: `go test ./...`
Expected: FAIL on path and output assertions.

**Step 3: Write minimal implementation**

- Change profile path constants/helpers to use manifest/layers.
- Print an apply success message with profile name and target path.
- Make list read files from the layers directory.

**Step 4: Run test to verify it passes**

Run: `go test ./...`
Expected: PASS for the new tests.

### Task 2: Add migration coverage and implementation

**Files:**
- Modify: `main_test.go`
- Modify: `main.go`
- Test: `main_test.go`

**Step 1: Write the failing tests**

- Add a migrate test that starts with:
  - `profiles/<name>/profile.json`
  - `profiles/<name>/10-config.json`
  - additional root-level profile JSON files
- Expect:
  - `manifest.json` created
  - `layers/010-config.json` created
  - other config files moved into `layers/`
  - old files removed
  - stdout reports migrated profiles

**Step 2: Run test to verify it fails**

Run: `go test ./...`
Expected: FAIL because the command does not exist yet.

**Step 3: Write minimal implementation**

- Add `migrate` Cobra command.
- Implement one-shot migration for every profile directory.
- Rename `10-config.json` to `010-config.json` during migration.

**Step 4: Run test to verify it passes**

Run: `go test ./...`
Expected: PASS for migration tests.

### Task 3: Add profile-name shell completion

**Files:**
- Modify: `main.go`
- Modify: `main_test.go`
- Test: `main_test.go`

**Step 1: Write the failing test**

- Add a completion test using Cobra’s hidden `__complete` command for `apply`.
- Expect existing profile names in stdout and no file-completion fallback.

**Step 2: Run test to verify it fails**

Run: `go test ./...`
Expected: FAIL because `apply` has no dynamic positional completion.

**Step 3: Write minimal implementation**

- Add a shared profile completion function.
- Wire it into `apply` and `delete`.

**Step 4: Run test to verify it passes**

Run: `go test ./...`
Expected: PASS for completion tests.

### Task 4: Update docs and migrate the real local repository

**Files:**
- Modify: `README.md`
- Modify: `main_test.go` if doc examples affect tests

**Step 1: Write the doc updates**

- Update README layout examples and wording from `profile.json`/root config files to `manifest.json`/`layers/*.json`.
- Document `migrate`.

**Step 2: Verify code and docs together**

Run: `go test ./...`
Expected: PASS.

**Step 3: Migrate the actual local data**

Run: `go run . migrate`
Expected: Local `~/.claude-profile` data moved to the new layout.

**Step 4: Verify migrated repository**

Run: `claude-profile list` or `go run . list`
Expected: Profiles still list correctly with `layers/` files and active profile intact.
