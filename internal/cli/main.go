package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/breinzhang/cc-link/internal/statusline"
)

const (
	appName           = "cc-link"
	configDirName     = ".cc-link"
	configFileName    = "cc-link.json"
	projectConfigPath = configDirName + string(filepath.Separator) + configFileName
	lockFileName      = ".cc-link.lock.json"
	defaultTargetRoot = ".claude"
)

var errSourceRootNotConfigured = errors.New("sourceRoot is not configured")

type Config struct {
	SourceRoot string              `json:"sourceRoot,omitempty"`
	TargetRoot string              `json:"targetRoot,omitempty"`
	Statusline json.RawMessage     `json:"statusline,omitempty"`
	Links      map[string][]string `json:"links,omitempty"`
}

type Options struct {
	SourceRoot string
	TargetRoot string
	ConfigPath string
	DryRun     bool
	Force      bool
	Prune      bool
}

type LinkItem struct {
	Kind   string
	Name   string
	Group  string
	Source string
	Target string
}

type Lock struct {
	Version   int         `json:"version"`
	UpdatedAt string      `json:"updatedAt"`
	Entries   []LockEntry `json:"entries"`
}

type LockEntry struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Group     string `json:"group,omitempty"`
	Source    string `json:"source"`
	Target    string `json:"target"`
	LinkType  string `json:"linkType,omitempty"`
	CreatedAt string `json:"createdAt"`
}

type statuslineProviderPriceDefault struct {
	Provider string                        `json:"provider"`
	Models   []statuslineModelPriceDefault `json:"models"`
}

type statuslineModelPriceDefault struct {
	Match      string  `json:"match"`
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheWrite float64 `json:"cacheWrite"`
	CacheRead  float64 `json:"cacheRead,omitempty"`
}

var defaultStatuslinePrices = []statuslineProviderPriceDefault{
	{Provider: "Kimi/Moonshot", Models: []statuslineModelPriceDefault{
		{Match: "kimi-k2.7-code*", Input: 0.95, Output: 4.0, CacheWrite: 0.95, CacheRead: 0.19},
		{Match: "kimi-k2.6*", Input: 0.95, Output: 4.0, CacheWrite: 0.95, CacheRead: 0.16},
		{Match: "kimi-k2.5*", Input: 0.60, Output: 3.0, CacheWrite: 0.60, CacheRead: 0.10},
	}},
	{Provider: "GLM/Z.AI", Models: []statuslineModelPriceDefault{
		{Match: "glm-5.2*", Input: 1.4, Output: 4.4, CacheWrite: 1.4, CacheRead: 0.26},
		{Match: "glm-5.1*", Input: 1.4, Output: 4.4, CacheWrite: 1.4, CacheRead: 0.26},
		{Match: "glm-5-turbo*", Input: 1.2, Output: 4.0, CacheWrite: 1.2, CacheRead: 0.24},
		{Match: "glm-5*", Input: 1.0, Output: 3.2, CacheWrite: 1.0, CacheRead: 0.20},
		{Match: "glm-4.7-flashx*", Input: 0.07, Output: 0.4, CacheWrite: 0.07, CacheRead: 0.01},
		{Match: "glm-4.7-flash*", Input: 0, Output: 0},
		{Match: "glm-4.7*", Input: 0.6, Output: 2.2, CacheWrite: 0.6, CacheRead: 0.11},
		{Match: "glm-4.6*", Input: 0.6, Output: 2.2, CacheWrite: 0.6, CacheRead: 0.11},
		{Match: "glm-4.5-airx*", Input: 1.1, Output: 4.5, CacheWrite: 1.1, CacheRead: 0.22},
		{Match: "glm-4.5-air*", Input: 0.2, Output: 1.1, CacheWrite: 0.2, CacheRead: 0.03},
		{Match: "glm-4.5-x*", Input: 2.2, Output: 8.9, CacheWrite: 2.2, CacheRead: 0.45},
		{Match: "glm-4.5-flash*", Input: 0, Output: 0},
		{Match: "glm-4.5*", Input: 0.6, Output: 2.2, CacheWrite: 0.6, CacheRead: 0.11},
		{Match: "glm-4-32b-0414-128k*", Input: 0.1, Output: 0.1, CacheWrite: 0.1},
	}},
	{Provider: "DeepSeek", Models: []statuslineModelPriceDefault{
		{Match: "deepseek-v4-pro*", Input: 0.435, Output: 0.87, CacheWrite: 0.435, CacheRead: 0.003625},
		{Match: "deepseek-v4-flash*", Input: 0.14, Output: 0.28, CacheWrite: 0.14, CacheRead: 0.0028},
		{Match: "deepseek-chat", Input: 0.14, Output: 0.28, CacheWrite: 0.14, CacheRead: 0.0028},
		{Match: "deepseek-reasoner", Input: 0.14, Output: 0.28, CacheWrite: 0.14, CacheRead: 0.0028},
	}},
	{Provider: "MiniMax", Models: []statuslineModelPriceDefault{
		{Match: "minimax-m3*", Input: 0.30, Output: 1.20, CacheWrite: 0.30, CacheRead: 0.06},
		{Match: "minimax-m2.7-highspeed*", Input: 0.60, Output: 2.40, CacheWrite: 0.375, CacheRead: 0.06},
		{Match: "minimax-m2.7*", Input: 0.30, Output: 1.20, CacheWrite: 0.375, CacheRead: 0.06},
		{Match: "minimax-m2.5-highspeed*", Input: 0.60, Output: 2.40, CacheWrite: 0.375, CacheRead: 0.03},
		{Match: "minimax-m2.5*", Input: 0.30, Output: 1.20, CacheWrite: 0.375, CacheRead: 0.03},
	}},
}

