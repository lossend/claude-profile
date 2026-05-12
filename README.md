# claude-profile

`claude-profile` is a small Go CLI for managing layered Claude settings profiles out of a local `~/.claude-profile` repository.

## What It Does

- Bootstraps `~/.claude-profile` as a local Git repository on first `create`
- Splits shared Claude settings into `common/*.json`
- Stores per-profile overrides in `profiles/<name>/*.json`
- Stores sensitive overrides in `secrets/<name>.json`, ignored by Git
- Rebuilds `~/.claude/settings.json` from layered config with `apply`
- Tracks the active profile in `state/active.json`
- Installs shell completion for `zsh`, `bash`, and `fish`

## Repository Layout

```text
~/.claude-profile/
  .git/
  .gitignore
  common/
    10-hooks.json
    20-security.json
    30-marketplace-plugin.json
    90-shared.json
  profiles/
    <name>/
      profile.json
      10-config.json
  secrets/
    <name>.json
  state/
    active.json
    completion.json
  backups/
```

## Commands

### `create`

Create a profile from the current Claude settings file.

```bash
go run . create openai --description "OpenAI profile"
```

Useful flags:

- `--description`: store profile metadata
- `--source`: read from a non-default settings file
- `--force`: overwrite an existing profile directory

### `apply`

Rebuild `~/.claude/settings.json` from `common`, profile-specific JSON files, and the local secret overlay.

```bash
go run . apply openai
```

Useful flags:

- `--target`: write to a non-default settings file

### `list`

List available profiles, their description, config files, secret presence, and active marker.

```bash
go run . list
```

## Merge Rules

- Objects merge recursively
- Arrays replace the previous value
- Scalars are overwritten by later files
- Files are applied in lexicographic order
- `profile.json` is metadata only and never merged into Claude settings

## Sensitive Fields

Keys are treated as sensitive when the final path segment:

- contains `TOKEN`
- contains `PASSWORD`
- contains `SECRET`
- ends with `_KEY`

Sensitive paths are written to `secrets/<name>.json` and stay out of Git history.

## Development

Run the test suite with:

```bash
go test ./...
```
