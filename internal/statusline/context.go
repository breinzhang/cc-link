package statusline

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type statuslineContextState struct {
	ParentPID int               `json:"parentPid"`
	Context   statuslineContext `json:"context"`
}

func stableContextWindow(payload statuslinePayload, stateDir string, parentPID int) (*statuslineContext, error) {
	if payload.SessionID == "" {
		return payload.ContextWindow, nil
	}
	path := statuslineContextPath(stateDir, payload.SessionID)
	if hasCurrentUsage(payload.ContextWindow) {
		if err := writeStatuslineContext(path, payload.ContextWindow, parentPID); err != nil {
			return payload.ContextWindow, err
		}
		return payload.ContextWindow, nil
	}
	previous, ok, err := readStatuslineContext(path)
	if err != nil {
		return payload.ContextWindow, err
	}
	if ok {
		return &previous.Context, nil
	}
	return payload.ContextWindow, nil
}

func hasCurrentUsage(ctx *statuslineContext) bool {
	return ctx != nil && ctx.ContextWindowSize > 0 && ctx.CurrentUsage != nil
}

func statuslineContextPath(stateDir, sessionID string) string {
	return filepath.Join(stateDir, "cc-link-statusline-context-"+hashSessionID(sessionID)+".json")
}

func readStatuslineContext(path string) (*statuslineContextState, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var state statuslineContextState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, true, err
	}
	if state.Context.ContextWindowSize <= 0 {
		return nil, false, nil
	}
	return &state, true, nil
}

func writeStatuslineContext(path string, ctx *statuslineContext, parentPID int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.Marshal(statuslineContextState{ParentPID: parentPID, Context: *ctx})
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}
