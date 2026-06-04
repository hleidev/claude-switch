load test_helper

# ── dispatch / help ────────────────────────────────────────────────────────

@test "no args shows help" {
  run "$CS"
  [ "$status" -eq 0 ]
  [[ "$output" == *"cs — Claude Code provider switcher"* ]]
  [[ "$output" == *"cs init zsh"* ]]
}

@test "unknown command exits non-zero with hint" {
  run "$CS" bogus
  [ "$status" -ne 0 ]
  [[ "$output" == *"unknown command: bogus"* ]]
  [[ "$output" == *"run 'cs' for help"* ]]
}

# ── cs init zsh ───────────────────────────────────────────────────────────

@test "init zsh prints shell integration" {
  run "$CS" init zsh
  [ "$status" -eq 0 ]
  [[ "$output" == *"_cs_reset"* ]]
  [[ "$output" == *"function cs"* ]]
  [[ "$output" == *"compdef _cs_complete cs"* ]]
}

@test "init zsh is the only subcommand of init" {
  run "$CS" init bash
  [ "$status" -ne 0 ]
  [[ "$output" == *"cs init zsh"* ]]
}

# ── cs use without shell integration ──────────────────────────────────────

@test "cs use without shell integration errors" {
  run "$CS" use minimax
  [ "$status" -ne 0 ]
  [[ "$output" == *"requires shell integration"* ]]
}

# ── cs default ────────────────────────────────────────────────────────────

@test "default with no config returns 'claude'" {
  run "$CS" default
  [ "$status" -eq 0 ]
  [ "$output" = "default: claude" ]
}

@test "default claude writes config.toml" {
  run "$CS" default claude
  [ "$status" -eq 0 ]
  [ -f "$CS_HOME/config.toml" ]
  run cat "$CS_HOME/config.toml"
  [[ "$output" == *"default_provider = \"claude\""* ]]
}

@test "default claude removes env.zsh" {
  touch "$CS_HOME/env.zsh"
  run "$CS" default claude
  [ "$status" -eq 0 ]
  [ ! -f "$CS_HOME/env.zsh" ]
}

@test "default unknown provider errors" {
  run "$CS" default nonexistent
  [ "$status" -ne 0 ]
  [[ "$output" == *"not found"* ]]
}

@test "default persists across invocations" {
  "$CS" default claude
  out=$("$CS" default)
  [ "$out" = "default: claude" ]
}

# ── cs add ─────────────────────────────────────────────────────────────────

@test "add without provider errors with usage" {
  run "$CS" add
  [ "$status" -ne 0 ]
  [[ "$output" == *"usage: cs add <provider>"* ]]
  [[ "$output" == *"minimax, deepseek, claude"* ]]
}

@test "add minimax creates provider file" {
  run "$CS" add minimax
  [ "$status" -eq 0 ]
  [ -f "$CS_HOME/providers/minimax.zsh" ]
  run cat "$CS_HOME/providers/minimax.zsh"
  [[ "$output" == *"MINIMAX_API_KEY"* ]]
  [[ "$output" == *"ANTHROPIC_BASE_URL"* ]]
  [[ "$output" == *"CLAUDE_SWITCH_PROVIDER=\"minimax\""* ]]
}

@test "add deepseek creates provider file" {
  run "$CS" add deepseek
  [ "$status" -eq 0 ]
  [ -f "$CS_HOME/providers/deepseek.zsh" ]
  run cat "$CS_HOME/providers/deepseek.zsh"
  [[ "$output" == *"DEEPSEEK_API_KEY"* ]]
  [[ "$output" == *"CLAUDE_SWITCH_PROVIDER=\"deepseek\""* ]]
}

@test "add unknown provider errors" {
  run "$CS" add unknownprovider
  [ "$status" -ne 0 ]
  [[ "$output" == *"unknown provider: unknownprovider"* ]]
  [[ "$output" == *"available templates: minimax, deepseek, claude"* ]]
}

@test "add duplicate provider errors and points to edit" {
  "$CS" add minimax
  run "$CS" add minimax
  [ "$status" -ne 0 ]
  [[ "$output" == *"already exists"* ]]
  [[ "$output" == *"cs edit minimax"* ]]
}

