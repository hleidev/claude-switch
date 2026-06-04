setup() {
  # Isolated CS_HOME — never touches ~/.claude-switch
  # Force /tmp on macOS where mktemp -t defaults to /var/folders/...
  export CS_HOME="$(mktemp -d /tmp/cs-test-XXXXXX)"
  export CS="$BATS_TEST_DIRNAME/../cs"
  export EDITOR="true"  # disable interactive $EDITOR in cs add / cs edit
  export VISUAL="true"

  # Unset any provider-related env vars from the parent shell or prior tests
  unset ANTHROPIC_AUTH_TOKEN ANTHROPIC_BASE_URL ANTHROPIC_MODEL
  unset ANTHROPIC_SMALL_FAST_MODEL ANTHROPIC_DEFAULT_SONNET_MODEL
  unset ANTHROPIC_DEFAULT_OPUS_MODEL ANTHROPIC_DEFAULT_HAIKU_MODEL
  unset API_TIMEOUT_MS CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC
  unset CLAUDE_CODE_EFFORT_LEVEL MINIMAX_API_KEY DEEPSEEK_API_KEY
  unset CLAUDE_SWITCH_PROVIDER
}

teardown() {
  rm -rf "$CS_HOME"
}
