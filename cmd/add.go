package cmd

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/hleidev/claude-switch/internal/config"
	"github.com/hleidev/claude-switch/internal/presets"
	"github.com/hleidev/claude-switch/internal/secret"
)

// addOptions is the resolved input for adding a provider. The interactive
// addOptions is the resolved input for adding a provider. The interactive
// prompts and flag parsing both reduce to this struct so the core is testable.
type addOptions struct {
	Name        string
	Protocol    string
	BaseURL     string // only for custom providers; presets supply their own
	Model       string
	SonnetModel string
	HaikuModel  string
	OpusModel   string
	FableModel  string
	Key         string
	Force       bool
}

// applyAdd validates options and writes the provider into cfg (without saving).
// A preset provider needs only the key; a custom one must supply a base_url,
// since there is no template to resolve it from.
func applyAdd(cfg *config.Config, opts addOptions) error {
	if opts.Name == "" {
		return fmt.Errorf("provider name is required")
	}
	if opts.Name == "claude" {
		return fmt.Errorf("claude is the built-in OAuth fallback; it is not a configurable provider")
	}
	if _, exists := cfg.Providers[opts.Name]; exists && !opts.Force {
		return fmt.Errorf("provider %q already exists (edit it with `cs edit`, or pass --force)", opts.Name)
	}
	_, isPreset := presets.Lookup(opts.Name)
	if !isPreset && opts.BaseURL == "" {
		return fmt.Errorf("base_url is required for custom provider %q", opts.Name)
	}

	proto := strings.ToLower(strings.TrimSpace(opts.Protocol))
	if proto == "" {
		proto = config.ProtocolAnthropic
	}
	if proto != config.ProtocolAnthropic && proto != config.ProtocolOpenAI {
		return fmt.Errorf("unsupported protocol %q (supported: anthropic, openai)", opts.Protocol)
	}

	if cfg.Providers == nil {
		cfg.Providers = map[string]config.Provider{}
	}
	p := config.Provider{}
	if opts.BaseURL != "" {
		p[config.BaseURLKey] = opts.BaseURL
	}

	if proto == config.ProtocolOpenAI {
		p[config.ProtocolKey] = config.ProtocolOpenAI
		p[config.AutoCompactWindowKey] = config.Default1MWindow

		if m := strings.TrimSpace(opts.Model); m != "" {
			// Explicit --model: apply to all roles unless individually overridden
			p[config.ModelKey] = m
			p[config.SonnetModelKey] = m
			p[config.HaikuModelKey] = m
			p[config.OpusModelKey] = m
			p[config.FableModelKey] = m
			p[config.SubagentModelKey] = m
		} else {
			// Tier-specific default model mappings
			p[config.OpusModelKey] = config.DefaultOpenAIOpusModel         // gpt-6.1-sol (heavy reasoning/flagship)
			p[config.SonnetModelKey] = config.DefaultOpenAISonnetModel     // gpt-6-sol (main coding/reasoning)
			p[config.HaikuModelKey] = config.DefaultOpenAIHaikuModel       // gpt-6-luna (fast/lightweight)
			p[config.FableModelKey] = config.DefaultOpenAIFableModel       // gpt-6-astra (exploratory/frontier)
			p[config.SubagentModelKey] = config.DefaultOpenAISubagentModel // gpt-6-luna (subagents)
			p[config.ModelKey] = config.DefaultOpenAIModel                 // gpt-6-sol (primary fallback)
		}

		if opts.SonnetModel != "" {
			p[config.SonnetModelKey] = opts.SonnetModel
		}
		if opts.HaikuModel != "" {
			p[config.HaikuModelKey] = opts.HaikuModel
			p[config.SubagentModelKey] = opts.HaikuModel
		}
		if opts.OpusModel != "" {
			p[config.OpusModelKey] = opts.OpusModel
		}
		if opts.FableModel != "" {
			p[config.FableModelKey] = opts.FableModel
		}
	} else {

		if opts.Model != "" {
			p[config.ModelKey] = opts.Model
		}
		if opts.SonnetModel != "" {
			p[config.SonnetModelKey] = opts.SonnetModel
		}
		if opts.HaikuModel != "" {
			p[config.HaikuModelKey] = opts.HaikuModel
		}
		if opts.OpusModel != "" {
			p[config.OpusModelKey] = opts.OpusModel
		}
		if opts.FableModel != "" {
			p[config.FableModelKey] = opts.FableModel
		}
	}

	if opts.Key != "" {
		p[config.AuthTokenKey] = opts.Key
	}
	cfg.Providers[opts.Name] = p
	return nil
}