@test "add claude prints OAuth guidance (does not create file)" {
  run "$CS" add claude
  [ "$status" -eq 0 ]
  [ ! -f "$CS_HOME/providers/claude.zsh" ]
  [[ "$output" == *"claude uses OAuth"* ]]
  [[ "$output" == *"/login"* ]]
}

# ── cs edit ────────────────────────────────────────────────────────────────

@test "edit without provider errors" {
  run "$CS" edit
  [ "$status" -ne 0 ]
  [[ "$output" == *"usage: cs edit <provider>"* ]]
}

@test "edit claude is rejected (no config to edit)" {
  run "$CS" edit claude
  [ "$status" -ne 0 ]
  [[ "$output" == *"OAuth"* ]] || [[ "$output" == *"no config"* ]]
}

@test "edit unknown provider errors" {
  run "$CS" edit ghost
  [ "$status" -ne 0 ]
  [[ "$output" == *"not found"* ]]
  [[ "$output" == *"cs add ghost"* ]]
}

# ── cs remove ──────────────────────────────────────────────────────────────

@test "remove without provider errors" {
  run "$CS" remove
  [ "$status" -ne 0 ]
  [[ "$output" == *"usage: cs remove <provider>"* ]]
}

@test "remove claude is rejected" {
  run "$CS" remove claude
  [ "$status" -ne 0 ]
  [[ "$output" == *"built-in"* ]]
}

@test "remove unknown provider errors" {
  run "$CS" remove ghost
  [ "$status" -ne 0 ]
  [[ "$output" == *"not found"* ]]
}

@test "remove existing provider deletes file" {
  "$CS" add minimax
  run "$CS" remove minimax
  [ "$status" -eq 0 ]
  [ ! -f "$CS_HOME/providers/minimax.zsh" ]
  [[ "$output" == *"removed 'minimax'"* ]]
}

@test "remove resets default to claude if it was the active default" {
  "$CS" add minimax
  "$CS" default minimax
  out=$("$CS" remove minimax)
  [[ "$out" == *"default reset to 'claude'"* ]]
  cur=$("$CS" default)
  [ "$cur" = "default: claude" ]
}

@test "remove does not reset default if it was different" {
  "$CS" add minimax
  "$CS" add deepseek
  "$CS" default minimax
  out=$("$CS" remove deepseek)
  [[ "$out" != *"reset"* ]]
  cur=$("$CS" default)
  [ "$cur" = "default: minimax" ]
}

# ── cs list ────────────────────────────────────────────────────────────────

@test "list shows claude as built-in" {
  run "$CS" list
  [ "$status" -eq 0 ]
  [[ "$output" == *"claude"* ]]
  [[ "$output" == *"Providers:"* ]]
}

@test "list shows installed providers" {
  "$CS" add minimax
  run "$CS" list
  [[ "$output" == *"minimax"* ]]
}

@test "list marks the default" {
  "$CS" add minimax
  "$CS" default minimax
  out=$("$CS" list)
  [[ "$out" == *"minimax"* ]]
  [[ "$out" == *"← default"* ]] || [[ "$out" == *"<default"* ]]
}

# ── cs status ──────────────────────────────────────────────────────────────

@test "status with no env shows claude and unset token" {
  out=$("$CS" status)
  echo "$out" | grep -q "Current terminal"
  echo "$out" | grep -q "claude"
  echo "$out" | grep -q "Auth token"
  echo "$out" | grep -q "unset"
}

@test "status reads ANTHROPIC_BASE_URL from env" {
  export ANTHROPIC_BASE_URL="https://example.test"
  run "$CS" status
  [[ "$output" == *"https://example.test"* ]]
}

# ── isolation: tests never touch real ~/.claude-switch ────────────────────

@test "CS_HOME is fully isolated from real user config" {
  # If this test passes, no test in the file can have leaked into $HOME
  [ "$CS_HOME" != "$HOME/.claude-switch" ]
  [ "${CS_HOME:0:5}" = "/tmp/" ]
}