func Main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "init":
		err = cmdInit(os.Args[2:])
	case "apply":
		err = cmdApply(os.Args[2:])
	case "link", "add":
		err = cmdLink(os.Args[2:])
	case "unlink", "remove", "rm", "clean":
		err = cmdUnlink(os.Args[2:])
	case "status", "list", "ls":
		err = cmdStatus(os.Args[2:])
	case "statusline":
		err = statusline.Command(os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
		return
	case "version", "--version":
		fmt.Println(appName + " v0.1.0")
		return
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`%s - link shared Claude Code assets into the current workspace.

Usage:
  %[1]s init --src <library-src-root> [--target .claude] [--statusline]
  %[1]s apply [--config .cc-link/cc-link.json] [--src <root>] [--target .claude] [--force] [--dry-run] [--prune]
  %[1]s link [--src <root>] [--target .claude] [--force] [--dry-run] <skills|kind|all> [name...]
  %[1]s unlink [--src <root>] [--target .claude] [--dry-run] <skills|kind|all> [--all] [name...]
  %[1]s status [--target .claude]
  %[1]s statusline [enable|disable]

Examples:
  %[1]s init --src "/path/to/claude-assets"
  %[1]s init --src "/path/to/claude-assets" --statusline
  %[1]s statusline enable
  %[1]s link skills mattpocock superpowers
  %[1]s link skills mattpocock/some-skill
  %[1]s link agents reviewer.md
  %[1]s link commands git/commit.md
  %[1]s link rules memory.md
  %[1]s unlink skills mattpocock
  %[1]s apply --config .cc-link/cc-link.json --prune

Notes:
  - The target root defaults to .claude under the current working directory.
  - Skills are flattened: skills/mattpocock/<skill> -> .claude/skills/<skill>.
  - Other kinds are mapped directly: rules/x.md -> .claude/rules/x.md.
  - Existing real files/directories are never overwritten. --force only replaces existing symlinks.
`, appName)
}

func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	src := fs.String("src", "", "library src root, e.g. /path/to/claude-assets")
	target := fs.String("target", defaultTargetRoot, "default target root")
	enableLine := fs.Bool("statusline", false, "enable Claude Code statusline")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*src) == "" {
		return errors.New("--src is required")
	}
	cfg := Config{SourceRoot: filepath.Clean(expandVars(*src)), TargetRoot: filepath.Clean(expandVars(*target))}
	if err := validateSourceRoot(cfg.SourceRoot); err != nil {
		return err
	}
	p, err := globalConfigPath()
	if err != nil {
		return err
	}
	if existing, ok, err := readConfigIfExists(p); err != nil {
		return err
	} else if ok {
		existing.SourceRoot = cfg.SourceRoot
		existing.TargetRoot = cfg.TargetRoot
		cfg = existing
	}
	cfg.Statusline = withDefaultStatuslineConfig(cfg.Statusline)
	if err := writeConfigFile(p, cfg); err != nil {
		return err
	}
	fmt.Println("wrote global config:", p)
	if *enableLine {
		if err := statusline.Enable(); err != nil {
			return err
		}
		fmt.Println("enabled Claude Code statusline")
	}
	return nil
}

func cmdApply(args []string) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	opts := newCommonOptions(fs)
	fs.BoolVar(&opts.Prune, "prune", false, "remove symlinks created by this tool but not present in the config")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadEffectiveConfig(*opts)
	if err != nil {
		return err
	}
	if isEmptyLinks(cfg.Links) {
		kinds, err := allSourceKinds(cfg.SourceRoot)
		if err != nil {
			return err
		}
		cfg.Links = map[string][]string{}
		for _, kind := range kinds {
			cfg.Links[kind] = []string{"*"}
		}
	}
	items, err := resolveLinkSpec(cfg)
	if err != nil {
		return err
	}
	opts.TargetRoot = cfg.TargetRoot
	linkedTargets, err := linkItems(items, *opts)
	if err != nil {
		return err
	}
	if opts.Prune {
		return pruneNotInSet(cfg.TargetRoot, linkedTargets, opts.DryRun)
	}
	return nil
}

