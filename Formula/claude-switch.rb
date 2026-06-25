class ClaudeSwitch < Formula
  desc "Switch Claude Code's backend per terminal — one command, no global config"
  homepage "https://github.com/hleidev/claude-switch"
  url "https://github.com/hleidev/claude-switch/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "ad23d4002d84ef0c3f8933ad5e0d92dd356b69a2febc1b754d1dd14538ed7723"
  license "MIT"
  head "https://github.com/hleidev/claude-switch.git", branch: "main"

  depends_on "go" => :build

  def install
    ldflags = "-s -w -X main.version=#{version}"
    system "go", "build", *std_go_args(ldflags: ldflags)
  end

  def caveats
    <<~EOS
      `cs` is a shell function, not a binary. To enable it in the current terminal:
        source ~/.zshrc          # or ~/.bashrc / ~/.bash_profile
      To install the integration into your rc file (idempotent):
        claude-switch setup
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/claude-switch version")
  end
end
