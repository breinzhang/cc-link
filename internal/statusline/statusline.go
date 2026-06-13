package statusline

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	ansiGreen       = "\033[32m"
	ansiYellow      = "\033[33m"
	ansiRed         = "\033[31m"
	ansiReset       = "\033[0m"
	contextBarWidth = 20
)

type statuslinePayload struct {
	SessionID      string              `json:"session_id"`
	TranscriptPath string              `json:"transcript_path"`
	Model          statuslineModel     `json:"model"`
	Workspace      statuslineWorkspace `json:"workspace"`
	Cost           statuslineCost      `json:"cost"`
	ContextWindow  *statuslineContext  `json:"context_window"`
}

type statuslineModel struct {
	DisplayName string `json:"display_name"`
	ID          string `json:"id"`
}

type statuslineWorkspace struct {
	CurrentDir string `json:"current_dir"`
	ProjectDir string `json:"project_dir"`
}

type statuslineCost struct {
	TotalCostUSD float64 `json:"total_cost_usd"`
}

type statuslineContext struct {
	TotalInputTokens    int64            `json:"total_input_tokens"`
	ContextWindowSize   int64            `json:"context_window_size"`
	UsedPercentage      float64          `json:"used_percentage"`
	RemainingPercentage float64          `json:"remaining_percentage"`
	CurrentUsage        *statuslineUsage `json:"current_usage"`
}

type statuslineUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

type statuslineRenderOptions struct {
	Branch         string
	CacheRemaining time.Duration
	CacheVisible   bool
	CacheTTL       time.Duration
	Columns        int
	Lines          int
	Now            time.Time
	CostSummary    statuslineCostSummary
}

type statuslineCostSummary struct {
	Session float64
	Today   float64
	Week    float64
	Month   float64
}

type statuslineCacheState struct {
	TranscriptMarker int64 `json:"transcriptMarker"`
	LastReplyUnix    int64 `json:"lastReplyUnix"`
	ParentPID        int   `json:"parentPid"`
}

func Command(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "enable":
			return Enable()
		case "disable":
			return Disable()
		default:
			return fmt.Errorf("unknown statusline command %q", args[0])
		}
	}

	var payload statuslinePayload
	if err := json.NewDecoder(os.Stdin).Decode(&payload); err != nil {
		return fmt.Errorf("read statusline input: %w", err)
	}
	if shouldHideStatusline(payload) {
		return nil
	}
	cfg, err := loadStatuslineConfig(payload.projectRoot())
	if err != nil {
		return err
	}
	summary := statuslineCostSummary{Session: payload.Cost.TotalCostUSD}
	if cfg.Cost.isEnabled() {
		if cacheDir, err := os.UserCacheDir(); err == nil {
			if s, err := updateCostSummary(payload, cfg, os.TempDir(), filepath.Join(cacheDir, "cc-link", "statusline-costs.jsonl"), time.Now()); err == nil {
				summary = s
			}
		}
	}
	parentPID := os.Getppid()
	renderPayload := payload
	if ctx, err := stableContextWindow(payload, os.TempDir(), parentPID); err == nil {
		renderPayload.ContextWindow = ctx
	}
	ttl := statuslineTTL()
	remaining := -time.Second
	cacheVisible := false
	if payload.SessionID != "" && payload.TranscriptPath != "" && hasCurrentUsage(renderPayload.ContextWindow) && contextBelongsToCurrentParent(payload, os.TempDir(), parentPID) {
		if r, visible, err := updateCacheCountdown(payload, os.TempDir(), ttl, time.Now(), parentPID); err == nil {
			remaining = r
			cacheVisible = visible
		}
	}
	fmt.Println(renderStatusline(renderPayload, statuslineRenderOptions{
		Branch:         currentGitBranch(payload.Workspace.CurrentDir),
		CacheRemaining: remaining,
		CacheVisible:   cacheVisible,
		CacheTTL:       ttl,
		Columns:        envInt("COLUMNS"),
		Lines:          cfg.Lines,
		Now:            time.Now(),
		CostSummary:    summary,
	}))
	return nil
}

