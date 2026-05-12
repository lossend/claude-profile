package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateBootstrapsProfileRepo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	settings := map[string]any{
		"hooks": map[string]any{
			"preRun": []any{"echo hi"},
		},
		"permissions": map[string]any{
			"defaultMode": "acceptEdits",
		},
		"sandbox": map[string]any{
			"mode": "workspace-write",
		},
		"skipAutoPermissionPrompt":          true,
		"skipDangerousModePermissionPrompt": true,
		"enabledPlugins":                    []any{"marketplace/foo"},
		"extraKnownMarketplaces":            []any{"local"},
		"env": map[string]any{
			"OPENAI_API_KEY": "secret-key",
			"LOG_LEVEL":      "debug",
		},
		"model":      "claude-sonnet",
		"apiBaseUrl": "https://example.test",
	}
	writeJSONFileForTest(t, filepath.Join(home, ".claude", "settings.json"), settings)

	_, stderr, err := runCLI(t, "create", "openai", "--description", "OpenAI profile")
	if err != nil {
		t.Fatalf("create failed: %v\nstderr=%s", err, stderr)
	}
	stdout, _, err := runCLI(t, "list")
	if err != nil {
		t.Fatalf("list failed after create: %v", err)
	}
	if !strings.Contains(stdout, "* openai") {
		t.Fatalf("expected profile to be active after create, got %q", stdout)
	}

	repoRoot := filepath.Join(home, ".claude-profile")
	assertFileExists(t, filepath.Join(repoRoot, ".git"))
	assertFileContent(t, filepath.Join(repoRoot, ".gitignore"), "secrets/\nstate/\nbackups/\n")

	profileMeta := readJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "openai", "profile.json"))
	if profileMeta["name"] != "openai" || profileMeta["description"] != "OpenAI profile" {
		t.Fatalf("unexpected profile metadata: %#v", profileMeta)
	}

	commonHooks := readJSONFileForTest(t, filepath.Join(repoRoot, "common", "10-hooks.json"))
	if _, ok := commonHooks["hooks"]; !ok {
		t.Fatalf("expected hooks in common split: %#v", commonHooks)
	}

	commonSecurity := readJSONFileForTest(t, filepath.Join(repoRoot, "common", "20-security.json"))
	if _, ok := commonSecurity["permissions"]; !ok {
		t.Fatalf("expected permissions in security split: %#v", commonSecurity)
	}

	commonPlugins := readJSONFileForTest(t, filepath.Join(repoRoot, "common", "30-marketplace-plugin.json"))
	if _, ok := commonPlugins["enabledPlugins"]; !ok {
		t.Fatalf("expected enabledPlugins in marketplace split: %#v", commonPlugins)
	}

	commonShared := readJSONFileForTest(t, filepath.Join(repoRoot, "common", "90-shared.json"))
	if commonShared["env"].(map[string]any)["LOG_LEVEL"] != "debug" {
		t.Fatalf("expected non-sensitive env to remain in shared config: %#v", commonShared)
	}
	if _, ok := commonShared["env"].(map[string]any)["OPENAI_API_KEY"]; ok {
		t.Fatalf("did not expect sensitive key in shared config: %#v", commonShared)
	}

	profileConfig := readJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "openai", starterProfileConfigFile))
	if len(profileConfig) != 0 {
		t.Fatalf("expected empty profile diff for first profile, got %#v", profileConfig)
	}

	secrets := readJSONFileForTest(t, filepath.Join(repoRoot, "secrets", "openai.json"))
	if secrets["env"].(map[string]any)["OPENAI_API_KEY"] != "secret-key" {
		t.Fatalf("expected sensitive env extracted into secret file: %#v", secrets)
	}

	active := readJSONFileForTest(t, filepath.Join(repoRoot, "state", "active.json"))
	if active["name"] != "openai" {
		t.Fatalf("expected create to mark active profile, got %#v", active)
	}
}

func TestCreatePrintsSuccessMessageWithPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeJSONFileForTest(t, filepath.Join(home, ".claude", "settings.json"), map[string]any{
		"model": "claude-sonnet",
	})

	stdout, stderr, err := runCLI(t, "create", "litellm-glm", "--description", "Work profile")
	if err != nil {
		t.Fatalf("create failed: %v\nstderr=%s", err, stderr)
	}

	repoRoot := filepath.Join(home, ".claude-profile")
	profileDir := filepath.Join(repoRoot, "profiles", "litellm-glm")
	secretPath := filepath.Join(repoRoot, "secrets", "litellm-glm.json")

	if !strings.Contains(stdout, "created profile \"litellm-glm\"") {
		t.Fatalf("expected success message, got %q", stdout)
	}
	if !strings.Contains(stdout, profileDir) {
		t.Fatalf("expected profile directory in output, got %q", stdout)
	}
	if !strings.Contains(stdout, secretPath) {
		t.Fatalf("expected secret path in output, got %q", stdout)
	}
	if !strings.Contains(stdout, "claude-profile apply litellm-glm") {
		t.Fatalf("expected next step in output, got %q", stdout)
	}
}

func TestCreateDerivesProfileDiffAgainstExistingCommon(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(home, ".claude-profile")
	writeJSONFileForTest(t, filepath.Join(repoRoot, "common", "10-hooks.json"), map[string]any{
		"hooks": map[string]any{
			"preRun": []any{"echo common"},
		},
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "common", "20-security.json"), map[string]any{
		"permissions": map[string]any{
			"defaultMode": "acceptEdits",
		},
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "common", "90-shared.json"), map[string]any{
		"model": "common-model",
		"env": map[string]any{
			"LOG_LEVEL": "info",
		},
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "state", "active.json"), map[string]any{
		"name": "existing",
	})

	settings := map[string]any{
		"hooks": map[string]any{
			"preRun": []any{"echo common"},
		},
		"permissions": map[string]any{
			"defaultMode": "acceptEdits",
		},
		"model": "profile-model",
		"env": map[string]any{
			"LOG_LEVEL":       "warn",
			"AWS_SECRET_KEY":  "abc123",
			"AWS_REGION":      "us-east-1",
			"EXPERIMENT_FLAG": true,
		},
		"experimental": map[string]any{
			"nested": map[string]any{
				"enabled": true,
			},
		},
	}
	writeJSONFileForTest(t, filepath.Join(home, ".claude", "settings.json"), settings)

	_, stderr, err := runCLI(t, "create", "bedrock", "--description", "Bedrock profile")
	if err != nil {
		t.Fatalf("create failed: %v\nstderr=%s", err, stderr)
	}

	profileConfig := readJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "bedrock", starterProfileConfigFile))
	if profileConfig["model"] != "profile-model" {
		t.Fatalf("expected model diff in profile config: %#v", profileConfig)
	}
	envDiff := profileConfig["env"].(map[string]any)
	if envDiff["LOG_LEVEL"] != "warn" || envDiff["AWS_REGION"] != "us-east-1" || envDiff["EXPERIMENT_FLAG"] != true {
		t.Fatalf("unexpected env diff: %#v", envDiff)
	}
	if _, ok := envDiff["AWS_SECRET_KEY"]; ok {
		t.Fatalf("did not expect sensitive env in profile config: %#v", envDiff)
	}

	secrets := readJSONFileForTest(t, filepath.Join(repoRoot, "secrets", "bedrock.json"))
	if secrets["env"].(map[string]any)["AWS_SECRET_KEY"] != "abc123" {
		t.Fatalf("expected sensitive diff in secret file: %#v", secrets)
	}

	commonShared := readJSONFileForTest(t, filepath.Join(repoRoot, "common", "90-shared.json"))
	if commonShared["model"] != "common-model" {
		t.Fatalf("expected existing common to be preserved: %#v", commonShared)
	}
}

