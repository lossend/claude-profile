// @author yangjie.sun
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	gitIgnoreContent         = "secrets/\nstate/\nbackups/\n"
	starterProfileConfigFile = "10-config.json"
)

var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

var profileScopedEnvKeys = map[string]struct{}{
	"ANTHROPIC_BASE_URL":             {},
	"ANTHROPIC_DEFAULT_HAIKU_MODEL":  {},
	"ANTHROPIC_DEFAULT_OPUS_MODEL":   {},
	"ANTHROPIC_DEFAULT_SONNET_MODEL": {},
	"ANTHROPIC_MODEL":                {},
	"ANTHROPIC_SMALL_FAST_MODEL":     {},
	"AWS_PROFILE":                    {},
	"CLAUDE_CODE_USE_BEDROCK":        {},
}

type app struct {
	home     string
	repoRoot string
}

type profileMeta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type activeState struct {
	Name      string `json:"name"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type completionState struct {
	Installed bool   `json:"installed"`
	RCPath    string `json:"rcPath"`
	UpdatedAt string `json:"updatedAt"`
}

func main() {
	cmd := newRootCmd()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "claude-profile",
		Short:         "Manage layered Claude settings profiles",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Name() == "completion" {
				return nil
			}

			app, err := newApp()
			if err != nil {
				return err
			}
			return app.ensureCompletionInstall(cmd.Root())
		},
	}

	root.AddCommand(newCreateCmd())
	root.AddCommand(newApplyCmd())
	root.AddCommand(newDeleteCmd())
	root.AddCommand(newListCmd())
	root.AddCommand(newCommitCmd())
	root.AddCommand(newVersionCmd())
	root.AddCommand(newCompletionCmd(root))
	return root
}

func newCreateCmd() *cobra.Command {
	var description string
	var sourcePath string
	var force bool

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create or refresh a profile from Claude settings",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := newApp()
			if err != nil {
				return err
			}
			if sourcePath == "" {
				sourcePath = filepath.Join(app.home, ".claude", "settings.json")
			}
			return app.createProfile(cmd.InOrStdin(), cmd.OutOrStdout(), args[0], description, sourcePath, force)
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "Profile description")
	cmd.Flags().StringVar(&sourcePath, "source", "", "Source Claude settings path")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing profile")
	return cmd
}

func newApplyCmd() *cobra.Command {
	var targetPath string

	cmd := &cobra.Command{
		Use:   "apply <name>",
		Short: "Apply a layered profile into Claude settings",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := newApp()
			if err != nil {
				return err
			}
			if targetPath == "" {
				targetPath = filepath.Join(app.home, ".claude", "settings.json")
			}
			return app.applyProfile(cmd.ErrOrStderr(), args[0], targetPath)
		},
	}

	cmd.Flags().StringVar(&targetPath, "target", "", "Target Claude settings path")
	return cmd
}

func newDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a profile after double confirmation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := newApp()
			if err != nil {
				return err
			}
			return app.deleteProfile(cmd.InOrStdin(), cmd.OutOrStdout(), args[0])
		},
	}
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := newApp()
			if err != nil {
				return err
			}
			return app.listProfiles(cmd.OutOrStdout())
		},
	}
}

func newCommitCmd() *cobra.Command {
	var message string

	cmd := &cobra.Command{
		Use:   "commit [message]",
		Short: "Commit all changes in the profile repository",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := newApp()
			if err != nil {
				return err
			}
			if len(args) > 0 {
				message = args[0]
			}
			return app.commitProfile(cmd.OutOrStdout(), message)
		},
	}

	cmd.Flags().StringVarP(&message, "message", "m", "", "Commit message")
	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "claude-profile %s commit=%s date=%s\n", version, commit, buildDate)
			return err
		},
	}
}

func newCompletionCmd(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:                   "completion [bash|zsh|fish]",
		Short:                 "Generate shell completion",
		Hidden:                true,
		Args:                  cobra.ExactArgs(1),
		ValidArgs:             []string{"bash", "zsh", "fish"},
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return root.GenBashCompletion(cmd.OutOrStdout())
			case "zsh":
				return root.GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return root.GenFishCompletion(cmd.OutOrStdout(), true)
			default:
				return fmt.Errorf("unsupported shell %q", args[0])
			}
		},
	}
}

func newApp() (*app, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	return &app{
		home:     home,
		repoRoot: filepath.Join(home, ".claude-profile"),
	}, nil
}

func (a *app) createProfile(stdin io.Reader, stdout io.Writer, name, description, sourcePath string, force bool) error {
	if err := a.ensureRepoDirs(); err != nil {
		return err
	}
	if err := a.ensureGitRepo(); err != nil {
		return err
	}
	if err := a.ensureGitIgnore(); err != nil {
		return err
	}

	settings, err := a.readJSONFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read source settings: %w", err)
	}

	commonDir := filepath.Join(a.repoRoot, "common")
	commonFiles, err := readConfigFilenames(commonDir, true)
	if err != nil {
		return err
	}
	if len(commonFiles) == 0 {
		if err := a.writeStarterCommon(settings); err != nil {
			return err
		}
	}

	baseline, err := a.mergeConfigDir(commonDir, nil)
	if err != nil {
		return err
	}
	diff := diffValues(baseline, settings)
	profileDiff, secretDiff := splitSensitiveTree(diff)
	profileDiff = mergeMaps(
		ensureJSONObject(profileDiff),
		extractProfileScopedSettings(settings),
	)

	profileDir := filepath.Join(a.repoRoot, "profiles", name)
	secretPath := filepath.Join(a.repoRoot, "secrets", name+".json")
	if err := a.prepareProfileArtifacts(stdin, stdout, profileDir, secretPath, force); err != nil {
		return err
	}

	meta := profileMeta{Name: name, Description: description}
	if err := a.writeJSONFile(filepath.Join(profileDir, "profile.json"), structToMap(meta)); err != nil {
		return err
	}
	if err := a.writeJSONFile(filepath.Join(profileDir, starterProfileConfigFile), ensureJSONObject(profileDiff)); err != nil {
		return err
	}
	if err := a.writeJSONFile(secretPath, ensureJSONObject(secretDiff)); err != nil {
		return err
	}
	if err := a.writeActiveProfile(name); err != nil {
		return err
	}

	_, err = fmt.Fprintf(
		stdout,
		"created profile %q\nprofile config: %s\nlocal secrets: %s\nnext: claude-profile apply %s\n",
		name,
		profileDir,
		secretPath,
		name,
	)
	return err
}

func (a *app) applyProfile(stderr io.Writer, name, targetPath string) error {
	if err := a.ensureRepoDirs(); err != nil {
		return err
	}

	profileDir := filepath.Join(a.repoRoot, "profiles", name)
	if _, err := os.Stat(filepath.Join(profileDir, "profile.json")); err != nil {
		return fmt.Errorf("profile %q not found", name)
	}

	merged, err := a.mergeIntoExisting(map[string]any{}, filepath.Join(a.repoRoot, "common"), nil)
	if err != nil {
		return err
	}
	merged, err = a.mergeIntoExisting(merged, profileDir, map[string]struct{}{"profile.json": {}})
	if err != nil {
		return err
	}

	secretPath := filepath.Join(a.repoRoot, "secrets", name+".json")
	if secretConfig, err := a.readOptionalJSONFile(secretPath); err != nil {
		return err
	} else if secretConfig == nil {
		fmt.Fprintf(stderr, "warning: secret override %s not found\n", secretPath)
	} else {
		merged = mergeMaps(merged, secretConfig)
	}

	if err := a.backupTarget(targetPath); err != nil {
		return err
	}
	if err := a.writeAtomicJSON(targetPath, merged); err != nil {
		return err
	}
	return a.writeActiveProfile(name)
}

func (a *app) deleteProfile(stdin io.Reader, stdout io.Writer, name string) error {
	profileDir := filepath.Join(a.repoRoot, "profiles", name)
	if _, err := os.Stat(filepath.Join(profileDir, "profile.json")); err != nil {
		return fmt.Errorf("profile %q not found", name)
	}

	reader := bufio.NewReader(stdin)
	if err := confirmDelete(reader, stdout, fmt.Sprintf("Type the profile name (%s) to continue: ", name), name); err != nil {
		return err
	}
	if err := confirmDelete(reader, stdout, "Type DELETE to permanently remove this profile: ", "DELETE"); err != nil {
		return err
	}

	if err := os.RemoveAll(profileDir); err != nil {
		return fmt.Errorf("delete profile directory: %w", err)
	}

	secretPath := filepath.Join(a.repoRoot, "secrets", name+".json")
	if err := os.Remove(secretPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete secret file: %w", err)
	}

	if active, err := a.readOptionalJSONFile(filepath.Join(a.repoRoot, "state", "active.json")); err == nil && active != nil {
		if activeName, ok := active["name"].(string); ok && activeName == name {
			if err := os.Remove(filepath.Join(a.repoRoot, "state", "active.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("delete active state: %w", err)
			}
		}
	} else if err != nil {
		return err
	}

	_, err := fmt.Fprintf(stdout, "deleted profile %q\n", name)
	return err
}

func (a *app) listProfiles(stdout io.Writer) error {
	profilesDir := filepath.Join(a.repoRoot, "profiles")
	entries, err := os.ReadDir(profilesDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	activeName := ""
	if active, err := a.readOptionalJSONFile(filepath.Join(a.repoRoot, "state", "active.json")); err == nil && active != nil {
		if value, ok := active["name"].(string); ok {
			activeName = value
		}
	}

	var lines []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		profileDir := filepath.Join(profilesDir, entry.Name())
		metaMap, err := a.readJSONFile(filepath.Join(profileDir, "profile.json"))
		if err != nil {
			return err
		}
		files, err := readConfigFilenames(profileDir, false)
		if err != nil {
			return err
		}
		filtered := make([]string, 0, len(files))
		for _, file := range files {
			if file != "profile.json" {
				filtered = append(filtered, file)
			}
		}
		secret := "no"
		if _, err := os.Stat(filepath.Join(a.repoRoot, "secrets", entry.Name()+".json")); err == nil {
			secret = "yes"
		}

		prefix := "  "
		if entry.Name() == activeName {
			prefix = "* "
		}
		description, _ := metaMap["description"].(string)
		fileList := "-"
		if len(filtered) > 0 {
			fileList = strings.Join(filtered, ",")
		}
		lines = append(lines, fmt.Sprintf("%s%s | %s | files=%s | secret=%s", prefix, entry.Name(), description, fileList, secret))
	}

	sort.Strings(lines)
	for _, line := range lines {
		if _, err := fmt.Fprintln(stdout, line); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) commitProfile(stdout io.Writer, message string) error {
	if _, err := os.Stat(filepath.Join(a.repoRoot, ".git")); err != nil {
		return fmt.Errorf("not a git repository: %s", a.repoRoot)
	}

	cmd := exec.Command("git", "-C", a.repoRoot, "add", "-A")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git add failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	cmd = exec.Command("git", "-C", a.repoRoot, "status", "--porcelain")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git status failed: %w", err)
	}
	if len(strings.TrimSpace(string(output))) == 0 {
		_, err = fmt.Fprintln(stdout, "nothing to commit")
		return err
	}

	if message == "" {
		message = "update profile config"
	}
	cmd = exec.Command("git", "-C", a.repoRoot, "commit", "-m", message)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git commit failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	_, err = fmt.Fprintf(stdout, "committed: %s\n", strings.Split(string(output), "\n")[0])
	return err
}

func (a *app) ensureCompletionInstall(root *cobra.Command) error {
	if err := a.ensureRepoDirs(); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	state := map[string]completionState{}
	shells := []struct {
		name    string
		rcPath  string
		command string
		block   string
	}{
		{
			name:    "zsh",
			rcPath:  filepath.Join(a.home, ".zshrc"),
			command: "source <(claude-profile completion zsh)",
			block:   "# claude-profile completion start\nsource <(claude-profile completion zsh)\n# claude-profile completion end\n",
		},
		{
			name:    "bash",
			rcPath:  filepath.Join(a.home, ".bashrc"),
			command: "source <(claude-profile completion bash)",
			block:   "# claude-profile completion start\nsource <(claude-profile completion bash)\n# claude-profile completion end\n",
		},
		{
			name:    "fish",
			rcPath:  filepath.Join(a.home, ".config", "fish", "config.fish"),
			command: "claude-profile completion fish | source",
			block:   "# claude-profile completion start\nclaude-profile completion fish | source\n# claude-profile completion end\n",
		},
	}

	for _, shell := range shells {
		if err := ensureTextContains(shell.rcPath, shell.command, shell.block); err != nil {
			return err
		}
		state[shell.name] = completionState{
			Installed: true,
			RCPath:    shell.rcPath,
			UpdatedAt: now,
		}
	}

	return a.writeJSONFile(filepath.Join(a.repoRoot, "state", "completion.json"), structMap(state))
}

func (a *app) ensureRepoDirs() error {
	for _, path := range []string{
		a.repoRoot,
		filepath.Join(a.repoRoot, "common"),
		filepath.Join(a.repoRoot, "profiles"),
		filepath.Join(a.repoRoot, "secrets"),
		filepath.Join(a.repoRoot, "state"),
		filepath.Join(a.repoRoot, "backups"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
	}
	return nil
}

func (a *app) ensureGitRepo() error {
	if _, err := os.Stat(filepath.Join(a.repoRoot, ".git")); err == nil {
		return nil
	}
	cmd := exec.Command("git", "init", a.repoRoot)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git init failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (a *app) ensureGitIgnore() error {
	path := filepath.Join(a.repoRoot, ".gitignore")
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return os.WriteFile(path, []byte(gitIgnoreContent), 0o644)
	}
	if err != nil {
		return err
	}
	if string(raw) == gitIgnoreContent {
		return nil
	}

	content := string(raw)
	for _, line := range strings.Split(strings.TrimSpace(gitIgnoreContent), "\n") {
		if !strings.Contains(content, line) {
			if !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
			content += line + "\n"
		}
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func (a *app) writeStarterCommon(settings map[string]any) error {
	nonSensitive, _ := splitSensitiveTree(settings)
	nonSensitiveMap := ensureJSONObject(nonSensitive)
	nonSensitiveMap = stripProfileScopedSettings(nonSensitiveMap)

	hooks := selectKeys(nonSensitiveMap, "hooks")
	security := selectKeys(nonSensitiveMap, "permissions", "sandbox", "skipAutoPermissionPrompt", "skipDangerousModePermissionPrompt")
	plugins := selectKeys(nonSensitiveMap, "enabledPlugins", "extraKnownMarketplaces")
	shared := map[string]any{}
	for key, value := range nonSensitiveMap {
		if key == "hooks" || key == "permissions" || key == "sandbox" || key == "skipAutoPermissionPrompt" || key == "skipDangerousModePermissionPrompt" || key == "enabledPlugins" || key == "extraKnownMarketplaces" {
			continue
		}
		shared[key] = cloneValue(value)
	}

	files := []struct {
		name string
		data map[string]any
	}{
		{"10-hooks.json", hooks},
		{"20-security.json", security},
		{"30-marketplace-plugin.json", plugins},
		{"90-shared.json", shared},
	}
	for _, file := range files {
		if err := a.writeJSONFile(filepath.Join(a.repoRoot, "common", file.name), file.data); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) prepareProfileDir(profileDir string, force bool) error {
	if _, err := os.Stat(profileDir); err == nil {
		if !force {
			return fmt.Errorf("profile %q already exists; use --force to overwrite", filepath.Base(profileDir))
		}
		if err := os.RemoveAll(profileDir); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.MkdirAll(profileDir, 0o755)
}

func (a *app) prepareProfileArtifacts(stdin io.Reader, stdout io.Writer, profileDir, secretPath string, force bool) error {
	profileExists := pathExists(profileDir)
	secretExists := pathExists(secretPath)
	if !profileExists && !secretExists {
		return os.MkdirAll(profileDir, 0o755)
	}

	name := filepath.Base(profileDir)
	if !force {
		return fmt.Errorf("profile %q already exists; use --force to overwrite", name)
	}

	reader := bufio.NewReader(stdin)
	if err := confirmDelete(reader, stdout, fmt.Sprintf("Type the profile name (%s) to overwrite it: ", name), name); err != nil {
		return err
	}
	if err := confirmDelete(reader, stdout, "Type DELETE to permanently overwrite this profile: ", "DELETE"); err != nil {
		return err
	}

	if profileExists {
		if err := os.RemoveAll(profileDir); err != nil {
			return err
		}
	}
	return os.MkdirAll(profileDir, 0o755)
}

func (a *app) mergeConfigDir(dir string, ignore map[string]struct{}) (map[string]any, error) {
	files, err := readConfigFilenames(dir, false)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	merged := map[string]any{}
	for _, file := range files {
		if _, skipped := ignore[file]; skipped {
			continue
		}
		data, err := a.readJSONFile(filepath.Join(dir, file))
		if err != nil {
			return nil, err
		}
		merged = mergeMaps(merged, data)
	}
	return merged, nil
}

func (a *app) mergeIntoExisting(base map[string]any, dir string, ignore map[string]struct{}) (map[string]any, error) {
	files, err := readConfigFilenames(dir, false)
	if errors.Is(err, os.ErrNotExist) {
		return base, nil
	}
	if err != nil {
		return nil, err
	}
	merged := cloneMap(base)
	for _, file := range files {
		if _, skipped := ignore[file]; skipped {
			continue
		}
		data, err := a.readJSONFile(filepath.Join(dir, file))
		if err != nil {
			return nil, err
		}
		merged = mergeMaps(merged, data)
	}
	return merged, nil
}

func (a *app) backupTarget(targetPath string) error {
	if _, err := os.Stat(targetPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}

	raw, err := os.ReadFile(targetPath)
	if err != nil {
		return err
	}
	backupName := fmt.Sprintf("%s-%s.json", strings.TrimSuffix(filepath.Base(targetPath), filepath.Ext(targetPath)), time.Now().UTC().Format("20060102T150405.000000000Z"))
	backupPath := filepath.Join(a.repoRoot, "backups", backupName)
	return os.WriteFile(backupPath, raw, 0o644)
}

func (a *app) writeActiveProfile(name string) error {
	return a.writeJSONFile(filepath.Join(a.repoRoot, "state", "active.json"), structToMap(activeState{
		Name:      name,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}))
}

func (a *app) writeAtomicJSON(path string, data map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := marshalJSON(data)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".claude-profile-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func (a *app) writeJSONFile(path string, data map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := marshalJSON(data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func (a *app) readJSONFile(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if len(strings.TrimSpace(string(raw))) == 0 {
		return map[string]any{}, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func (a *app) readOptionalJSONFile(path string) (map[string]any, error) {
	data, err := a.readJSONFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

func readConfigFilenames(dir string, emptyMissing bool) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) && emptyMissing {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) == ".json" {
			files = append(files, name)
		}
	}
	sort.Strings(files)
	return files, nil
}

func ensureTextContains(path, marker, block string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return os.WriteFile(path, []byte(block), 0o644)
	}
	if err != nil {
		return err
	}
	content := string(raw)
	if strings.Contains(content, marker) {
		return nil
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += block
	return os.WriteFile(path, []byte(content), 0o644)
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func confirmDelete(reader *bufio.Reader, stdout io.Writer, prompt, expected string) error {
	if _, err := fmt.Fprint(stdout, prompt); err != nil {
		return err
	}
	input, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if strings.TrimSpace(input) != expected {
		return fmt.Errorf("delete aborted: confirmation did not match")
	}
	return nil
}

func marshalJSON(data map[string]any) ([]byte, error) {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func diffValues(base, target any) map[string]any {
	diff, changed := diffValue(base, target)
	if !changed {
		return map[string]any{}
	}
	if out, ok := diff.(map[string]any); ok {
		return out
	}
	return map[string]any{}
}

func diffValue(base, target any) (any, bool) {
	baseMap, baseIsMap := asMap(base)
	targetMap, targetIsMap := asMap(target)
	if baseIsMap && targetIsMap {
		out := map[string]any{}
		for key, targetValue := range targetMap {
			baseValue, exists := baseMap[key]
			if !exists {
				out[key] = cloneValue(targetValue)
				continue
			}
			diff, changed := diffValue(baseValue, targetValue)
			if changed {
				out[key] = diff
			}
		}
		if len(out) == 0 {
			return nil, false
		}
		return out, true
	}

	if reflect.DeepEqual(base, target) {
		return nil, false
	}
	return cloneValue(target), true
}

func splitSensitiveTree(value any) (any, any) {
	return splitSensitiveValue("", value, false)
}

func splitSensitiveValue(key string, value any, forceSensitive bool) (any, any) {
	if forceSensitive || isSensitiveKey(key) {
		return nil, cloneValue(value)
	}

	valueMap, ok := asMap(value)
	if !ok {
		return cloneValue(value), nil
	}

	nonSensitive := map[string]any{}
	sensitive := map[string]any{}
	for childKey, childValue := range valueMap {
		ns, secret := splitSensitiveValue(childKey, childValue, false)
		if ns != nil {
			nonSensitive[childKey] = ns
		}
		if secret != nil {
			sensitive[childKey] = secret
		}
	}

	var nsOut any
	var secretOut any
	if len(nonSensitive) > 0 {
		nsOut = nonSensitive
	}
	if len(sensitive) > 0 {
		secretOut = sensitive
	}
	return nsOut, secretOut
}

func isSensitiveKey(key string) bool {
	if key == "" {
		return false
	}
	upper := strings.ToUpper(key)
	return strings.Contains(upper, "TOKEN") || strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "SECRET") || strings.HasSuffix(upper, "_KEY")
}

func mergeMaps(base, overlay map[string]any) map[string]any {
	out := cloneMap(base)
	for key, overlayValue := range overlay {
		if baseValue, ok := out[key]; ok {
			out[key] = mergeValue(baseValue, overlayValue)
			continue
		}
		out[key] = cloneValue(overlayValue)
	}
	return out
}

func mergeValue(base, overlay any) any {
	baseMap, baseOK := asMap(base)
	overlayMap, overlayOK := asMap(overlay)
	if baseOK && overlayOK {
		return mergeMaps(baseMap, overlayMap)
	}
	return cloneValue(overlay)
}

func selectKeys(input map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for _, key := range keys {
		if value, ok := input[key]; ok {
			out[key] = cloneValue(value)
		}
	}
	return out
}

func extractProfileScopedSettings(settings map[string]any) map[string]any {
	envValue, ok := settings["env"]
	if !ok {
		return map[string]any{}
	}
	envMap, ok := envValue.(map[string]any)
	if !ok {
		return map[string]any{}
	}

	profileEnv := map[string]any{}
	for key := range profileScopedEnvKeys {
		if value, exists := envMap[key]; exists {
			profileEnv[key] = cloneValue(value)
		}
	}
	if len(profileEnv) == 0 {
		return map[string]any{}
	}
	return map[string]any{"env": profileEnv}
}

func stripProfileScopedSettings(settings map[string]any) map[string]any {
	out := cloneMap(settings)
	envValue, ok := out["env"]
	if !ok {
		return out
	}
	envMap, ok := envValue.(map[string]any)
	if !ok {
		return out
	}

	filteredEnv := cloneMap(envMap)
	for key := range profileScopedEnvKeys {
		delete(filteredEnv, key)
	}
	if len(filteredEnv) == 0 {
		delete(out, "env")
		return out
	}
	out["env"] = filteredEnv
	return out
}

func ensureJSONObject(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if out, ok := value.(map[string]any); ok {
		return out
	}
	return map[string]any{}
}

func cloneMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = cloneValue(value)
	}
	return out
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, cloneValue(item))
		}
		return out
	default:
		return typed
	}
}

func asMap(value any) (map[string]any, bool) {
	typed, ok := value.(map[string]any)
	return typed, ok
}

func structToMap(value any) map[string]any {
	raw, _ := json.Marshal(value)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

func structMap(input any) map[string]any {
	raw, _ := json.Marshal(input)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}
