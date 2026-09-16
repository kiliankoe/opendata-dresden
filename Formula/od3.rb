# Homebrew installs this from the repository itself, see the README. There are
# no tagged releases yet, so url, version and sha256 are bumped together.
class Od3 < Formula
  desc "CLI and MCP server for Dresden's OpenData portal"
  homepage "https://github.com/kiliankoe/opendata-dresden"
  url "https://github.com/kiliankoe/opendata-dresden/archive/b0e061900737bdeabfb1c76db3eb1010ebe8b083.tar.gz"
  version "0.2.0"
  sha256 "0185c85203c451fa0277ab56199f94f2eb9c8227d8031ee93025e826cca8c6cc"
  license "MIT"
  head "https://github.com/kiliankoe/opendata-dresden.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w"), "./cmd/od3"
  end

  test do
    assert_match "run the MCP server", shell_output("#{bin}/od3 help")
  end
end
