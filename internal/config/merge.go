package config

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/hleidev/claude-switch/internal/secret"
)

// WrapBridgeURL wraps a raw upstream URL into the local bridge proxy URL.
// If the URL is already wrapped or points to localhost/127.0.0.1 with a proto path, it is returned unchanged.
func WrapBridgeURL(bridgeBase, rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return rawURL
	}
	if (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost") && (strings.Contains(u.Path, "/http/") || strings.Contains(u.Path, "/https/")) {
		return rawURL
	}
	if bridgeBase == "" {
		bridgeBase = DefaultBridgeBase
	}
	bridgeBase = strings.TrimRight(bridgeBase, "/")
	cleanPath := strings.TrimRight(u.Path, "/")
	return fmt.Sprintf("%s/%s/%s%s", bridgeBase, u.Scheme, u.Host, cleanPath)
}

// EnvVar is a single environment-variable assignment.
type EnvVar struct {
	Key   string
	Value string
}

// BuildEnv computes the ordered environment assignments for a provider by
// merging three flat layers, each overriding the previous on a per-key basis:
//
//	[defaults]  →  preset (built-in template, may be nil)  →  the provider's own entries
//
// A key whose final value is empty is treated as "not exported" (so an override
// can blank out an inherited variable). Unsafe variable names are skipped as
// defense in depth. Output is sorted for determinism, with ProviderVar appended
// last. The preset is injected by the caller (cmd layer) so this package stays
// decoupled from the embedded presets and is easy to test in isolation.
func (c *Config) BuildEnv(name string, preset map[string]string) ([]EnvVar, error) {
	p, ok := c.Providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %q not found", name)
	}

	merged := map[string]string{}
	for k, v := range c.Defaults {
		merged[k] = v
	}
	for k, v := range preset {
		merged[k] = v
	}
	for k, v := range p {
		merged[k] = v
	}

	if p.Protocol() == ProtocolOpenAI {
		bridgeBase := os.Getenv("CLAUDE_SWITCH_BRIDGE_BASE")
		if bridgeBase == "" {
			bridgeBase = DefaultBridgeBase
		}
		if rawURL, ok := merged[BaseURLKey]; ok && rawURL != "" {
			merged[BaseURLKey] = WrapBridgeURL(bridgeBase, rawURL)
		}
	}

	keys := make([]string, 0, len(merged))
	for k, v := range merged {
		if k == ProviderVar || v == "" || !ValidEnvKey(k) {
			continue
		}
		if secret.DetectKind(v) != secret.KindPlaintext {
			resolved, err := secret.Resolve(v)
			if err != nil {
				return nil, fmt.Errorf("resolve %s for %s: %w", k, name, err)
			}
			merged[k] = resolved
			if resolved == "" {
				continue
			}
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]EnvVar, 0, len(keys)+1)
	for _, k := range keys {
		out = append(out, EnvVar{Key: k, Value: merged[k]})
	}
	out = append(out, EnvVar{Key: ProviderVar, Value: name})
	return out, nil
}