func TestCreateKeepsProviderEnvDefaultsInProfileOverrides(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	settings := map[string]any{
		"env": map[string]any{
			"ANTHROPIC_BASE_URL":             "https://litellm-sg.mayfair-inc.com",
			"ANTHROPIC_DEFAULT_HAIKU_MODEL":  "pub-deepseek-v4-flash",
			"ANTHROPIC_DEFAULT_OPUS_MODEL":   "pub-claude-opus-4-6",
			"ANTHROPIC_DEFAULT_SONNET_MODEL": "pub-glm-5",
			"ANTHROPIC_MODEL":                "us.anthropic.claude-sonnet-4-6",
			"ANTHROPIC_SMALL_FAST_MODEL":     "us.anthropic.claude-haiku-4-5-20251001-v1:0",
			"AWS_PROFILE":                    "prod-use-uis",
			"CLAUDE_CODE_USE_BEDROCK":        1,
			"LOG_LEVEL":                      "debug",
		},
		"model": "claude-sonnet",
	}
	writeJSONFileForTest(t, filepath.Join(home, ".claude", "settings.json"), settings)

	_, stderr, err := runCLI(t, "create", "bedrock", "--description", "Bedrock profile")
	if err != nil {
		t.Fatalf("create failed: %v\nstderr=%s", err, stderr)
	}

	repoRoot := filepath.Join(home, ".claude-profile")
	commonShared := readJSONFileForTest(t, filepath.Join(repoRoot, "common", "90-shared.json"))
	sharedEnv := commonShared["env"].(map[string]any)
	if sharedEnv["LOG_LEVEL"] != "debug" {
		t.Fatalf("expected unrelated env to stay in common shared config: %#v", sharedEnv)
	}
	for _, key := range []string{
		"ANTHROPIC_BASE_URL",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL",
		"ANTHROPIC_DEFAULT_OPUS_MODEL",
		"ANTHROPIC_DEFAULT_SONNET_MODEL",
		"ANTHROPIC_MODEL",
		"ANTHROPIC_SMALL_FAST_MODEL",
		"AWS_PROFILE",
		"CLAUDE_CODE_USE_BEDROCK",
	} {
		if _, ok := sharedEnv[key]; ok {
			t.Fatalf("did not expect %s in common shared config: %#v", key, sharedEnv)
		}
	}

	profileConfig := readJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "bedrock", starterProfileConfigFile))
	profileEnv := profileConfig["env"].(map[string]any)
	if profileEnv["ANTHROPIC_BASE_URL"] != "https://litellm-sg.mayfair-inc.com" ||
		profileEnv["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "pub-deepseek-v4-flash" ||
		profileEnv["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "pub-claude-opus-4-6" ||
		profileEnv["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "pub-glm-5" ||
		profileEnv["ANTHROPIC_MODEL"] != "us.anthropic.claude-sonnet-4-6" ||
		profileEnv["ANTHROPIC_SMALL_FAST_MODEL"] != "us.anthropic.claude-haiku-4-5-20251001-v1:0" ||
		profileEnv["AWS_PROFILE"] != "prod-use-uis" ||
		profileEnv["CLAUDE_CODE_USE_BEDROCK"] != float64(1) {
		t.Fatalf("expected provider env defaults in profile overrides: %#v", profileEnv)
	}
}

func TestCreateRejectsWhenOnlySecretAlreadyExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeJSONFileForTest(t, filepath.Join(home, ".claude", "settings.json"), map[string]any{
		"model": "claude-sonnet",
	})
	repoRoot := filepath.Join(home, ".claude-profile")
	writeJSONFileForTest(t, filepath.Join(repoRoot, "secrets", "work.json"), map[string]any{
		"env": map[string]any{"OPENAI_API_KEY": "existing-secret"},
	})

	stdout, stderr, err := runCLI(t, "create", "work", "--description", "Work profile")
	if err == nil {
		t.Fatalf("expected create to fail when secret already exists\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestCreateForceRequiresDoubleConfirmation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(home, ".claude-profile")
	writeJSONFileForTest(t, filepath.Join(home, ".claude", "settings.json"), map[string]any{
		"model": "new-model",
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "work", "profile.json"), map[string]any{
		"name":        "work",
		"description": "Old work profile",
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "work", starterProfileConfigFile), map[string]any{
		"model": "old-model",
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "secrets", "work.json"), map[string]any{
		"env": map[string]any{"OPENAI_API_KEY": "old-secret"},
	})

	stdout, stderr, err := runCLIWithInput(t, "work\nDELETE\n", "create", "work", "--description", "New work profile", "--force")
	if err != nil {
		t.Fatalf("expected forced create to succeed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	profileMeta := readJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "work", "profile.json"))
	if profileMeta["description"] != "New work profile" {
		t.Fatalf("expected profile metadata to be replaced, got %#v", profileMeta)
	}
}

func TestCreateForceRejectsWrongConfirmation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(home, ".claude-profile")
	writeJSONFileForTest(t, filepath.Join(home, ".claude", "settings.json"), map[string]any{
		"model": "new-model",
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "work", "profile.json"), map[string]any{
		"name":        "work",
		"description": "Old work profile",
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "work", starterProfileConfigFile), map[string]any{
		"model": "old-model",
	})

	stdout, stderr, err := runCLIWithInput(t, "wrong\nDELETE\n", "create", "work", "--description", "New work profile", "--force")
	if err == nil {
		t.Fatalf("expected forced create to abort on wrong confirmation\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if !strings.Contains(err.Error(), "aborted") {
		t.Fatalf("expected abort error, got %v", err)
	}

	profileMeta := readJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "work", "profile.json"))
	if profileMeta["description"] != "Old work profile" {
		t.Fatalf("expected original profile to remain unchanged, got %#v", profileMeta)
	}
}

func TestApplyMergesCommonProfileAndSecretsWithBackup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(home, ".claude-profile")
	writeJSONFileForTest(t, filepath.Join(repoRoot, "common", "10-base.json"), map[string]any{
		"model":     "common-model",
		"providers": []any{"common"},
		"nested": map[string]any{
			"level":  "common",
			"shared": "common",
		},
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "openai", "profile.json"), map[string]any{
		"name":        "openai",
		"description": "OpenAI profile",
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "openai", starterProfileConfigFile), map[string]any{
		"nested": map[string]any{
			"level": "profile",
		},
		"provider":  "openai",
		"providers": []any{"profile"},
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "openai", "20-models.json"), map[string]any{
		"model":     "gpt-4.1",
		"providers": []any{"override"},
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "secrets", "openai.json"), map[string]any{
		"env": map[string]any{
			"OPENAI_API_KEY": "super-secret",
		},
	})
	writeJSONFileForTest(t, filepath.Join(home, ".claude", "settings.json"), map[string]any{
		"legacy": true,
	})

	_, stderr, err := runCLI(t, "apply", "openai")
	if err != nil {
		t.Fatalf("apply failed: %v\nstderr=%s", err, stderr)
	}

	applied := readJSONFileForTest(t, filepath.Join(home, ".claude", "settings.json"))
	if applied["model"] != "gpt-4.1" {
		t.Fatalf("expected lexicographically later profile file to win: %#v", applied)
	}
	if applied["provider"] != "openai" {
		t.Fatalf("expected profile config fields in final settings: %#v", applied)
	}
	if applied["providers"].([]any)[0] != "override" {
		t.Fatalf("expected arrays to be replaced, got %#v", applied["providers"])
	}
	nested := applied["nested"].(map[string]any)
	if nested["level"] != "profile" || nested["shared"] != "common" {
		t.Fatalf("expected recursive object merge: %#v", nested)
	}
	if applied["env"].(map[string]any)["OPENAI_API_KEY"] != "super-secret" {
		t.Fatalf("expected secrets merged last: %#v", applied)
	}

	backupsDir := filepath.Join(repoRoot, "backups")
	entries, err := os.ReadDir(backupsDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected a single backup file, got err=%v entries=%d", err, len(entries))
	}

	active := readJSONFileForTest(t, filepath.Join(repoRoot, "state", "active.json"))
	if active["name"] != "openai" {
		t.Fatalf("expected apply to update active profile: %#v", active)
	}
}

func TestApplyWarnsWhenSecretFileMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(home, ".claude-profile")
	writeJSONFileForTest(t, filepath.Join(repoRoot, "common", "10-base.json"), map[string]any{
		"model": "common-model",
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "openai", "profile.json"), map[string]any{
		"name": "openai",
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "openai", starterProfileConfigFile), map[string]any{
		"model": "profile-model",
	})

	_, stderr, err := runCLI(t, "apply", "openai")
	if err != nil {
		t.Fatalf("apply failed: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stderr, "warning") || !strings.Contains(stderr, "secret") {
		t.Fatalf("expected warning about missing secret file, got %q", stderr)
	}
}

func TestListShowsProfilesFilesSecretsAndActiveMarker(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(home, ".claude-profile")
	writeJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "openai", "profile.json"), map[string]any{
		"name":        "openai",
		"description": "OpenAI profile",
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "openai", starterProfileConfigFile), map[string]any{
		"model": "gpt-4.1",
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "openai", "20-models.json"), map[string]any{
		"model": "gpt-4.1-mini",
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "secrets", "openai.json"), map[string]any{
		"env": map[string]any{"OPENAI_API_KEY": "x"},
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "bedrock", "profile.json"), map[string]any{
		"name":        "bedrock",
		"description": "Bedrock profile",
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "bedrock", starterProfileConfigFile), map[string]any{
		"model": "claude-sonnet",
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "state", "active.json"), map[string]any{
		"name": "openai",
	})

	stdout, stderr, err := runCLI(t, "list")
	if err != nil {
		t.Fatalf("list failed: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "* openai") {
		t.Fatalf("expected active profile marker in output: %q", stdout)
	}
	if !strings.Contains(stdout, "files="+starterProfileConfigFile+",20-models.json") {
		t.Fatalf("expected config file names in output: %q", stdout)
	}
	if !strings.Contains(stdout, "secret=yes") || !strings.Contains(stdout, "secret=no") {
		t.Fatalf("expected secret presence in output: %q", stdout)
	}
}

func TestShellCompletionInstallationIsIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeJSONFileForTest(t, filepath.Join(home, ".claude", "settings.json"), map[string]any{
		"model": "common-model",
	})

	for i := 0; i < 2; i++ {
		stdin := ""
		if i > 0 {
			stdin = "openai\nDELETE\n"
		}
		_, stderr, err := runCLIWithInput(t, stdin, "create", "openai", "--description", "OpenAI profile", "--force")
		if err != nil {
			t.Fatalf("create failed on iteration %d: %v\nstderr=%s", i, err, stderr)
		}
	}

	zshrc := readTextFileForTest(t, filepath.Join(home, ".zshrc"))
	if strings.Count(zshrc, "claude-profile completion zsh") != 1 {
		t.Fatalf("expected single zsh completion block, got %q", zshrc)
	}

	bashrc := readTextFileForTest(t, filepath.Join(home, ".bashrc"))
	if strings.Count(bashrc, "claude-profile completion bash") != 1 {
		t.Fatalf("expected single bash completion block, got %q", bashrc)
	}

	fishConfig := readTextFileForTest(t, filepath.Join(home, ".config", "fish", "config.fish"))
	if strings.Count(fishConfig, "claude-profile completion fish") != 1 {
		t.Fatalf("expected single fish completion block, got %q", fishConfig)
	}

	state := readJSONFileForTest(t, filepath.Join(home, ".claude-profile", "state", "completion.json"))
	for _, shell := range []string{"zsh", "bash", "fish"} {
		shellState, ok := state[shell].(map[string]any)
		if !ok || shellState["installed"] != true {
			t.Fatalf("expected installed state for %s, got %#v", shell, state)
		}
	}
}