func cmdLink(args []string) error {
	fs := flag.NewFlagSet("link", flag.ContinueOnError)
	opts := newCommonOptions(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	cliSource := opts.SourceRoot
	cliTarget := opts.TargetRoot
	rest := fs.Args()
	if len(rest) < 1 {
		return errors.New("missing kind: skills, agents, commands or all")
	}
	kind := normalizeKind(rest[0])
	names := rest[1:]
	cfg, err := loadEffectiveConfig(*opts)
	if err != nil {
		return err
	}
	spec := map[string][]string{}
	switch kind {
	case "skills":
		if len(names) == 0 {
			return errors.New("missing skill category or name")
		}
		spec[kind] = names
	case "all":
		if len(names) == 0 {
			names = []string{"*"}
		}
		kinds, err := allSourceKinds(cfg.SourceRoot)
		if err != nil {
			return err
		}
		for _, k := range kinds {
			spec[k] = names
		}
	default:
		if len(names) == 0 {
			return fmt.Errorf("missing %s item name", kind)
		}
		if err := validateKindRoot(cfg.SourceRoot, kind); err != nil {
			return err
		}
		spec[kind] = names
	}
	cfg.Links = spec
	items, err := resolveLinkSpec(cfg)
	if err != nil {
		return err
	}
	opts.TargetRoot = cfg.TargetRoot
	if _, err = linkItems(items, *opts); err != nil {
		return err
	}
	if opts.DryRun {
		return nil
	}
	configSourceRoot := ""
	if cliSource != "" {
		configSourceRoot = cfg.SourceRoot
	}
	configTargetRoot := ""
	if cliTarget != "" {
		configTargetRoot = cfg.TargetRoot
	}
	return mergeProjectConfigLinks(chooseConfigPath(opts.ConfigPath), spec, configSourceRoot, configTargetRoot)
}

func cmdUnlink(args []string) error {
	fs := flag.NewFlagSet("unlink", flag.ContinueOnError)
	opts := newCommonOptions(fs)
	allFlag := fs.Bool("all", false, "unlink all symlinks of the selected kind")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) < 1 {
		return errors.New("missing kind: skills, agents, commands or all")
	}
	kind := normalizeKind(rest[0])
	names := rest[1:]
	if kind == "all" {
		*allFlag = true
	}
	if !*allFlag && len(names) == 0 {
		return errors.New("missing name; use --all to remove all symlinks of this kind")
	}
	cfg, err := loadEffectiveConfig(*opts)
	if err != nil {
		if !errors.Is(err, errSourceRootNotConfigured) {
			return err
		}
		cfg = Config{TargetRoot: pickTargetRoot(opts.TargetRoot, "")}
	}
	return unlinkItems(cfg, *opts, kind, names, *allFlag)
}

func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	opts := newCommonOptions(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := loadEffectiveConfig(*opts)
	if err != nil {
		if !errors.Is(err, errSourceRootNotConfigured) {
			return err
		}
		cfg = Config{TargetRoot: pickTargetRoot(opts.TargetRoot, "")}
	}
	lock, err := readLock(cfg.TargetRoot)
	if err != nil {
		return err
	}
	if len(lock.Entries) == 0 {
		fmt.Println("no symlinks recorded by", appName)
		return nil
	}
	sort.Slice(lock.Entries, func(i, j int) bool {
		if lock.Entries[i].Kind != lock.Entries[j].Kind {
			return lock.Entries[i].Kind < lock.Entries[j].Kind
		}
		return lock.Entries[i].Name < lock.Entries[j].Name
	})
	for _, e := range lock.Entries {
		state := "ok"
		if _, err := os.Lstat(e.Target); err != nil {
			state = "missing"
		}
		group := ""
		if e.Group != "" {
			group = " group=" + e.Group
		}
		fmt.Printf("%-8s %-32s %-8s%s\n  %s -> %s\n", e.Kind, e.Name, state, group, e.Target, e.Source)
	}
	return nil
}

func newCommonOptions(fs *flag.FlagSet) *Options {
	opts := &Options{}
	fs.StringVar(&opts.SourceRoot, "src", "", "library src root")
	fs.StringVar(&opts.TargetRoot, "target", "", "target root; defaults to .claude")
	fs.StringVar(&opts.ConfigPath, "config", "", "project config path; defaults to .cc-link/cc-link.json")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "print actions without changing files")
	fs.BoolVar(&opts.Force, "force", false, "replace existing symlinks when they point to a different source")
	return opts
}

func loadEffectiveConfig(opts Options) (Config, error) {
	cfg := Config{TargetRoot: defaultTargetRoot}

	globalPath, err := globalConfigPath()
	if err != nil {
		return cfg, err
	}
	if c, ok, err := readConfigIfExists(globalPath); err != nil {
		return cfg, err
	} else if ok {
		cfg = mergeConfig(cfg, c)
	}

	projectPath := chooseConfigPath(opts.ConfigPath)
	if c, ok, err := readConfigIfExists(projectPath); err != nil {
		return cfg, err
	} else if ok {
		cfg = mergeConfig(cfg, c)
	}

	if opts.SourceRoot != "" {
		cfg.SourceRoot = opts.SourceRoot
	}
	if opts.TargetRoot != "" {
		cfg.TargetRoot = opts.TargetRoot
	}

	cfg.SourceRoot = strings.TrimSpace(expandVars(cfg.SourceRoot))
	if cfg.SourceRoot != "" {
		cfg.SourceRoot = filepath.Clean(cfg.SourceRoot)
	}
	cfg.TargetRoot = filepath.Clean(expandVars(cfg.TargetRoot))
	if cfg.TargetRoot == "." || cfg.TargetRoot == "" {
		cfg.TargetRoot = defaultTargetRoot
	}

	if cfg.SourceRoot == "" {
		return cfg, fmt.Errorf("%w; run `%s init --src <path>` or pass --src", errSourceRootNotConfigured, appName)
	}
	if err := validateSourceRoot(cfg.SourceRoot); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func pickTargetRoot(cliTarget, cfgTarget string) string {
	if cliTarget != "" {
		return filepath.Clean(expandVars(cliTarget))
	}
	if cfgTarget != "" {
		return filepath.Clean(expandVars(cfgTarget))
	}
	return defaultTargetRoot
}

func chooseConfigPath(p string) string {
	if p != "" {
		return filepath.Clean(expandVars(p))
	}
	return projectConfigPath
}

func globalConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDirName, configFileName), nil
}

