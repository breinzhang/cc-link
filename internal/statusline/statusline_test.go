package statusline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRenderStatuslineShowsContextBranchAndCountdown(t *testing.T) {
	out := renderStatusline(statuslinePayload{
		Model: statuslineModel{DisplayName: "Opus"},
		Workspace: statuslineWorkspace{
			CurrentDir: filepath.Join("tmp", "cc-link-tool"),
		},
		ContextWindow: &statuslineContext{
			TotalInputTokens:    56000,
			ContextWindowSize:   200000,
			RemainingPercentage: 72,
			CurrentUsage: &statuslineUsage{
				InputTokens:          49000,
				CacheReadInputTokens: 7000,
			},
		},
	}, statuslineRenderOptions{
		Branch:         "main",
		CacheRemaining: 278 * time.Second,
		CacheVisible:   true,
		CacheTTL:       300 * time.Second,
		Now:            time.Date(2026, 6, 13, 15, 4, 5, 0, time.Local),
	})

	for _, want := range []string{"❬Opus❭", "📁 cc-link-tool", "🌿 main", "15:04:05", "ctx ", "72% free", "56k/200k", "⏳ 4:38"} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered statusline missing %q:\n%s", want, out)
		}
	}
	if got := strings.Count(out, "\n"); got != 1 {
		t.Fatalf("statusline newline count = %d, want 1: %q", got, out)
	}
}

func TestRenderStatuslineHandlesNullContextAndExpiredCache(t *testing.T) {
	out := renderStatusline(statuslinePayload{
		Model: statuslineModel{ID: "claude-sonnet-4"},
		Workspace: statuslineWorkspace{
			CurrentDir: filepath.Join("tmp", "project"),
		},
	}, statuslineRenderOptions{
		CacheRemaining: -time.Second,
		CacheTTL:       300 * time.Second,
	})

	for _, want := range []string{"❬claude-sonnet-4❭", "📁 project"} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered statusline missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "缓存") || strings.Contains(out, "⏳") {
		t.Fatalf("cache should be hidden before current usage exists:\n%s", out)
	}
	if !strings.Contains(out, "ctx ") || !strings.Contains(out, "--% free") {
		t.Fatalf("initial statusline should render an unknown free context:\n%q", out)
	}
}

func TestRenderCachePrefixesExpiredCacheWithWarning(t *testing.T) {
	out := renderCache(&statuslineContext{CurrentUsage: &statuslineUsage{InputTokens: 1}}, -time.Second, 300*time.Second, true)
	if !strings.Contains(out, ansiYellow+"⚠️"+ansiRed+"缓存已过期") {
		t.Fatalf("expired cache = %q, want yellow warning immediately before red text", out)
	}
}

func TestRenderContextUsesBatteryFreeBar(t *testing.T) {
	tests := []struct {
		name      string
		total     int64
		remaining float64
		color     string
		bar       string
		want      string
	}{
		{name: "mostly free", total: 32000, remaining: 97, color: "\033[38;5;114m", bar: "▰▰▰▰▰▰▰▰▰▰▰▰", want: "ctx 97% free"},
		{name: "comfortable", total: 280000, remaining: 72, color: "\033[38;5;114m", bar: "▰▰▰▰▰▰▰▰▰▱▱▱", want: "ctx 72% free"},
		{name: "low", total: 720000, remaining: 28, color: "\033[38;5;208m", bar: "▰▰▰▱▱▱▱▱▱▱▱▱", want: "ctx 28% free"},
		{name: "critical", total: 910000, remaining: 9, color: "\033[38;5;203m", bar: "▰▱▱▱▱▱▱▱▱▱▱▱", want: "ctx 09% free"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := renderContext(&statuslineContext{
				TotalInputTokens:    tt.total,
				ContextWindowSize:   1000000,
				RemainingPercentage: tt.remaining,
				CurrentUsage:        &statuslineUsage{InputTokens: tt.total},
			})
			want := "ctx " + tt.color + tt.want[len("ctx "):] + " " + tt.bar + ansiReset + " " + formatTokens(tt.total) + "/1m"
			if out != want {
				t.Fatalf("context = %q, want %q", out, want)
			}
		})
	}
}

