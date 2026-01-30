# typed: false
# frozen_string_literal: true

# Homebrew formula for Terminal Agent
# To install: brew install laszukdawid/tap/terminal-agent
class TerminalAgent < Formula
  desc "An LLM Agent to help you from and within the terminal"
  homepage "https://github.com/laszukdawid/terminal-agent"
  license "MIT"
  version "0.0.0" # This will be updated by GoReleaser

  on_macos do
    on_arm do
      url "https://github.com/laszukdawid/terminal-agent/releases/download/v#{version}/terminal-agent_Darwin_arm64.tar.gz"
      sha256 "PLACEHOLDER" # Updated by GoReleaser
    end
    on_intel do
      url "https://github.com/laszukdawid/terminal-agent/releases/download/v#{version}/terminal-agent_Darwin_x86_64.tar.gz"
      sha256 "PLACEHOLDER" # Updated by GoReleaser
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/laszukdawid/terminal-agent/releases/download/v#{version}/terminal-agent_Linux_arm64.tar.gz"
      sha256 "PLACEHOLDER" # Updated by GoReleaser
    end
    on_intel do
      url "https://github.com/laszukdawid/terminal-agent/releases/download/v#{version}/terminal-agent_Linux_x86_64.tar.gz"
      sha256 "PLACEHOLDER" # Updated by GoReleaser
    end
  end

  def install
    bin.install "agent"
  end

  test do
    assert_match "Terminal Agent", shell_output("#{bin}/agent --help")
  end
end
