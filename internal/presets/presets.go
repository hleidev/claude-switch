// Package presets embeds built-in provider definitions so `cs add` can offer a
// curated menu. Presets carry no secrets; the user supplies auth_token.
package presets

import (
	_ "embed"
	"sort"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/hleidev/claude-switch/internal/config"
)

//go:embed data/presets.toml
var presetsData []byte

var presets map[string]config.Provider

func init() {
	if err := toml.Unmarshal(presetsData, &presets); err != nil {
		panic("claude-switch: invalid embedded presets: " + err.Error())
	}
}

// Lookup returns the preset for name (auth_token empty) and whether it exists.
func Lookup(name string) (config.Provider, bool) {
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
