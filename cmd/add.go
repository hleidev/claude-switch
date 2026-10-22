package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/hleidev/claude-switch/internal/config"
	"github.com/hleidev/claude-switch/internal/presets"
	"github.com/hleidev/claude-switch/internal/secret"
)

// addOptions is the resolved input for adding a provider. The interactive
// prompts and flag parsing both reduce to this struct so the core is testable.
type addOptions struct {
	Name          string
	Protocol      string
	BaseURL       string // only for custom providers; presets supply their own
	Model         string
	SonnetModel   string
	HaikuModel    string
	OpusModel     string
	FableModel    string
	Key           string
	Force         bool
	UpdateKeyOnly bool // preserve existing provider fields and only update the key
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
	existing, exists := cfg.Providers[opts.Name]
	if exists && !opts.Force && !opts.UpdateKeyOnly {
		return fmt.Errorf("provider %q already exists (edit it with `cs edit`, or pass --force)", opts.Name)
	}

	if cfg.Providers == nil {
		cfg.Providers = map[string]config.Provider{}
	}

	if exists && opts.UpdateKeyOnly {
		p := config.Provider{}
		for k, v := range existing {
			p[k] = v
		}
		if opts.Key != "" {
			p[config.AuthTokenKey] = opts.Key
		}
		if opts.BaseURL != "" {
			p[config.BaseURLKey] = opts.BaseURL
		}
		if opts.Model != "" {
			p[config.ModelKey] = opts.Model
		}
		cfg.Providers[opts.Name] = p
		return nil
	}

	_, isPreset := presets.Lookup(opts.Name)
	if opts.Name == "codex" {
		p := config.Provider{}
		if opts.Key != "" && opts.Key != "auth:codex" {
			p[config.AuthTokenKey] = opts.Key
		}
		if opts.BaseURL != "" {
			p[config.BaseURLKey] = opts.BaseURL
		}
		if opts.Protocol != "" && opts.Protocol != config.ProtocolOpenAI {
			p[config.ProtocolKey] = opts.Protocol
		}
		cfg.Providers[opts.Name] = p
		return nil
	}
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
			p[config.SonnetModelKey] = config.DefaultOpenAISonnetModel     // gpt-6.1-sol (main coding/reasoning)
			p[config.HaikuModelKey] = config.DefaultOpenAIHaikuModel       // gpt-6-luna (fast/lightweight)
			p[config.FableModelKey] = config.DefaultOpenAIFableModel       // gpt-6-astra (exploratory/frontier)
			p[config.SubagentModelKey] = config.DefaultOpenAISubagentModel // gpt-6-luna (subagents)
			p[config.ModelKey] = config.DefaultOpenAIModel                 // gpt-6.1-sol (primary fallback)
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
	addFromEnv      string
)