func readConfigIfExists(path string) (Config, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, false, nil
		}
		return Config{}, false, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, true, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return cfg, true, nil
}

func mergeConfig(base, override Config) Config {
	if override.SourceRoot != "" {
		base.SourceRoot = override.SourceRoot
	}
	if override.TargetRoot != "" {
		base.TargetRoot = override.TargetRoot
	}
	if len(override.Statusline) > 0 {
		base.Statusline = append(json.RawMessage(nil), override.Statusline...)
	}
	if len(override.Links) > 0 {
		if base.Links == nil {
			base.Links = map[string][]string{}
		}
		for kind, args := range override.Links {
			if len(args) > 0 {
				base.Links[kind] = args
			}
		}
	}
	return base
}

func mergeProjectConfigLinks(path string, links map[string][]string, sourceRoot, targetRoot string) error {
	cfg, ok, err := readConfigIfExists(path)
	if err != nil {
		return err
	}
	if !ok {
		cfg = Config{}
	}
	if cfg.Links == nil {
		cfg.Links = map[string][]string{}
	}
	if sourceRoot != "" {
		cfg.SourceRoot = sourceRoot
	}
	if targetRoot != "" {
		cfg.TargetRoot = targetRoot
	}
	var kinds []string
	for kind := range links {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		cfg.Links[kind] = appendUnique(cfg.Links[kind], links[kind]...)
	}
	return writeConfigFile(path, cfg)
}

func writeConfigFile(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func withDefaultStatuslineConfig(raw json.RawMessage) json.RawMessage {
	statusline := map[string]json.RawMessage{}
	if len(raw) > 0 && string(raw) != "null" {
		_ = json.Unmarshal(raw, &statusline)
	}
	if statusline == nil {
		statusline = map[string]json.RawMessage{}
	}
	if _, ok := statusline["lines"]; !ok {
		statusline["lines"] = json.RawMessage("2")
	}

	cost := map[string]json.RawMessage{}
	if rawCost, ok := statusline["cost"]; ok && len(rawCost) > 0 && string(rawCost) != "null" {
		_ = json.Unmarshal(rawCost, &cost)
	}
	if cost == nil {
		cost = map[string]json.RawMessage{}
	}
	if _, ok := cost["enabled"]; !ok {
		cost["enabled"] = json.RawMessage("true")
	}
	if _, ok := cost["currency"]; !ok {
		cost["currency"] = json.RawMessage(`"USD"`)
	}
	cost["prices"] = withDefaultStatuslinePrices(cost["prices"])
	costData, _ := json.Marshal(cost)
	statusline["cost"] = costData

	data, _ := json.Marshal(statusline)
	return data
}

func withDefaultStatuslinePrices(raw json.RawMessage) json.RawMessage {
	var prices []statuslineProviderPriceDefault
	if len(raw) > 0 && string(raw) != "null" {
		_ = json.Unmarshal(raw, &prices)
	}
	prices = validStatuslineProviderPrices(prices)

	providerIndexes := map[string]int{}
	for i, provider := range prices {
		key := strings.ToLower(strings.TrimSpace(provider.Provider))
		if key == "" {
			continue
		}
		providerIndexes[key] = i
	}
	for _, defaults := range defaultStatuslinePrices {
		key := strings.ToLower(strings.TrimSpace(defaults.Provider))
		i, ok := providerIndexes[key]
		if !ok {
			prices = append(prices, defaults)
			providerIndexes[key] = len(prices) - 1
			continue
		}
		prices[i].Models = appendMissingStatuslineModels(prices[i].Models, defaults.Models)
	}
	data, _ := json.Marshal(prices)
	return data
}

func validStatuslineProviderPrices(prices []statuslineProviderPriceDefault) []statuslineProviderPriceDefault {
	out := make([]statuslineProviderPriceDefault, 0, len(prices))
	for _, provider := range prices {
		provider.Provider = strings.TrimSpace(provider.Provider)
		if provider.Provider == "" {
			continue
		}
		models := appendMissingStatuslineModels(nil, provider.Models)
		if len(models) == 0 {
			continue
		}
		provider.Models = orderSpecificModelsBeforeBroadWildcards(models)
		out = append(out, provider)
	}
	return out
}

func appendMissingStatuslineModels(models, add []statuslineModelPriceDefault) []statuslineModelPriceDefault {
	seen := map[string]int{}
	for i, model := range models {
		key := strings.ToLower(strings.TrimSpace(model.Match))
		if key != "" {
			seen[key] = i
		}
	}
	for _, model := range add {
		key := strings.ToLower(strings.TrimSpace(model.Match))
		if key == "" {
			continue
		}
		if i, ok := seen[key]; ok {
			if models[i].CacheWrite == 0 && model.CacheWrite != 0 {
				models[i].CacheWrite = defaultCacheWriteForExistingModel(models[i], model)
			}
			continue
		}
		insertAt := shadowingModelIndex(models, model.Match)
		if insertAt == len(models) {
			models = append(models, model)
		} else {
			models = append(models, statuslineModelPriceDefault{})
			copy(models[insertAt+1:], models[insertAt:])
			models[insertAt] = model
		}
		seen = map[string]int{}
		for i, model := range models {
			key := strings.ToLower(strings.TrimSpace(model.Match))
			if key != "" {
				seen[key] = i
			}
		}
	}
	return orderSpecificModelsBeforeBroadWildcards(models)
}

func shadowingModelIndex(models []statuslineModelPriceDefault, match string) int {
	for i, model := range models {
		if modelPatternShadows(model.Match, match) {
			return i
		}
	}
	return len(models)
}

func modelPatternShadows(pattern, match string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	match = strings.ToLower(strings.TrimSpace(match))
	if pattern == "" || match == "" || pattern == match || !strings.HasSuffix(pattern, "*") {
		return false
	}
	return strings.HasPrefix(strings.TrimSuffix(match, "*"), strings.TrimSuffix(pattern, "*"))
}

func orderSpecificModelsBeforeBroadWildcards(models []statuslineModelPriceDefault) []statuslineModelPriceDefault {
	ordered := append([]statuslineModelPriceDefault(nil), models...)
	for i := 0; i < len(ordered); i++ {
		for j := 0; j < i; j++ {
			if modelPatternShadows(ordered[j].Match, ordered[i].Match) {
				model := ordered[i]
				copy(ordered[j+1:i+1], ordered[j:i])
				ordered[j] = model
				break
			}
		}
	}
	return ordered
}

func defaultCacheWriteForExistingModel(existing, defaults statuslineModelPriceDefault) float64 {
	if defaults.CacheWrite == defaults.Input && existing.Input != 0 {
		return existing.Input
	}
	return defaults.CacheWrite
}

func appendUnique(values []string, add ...string) []string {
	seen := map[string]struct{}{}
	for _, v := range values {
		seen[v] = struct{}{}
	}
	for _, v := range add {
		if _, ok := seen[v]; ok {
			continue
		}
		values = append(values, v)
		seen[v] = struct{}{}
	}
	return values
}

func validateSourceRoot(src string) error {
	if src == "" {
		return errors.New("sourceRoot is empty")
	}
	st, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("sourceRoot not found: %s", src)
	}
	if !st.IsDir() {
		return fmt.Errorf("sourceRoot is not a directory: %s", src)
	}
	for _, d := range []string{"skills", "agents", "commands"} {
		p := filepath.Join(src, d)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return fmt.Errorf("%s exists but is not a directory", p)
		}
	}
	return nil
}

