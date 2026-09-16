# Homebrew installs this from the repository itself, see the README. Building
# from a tag instead of a tarball needs no checksum, so `make release` can bump
# the formula in the same commit it tags.
class Od3 < Formula
  desc "CLI and MCP server for Dresden's OpenData portal"
  homepage "https://github.com/kiliankoe/opendata-dresden"
  url "https://github.com/kiliankoe/opendata-dresden.git", tag: "0.2.1"
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