func contextBelongsToCurrentParent(payload statuslinePayload, stateDir string, parentPID int) bool {
	if hasCurrentUsage(payload.ContextWindow) || payload.SessionID == "" {
		return true
	}
	state, ok, err := readStatuslineContext(statuslineContextPath(stateDir, payload.SessionID))
	if err != nil || !ok {
		return false
	}
	return state.ParentPID == parentPID
}

func (payload statuslinePayload) projectRoot() string {
	if payload.Workspace.ProjectDir != "" {
		return payload.Workspace.ProjectDir
	}
	return payload.Workspace.CurrentDir
}

func renderStatusline(payload statuslinePayload, opts statuslineRenderOptions) string {
	model := payload.Model.DisplayName
	if model == "" {
		model = payload.Model.ID
	}
	dir := filepath.Base(filepath.Clean(payload.Workspace.CurrentDir))
	if dir == "." || dir == string(filepath.Separator) {
		dir = payload.Workspace.CurrentDir
	}
	lines := opts.Lines
	if lines <= 0 {
		lines = 2
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	first := fitFirstLine(model, dir, opts.Branch, now.Format("15:04:05"), opts.Columns)
	context := renderContext(payload.ContextWindow)
	cache := renderCache(payload.ContextWindow, opts.CacheRemaining, opts.CacheTTL, opts.CacheVisible)
	cost := renderCostSummary(opts.CostSummary)
	second := joinNonEmpty("  ", context, cache)
	if lines == 1 {
		return joinNonEmpty("  ", first, second, cost)
	}
	if lines >= 3 && cost != "" {
		return first + "\n" + second + "\n" + cost
	}
	return first + "\n" + joinNonEmpty("  ", second, cost)
}

func fitFirstLine(model, dir, branch, clock string, columns int) string {
	line := buildFirstLine(model, dir, branch, clock)
	if columns <= 0 || visibleLen(line) <= columns {
		return line
	}
	overhead := visibleLen(line) - utf8.RuneCountInString(dir)
	maxDir := columns - overhead
	if maxDir < 4 {
		maxDir = 4
	}
	line = buildFirstLine(model, shortenRunes(dir, maxDir), branch, clock)
	if visibleLen(line) <= columns {
		return line
	}
	return buildFirstLine("", shortenRunes(dir, maxDir), branch, clock)
}

func buildFirstLine(model, dir, branch, clock string) string {
	var parts []string
	if model != "" {
		parts = append(parts, "❬"+model+"❭")
	}
	if dir != "" {
		parts = append(parts, "📁 "+dir)
	}
	if branch != "" {
		parts = append(parts, "🌿 "+branch)
	}
	if clock != "" {
		parts = append(parts, "🕒 "+clock)
	}
	return strings.Join(parts, " ")
}

func renderContext(ctx *statuslineContext) string {
	if ctx == nil || ctx.ContextWindowSize <= 0 {
		return ansiGreen + dottedProgressBackground(contextBarWidth) + ansiReset + " --/-- " + ansiGreen + "--% left" + ansiReset
	}
	if ctx.CurrentUsage == nil {
		return fmt.Sprintf("%s%s%s --/%s %s100%% left%s",
			ansiGreen,
			dottedProgressBackground(contextBarWidth),
			ansiReset,
			formatTokens(ctx.ContextWindowSize),
			ansiGreen,
			ansiReset,
		)
	}
	remaining := int(math.Round(ctx.RemainingPercentage))
	usedPct := ctx.UsedPercentage
	if usedPct == 0 {
		usedPct = 100 - ctx.RemainingPercentage
	}
	filled := int(math.Round(usedPct * contextBarWidth / 100))
	if usedPct > 0 && filled == 0 {
		filled = 1
	}
	if filled < 0 {
		filled = 0
	}
	if filled > contextBarWidth {
		filled = contextBarWidth
	}
	bar := strings.Repeat("█", filled) + dottedProgressBackground(contextBarWidth-filled)
	color := contextColor(ctx.RemainingPercentage)
	return fmt.Sprintf("%s%s%s %s/%s %s%d%% left%s",
		color, bar, ansiReset,
		formatTokens(ctx.TotalInputTokens),
		formatTokens(ctx.ContextWindowSize),
		color, remaining, ansiReset,
	)
}

func renderCache(ctx *statuslineContext, remaining, ttl time.Duration, visible bool) string {
	if ctx == nil || ctx.CurrentUsage == nil {
		return ""
	}
	if !visible {
		return ""
	}
	if remaining <= 0 {
		return ansiYellow + "⚠️" + ansiRed + "缓存已过期" + ansiReset
	}
	color := cacheColor(remaining, ttl)
	return color + "⏳ " + formatClock(remaining) + ansiReset
}

func renderCostSummary(cost statuslineCostSummary) string {
	if cost.Session == 0 && cost.Today == 0 && cost.Week == 0 && cost.Month == 0 {
		return ""
	}
	return fmt.Sprintf("Session %s Today %s Week %s Month %s",
		formatUSD(cost.Session),
		formatUSD(cost.Today),
		formatUSD(cost.Week),
		formatUSD(cost.Month),
	)
}

func formatUSD(v float64) string {
	if v < 1 {
		return fmt.Sprintf("$%.3f", v)
	}
	return fmt.Sprintf("$%.2f", v)
}

func joinNonEmpty(sep string, values ...string) string {
	var out []string
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return strings.Join(out, sep)
}

func dottedProgressBackground(width int) string {
	if width <= 0 {
		return ""
	}
	return strings.Repeat("⣿", width)
}

func formatTokens(n int64) string {
	switch {
	case n <= 0:
		return "--"
	case n >= 1000000 && n%1000000 == 0:
		return fmt.Sprintf("%dm", n/1000000)
	case n >= 1000000:
		return fmt.Sprintf("%.1fm", float64(n)/1000000)
	case n >= 1000:
		return fmt.Sprintf("%dk", n/1000)
	default:
		return strconv.FormatInt(n, 10)
	}
}

func formatClock(d time.Duration) string {
	seconds := int(d.Round(time.Second).Seconds())
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

func contextColor(remaining float64) string {
	if remaining >= 70 {
		return ansiGreen
	}
	if remaining >= 30 {
		return ansiYellow
	}
	return ansiRed
}

func cacheColor(remaining, ttl time.Duration) string {
	if ttl <= 0 {
		ttl = 300 * time.Second
	}
	ratio := float64(remaining) / float64(ttl)
	if ratio > 0.5 {
		return ansiGreen
	}
	if ratio >= 0.2 {
		return ansiYellow
	}
	return ansiRed
}

func statuslineTTL() time.Duration {
	if v := envInt("CC_LINK_CACHE_TTL"); v > 0 {
		return time.Duration(v) * time.Second
	}
	return 300 * time.Second
}

func updateCacheCountdown(payload statuslinePayload, stateDir string, ttl time.Duration, now time.Time, parentPID int) (time.Duration, bool, error) {
	if ttl <= 0 {
		ttl = 300 * time.Second
	}
	st, err := os.Stat(payload.TranscriptPath)
	if err != nil {
		return 0, false, err
	}
	marker := st.ModTime().UnixNano()
	statePath := statuslineCachePath(stateDir, payload.SessionID)
	state := statuslineCacheState{}
	if data, err := os.ReadFile(statePath); err == nil {
		if err := json.Unmarshal(data, &state); err != nil {
			return 0, false, err
		}
	} else if !os.IsNotExist(err) {
		return 0, false, err
	}
	if state.ParentPID != 0 && state.ParentPID != parentPID {
		state.TranscriptMarker = marker
		state.LastReplyUnix = 0
		state.ParentPID = parentPID
		if err := writeStatuslineCache(statePath, state); err != nil {
			return 0, false, err
		}
		return 0, false, nil
	}
	if state.TranscriptMarker != marker || state.LastReplyUnix == 0 {
		state.TranscriptMarker = marker
		state.ParentPID = parentPID
		if transcriptLatestLineIsCompletedAssistant(payload.TranscriptPath) {
			state.LastReplyUnix = now.Unix()
			if err := writeStatuslineCache(statePath, state); err != nil {
				return 0, false, err
			}
			return ttl, true, nil
		}
		state.LastReplyUnix = 0
		if err := writeStatuslineCache(statePath, state); err != nil {
			return 0, false, err
		}
		return 0, false, nil
	}
	return ttl - now.Sub(time.Unix(state.LastReplyUnix, 0)), true, nil
}

func writeStatuslineCache(path string, state statuslineCacheState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, _ := json.Marshal(state)
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func shouldHideStatusline(payload statuslinePayload) bool {
	return payload.TranscriptPath != "" && transcriptLatestSemanticEventIsExit(payload.TranscriptPath)
}

func transcriptLatestSemanticEventIsExit(path string) bool {
	lines, ok := readTranscriptLines(path)
	if !ok {
		return false
	}
	for i := len(lines) - 1; i >= 0; i-- {
		if content, ok := transcriptLineUserContent(lines[i]); ok {
			return isExitCommandContent(content)
		}
		switch transcriptLineRole(lines[i]) {
		case "assistant_complete", "assistant_incomplete":
			return false
		}
	}
	return false
}

func isExitCommandContent(content string) bool {
	content = strings.TrimSpace(content)
	return content == "/exit" || strings.Contains(content, "<command-name>/exit</command-name>")
}

func transcriptLatestLineIsCompletedAssistant(path string) bool {
	lines, ok := readTranscriptLines(path)
	if !ok {
		return false
	}
	for i := len(lines) - 1; i >= 0; i-- {
		switch transcriptLineRole(lines[i]) {
		case "assistant_complete":
			return true
		case "assistant_incomplete", "user":
			return false
		}
	}
	return false
}

func readTranscriptLines(path string) ([]string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	var lines []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if scanner.Err() != nil {
		return nil, false
	}
	return lines, true
}

func transcriptLineRole(line string) string {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &envelope); err != nil {
		return ""
	}
	if raw, ok := envelope["message"]; ok {
		if role := assistantMessageRole(raw); role != "" {
			return role
		}
	}
	return assistantMessageRole([]byte(line))
}

func transcriptLineUserContent(line string) (string, bool) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &envelope); err != nil {
		return "", false
	}
	if raw, ok := envelope["message"]; ok {
		if content, ok := userMessageContent(raw); ok {
			return content, true
		}
	}
	return userMessageContent([]byte(line))
}