var (
	addKeyStdin     bool
	addBaseURL      string
	addProtocol     string
	addModel        string
	addSonnetModel  string
	addHaikuModel   string
	addOpusModel    string
	addFableModel   string
	addForce        bool
	addNoVerify     bool
	addFromKeychain string
)

var addCmd = &cobra.Command{
	Use:   "add [provider]",
	Short: "Add a provider (interactive, or scriptable with --key-stdin)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runAdd,
}

func runAdd(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	errOut := cmd.ErrOrStderr()

	var name string
	if len(args) == 1 {
		name = args[0]
	}

	// Non-interactive path: caller pipes the key in or points to existing keychain item.
	if addKeyStdin || addFromKeychain != "" {
		var key string
		if addFromKeychain != "" {
			key = secret.KeychainRef(addFromKeychain)
			if secret.DefaultBackend.IsAvailable() {
				s, a := secret.ParseKeychainRef(key)
				if _, err := secret.DefaultBackend.Get(s, a); err != nil {
					return fmt.Errorf("keychain item (%s/%s) not found: %w", s, a, err)
				}
			}
		} else {
			rawKey, err := readAllStdin(cmd.InOrStdin())
			if err != nil {
				return err
			}
			if secret.DetectKind(rawKey) == secret.KindPlaintext && secret.DefaultBackend.IsAvailable() {
				if err := secret.SaveToKeychain(name, rawKey); err == nil {
					key = secret.KeychainRef(name)
				} else {
					key = rawKey
				}
			} else {
				key = rawKey
			}
		}
		opts := addOptions{
			Name:        name,
			Protocol:    addProtocol,
			BaseURL:     addBaseURL,
			Model:       addModel,
			SonnetModel: addSonnetModel,
			HaikuModel:  addHaikuModel,
			OpusModel:   addOpusModel,
			FableModel:  addFableModel,
			Key:         key,
			Force:       addForce,
		}
		if err := applyAdd(cfg, opts); err != nil {
			return err
		}
		if err := config.Save(cfg); err != nil {
			return err
		}
		finishAdd(cmd, opts)
		return nil
	}

	if !isInteractive() {
		return fmt.Errorf("not a terminal: provide the key non-interactively with --key-stdin (and --base-url for custom providers)")
	}

	// Interactive path.
	var customURL, customProto, customModel string
	if name == "" {
		name, customURL, customProto, customModel, err = pickProvider()
		if err != nil {
			return err
		}
	}
	if name == "claude" {
		return fmt.Errorf("claude is the built-in OAuth fallback; it is not a configurable provider")
	}

	var key string
	effectiveBaseURL := customURL
	if effectiveBaseURL == "" {
		if preset, ok := presets.Lookup(name); ok {
			effectiveBaseURL = preset[config.BaseURLKey]
		}
	}

	// 1. Smart check: check if a key exists in Keychain (exact match or from other tools).
	if match, exists := secret.DetectKeychainSmart(name, effectiveBaseURL); exists {
		var promptMsg string
		if match.Service == secret.DefaultService {
			promptMsg = fmt.Sprintf("Existing key for %s found in macOS Keychain. Reuse it?", name)
		} else {
			promptMsg = fmt.Sprintf("Found existing Keychain item %q (from other tools). Reuse it?", match.Service+"/"+match.Account)
		}
		reuse, err := confirmWithDefault(promptMsg, true)
		if err != nil {
			return err
		}
		if reuse {
			key = match.Ref()
		}
	}

	// 2. If not reused, check if an environment variable exists.
	if key == "" {
		if envVar, envVal, found := secret.DetectEnv(name); found {
			useEnv, err := confirmWithDefault(fmt.Sprintf("Found $%s in environment. Import to macOS Keychain?", envVar), true)
			if err != nil {
				return err
			}
			if useEnv {
				if secret.DefaultBackend.IsAvailable() {
					if err := secret.SaveToKeychain(name, envVal); err != nil {
						return fmt.Errorf("save to keychain: %w", err)
					}
					key = secret.KeychainRef(name)
				} else {
					key = "env:" + envVar
				}
			}
		}
	}

	// 3. Otherwise prompt for key input (allows pasting raw key or entering keychain:service/account).
	if key == "" {
		promptTitle := fmt.Sprintf("Paste API key for %s (or enter keychain:service/account)", name)
		inputKey, err := readSecret(promptTitle)
		if err != nil {
			return err
		}
		inputKey = strings.TrimSpace(inputKey)
		if inputKey == "" {
			return fmt.Errorf("no key entered")
		}
		if strings.HasPrefix(inputKey, "keychain:") {
			s, a := secret.ParseKeychainRef(inputKey)
			if _, err := secret.DefaultBackend.Get(s, a); err != nil {
				return fmt.Errorf("keychain item (%s/%s) not found: %w", s, a, err)
			}
			key = inputKey
		} else if secret.DetectKind(inputKey) != secret.KindPlaintext {
			key = inputKey
		} else if secret.DefaultBackend.IsAvailable() {
			if err := secret.SaveToKeychain(name, inputKey); err != nil {
				fmt.Fprintf(errOut, "⚠ could not save to keychain (%v); saving to config\n", err)
				key = inputKey
			} else {
				key = secret.KeychainRef(name)
			}
		} else {
			key = inputKey
		}
	}

	proto := addProtocol
	if customProto != "" && (addProtocol == "" || addProtocol == config.ProtocolAnthropic) {
		proto = customProto
	}
	model := addModel
	if customModel != "" && addModel == "" {
		model = customModel
	}

	opts := addOptions{
		Name:        name,
		Protocol:    proto,
		BaseURL:     addBaseURL,
		Model:       model,
		SonnetModel: addSonnetModel,
		HaikuModel:  addHaikuModel,
		OpusModel:   addOpusModel,
		FableModel:  addFableModel,
		Key:         key,
		Force:       addForce,
	}
	if customURL != "" {
		opts.BaseURL = customURL
	}
	if err := applyAdd(cfg, opts); err != nil {
		return err
	}
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Fprintln(errOut)
	finishAdd(cmd, opts)
	return nil
}