func validateKindRoot(sourceRoot, kind string) error {
	p := filepath.Join(sourceRoot, kind)
	st, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("link kind %q not found under sourceRoot: %s", kind, p)
		}
		return err
	}
	if !st.IsDir() {
		return fmt.Errorf("link kind %q is not a directory: %s", kind, p)
	}
	return nil
}

func allSourceKinds(sourceRoot string) ([]string, error) {
	entries, err := os.ReadDir(sourceRoot)
	if err != nil {
		return nil, err
	}
	var kinds []string
	for _, e := range entries {
		if shouldSkipName(e.Name()) {
			continue
		}
		if e.IsDir() {
			kinds = append(kinds, e.Name())
		}
	}
	sort.Strings(kinds)
	return kinds, nil
}

func isEmptyLinks(links map[string][]string) bool {
	for _, args := range links {
		if len(args) > 0 {
			return false
		}
	}
	return true
}

func resolveLinkSpec(cfg Config) ([]LinkItem, error) {
	var items []LinkItem
	var kinds []string
	for kind := range cfg.Links {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		for _, n := range cfg.Links[kind] {
			var xs []LinkItem
			var err error
			if kind == "skills" {
				xs, err = resolveSkills(cfg.SourceRoot, cfg.TargetRoot, n)
			} else {
				xs, err = resolveGenericKind(cfg.SourceRoot, cfg.TargetRoot, kind, n)
			}
			if err != nil {
				return nil, err
			}
			items = append(items, xs...)
		}
	}
	return dedupeItems(items)
}