func userMessageContent(raw json.RawMessage) (string, bool) {
	var msg map[string]json.RawMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return "", false
	}
	if !jsonStringEquals(msg["role"], "user") && !jsonStringEquals(msg["type"], "user") {
		return "", false
	}
	var content string
	if err := json.Unmarshal(msg["content"], &content); err != nil {
		return "", false
	}
	return content, true
}

func assistantMessageRole(raw json.RawMessage) string {
	var msg map[string]json.RawMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return ""
	}
	if jsonStringEquals(msg["role"], "user") || jsonStringEquals(msg["type"], "user") {
		return "user"
	}
	if !jsonStringEquals(msg["role"], "assistant") && !jsonStringEquals(msg["type"], "assistant") {
		return ""
	}
	if rawStop, ok := msg["stop_reason"]; ok {
		if string(rawStop) == "null" {
			return "assistant_incomplete"
		}
		var stop string
		if err := json.Unmarshal(rawStop, &stop); err == nil {
			if strings.TrimSpace(stop) == "end_turn" {
				return "assistant_complete"
			}
			return "assistant_incomplete"
		}
	}
	return "assistant_complete"
}

func jsonStringEquals(raw json.RawMessage, want string) bool {
	var got string
	return len(raw) > 0 && json.Unmarshal(raw, &got) == nil && got == want
}

func statuslineCachePath(stateDir, sessionID string) string {
	if sessionID == "" {
		sessionID = "unknown"
	}
	safe := regexp.MustCompile(`[^A-Za-z0-9._-]+`).ReplaceAllString(sessionID, "_")
	return filepath.Join(stateDir, "cc-link-statusline-"+safe+".json")
}

func currentGitBranch(dir string) string {
	if dir == "" {
		return ""
	}
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func visibleLen(s string) int {
	return utf8.RuneCountInString(ansiPattern.ReplaceAllString(s, ""))
}

func shortenRunes(s string, max int) string {
	r := []rune(s)
	if max <= 0 || len(r) <= max {
		return s
	}
	if max <= 3 {
		return string(r[:max])
	}
	return string(r[:max-3]) + "..."
}

func envInt(name string) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}