// finishAdd runs the post-write side effects: connectivity probe, key
// registration, and success hints.
func finishAdd(cmd *cobra.Command, opts addOptions) {
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()
	path, _ := config.ConfigPath()

	if !addNoVerify {
		baseURL := opts.BaseURL
		if baseURL == "" {
			if preset, ok := presets.Lookup(opts.Name); ok {
				baseURL = preset[config.BaseURLKey]
			}
		}
		switch {
		case baseURL == "":
			fmt.Fprintln(errOut, "⚠ no base_url to verify; saved anyway")
		default:
			if err := probe(baseURL); err != nil {
				fmt.Fprintf(errOut, "⚠ host unreachable (%v); saved anyway\n", err)
			} else {
				// We only verified DNS/TCP/TLS; an API error would still surface at runtime.
				fmt.Fprintln(errOut, "… host reachable ✓")
			}
		}
	}
	registered := registerKeyBestEffort(cmd, opts.Key)

	fmt.Fprintf(out, "✓ wrote '%s' → %s\n", opts.Name, path)
	if secret.DetectKind(opts.Key) == secret.KindKeychain {
		fmt.Fprintln(out, "✓ key secured in macOS Keychain")
	}
	if opts.Protocol == config.ProtocolOpenAI {
		fmt.Fprintf(out, "✓ protocol: openai (Opus: %s, Sonnet: %s, Haiku: %s, Fable: %s via bridge :8088)\n",
			config.DefaultOpenAIOpusModel, config.DefaultOpenAISonnetModel, config.DefaultOpenAIHaikuModel, config.DefaultOpenAIFableModel)
	}

	if registered {
		fmt.Fprintln(out, "✓ key registered with Claude Code (~/.claude.json)")
	} else {
		fmt.Fprintln(out, "  (Claude Code will approve this key on first run)")
	}
	fmt.Fprintf(out, "  cs use %s      switch this terminal\n", opts.Name)
	fmt.Fprintf(out, "  cs default %s  make it the default for new terminals\n", opts.Name)
}