func resolveSkills(sourceRoot, targetRoot, arg string) ([]LinkItem, error) {
	arg = cleanArg(arg)
	base := filepath.Join(sourceRoot, "skills")
	if isAllArg(arg) {
		cats, err := immediateDirs(base)
		if err != nil {
			return nil, err
		}
		var out []LinkItem
		for _, cat := range cats {
			xs, err := resolveSkillCategory(sourceRoot, targetRoot, filepath.Base(cat), cat)
			if err != nil {
				return nil, err
			}
			out = append(out, xs...)
		}
		return out, nil
	}

	parts := splitLinkArg(arg)
	if len(parts) == 1 {
		catDir := filepath.Join(base, parts[0])
		if st, err := os.Stat(catDir); err == nil && st.IsDir() {
			return resolveSkillCategory(sourceRoot, targetRoot, parts[0], catDir)
		}

		matches, err := findSkillByName(base, parts[0])
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("skill category or skill not found: %s", arg)
		}
		if len(matches) > 1 {
			var names []string
			for _, m := range matches {
				names = append(names, m)
			}
			return nil, fmt.Errorf("multiple skills named %q found; use category/name. matches: %s", arg, strings.Join(names, ", "))
		}
		m := matches[0]
		return []LinkItem{skillItem(targetRoot, m, filepath.Base(filepath.Dir(m)))}, nil
	}

	if len(parts) == 2 {
		direct := filepath.Join(base, parts[0], parts[1])
		if _, err := os.Stat(direct); err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("skill not found: %s", arg)
			}
			return nil, err
		}
		return []LinkItem{skillItem(targetRoot, direct, parts[0])}, nil
	}

	return nil, fmt.Errorf("skill argument must be category or category/name: %s", arg)
}

func resolveSkillCategory(sourceRoot, targetRoot, group, dir string) ([]LinkItem, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []LinkItem
	for _, e := range entries {
		if shouldSkipName(e.Name()) {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if e.IsDir() || isFile(p) {
			out = append(out, skillItem(targetRoot, p, group))
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("skill category %s has no linkable children", group)
	}
	return out, nil
}

func skillItem(targetRoot, src, group string) LinkItem {
	name := filepath.Base(src)
	return LinkItem{Kind: "skills", Name: name, Group: group, Source: filepath.Clean(src), Target: filepath.Join(targetRoot, "skills", name)}
}

func findSkillByName(skillsRoot, name string) ([]string, error) {
	cats, err := immediateDirs(skillsRoot)
	if err != nil {
		return nil, err
	}
	var matches []string
	for _, cat := range cats {
		p := filepath.Join(cat, name)
		if _, err := os.Stat(p); err == nil {
			matches = append(matches, p)
		}
	}
	return matches, nil
}

func splitLinkArg(arg string) []string {
	arg = strings.ReplaceAll(arg, "\\", "/")
	arg = strings.Trim(arg, "/")
	if arg == "" {
		return nil
	}
	raw := strings.Split(arg, "/")
	parts := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func resolveGenericKind(sourceRoot, targetRoot, kind, arg string) ([]LinkItem, error) {
	arg = cleanArg(arg)
	base := filepath.Join(sourceRoot, kind)
	if isAllArg(arg) {
		return resolveAllUnderKind(base, targetRoot, kind)
	}

	rel := filepath.FromSlash(arg)
	direct := filepath.Join(base, rel)
	if _, err := os.Stat(direct); err == nil {
		return []LinkItem{simpleItem(targetRoot, kind, direct, rel)}, nil
	}

	matches, err := findDirectChild(base, arg)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("%s item not found: %s", kind, arg)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("multiple %s items match %q: %s", kind, arg, strings.Join(matches, ", "))
	}
	rel, err = filepath.Rel(base, matches[0])
	if err != nil {
		return nil, err
	}
	return []LinkItem{simpleItem(targetRoot, kind, matches[0], rel)}, nil
}

func resolveAllUnderKind(base, targetRoot, kind string) ([]LinkItem, error) {
	if _, err := os.Stat(base); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []LinkItem
	err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == base {
			return nil
		}
		if shouldSkipName(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			// For agents/commands, link top-level directories as one symlink and skip their children.
			rel, _ := filepath.Rel(base, p)
			if !strings.Contains(rel, string(os.PathSeparator)) {
				out = append(out, simpleItem(targetRoot, kind, p, rel))
				return filepath.SkipDir
			}
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(base, p)
			out = append(out, simpleItem(targetRoot, kind, p, rel))
		}
		return nil
	})
	return out, err
}

func simpleItem(targetRoot, kind, src, rel string) LinkItem {
	rel = filepath.Clean(rel)
	name := filepath.ToSlash(rel)
	return LinkItem{Kind: kind, Name: name, Source: filepath.Clean(src), Target: filepath.Join(targetRoot, kind, rel)}
}

func immediateDirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var dirs []string
	for _, e := range entries {
		if shouldSkipName(e.Name()) {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(dirs)
	return dirs, nil
}

func findDirectChild(base, arg string) ([]string, error) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	clean := filepath.Base(filepath.FromSlash(arg))
	var matches []string
	for _, e := range entries {
		n := e.Name()
		if n == clean || strings.TrimSuffix(n, filepath.Ext(n)) == clean {
			matches = append(matches, filepath.Join(base, n))
		}
	}
	return matches, nil
}

func cleanArg(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'`)
	return s
}

func isAllArg(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "*" || s == "all" || s == "."
}

func shouldSkipName(name string) bool {
	return name == "" || strings.HasPrefix(name, ".") || name == "node_modules" || name == "__pycache__"
}

func isFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func dedupeItems(items []LinkItem) ([]LinkItem, error) {
	byTarget := map[string]LinkItem{}
	for _, it := range items {
		key := filepath.Clean(it.Target)
		if old, ok := byTarget[key]; ok && filepath.Clean(old.Source) != filepath.Clean(it.Source) {
			return nil, fmt.Errorf("target conflict: %s from both %s and %s", key, old.Source, it.Source)
		}
		byTarget[key] = it
	}
	out := make([]LinkItem, 0, len(byTarget))
	for _, it := range byTarget {
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func linkItems(items []LinkItem, opts Options) (map[string]struct{}, error) {
	if len(items) == 0 {
		fmt.Println("nothing to link")
		return map[string]struct{}{}, nil
	}
	targetRoot := opts.TargetRoot
	if targetRoot == "" {
		targetRoot = targetRootFromItems(items)
	}
	lock, err := readLock(targetRoot)
	if err != nil {
		return nil, err
	}
	targetSet := map[string]struct{}{}
	now := time.Now().Format(time.RFC3339)

	for _, it := range items {
		targetSet[filepath.Clean(it.Target)] = struct{}{}
		linkType, err := ensureLink(it, opts, lock.entryForTarget(it.Target))
		if err != nil {
			return nil, err
		}
		lock.upsert(LockEntry{Kind: it.Kind, Name: it.Name, Group: it.Group, Source: filepath.Clean(it.Source), Target: filepath.Clean(it.Target), LinkType: linkType, CreatedAt: now})
	}
	if !opts.DryRun {
		if err := writeLock(targetRoot, lock); err != nil {
			return nil, err
		}
	}
	return targetSet, nil
}

func targetRootFromItems(items []LinkItem) string {
	if len(items) == 0 {
		return defaultTargetRoot
	}
	// target = targetRoot/kind/name; go two levels up from target parent.
	return filepath.Dir(filepath.Dir(items[0].Target))
}

func ensureLink(it LinkItem, opts Options, existing *LockEntry) (string, error) {
	if _, err := os.Stat(it.Source); err != nil {
		return "", fmt.Errorf("source missing: %s", it.Source)
	}
	action := fmt.Sprintf("LINK %s %-32s %s -> %s", it.Kind, it.Name, it.Target, it.Source)

	if st, err := os.Lstat(it.Target); err == nil {
		if st.Mode()&os.ModeSymlink != 0 {
			dst, err := os.Readlink(it.Target)
			if err != nil {
				return "", err
			}
			same := samePathOrSameEval(dst, it.Source, filepath.Dir(it.Target))
			if same {
				fmt.Println("SKIP", it.Target, "already linked")
				return "symlink", nil
			}
			if !opts.Force {
				return "", fmt.Errorf("target symlink exists but points elsewhere: %s -> %s; use --force to replace", it.Target, dst)
			}
			if opts.DryRun {
				fmt.Println("REPLACE", it.Target, "->", it.Source)
				return "symlink", nil
			}
			if err := os.Remove(it.Target); err != nil {
				return "", err
			}
		} else {
			if managedNonSymlinkLink(existing, it) {
				fmt.Println("SKIP", it.Target, "already linked")
				return existing.LinkType, nil
			}
			return "", fmt.Errorf("target exists and is not a symlink: %s", it.Target)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}

	if opts.DryRun {
		fmt.Println(action)
		return "symlink", nil
	}
	if err := os.MkdirAll(filepath.Dir(it.Target), 0755); err != nil {
		return "", err
	}
	linkType, err := createLink(it, opts)
	if err != nil {
		return "", err
	}
	fmt.Println(action)
	return linkType, nil
}

func managedNonSymlinkLink(existing *LockEntry, it LinkItem) bool {
	if existing == nil {
		return false
	}
	if existing.LinkType != "junction" && existing.LinkType != "hardlink" {
		return false
	}
	return sameCleanPath(existing.Target, it.Target) && sameCleanPath(existing.Source, it.Source)
}

func sameCleanPath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func samePathOrSameEval(linkDest, source, targetDir string) bool {
	if !filepath.IsAbs(linkDest) {
		linkDest = filepath.Join(targetDir, linkDest)
	}
	a := filepath.Clean(linkDest)
	b := filepath.Clean(source)
	if strings.EqualFold(a, b) {
		return true
	}
	ea, erra := filepath.EvalSymlinks(a)
	eb, errb := filepath.EvalSymlinks(b)
	return erra == nil && errb == nil && strings.EqualFold(filepath.Clean(ea), filepath.Clean(eb))
}

func unlinkItems(cfg Config, opts Options, kind string, names []string, all bool) error {
	lock, err := readLock(cfg.TargetRoot)
	if err != nil {
		return err
	}
	candidates := selectLockEntries(lock, kind, names, all)

	// If lock is empty or does not know this category, resolve from source as fallback.
	if len(candidates) == 0 && !all && kind != "all" {
		cfg.Links = map[string][]string{kind: names}
		items, rerr := resolveLinkSpec(cfg)
		if rerr == nil {
			for _, it := range items {
				candidates = append(candidates, LockEntry{Kind: it.Kind, Name: it.Name, Group: it.Group, Source: it.Source, Target: it.Target})
			}
		}
	}

	if len(candidates) == 0 {
		fmt.Println("nothing to unlink")
		return nil
	}

	removedTargets := map[string]struct{}{}
	for _, e := range candidates {
		if err := removeLink(e, opts.DryRun); err != nil {
			return err
		}
		removedTargets[filepath.Clean(e.Target)] = struct{}{}
	}
	if !opts.DryRun {
		lock.removeTargets(removedTargets)
		if err := writeLock(cfg.TargetRoot, lock); err != nil {
			return err
		}
	}
	return nil
}

func selectLockEntries(lock Lock, kind string, names []string, all bool) []LockEntry {
	nameSet := map[string]struct{}{}
	for _, n := range names {
		nameSet[cleanArg(n)] = struct{}{}
	}
	var out []LockEntry
	for _, e := range lock.Entries {
		if kind != "all" && e.Kind != kind {
			continue
		}
		if all {
			out = append(out, e)
			continue
		}
		if _, ok := nameSet[e.Name]; ok {
			out = append(out, e)
			continue
		}
		if e.Group != "" {
			if _, ok := nameSet[e.Group]; ok {
				out = append(out, e)
				continue
			}
		}
		// Allow matching basename for commands/agents.
		if _, ok := nameSet[filepath.Base(e.Name)]; ok {
			out = append(out, e)
			continue
		}
	}
	return out
}

func removeLink(entry LockEntry, dryRun bool) error {
	st, err := os.Lstat(entry.Target)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("MISSING", entry.Target)
			return nil
		}
		return err
	}
	if entry.LinkType == "" && st.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("refusing to remove non-symlink without linkType in lock: %s", entry.Target)
	}
	switch entry.LinkType {
	case "", "symlink", "junction", "hardlink":
	default:
		return fmt.Errorf("unknown linkType %q for %s", entry.LinkType, entry.Target)
	}
	if dryRun {
		fmt.Println("UNLINK", entry.Target)
		return nil
	}
	if err := os.Remove(entry.Target); err != nil {
		return err
	}
	fmt.Println("UNLINK", entry.Target)
	return nil
}

