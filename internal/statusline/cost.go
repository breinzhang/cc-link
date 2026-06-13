package statusline

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type statuslineCostState struct {
	LastMarker        int64   `json:"lastMarker"`
	SessionUsageTotal float64 `json:"sessionUsageTotal"`
	LastOfficialTotal float64 `json:"lastOfficialTotal"`
}

type statuslineCostLedgerEntry struct {
	Time      time.Time `json:"time"`
	SessionID string    `json:"sessionId"`
	AmountUSD float64   `json:"amountUsd"`
}

func updateCostSummary(payload statuslinePayload, cfg statuslineConfig, stateDir, ledgerPath string, now time.Time) (statuslineCostSummary, error) {
	if !cfg.Cost.isEnabled() {
		return statuslineCostSummary{}, nil
	}
	sessionID := hashSessionID(payload.SessionID)
	statePath := filepath.Join(stateDir, "cc-link-statusline-cost-"+sessionID+".json")
	state := statuslineCostState{}
	if data, err := os.ReadFile(statePath); err == nil {
		if err := json.Unmarshal(data, &state); err != nil {
			return statuslineCostSummary{}, err
		}
	} else if !os.IsNotExist(err) {
		return statuslineCostSummary{}, err
	}

	marker := transcriptMarker(payload.TranscriptPath)
	sessionTotal, delta := costDelta(payload, cfg, marker, &state)
	if delta > 0 {
		if err := appendCostLedgerEntry(ledgerPath, statuslineCostLedgerEntry{
			Time:      now,
			SessionID: sessionID,
			AmountUSD: delta,
		}); err != nil {
			return statuslineCostSummary{}, err
		}
	}
	if err := writeCostState(statePath, state); err != nil {
		return statuslineCostSummary{}, err
	}

	summary, err := readCostSummary(ledgerPath, now)
	if err != nil {
		return statuslineCostSummary{}, err
	}
	summary.Session = sessionTotal
	return summary, nil
}

func costDelta(payload statuslinePayload, cfg statuslineConfig, marker int64, state *statuslineCostState) (sessionTotal, delta float64) {
	if price, ok := matchModelPrice(payload.Model.ID, payload.Model.DisplayName, cfg.Cost.modelPrices()); ok {
		if payload.ContextWindow == nil || payload.ContextWindow.CurrentUsage == nil || marker == 0 {
			return 0, 0
		}
		if marker == state.LastMarker {
			return state.SessionUsageTotal, 0
		}
		delta = calculateUsageCost(*payload.ContextWindow.CurrentUsage, price)
		state.SessionUsageTotal += delta
		state.LastMarker = marker
		return state.SessionUsageTotal, delta
	}
	if payload.Cost.TotalCostUSD > 0 {
		sessionTotal = payload.Cost.TotalCostUSD
		if payload.Cost.TotalCostUSD > state.LastOfficialTotal {
			delta = payload.Cost.TotalCostUSD - state.LastOfficialTotal
			state.LastOfficialTotal = payload.Cost.TotalCostUSD
		}
		return sessionTotal, delta
	}
	return 0, 0
}

func calculateUsageCost(usage statuslineUsage, price statuslineModelPrice) float64 {
	cacheWrite := price.CacheWrite
	if cacheWrite == 0 {
		cacheWrite = price.Input
	}
	cacheRead := price.CacheRead
	if cacheRead == 0 {
		cacheRead = price.Input
	}
	return (float64(usage.InputTokens)*price.Input +
		float64(usage.OutputTokens)*price.Output +
		float64(usage.CacheCreationInputTokens)*cacheWrite +
		float64(usage.CacheReadInputTokens)*cacheRead) / 1_000_000
}

func matchModelPrice(modelID, displayName string, prices []statuslineModelPrice) (statuslineModelPrice, bool) {
	for _, p := range prices {
		if modelMatches(p.Match, modelID) || modelMatches(p.Match, displayName) {
			return p, true
		}
	}
	return statuslineModelPrice{}, false
}

func modelMatches(pattern, model string) bool {
	pattern = strings.TrimSpace(pattern)
	model = strings.TrimSpace(model)
	if pattern == "" || model == "" {
		return false
	}
	patternLower := strings.ToLower(pattern)
	modelLower := strings.ToLower(model)
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(modelLower, strings.TrimSuffix(patternLower, "*"))
	}
	return strings.EqualFold(pattern, model)
}

func transcriptMarker(path string) int64 {
	if path == "" {
		return 0
	}
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.ModTime().UnixNano()
}

func appendCostLedgerEntry(path string, entry statuslineCostLedgerEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func readCostSummary(path string, now time.Time) (statuslineCostSummary, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return statuslineCostSummary{}, nil
		}
		return statuslineCostSummary{}, err
	}
	defer f.Close()

	dayStart := startOfDay(now)
	weekStart := startOfWeek(now)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	var summary statuslineCostSummary
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry statuslineCostLedgerEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return statuslineCostSummary{}, err
		}
		if !entry.Time.Before(dayStart) {
			summary.Today += entry.AmountUSD
		}
		if !entry.Time.Before(weekStart) {
			summary.Week += entry.AmountUSD
		}
		if !entry.Time.Before(monthStart) {
			summary.Month += entry.AmountUSD
		}
	}
	return summary, scanner.Err()
}

func writeCostState(path string, state statuslineCostState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func startOfWeek(t time.Time) time.Time {
	dayStart := startOfDay(t)
	weekday := int(dayStart.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return dayStart.AddDate(0, 0, 1-weekday)
}

func hashSessionID(sessionID string) string {
	if sessionID == "" {
		sessionID = "unknown"
	}
	sum := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(sum[:])[:16]
}
