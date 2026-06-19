// Package claudejson registers third-party API key suffixes into Claude Code's
// ~/.claude.json so it does not re-prompt for approval. Writes are atomic and
// preserve all unknown fields; corrupting this file breaks Claude Code startup.
package claudejson

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// suffixLen is the trailing portion of the key recorded for approval, matching
// the legacy bash implementation.
const suffixLen = 20

// DefaultPath returns ~/.claude.json.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude.json"), nil
}

// RegisterKey ensures the last suffixLen characters of authToken are present in
// customApiKeyResponses.approved. Missing file or a token shorter than suffixLen
// is a no-op. Unknown fields are preserved; the write is atomic. The bool reports
// whether the file was actually written (false for a no-op or an already-present
// suffix), so callers don't claim a registration that didn't happen.
func RegisterKey(path, authToken string) (bool, error) {
	if len(authToken) < suffixLen {
		return false, nil
	}
	suffix := authToken[len(authToken)-suffixLen:]

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return false, err
	}

	resp, _ := doc["customApiKeyResponses"].(map[string]any)
	if resp == nil {
		resp = map[string]any{}
		doc["customApiKeyResponses"] = resp
	}
	approved, _ := resp["approved"].([]any)
	for _, v := range approved {
		if s, ok := v.(string); ok && s == suffix {
			return false, nil // already approved
		}
	}
	resp["approved"] = append(approved, suffix)

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return false, err
	}
	if err := atomicWrite(path, out); err != nil {
		return false, err
	}
	return true, nil
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".claude-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
