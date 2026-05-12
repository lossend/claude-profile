# claude-profile

`claude-profile` is a small CLI for managing layered Claude settings profiles out of a local `~/.claude-profile` repository.

## Install

Install the latest release binary:

```bash
curl -fsSL https://raw.githubusercontent.com/lossend/claude-profile/main/scripts/install.sh | sh
```

Install to a custom directory:

```bash
curl -fsSL https://raw.githubusercontent.com/lossend/claude-profile/main/scripts/install.sh | BINDIR="$HOME/bin" sh
```

Install a specific release:

```bash
curl -fsSL https://raw.githubusercontent.com/lossend/claude-profile/main/scripts/install.sh | VERSION=v0.1.0 sh
```

### From a downloaded release asset

Download the archive for your OS and architecture from GitHub Releases, extract it, and place the `claude-profile` binary on your `PATH`.

Verify the installed binary:

```bash
claude-profile version
```

## Quick Start

Create your first profile from the current Claude settings:

```bash
claude-profile create openai --description "OpenAI profile"
```

Inspect available profiles:

```bash
claude-profile list
```

Apply a profile back into Claude:

```bash
claude-profile apply openai
```

Delete a profile when you no longer need it:

```bash
claude-profile delete openai
```

Edit layered config files under:

- `~/.claude-profile/common/`
- `~/.claude-profile/profiles/openai/`
- `~/.claude-profile/secrets/openai.json`

## What It Does

- Bootstraps `~/.claude-profile` as a local Git repository on first `create`
- Splits shared Claude settings into `common/*.json`
- Stores per-profile overrides in `profiles/<name>/*.json`
- `create` writes the initial profile diff to `profiles/<name>/10-config.json` by default
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
claude-profile create openai --description "OpenAI profile"
```

Useful flags:

- `--description`: store profile metadata
- `--source`: read from a non-default settings file
- `--force`: overwrite an existing profile directory

`10-config.json` is only the default starter file name created by `create`. It is not part of the merge protocol. After bootstrap, you can split a profile into any number of `*.json` files such as `20-models.json` or `30-provider.json`, and `apply` will merge every `*.json` file in the profile directory except `profile.json`.

### `apply`

Rebuild `~/.claude/settings.json` from `common`, profile-specific JSON files, and the local secret overlay.

```bash
claude-profile apply openai
```

Useful flags:

- `--target`: write to a non-default settings file

### `list`

List available profiles, their description, config files, secret presence, and active marker.

```bash
claude-profile list
```

### `delete`

Delete a profile directory and its local secret file.

```bash
claude-profile delete openai
```

`delete` requires two confirmations:

- first type the profile name exactly
- then type `DELETE`

If either confirmation does not match, the command aborts without changing any files.

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

### Build from source

If you are developing locally and do have Go installed:

```bash
go install .
```

### Test

Run the test suite with:

```bash
go test ./...
```

## Release

Build local release artifacts into `dist/`:

```bash
scripts/build-release.sh
```

Build a tagged release locally:

```bash
VERSION=v0.1.0 scripts/build-release.sh
```

The release workflow publishes artifacts when a tag like `v0.1.0` is pushed:

```bash
git tag v0.1.0
git push origin v0.1.0
```