func TestVersionCommandPrintsDefaultVersion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	originalVersion := version
	originalCommit := commit
	originalDate := buildDate
	t.Cleanup(func() {
		version = originalVersion
		commit = originalCommit
		buildDate = originalDate
	})

	version = "dev"
	commit = "none"
	buildDate = "unknown"

	stdout, stderr, err := runCLI(t, "version")
	if err != nil {
		t.Fatalf("version failed: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "claude-profile dev") {
		t.Fatalf("expected version string in output, got %q", stdout)
	}
	if !strings.Contains(stdout, "commit=none") || !strings.Contains(stdout, "date=unknown") {
		t.Fatalf("expected commit and date metadata in output, got %q", stdout)
	}
}

func TestDeleteRemovesProfileAfterDoubleConfirmation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(home, ".claude-profile")
	writeJSONFileForTest(t, filepath.Join(repoRoot, "common", "90-shared.json"), map[string]any{
		"model": "shared-model",
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "openai", "profile.json"), map[string]any{
		"name":        "openai",
		"description": "OpenAI profile",
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "openai", starterProfileConfigFile), map[string]any{
		"model": "profile-model",
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "secrets", "openai.json"), map[string]any{
		"env": map[string]any{"OPENAI_API_KEY": "secret"},
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "state", "active.json"), map[string]any{
		"name": "openai",
	})

	stdout, stderr, err := runCLIWithInput(t, "openai\nDELETE\n", "delete", "openai")
	if err != nil {
		t.Fatalf("delete failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "deleted profile \"openai\"") {
		t.Fatalf("expected success output, got %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "profiles", "openai")); !os.IsNotExist(err) {
		t.Fatalf("expected profile directory deleted, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "secrets", "openai.json")); !os.IsNotExist(err) {
		t.Fatalf("expected secret file deleted, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "common", "90-shared.json")); err != nil {
		t.Fatalf("expected common config preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "state", "active.json")); !os.IsNotExist(err) {
		t.Fatalf("expected active state removed for deleted profile, got err=%v", err)
	}
}

func TestDeleteRejectsWhenConfirmationDoesNotMatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(home, ".claude-profile")
	writeJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "openai", "profile.json"), map[string]any{
		"name": "openai",
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "openai", starterProfileConfigFile), map[string]any{
		"model": "profile-model",
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "secrets", "openai.json"), map[string]any{
		"env": map[string]any{"OPENAI_API_KEY": "secret"},
	})

	stdout, stderr, err := runCLIWithInput(t, "wrong\nDELETE\n", "delete", "openai")
	if err == nil {
		t.Fatalf("expected delete to fail on wrong confirmation\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if !strings.Contains(err.Error(), "aborted") {
		t.Fatalf("expected abort error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "profiles", "openai", "profile.json")); err != nil {
		t.Fatalf("expected profile to remain after rejected delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "secrets", "openai.json")); err != nil {
		t.Fatalf("expected secret to remain after rejected delete: %v", err)
	}
}

func TestGitIgnoreKeepsSecretsStateAndBackupsOutOfGitStatus(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeJSONFileForTest(t, filepath.Join(home, ".claude", "settings.json"), map[string]any{
		"env": map[string]any{
			"OPENAI_API_KEY": "secret-key",
		},
		"model": "common-model",
	})

	_, stderr, err := runCLI(t, "create", "openai", "--description", "OpenAI profile")
	if err != nil {
		t.Fatalf("create failed: %v\nstderr=%s", err, stderr)
	}
	_, stderr, err = runCLI(t, "apply", "openai")
	if err != nil {
		t.Fatalf("apply failed: %v\nstderr=%s", err, stderr)
	}

	repoRoot := filepath.Join(home, ".claude-profile")
	cmd := exec.Command("git", "-C", repoRoot, "status", "--short", "--untracked-files=all")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status failed: %v\noutput=%s", err, output)
	}

	status := string(output)
	if strings.Contains(status, "secrets/") || strings.Contains(status, "state/") || strings.Contains(status, "backups/") {
		t.Fatalf("expected ignored paths to stay out of git status, got %q", status)
	}
	if !strings.Contains(status, ".gitignore") || !strings.Contains(status, "common/") || !strings.Contains(status, "profiles/") {
		t.Fatalf("expected tracked config files to remain visible in git status, got %q", status)
	}
}

func TestEndToEndApplyStacksManualProfileFilesAndLocalSecrets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	initialSettings := map[string]any{
		"model": "gpt-4.1",
		"env": map[string]any{
			"OPENAI_API_KEY": "secret-key",
			"LOG_LEVEL":      "info",
		},
	}
	writeJSONFileForTest(t, filepath.Join(home, ".claude", "settings.json"), initialSettings)

	_, stderr, err := runCLI(t, "create", "openai", "--description", "OpenAI profile")
	if err != nil {
		t.Fatalf("create failed: %v\nstderr=%s", err, stderr)
	}

	repoRoot := filepath.Join(home, ".claude-profile")
	writeJSONFileForTest(t, filepath.Join(repoRoot, "profiles", "openai", "20-models.json"), map[string]any{
		"model": "gpt-4.1-mini",
		"features": map[string]any{
			"reasoning": "high",
		},
	})
	writeJSONFileForTest(t, filepath.Join(repoRoot, "secrets", "openai.json"), map[string]any{
		"env": map[string]any{
			"OPENAI_API_KEY": "local-secret",
		},
	})

	_, stderr, err = runCLI(t, "apply", "openai")
	if err != nil {
		t.Fatalf("apply failed: %v\nstderr=%s", err, stderr)
	}

	applied := readJSONFileForTest(t, filepath.Join(home, ".claude", "settings.json"))
	if applied["model"] != "gpt-4.1-mini" {
		t.Fatalf("expected manual profile file to override model, got %#v", applied)
	}
	if applied["features"].(map[string]any)["reasoning"] != "high" {
		t.Fatalf("expected manual profile file to merge into target settings, got %#v", applied)
	}
	if applied["env"].(map[string]any)["OPENAI_API_KEY"] != "local-secret" {
		t.Fatalf("expected local secret override to win, got %#v", applied)
	}

	cmd := exec.Command("git", "-C", repoRoot, "status", "--short", "--untracked-files=all")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status failed: %v\noutput=%s", err, output)
	}
	status := string(output)
	if strings.Contains(status, "secrets/openai.json") {
		t.Fatalf("expected local secret file to stay out of git status, got %q", status)
	}
	if !strings.Contains(status, "profiles/openai/20-models.json") {
		t.Fatalf("expected manual profile file to be visible in git status, got %q", status)
	}
}

func runCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	cmd := newRootCmd()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func runCLIWithInput(t *testing.T, input string, args ...string) (string, string, error) {
	t.Helper()

	cmd := newRootCmd()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetArgs(args)

	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func writeJSONFileForTest(t *testing.T, path string, data map[string]any) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
}

func readJSONFileForTest(t *testing.T, path string) map[string]any {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed for %s: %v", path, err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal failed for %s: %v", path, err)
	}
	return out
}

func readTextFileForTest(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read text failed for %s: %v", path, err)
	}
	return string(raw)
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got := readTextFileForTest(t, path)
	if got != want {
		t.Fatalf("unexpected content in %s:\nwant:\n%s\ngot:\n%s", path, want, got)
	}
}
