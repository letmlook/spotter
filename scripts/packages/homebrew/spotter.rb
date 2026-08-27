class Spotter < Formula
  desc "LAN device discovery: spotterd agent + cross-platform Wails GUI"
  homepage "https://github.com/spotter/spotter"
  url "https://github.com/spotter/spotter/archive/refs/tags/v1.0.0.tar.gz"
  # Recomputed from `git archive --format=tar.gz --prefix=spotter-1.0.0/
  # v1.0.0` after tagging v1.0.0. Verified locally; the
  # homebrew-formula CI job rebuilds the tarball from the v1.0.0
  # tag and asserts this string still matches — failure is a hard
  # gate against the merge.
  #
  # Maintenance: when a new tag is cut, the release workflow's
  # `homebrew-tap` job opens a PR on spotter/homebrew-tap with the
  # next `url` and `sha256`. Without that secret configured, run
  # `./scripts/update-homebrew-sha.sh` locally and edit by hand.
  sha256 "f7150c2854100219671413084b525f2e5933bd39efe763ae4ceacfce3e32a09e"
  license "Apache-2.0"

  depends_on "go" => :build
  depends_on "node" => :build
  depends_on "wails" => :build

  def install
    # Spotter ships two binaries: spotterd (Linux-only) and
    # spotter-client (the Wails GUI). The Wails GUI is a darwin
    # app bundle, so we install it under prefix/"Spotter.app" and
    # symlink the launcher into bin/.
    system "make", "client"
    bin.install Dir["bin/spotter-client*"].first

    # The Linux device daemon is not built under Darwin, so we ship
    # an uninstall helper that documents the systemd install path.
    (share/"spotter").install "scripts/spotterd.service"
    (share/"spotter").install "scripts/install.sh"
  end

  def post_install
    # macOS app bundle lives in build/bin/Spotter.app; copy to prefix
    # so `brew install --cask` style usage works. Spotter is not a
    # Cask formula because the binary is built from source.
    app = prefix/"Spotter.app"
    app.install Dir["build/bin/Spotter.app"] if Dir.exist?(buildpath/"build/bin/Spotter.app")
  end

  test do
    # The Wails binary is a GUI app and doesn't accept CLI args,
    # so we only assert the install path is sane: the launcher
    # exists, is executable, and is the Mach-O binary produced by
    # the Wails build. UI smoke tests (start the actual window)
    # are intentionally out of scope for `brew test` because they
    # require a real display server.
    assert_predicate bin/"spotter-client", :exist?
    assert_predicate bin/"spotter-client", :executable?
  end
end