func TestStableContextWindowReusesLastUsageWhenCurrentUsageDisappearsInSameParentProcess(t *testing.T) {
	tmp := t.TempDir()
	payload := statuslinePayload{
		SessionID: "session-1",
		ContextWindow: &statuslineContext{
			TotalInputTokens:    56000,
			ContextWindowSize:   200000,
			UsedPercentage:      28,
			RemainingPercentage: 72,
			CurrentUsage:        &statuslineUsage{InputTokens: 56000},
		},
	}
	got, err := stableContextWindow(payload, tmp, 111)
	if err != nil {
		t.Fatal(err)
	}
	if got.RemainingPercentage != 72 {
		t.Fatalf("initial remaining = %.1f, want 72", got.RemainingPercentage)
	}

	payload.ContextWindow = &statuslineContext{
		ContextWindowSize: 200000,
		CurrentUsage:      nil,
	}
	got, err = stableContextWindow(payload, tmp, 111)
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalInputTokens != 56000 || got.RemainingPercentage != 72 || got.CurrentUsage == nil {
		t.Fatalf("stable context = %#v, want previous non-empty usage", got)
	}
	out := renderStatusline(statuslinePayload{ContextWindow: got}, statuslineRenderOptions{})
	if strings.Contains(out, "100% free") || !strings.Contains(out, "72% free") {
		t.Fatalf("rendered stable context should keep previous percentage:\n%s", out)
	}
}

func TestStableContextWindowReusesLastUsageAcrossParentProcessForResume(t *testing.T) {
	tmp := t.TempDir()
	payload := statuslinePayload{
		SessionID: "session-1",
		ContextWindow: &statuslineContext{
			TotalInputTokens:    56000,
			ContextWindowSize:   200000,
			RemainingPercentage: 72,
			CurrentUsage:        &statuslineUsage{InputTokens: 56000},
		},
	}
	if _, err := stableContextWindow(payload, tmp, 111); err != nil {
		t.Fatal(err)
	}

	payload.ContextWindow = &statuslineContext{
		ContextWindowSize: 200000,
		CurrentUsage:      nil,
	}
	got, err := stableContextWindow(payload, tmp, 222)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.CurrentUsage == nil || got.RemainingPercentage != 72 {
		t.Fatalf("resume in a new parent process should reuse old context: %#v", got)
	}
	out := renderStatusline(statuslinePayload{ContextWindow: got}, statuslineRenderOptions{CacheRemaining: 5 * time.Minute})
	if !strings.Contains(out, "72% free") || strings.Contains(out, "⏳") {
		t.Fatalf("resumed startup should keep context but hide cache countdown:\n%s", out)
	}
	if contextBelongsToCurrentParent(payload, tmp, 222) {
		t.Fatal("resumed context from a previous parent should not be allowed to start cache countdown")
	}
}

func TestRenderStatuslineLinesOption(t *testing.T) {
	payload := statuslinePayload{
		Model: statuslineModel{DisplayName: "Sonnet"},
		Workspace: statuslineWorkspace{
			CurrentDir: filepath.Join("tmp", "project"),
		},
		ContextWindow: &statuslineContext{
			TotalInputTokens:    100000,
			ContextWindowSize:   200000,
			RemainingPercentage: 50,
			CurrentUsage:        &statuslineUsage{InputTokens: 100000},
		},
		Cost: statuslineCost{TotalCostUSD: 0.01234},
	}

	oneLine := renderStatusline(payload, statuslineRenderOptions{Lines: 1})
	if strings.Contains(oneLine, "\n") {
		t.Fatalf("one-line statusline should not contain newline: %q", oneLine)
	}

	threeLines := renderStatusline(payload, statuslineRenderOptions{Lines: 3, CostSummary: statuslineCostSummary{
		Session: 0.01234,
		Today:   0.31,
		Week:    1.24,
		Month:   5.8,
	}})
	if got := strings.Count(threeLines, "\n"); got != 2 {
		t.Fatalf("three-line statusline newline count = %d, want 2:\n%s", got, threeLines)
	}
	if !strings.Contains(threeLines, "Session $0.012") || !strings.Contains(threeLines, "Today $0.31") || !strings.Contains(threeLines, "Week $1.24") || !strings.Contains(threeLines, "Month $5.80") {
		t.Fatalf("three-line statusline missing cost summary:\n%s", threeLines)
	}
}