var addCmd = &cobra.Command{
	Use:   "add [provider]",
	Short: "Add a provider (interactive, or scriptable with --key-stdin / --from-keychain / --from-env)",
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
		name = strings.TrimSpace(args[0])
	}

	// Non-interactive path: caller pipes the key in or points to an existing keychain/env item.
	if addKeyStdin || addFromKeychain != "" || addFromEnv != "" {
		var key string
		switch {
		case addFromKeychain != "":
			key = secret.KeychainRef(addFromKeychain)
			if secret.DefaultBackend.IsAvailable() {
				if _, err := secret.Resolve(key); err != nil {
					return err
				}
			}
		case addFromEnv != "":
			varName := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(addFromEnv), "env:"), "$"))
			key = "env:" + varName
			if _, err := secret.Resolve(key); err != nil {
				return err
			}
		default:
			rawKey, err := readAllStdin(cmd.InOrStdin())
			if err != nil {
				return err
			}
			key, err = storeOrValidateKey(name, rawKey, errOut)
			if err != nil {
				return err
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
		return fmt.Errorf("not a terminal: provide the key non-interactively with --key-stdin, --from-keychain, or --from-env (and --base-url for custom providers)")
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

	// Upfront duplicate check: ask how to proceed BEFORE prompting for keys or touching Keychain.
	force := addForce
	updateKeyOnly := false
	if existing, exists := cfg.Providers[name]; exists && !force {
		const (
			actUpdateKey = "update_key"
			actOverwrite = "overwrite"
			actCancel    = "cancel"
		)
		var action string
		menuOpts := []huh.Option[string]{
			huh.NewOption("Update API key only (keep existing URL & model settings)", actUpdateKey),
			huh.NewOption("Reconfigure provider from scratch (overwrite)", actOverwrite),
			huh.NewOption("Cancel", actCancel),
		}
		if err := huh.NewSelect[string]().
			Title(fmt.Sprintf("Provider %q already exists. What would you like to do?", name)).
			Options(menuOpts...).
			Value(&action).
			Run(); err != nil {
			return err
		}
		switch action {
		case actCancel:
			fmt.Fprintln(errOut, "aborted")
			return nil
		case actUpdateKey:
			updateKeyOnly = true
			force = true
			if customURL == "" {
				customURL = existing.UpstreamBaseURL()
			}
		case actOverwrite:
			force = true
		}
	}

	// If adding/overwriting a custom provider named on the CLI (e.g. `cs add my-relay`),
	// collect protocol, base_url, and model if not already provided via flags.
	_, isPreset := presets.Lookup(name)
	if !isPreset && !updateKeyOnly && customURL == "" && addBaseURL == "" {
		customURL, customProto, customModel, err = promptCustomProviderDetails()
		if err != nil {
			return err
		}
	}

	effectiveBaseURL := customURL
	if effectiveBaseURL == "" {
		effectiveBaseURL = addBaseURL
	}
	if effectiveBaseURL == "" {
		if preset, ok := presets.Lookup(name); ok {
			effectiveBaseURL = preset[config.BaseURLKey]
		}
	}

	var key string
	if name == "codex" && addFromKeychain == "" && addFromEnv == "" && !addKeyStdin {
		hasAuth, _ := detectCodexAuth()
		if hasAuth {
			fmt.Fprintln(errOut, "✓ Detected official Codex authentication (~/.codex/auth.json)")
			key = "auth:codex"
		} else {
			fmt.Fprintln(errOut, "! No active Codex authentication found in ~/.codex/auth.json")
			fmt.Fprintln(errOut, "  Please run official OpenAI codex login first.")
			return fmt.Errorf("codex authentication not found in ~/.codex/auth.json")
		}
	} else {
		var err error
		key, err = promptProviderKey(name, effectiveBaseURL, errOut)
		if err != nil {
			return err
		}
	}

	proto := addProtocol
	if customProto != "" && (addProtocol == "" || addProtocol == config.ProtocolAnthropic) {
		proto = customProto
	} else if preset, ok := presets.Lookup(name); ok {
		if pProto, ok := preset[config.ProtocolKey]; ok && pProto != "" && (addProtocol == "" || addProtocol == config.ProtocolAnthropic) {
			proto = pProto
		}
	}
	model := addModel
	if customModel != "" && addModel == "" {
		model = customModel
	}

	opts := addOptions{
		Name:          name,
		Protocol:      proto,
		BaseURL:       addBaseURL,
		Model:         model,
		SonnetModel:   addSonnetModel,
		HaikuModel:    addHaikuModel,
		OpusModel:     addOpusModel,
		FableModel:    addFableModel,
		Key:           key,
		Force:         force,
		UpdateKeyOnly: updateKeyOnly,
	}
	if customURL != "" && !updateKeyOnly {
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

// promptProviderKey scans Keychain and environment variables for candidate keys.
// The first screen stays focused on the three decisions most users need; the
// individual candidates are shown only after choosing to reuse an existing key.
func promptProviderKey(name, effectiveBaseURL string, errOut io.Writer) (string, error) {
	kcMatches := secret.DetectKeychainAll(name, effectiveBaseURL)
	envMatches := secret.DetectEnvAll(name)

	const (
		choiceSelectExisting = "__select_existing__"
		choicePasteNew       = "__paste_new__"
		choiceEnterRef       = "__enter_ref__"
	)

	if len(kcMatches) > 0 || len(envMatches) > 0 {
		// The provider's own item is the least surprising default. Ask about it
		// directly so users do not have to understand Keychain naming conventions.
		if len(kcMatches) > 0 && kcMatches[0].Service == secret.DefaultService && kcMatches[0].Account == name {
			useSaved, err := confirmWithDefault(
				fmt.Sprintf("Use the saved API key for %s? (%s)", name, secret.Mask(kcMatches[0].Secret)),
				true,
			)
			if err != nil {
				return "", err
			}
			if useSaved {
				return kcMatches[0].Ref(), nil
			}
		}

		var options []huh.Option[string]
		available := len(kcMatches) + len(envMatches)
		options = append(options,
			huh.NewOption(fmt.Sprintf("Choose an existing key (%d available)", available), choiceSelectExisting),
			huh.NewOption("Paste a new API key", choicePasteNew),
			huh.NewOption("Enter a secret reference (advanced)", choiceEnterRef),
		)

		var selected string
		if err := huh.NewSelect[string]().
			Title(fmt.Sprintf("How should %s authenticate?", name)).
			Options(options...).
			Value(&selected).
			Run(); err != nil {
			return "", err
		}

		switch selected {
		case choiceSelectExisting:
			return promptExistingProviderKey(name, kcMatches, envMatches)
		case choiceEnterRef:
			return promptReferenceInput(name, errOut)
		}
	}

	// Direct secret input (when no candidates exist, or user selected "Paste a new API key").
	promptTitle := fmt.Sprintf("Paste API key for %s (or type 'ref' to enter an env:/keychain:/op:// reference)", name)
	inputKey, err := readSecret(promptTitle)
	if err != nil {
		return "", err
	}
	inputKey = strings.TrimSpace(inputKey)
	if inputKey == "" {
		return "", fmt.Errorf("no key entered")
	}
	if strings.EqualFold(inputKey, "ref") {
		return promptReferenceInput(name, errOut)
	}
	return storeOrValidateKey(name, inputKey, errOut)
}

// promptExistingProviderKey shows the detailed candidates only after the user
// has chosen the existing-key path. Environment values remain dynamic by
// default; users can opt into copying one into the managed Keychain item.
func promptExistingProviderKey(name string, kcMatches []*secret.KeychainMatch, envMatches []secret.EnvMatch) (string, error) {
	var options []huh.Option[string]
	for _, m := range kcMatches {
		options = append(options, huh.NewOption(
			fmt.Sprintf("%s (%s)", m.DisplayName(), secret.Mask(m.Secret)),
			m.Ref(),
		))
	}
	for _, em := range envMatches {
		options = append(options, huh.NewOption(
			fmt.Sprintf("$%s (dynamic, %s)", em.Name, secret.Mask(em.Value)),
			"env:"+em.Name,
		))
	}

	var selected string
	if err := huh.NewSelect[string]().
		Title("Choose the API key to use").
		Options(options...).
		Value(&selected).
		Run(); err != nil {
		return "", err
	}
	if strings.HasPrefix(selected, "keychain:") {
		return selected, nil
	}

	varName := strings.TrimPrefix(selected, "env:")
	for _, em := range envMatches {
		if em.Name != varName {
			continue
		}
		if secret.DefaultBackend.IsAvailable() {
			copyToKeychain, err := confirmWithDefault(
				fmt.Sprintf("Save a copy of $%s in Keychain for future use?", varName),
				false,
			)
			if err != nil {
				return "", err
			}
			if copyToKeychain {
				if err := secret.SaveToKeychain(name, em.Value); err != nil {
					return "", fmt.Errorf("save to keychain: %w", err)
				}
				return secret.KeychainRef(name), nil
			}
		}
		return em.Ref(), nil
	}
	return "", fmt.Errorf("selected secret candidate is no longer available")
}

// promptReferenceInput prompts with visible text input for a secret reference URI.
func promptReferenceInput(name string, errOut io.Writer) (string, error) {
	ref, err := readLine(
		fmt.Sprintf("Secret reference for %s", name),
		"env:MY_API_KEY, keychain:service/account, op://vault/item/field, or cmd:...",
	)
	if err != nil {
		return "", err
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("no secret reference entered")
	}
	return storeOrValidateKey(name, ref, errOut)
}

// storeOrValidateKey validates a secret reference (env:, keychain:, op://, cmd:)
// or stores a plaintext key into macOS Keychain when available.
func storeOrValidateKey(name, rawInput string, errOut io.Writer) (string, error) {
	rawInput = strings.TrimSpace(rawInput)
	if rawInput == "" {
		return "", fmt.Errorf("no key entered")
	}
	kind := secret.DetectKind(rawInput)
	if kind != secret.KindPlaintext {
		if _, err := secret.Resolve(rawInput); err != nil {
			return "", err
		}
		return rawInput, nil
	}
	if secret.DefaultBackend.IsAvailable() {
		if err := secret.SaveToKeychain(name, rawInput); err != nil {
			fmt.Fprintf(errOut, "⚠ could not save to keychain (%v); saving to config\n", err)
			return rawInput, nil
		}
		return secret.KeychainRef(name), nil
	}
	return rawInput, nil
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
	var registered bool
	fmt.Fprintf(out, "✓ wrote '%s' → %s\n", opts.Name, path)
	if opts.Name == "codex" && (opts.Key == "" || opts.Key == "auth:codex") {
		fmt.Fprintln(out, "✓ linked official authentication (~/.codex/auth.json)")
	} else {
		registered = registerKeyBestEffort(cmd, opts.Key)
		switch secret.DetectKind(opts.Key) {
		case secret.KindKeychain:
			fmt.Fprintf(out, "✓ key secured via %s\n", secret.FormatSource(opts.Key))
		case secret.KindEnv, secret.KindOnePassword, secret.KindCommand:
			fmt.Fprintf(out, "✓ key linked dynamically via %s\n", secret.FormatSource(opts.Key))
		}
	}
	if opts.Protocol == config.ProtocolOpenAI {
		fmt.Fprintf(out, "✓ protocol: openai (Opus: %s, Sonnet: %s, Haiku: %s, Fable: %s via bridge :8088)\n",
			config.DefaultOpenAIOpusModel, config.DefaultOpenAISonnetModel, config.DefaultOpenAIHaikuModel, config.DefaultOpenAIFableModel)
	}

	if opts.Name == "codex" && (opts.Key == "" || opts.Key == "auth:codex") {
		// Native OAuth token managed via Bridge, no key registration needed
	} else if registered {
		fmt.Fprintln(out, "✓ key registered with Claude Code (~/.claude.json)")
	} else {
		fmt.Fprintln(out, "  (Claude Code will approve this key on first run)")
	}
	fmt.Fprintf(out, "  cs use %s      switch this terminal\n", opts.Name)
	fmt.Fprintf(out, "  cs default %s  make it the default for new terminals\n", opts.Name)
}

// detectCodexAuth checks whether a valid official OpenAI Codex OAuth session exists in ~/.codex/auth.json.
func detectCodexAuth() (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	p := filepath.Join(home, ".codex", "auth.json")
	b, err := os.ReadFile(p)
	if err != nil {
		return false, nil
	}
	var data struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(b, &data); err != nil {
		return false, nil
	}
	return strings.TrimSpace(data.Tokens.AccessToken) != "", nil
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

	baseURL, protocol, model, err = promptCustomProviderDetails()
	if err != nil {
		return "", "", "", "", err
	}
	return name, baseURL, protocol, model, nil
}

// promptCustomProviderDetails prompts interactively for protocol, base_url, and model.
func promptCustomProviderDetails() (baseURL, protocol, model string, err error) {
	protocol = addProtocol
	if protocol == "" || protocol == config.ProtocolAnthropic {
		protoOpts := []huh.Option[string]{
			huh.NewOption("anthropic (Native / Anthropic-compatible API)", config.ProtocolAnthropic),
			huh.NewOption("openai (Codex / OpenAI-compatible -> via bridge)", config.ProtocolOpenAI),
		}
		if err := huh.NewSelect[string]().Title("Protocol").Options(protoOpts...).Value(&protocol).Run(); err != nil {
			return "", "", "", err
		}
	}

	// A flag-provided --base-url wins; otherwise prompt for it.
	if addBaseURL != "" {
		baseURL = addBaseURL
	} else {
		if err := huh.NewInput().Title("base_url").Value(&baseURL).Run(); err != nil {
			return "", "", "", err
		}
		baseURL = strings.TrimSpace(baseURL)
	}

	if protocol == config.ProtocolOpenAI {
		if addModel != "" {
			model = addModel
		} else {
			model = config.DefaultOpenAIModel
			if err := huh.NewInput().Title("Model (default: " + config.DefaultOpenAIModel + ")").Value(&model).Run(); err != nil {
				return "", "", "", err
			}
			model = strings.TrimSpace(model)
			if model == "" {
				model = config.DefaultOpenAIModel
			}
		}
	}
	return baseURL, protocol, model, nil
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
	addCmd.Flags().BoolVar(&addKeyStdin, "key-stdin", false, "read the API key or secret reference from stdin (non-interactive)")
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
	addCmd.Flags().StringVar(&addFromEnv, "from-env", "", "reference an environment variable dynamically (e.g. CODERPLAN_API_KEY)")
	rootCmd.AddCommand(addCmd)
}
