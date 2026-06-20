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
)

// addOptions is the resolved input for adding a provider. The interactive
// prompts and flag parsing both reduce to this struct so the core is testable.
type addOptions struct {
	Name    string
	BaseURL string // only for custom providers; presets supply their own
	Key     string
	Force   bool
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
		return fmt.Errorf("provider %q already exists (edit it with `cs set`/`cs edit`, or pass --force)", opts.Name)
	}
	_, isPreset := presets.Lookup(opts.Name)
	if !isPreset && opts.BaseURL == "" {
		return fmt.Errorf("base_url is required for custom provider %q", opts.Name)
	}
	if cfg.Providers == nil {
		cfg.Providers = map[string]config.Provider{}
	}
	p := config.Provider{}
	if opts.BaseURL != "" {
		p[config.BaseURLKey] = opts.BaseURL
	}
	if opts.Key != "" {
		p[config.AuthTokenKey] = opts.Key
	}
	cfg.Providers[opts.Name] = p
	return nil
}

var (
	addKeyStdin bool
	addBaseURL  string
	addForce    bool
	addNoVerify bool
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

	// Non-interactive path: caller pipes the key in.
	if addKeyStdin {
		key, err := readAllStdin(cmd.InOrStdin())
		if err != nil {
			return err
		}
		opts := addOptions{Name: name, BaseURL: addBaseURL, Key: key, Force: addForce}
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
	var customURL string
	if name == "" {
		name, customURL, err = pickProvider()
		if err != nil {
			return err
		}
	}
	if name == "claude" {
		return fmt.Errorf("claude is the built-in OAuth fallback; it is not a configurable provider")
	}
	key, err := readSecret(fmt.Sprintf("Paste API key for %s", name))
	if err != nil {
		return err
	}
	if key == "" {
		return fmt.Errorf("no key entered")
	}
	opts := addOptions{Name: name, BaseURL: addBaseURL, Key: key, Force: addForce}
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
// provider it also returns the entered base_url (empty for a preset choice).
func pickProvider() (name, baseURL string, err error) {
	const customOpt = "custom…"
	var opts []huh.Option[string]
	for _, n := range presets.Names() {
		opts = append(opts, huh.NewOption(n, n))
	}
	opts = append(opts, huh.NewOption(customOpt, customOpt))

	var choice string
	if err := huh.NewSelect[string]().Title("Choose a provider").Options(opts...).Value(&choice).Run(); err != nil {
		return "", "", err
	}
	if choice != customOpt {
		return choice, "", nil
	}

	if err := huh.NewInput().Title("Provider name").Value(&name).Run(); err != nil {
		return "", "", err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", fmt.Errorf("provider name is required")
	}
	// A flag-provided --base-url wins; otherwise prompt for it.
	if addBaseURL != "" {
		return name, addBaseURL, nil
	}
	if err := huh.NewInput().Title("base_url").Value(&baseURL).Run(); err != nil {
		return "", "", err
	}
	return name, strings.TrimSpace(baseURL), nil
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
	addCmd.Flags().BoolVar(&addForce, "force", false, "overwrite an existing provider")
	addCmd.Flags().BoolVar(&addNoVerify, "no-verify", false, "skip the connectivity check")
	rootCmd.AddCommand(addCmd)
}