func TestContextColorThresholdsUseRemainingPercentage(t *testing.T) {
	if contextColor(70) != "\033[38;5;114m" {
		t.Fatal("70% free should use muted cyan-green")
	}
	if contextColor(69.9) != "\033[38;5;215m" {
		t.Fatal("below 70% free should use amber")
	}
	if contextColor(40) != "\033[38;5;215m" {
		t.Fatal("40% free should use amber")
	}
	if contextColor(39.9) != "\033[38;5;208m" {
		t.Fatal("below 40% free should use orange")
	}
	if contextColor(15) != "\033[38;5;208m" {
		t.Fatal("15% free should use orange")
	}
	if contextColor(14.9) != "\033[38;5;203m" {
		t.Fatal("below 15% free should use soft red")
	}
}

func TestLoadStatuslineConfigMergesGlobalThenProject(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	projectRoot := filepath.Join(tmp, "project")
	t.Setenv("HOME", home)
	writeStatuslineConfig(t, filepath.Join(home, ".cc-link", "cc-link.json"), `{"statusline":{"lines":1,"cost":{"prices":[{"provider":"Anthropic","models":[{"match":"claude-*","input":3,"output":15}]}]}}}`)
	writeStatuslineConfig(t, filepath.Join(projectRoot, ".cc-link", "cc-link.json"), `{"statusline":{"lines":3,"cost":{"prices":[{"provider":"GLM/Z.AI","models":[{"match":"glm-5.1*","input":1,"output":2}]}]}}}`)

	cfg, err := loadStatuslineConfig(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Lines != 3 {
		t.Fatalf("lines = %d, want project override 3", cfg.Lines)
	}
	if len(cfg.Cost.Prices) != 2 {
		t.Fatalf("provider price count = %d, want project plus global price groups", len(cfg.Cost.Prices))
	}
	prices := cfg.Cost.modelPrices()
	if len(prices) != 2 {
		t.Fatalf("flattened price count = %d, want 2", len(prices))
	}
	if prices[0].Provider != "GLM/Z.AI" || prices[0].Match != "glm-5.1*" || prices[1].Provider != "Anthropic" || prices[1].Match != "claude-*" {
		t.Fatalf("price order = %#v, want project price before global fallback", prices)
	}
}

func TestCalculateUsageCostUsesConfiguredTokenPrices(t *testing.T) {
	got := calculateUsageCost(statuslineUsage{
		InputTokens:              1000,
		OutputTokens:             2000,
		CacheCreationInputTokens: 3000,
		CacheReadInputTokens:     4000,
	}, statuslineModelPrice{
		Input:      3,
		Output:     15,
		CacheWrite: 3.75,
		CacheRead:  0.30,
	})
	want := 0.003 + 0.030 + 0.01125 + 0.0012
	if got != want {
		t.Fatalf("cost = %.10f, want %.10f", got, want)
	}
}

func TestMatchModelPriceWildcardIsCaseInsensitive(t *testing.T) {
	price, ok := matchModelPrice("GLM-5.1", "", []statuslineModelPrice{
		{Match: "glm-5.1*", Input: 1, Output: 2},
	})
	if !ok {
		t.Fatal("expected lowercase wildcard to match uppercase model id")
	}
	if price.Input != 1 || price.Output != 2 {
		t.Fatalf("matched price = %#v, want configured price", price)
	}
}

func TestUpdateCostSummaryRecordsConfiguredModelCostOncePerTranscriptMarker(t *testing.T) {
	tmp := t.TempDir()
	transcript := filepath.Join(tmp, "transcript.jsonl")
	writeTestFile(t, transcript)
	payload := statuslinePayload{
		SessionID:      "session-1",
		TranscriptPath: transcript,
		Model:          statuslineModel{ID: "glm-5.1-air"},
		ContextWindow: &statuslineContext{
			CurrentUsage: &statuslineUsage{InputTokens: 1000, OutputTokens: 1000},
		},
	}
	cfg := statuslineConfig{Cost: statuslineCostConfig{
		Prices: []statuslineProviderPrice{{Provider: "GLM/Z.AI", Models: []statuslineModelPrice{{Match: "glm-5.1*", Input: 1, Output: 2}}}},
	}}
	stateDir := filepath.Join(tmp, "state")
	ledgerPath := filepath.Join(tmp, "ledger.jsonl")
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.Local)

	first, err := updateCostSummary(payload, cfg, stateDir, ledgerPath, now)
	if err != nil {
		t.Fatal(err)
	}
	if first.Session != 0.003 || first.Today != 0.003 || first.Week != 0.003 || first.Month != 0.003 {
		t.Fatalf("first summary = %#v, want all 0.003", first)
	}

	second, err := updateCostSummary(payload, cfg, stateDir, ledgerPath, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if second.Today != first.Today {
		t.Fatalf("same transcript marker should not be double counted: first=%#v second=%#v", first, second)
	}

	changedAt := now.Add(2 * time.Second)
	if err := os.Chtimes(transcript, changedAt, changedAt); err != nil {
		t.Fatal(err)
	}
	third, err := updateCostSummary(payload, cfg, stateDir, ledgerPath, changedAt)
	if err != nil {
		t.Fatal(err)
	}
	if third.Session != 0.006 || third.Today != 0.006 {
		t.Fatalf("third summary = %#v, want session/today 0.006", third)
	}
}

func TestUpdateCostSummarySumsDifferentConfiguredModelPrices(t *testing.T) {
	tmp := t.TempDir()
	cfg := statuslineConfig{Cost: statuslineCostConfig{
		Prices: []statuslineProviderPrice{
			{Provider: "GLM/Z.AI", Models: []statuslineModelPrice{{Match: "glm-5.1*", Input: 1, Output: 2}}},
			{Provider: "Kimi/Moonshot", Models: []statuslineModelPrice{{Match: "kimi-k2.6*", Input: 10, Output: 20}}},
		},
	}}
	stateDir := filepath.Join(tmp, "state")
	ledgerPath := filepath.Join(tmp, "ledger.jsonl")
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.Local)

	glmTranscript := filepath.Join(tmp, "glm.jsonl")
	kimiTranscript := filepath.Join(tmp, "kimi.jsonl")
	writeTestFile(t, glmTranscript)
	writeTestFile(t, kimiTranscript)

	glmPayload := statuslinePayload{
		SessionID:      "glm-session",
		TranscriptPath: glmTranscript,
		Model:          statuslineModel{ID: "glm-5.1"},
		ContextWindow: &statuslineContext{
			CurrentUsage: &statuslineUsage{InputTokens: 1000, OutputTokens: 1000},
		},
	}
	kimiPayload := statuslinePayload{
		SessionID:      "kimi-session",
		TranscriptPath: kimiTranscript,
		Model:          statuslineModel{ID: "kimi-k2.6"},
		ContextWindow: &statuslineContext{
			CurrentUsage: &statuslineUsage{InputTokens: 1000, OutputTokens: 1000},
		},
	}

	first, err := updateCostSummary(glmPayload, cfg, stateDir, ledgerPath, now)
	if err != nil {
		t.Fatal(err)
	}
	if first.Today != 0.003 {
		t.Fatalf("glm today cost = %.6f, want 0.003000", first.Today)
	}
	second, err := updateCostSummary(kimiPayload, cfg, stateDir, ledgerPath, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	want := 0.003 + 0.030
	if second.Today != want || second.Week != want || second.Month != want {
		t.Fatalf("summary = %#v, want day/week/month %.6f from GLM plus Kimi prices", second, want)
	}
}

func TestCostDeltaPrefersConfiguredModelPriceOverOfficialTotal(t *testing.T) {
	state := statuslineCostState{LastOfficialTotal: 99}
	payload := statuslinePayload{
		Cost:           statuslineCost{TotalCostUSD: 100},
		Model:          statuslineModel{ID: "kimi-k2.6"},
		TranscriptPath: "transcript.jsonl",
		ContextWindow: &statuslineContext{
			CurrentUsage: &statuslineUsage{InputTokens: 1000, OutputTokens: 1000},
		},
	}
	cfg := statuslineConfig{Cost: statuslineCostConfig{
		Prices: []statuslineProviderPrice{{Provider: "Kimi/Moonshot", Models: []statuslineModelPrice{{Match: "kimi-k2.6*", Input: 10, Output: 20}}}},
	}}

	sessionTotal, delta := costDelta(payload, cfg, 123, &state)
	if sessionTotal != 0.030 || delta != 0.030 {
		t.Fatalf("costDelta = session %.6f delta %.6f, want configured model cost 0.030", sessionTotal, delta)
	}
	if state.LastOfficialTotal != 99 {
		t.Fatalf("LastOfficialTotal = %.2f, want unchanged when configured price matches", state.LastOfficialTotal)
	}
}

func TestUpdateCostSummaryHidesConfiguredCostBeforeCurrentUsage(t *testing.T) {
	tmp := t.TempDir()
	transcript := filepath.Join(tmp, "transcript.jsonl")
	writeTestFile(t, transcript)
	cfg := statuslineConfig{Cost: statuslineCostConfig{
		Prices: []statuslineProviderPrice{{Provider: "GLM/Z.AI", Models: []statuslineModelPrice{{Match: "glm-5.1*", Input: 1, Output: 2}}}},
	}}
	payload := statuslinePayload{
		SessionID:      "session-1",
		TranscriptPath: transcript,
		Model:          statuslineModel{ID: "glm-5.1-air"},
		ContextWindow: &statuslineContext{
			CurrentUsage: &statuslineUsage{InputTokens: 1000, OutputTokens: 1000},
		},
	}
	stateDir := filepath.Join(tmp, "state")
	ledgerPath := filepath.Join(tmp, "ledger.jsonl")
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.Local)
	if _, err := updateCostSummary(payload, cfg, stateDir, ledgerPath, now); err != nil {
		t.Fatal(err)
	}

	payload.ContextWindow.CurrentUsage = nil
	got, err := updateCostSummary(payload, cfg, stateDir, ledgerPath, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if got.Session != 0 {
		t.Fatalf("session cost without current usage = %.3f, want 0", got.Session)
	}
}

func TestUpdateCacheCountdownStartsAfterCompletedAssistantReply(t *testing.T) {
	tmp := t.TempDir()
	transcript := filepath.Join(tmp, "transcript.jsonl")
	writeTranscript(t, transcript, `{"type":"user","message":{"role":"user","content":"hi"}}`)
	payload := statuslinePayload{SessionID: "session-1", TranscriptPath: transcript}
	ttl := 5 * time.Minute
	now := time.Unix(1000, 0)

	_, visible, err := updateCacheCountdown(payload, tmp, ttl, now, 111)
	if err != nil {
		t.Fatal(err)
	}
	if visible {
		t.Fatal("cache countdown should be hidden before an assistant reply completes")
	}

	writeTranscript(t, transcript, `{"type":"assistant","message":{"role":"assistant","content":"done","stop_reason":"end_turn"}}`)
	changedAt := now.Add(2 * time.Second)
	if err := os.Chtimes(transcript, changedAt, changedAt); err != nil {
		t.Fatal(err)
	}
	remaining, visible, err := updateCacheCountdown(payload, tmp, ttl, changedAt, 111)
	if err != nil {
		t.Fatal(err)
	}
	if !visible || remaining != ttl {
		t.Fatalf("completed assistant reply should start cache countdown: visible=%v remaining=%s", visible, remaining)
	}

	remaining, visible, err = updateCacheCountdown(payload, tmp, ttl, changedAt.Add(2*time.Second), 111)
	if err != nil {
		t.Fatal(err)
	}
	if !visible || remaining != ttl-2*time.Second {
		t.Fatalf("ticking remaining = %s visible=%v, want %s true", remaining, visible, ttl-2*time.Second)
	}
}

func TestUpdateCacheCountdownStartsWhenCompletedAssistantHasTrailingNonMessageLine(t *testing.T) {
	tmp := t.TempDir()
	transcript := filepath.Join(tmp, "transcript.jsonl")
	writeTranscript(t, transcript,
		`{"type":"assistant","message":{"role":"assistant","content":"done","stop_reason":"end_turn"}}`,
		`{"type":"result","duration_ms":123}`,
	)
	payload := statuslinePayload{SessionID: "session-1", TranscriptPath: transcript}
	ttl := 5 * time.Minute
	now := time.Unix(1000, 0)

	remaining, visible, err := updateCacheCountdown(payload, tmp, ttl, now, 111)
	if err != nil {
		t.Fatal(err)
	}
	if !visible || remaining != ttl {
		t.Fatalf("completed assistant followed by non-message line should start cache countdown: visible=%v remaining=%s", visible, remaining)
	}
}

func TestUpdateCacheCountdownHidesAfterToolUseAssistant(t *testing.T) {
	tmp := t.TempDir()
	transcript := filepath.Join(tmp, "transcript.jsonl")
	writeTranscript(t, transcript,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash"}],"stop_reason":"tool_use"}}`,
		`{"type":"tool_result","content":"running"}`,
	)
	payload := statuslinePayload{SessionID: "session-1", TranscriptPath: transcript}
	ttl := 5 * time.Minute
	now := time.Unix(1000, 0)

	remaining, visible, err := updateCacheCountdown(payload, tmp, ttl, now, 111)
	if err != nil {
		t.Fatal(err)
	}
	if visible || remaining != 0 {
		t.Fatalf("tool_use assistant should not start cache countdown: visible=%v remaining=%s", visible, remaining)
	}
}

func TestUpdateCacheCountdownBaselinesResumeParentWithoutVisibleTimer(t *testing.T) {
	tmp := t.TempDir()
	transcript := filepath.Join(tmp, "transcript.jsonl")
	writeTranscript(t, transcript, `{"type":"assistant","message":{"role":"assistant","content":"old","stop_reason":"end_turn"}}`)
	payload := statuslinePayload{SessionID: "session-1", TranscriptPath: transcript}
	ttl := 5 * time.Minute
	now := time.Unix(1000, 0)

	remaining, visible, err := updateCacheCountdown(payload, tmp, ttl, now, 111)
	if err != nil {
		t.Fatal(err)
	}
	if !visible || remaining != ttl {
		t.Fatalf("first completed reply in parent should start timer: visible=%v remaining=%s", visible, remaining)
	}

	remaining, visible, err = updateCacheCountdown(payload, tmp, ttl, now.Add(time.Minute), 222)
	if err != nil {
		t.Fatal(err)
	}
	if visible || remaining != 0 {
		t.Fatalf("resume parent should baseline old transcript without visible timer: visible=%v remaining=%s", visible, remaining)
	}
}

func TestUpdateCacheCountdownHidesWhileLatestTranscriptLineIsNotAssistant(t *testing.T) {
	tmp := t.TempDir()
	transcript := filepath.Join(tmp, "transcript.jsonl")
	writeTranscript(t, transcript, `{"type":"assistant","message":{"role":"assistant","content":"old","stop_reason":"end_turn"}}`)
	payload := statuslinePayload{SessionID: "session-1", TranscriptPath: transcript}
	ttl := 5 * time.Minute
	now := time.Unix(1000, 0)
	if _, _, err := updateCacheCountdown(payload, tmp, ttl, now, 111); err != nil {
		t.Fatal(err)
	}

	writeTranscript(t, transcript, `{"type":"user","message":{"role":"user","content":"next"}}`)
	changedAt := now.Add(2 * time.Second)
	if err := os.Chtimes(transcript, changedAt, changedAt); err != nil {
		t.Fatal(err)
	}
	remaining, visible, err := updateCacheCountdown(payload, tmp, ttl, changedAt, 111)
	if err != nil {
		t.Fatal(err)
	}
	if visible || remaining != 0 {
		t.Fatalf("cache countdown should hide while the latest transcript line is not assistant: visible=%v remaining=%s", visible, remaining)
	}
}

func TestShouldHideStatuslineAfterExitCommand(t *testing.T) {
	tmp := t.TempDir()
	transcript := filepath.Join(tmp, "transcript.jsonl")
	writeTranscript(t, transcript,
		`{"type":"assistant","message":{"role":"assistant","content":"old","stop_reason":"end_turn"}}`,
		`{"type":"user","message":{"role":"user","content":"<command-name>/exit</command-name>\n<command-message>/exit</command-message>"}}`,
		`{"type":"result","duration_ms":123}`,
	)

	if !shouldHideStatusline(statuslinePayload{TranscriptPath: transcript}) {
		t.Fatal("statusline should hide after the latest semantic event is /exit")
	}
}

func TestShouldHideStatuslineDoesNotPersistAfterNewAssistantReply(t *testing.T) {
	tmp := t.TempDir()
	transcript := filepath.Join(tmp, "transcript.jsonl")
	writeTranscript(t, transcript,
		`{"type":"user","message":{"role":"user","content":"<command-name>/exit</command-name>"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":"new session","stop_reason":"end_turn"}}`,
	)

	if shouldHideStatusline(statuslinePayload{TranscriptPath: transcript}) {
		t.Fatal("statusline should render again after a newer assistant reply")
	}
}

func writeTranscript(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeStatuslineConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
}