// probe performs a short best-effort reachability check against base_url.
func probe(baseURL string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(baseURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// pickProvider shows the preset menu plus a custom option. For a custom
// provider it also returns the entered base_url, protocol, and model.
func pickProvider() (name, baseURL, protocol, model string, err error) {
	const customOpt = "custom…"
	var opts []huh.Option[string]
	for _, n := range presets.Names() {
		opts = append(opts, huh.NewOption(n, n))
	}
	opts = append(opts, huh.NewOption(customOpt, customOpt))

	var choice string
	if err := huh.NewSelect[string]().Title("Choose a provider").Options(opts...).Value(&choice).Run(); err != nil {
		return "", "", "", "", err
	}
	if choice != customOpt {
		return choice, "", config.ProtocolAnthropic, "", nil
	}

	if err := huh.NewInput().Title("Provider name").Value(&name).Run(); err != nil {
		return "", "", "", "", err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", "", "", fmt.Errorf("provider name is required")
	}

	protocol = addProtocol
	if protocol == "" || protocol == config.ProtocolAnthropic {
		protoOpts := []huh.Option[string]{
			huh.NewOption("anthropic (Native / Anthropic-compatible API)", config.ProtocolAnthropic),
			huh.NewOption("openai (Codex / OpenAI-compatible -> via bridge)", config.ProtocolOpenAI),
		}
		if err := huh.NewSelect[string]().Title("Protocol").Options(protoOpts...).Value(&protocol).Run(); err != nil {
			return "", "", "", "", err
		}
	}

	// A flag-provided --base-url wins; otherwise prompt for it.
	if addBaseURL != "" {
		baseURL = addBaseURL
	} else {
		if err := huh.NewInput().Title("base_url").Value(&baseURL).Run(); err != nil {
			return "", "", "", "", err
		}
		baseURL = strings.TrimSpace(baseURL)
	}

	if protocol == config.ProtocolOpenAI {
		if addModel != "" {
			model = addModel
		} else {
			model = config.DefaultOpenAIModel
			if err := huh.NewInput().Title("Model (default: " + config.DefaultOpenAIModel + ")").Value(&model).Run(); err != nil {
				return "", "", "", "", err
			}
			model = strings.TrimSpace(model)
			if model == "" {
				model = config.DefaultOpenAIModel
			}
		}
	}

	return name, baseURL, protocol, model, nil
}

func readAllStdin(r io.Reader) (string, error) {
	br := bufio.NewReader(r)
	data, err := io.ReadAll(br)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func init() {
	addCmd.Flags().BoolVar(&addKeyStdin, "key-stdin", false, "read the API key from stdin (non-interactive)")
	addCmd.Flags().StringVar(&addBaseURL, "base-url", "", "API base URL (required for custom providers)")
	addCmd.Flags().StringVar(&addProtocol, "protocol", "anthropic", "protocol type: 'anthropic' (default) or 'openai'")
	addCmd.Flags().StringVar(&addModel, "model", "", "model name (applies to all 4 Claude roles if specific role flags omitted)")
	addCmd.Flags().StringVar(&addSonnetModel, "sonnet-model", "", "override model for Sonnet role (main coding/reasoning)")
	addCmd.Flags().StringVar(&addHaikuModel, "haiku-model", "", "override model for Haiku role (fast/subagent tasks)")
	addCmd.Flags().StringVar(&addOpusModel, "opus-model", "", "override model for Opus role (deep reasoning)")
	addCmd.Flags().StringVar(&addFableModel, "fable-model", "", "override model for Fable role (experimental tasks)")
	addCmd.Flags().BoolVar(&addForce, "force", false, "overwrite an existing provider")
	addCmd.Flags().BoolVar(&addNoVerify, "no-verify", false, "skip the connectivity check")
	addCmd.Flags().StringVar(&addFromKeychain, "from-keychain", "", "reuse existing Keychain item (account or service/account)")
	rootCmd.AddCommand(addCmd)
}