func pruneNotInSet(targetRoot string, keep map[string]struct{}, dryRun bool) error {
	lock, err := readLock(targetRoot)
	if err != nil {
		return err
	}
	var rm []LockEntry
	for _, e := range lock.Entries {
		if _, ok := keep[filepath.Clean(e.Target)]; !ok {
			rm = append(rm, e)
		}
	}
	if len(rm) == 0 {
		return nil
	}
	removed := map[string]struct{}{}
	for _, e := range rm {
		if err := removeLink(e, dryRun); err != nil {
			return err
		}
		removed[filepath.Clean(e.Target)] = struct{}{}
	}
	if !dryRun {
		lock.removeTargets(removed)
		return writeLock(targetRoot, lock)
	}
	return nil
}

func readLock(targetRoot string) (Lock, error) {
	p := lockPath(targetRoot)
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return Lock{Version: 1}, nil
		}
		return Lock{}, err
	}
	var lock Lock
	if err := json.Unmarshal(data, &lock); err != nil {
		return Lock{}, fmt.Errorf("invalid lock file %s: %w", p, err)
	}
	if lock.Version == 0 {
		lock.Version = 1
	}
	return lock, nil
}

func writeLock(targetRoot string, lock Lock) error {
	lock.Version = 1
	lock.UpdatedAt = time.Now().Format(time.RFC3339)
	sort.Slice(lock.Entries, func(i, j int) bool {
		if lock.Entries[i].Kind != lock.Entries[j].Kind {
			return lock.Entries[i].Kind < lock.Entries[j].Kind
		}
		return lock.Entries[i].Name < lock.Entries[j].Name
	})
	data, _ := json.MarshalIndent(lock, "", "  ")
	if err := os.MkdirAll(targetRoot, 0755); err != nil {
		return err
	}
	return os.WriteFile(lockPath(targetRoot), append(data, '\n'), 0644)
}

func lockPath(targetRoot string) string {
	return filepath.Join(targetRoot, lockFileName)
}

func (l *Lock) entryForTarget(target string) *LockEntry {
	target = filepath.Clean(target)
	for i := range l.Entries {
		if filepath.Clean(l.Entries[i].Target) == target {
			return &l.Entries[i]
		}
	}
	return nil
}

func (l *Lock) upsert(e LockEntry) {
	for i := range l.Entries {
		if filepath.Clean(l.Entries[i].Target) == filepath.Clean(e.Target) {
			if l.Entries[i].CreatedAt != "" {
				e.CreatedAt = l.Entries[i].CreatedAt
			}
			l.Entries[i] = e
			return
		}
	}
	l.Entries = append(l.Entries, e)
}

func (l *Lock) removeTargets(targets map[string]struct{}) {
	kept := l.Entries[:0]
	for _, e := range l.Entries {
		if _, ok := targets[filepath.Clean(e.Target)]; !ok {
			kept = append(kept, e)
		}
	}
	l.Entries = kept
}

func normalizeKind(s string) string {
	raw := strings.TrimSpace(s)
	lower := strings.ToLower(raw)
	switch lower {
	case "skill", "skills":
		return "skills"
	case "agent", "agents":
		return "agents"
	case "command", "commands", "cmd", "cmds":
		return "commands"
	case "all", "*":
		return "all"
	default:
		return raw
	}
}

var percentVar = regexp.MustCompile(`%([A-Za-z_][A-Za-z0-9_]*)%`)

func expandVars(s string) string {
	s = os.ExpandEnv(s)
	return percentVar.ReplaceAllStringFunc(s, func(m string) string {
		name := strings.Trim(m, "%")
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		return m
	})
}
