// Package presets embeds built-in provider definitions so `cs add` can offer a
// curated menu. A preset is a flat map of environment variables to export (the
// project-maintained template); it carries no secret — the user supplies the
// API key, which is merged in at use time.
package presets

import (
	_ "embed"
	"sort"

	toml "github.com/pelletier/go-toml/v2"
)

//go:embed data/presets.toml
var presetsData []byte

var presets map[string]map[string]string

func init() {
	if err := toml.Unmarshal(presetsData, &presets); err != nil {
		panic("claude-switch: invalid embedded presets: " + err.Error())
	}
}

// Lookup returns the preset's environment variables for name and whether it
// exists.
func Lookup(name string) (map[string]string, bool) {
	p, ok := presets[name]
	return p, ok
}

// Names returns all preset names, sorted.
func Names() []string {
	names := make([]string, 0, len(presets))
	for k := range presets {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
