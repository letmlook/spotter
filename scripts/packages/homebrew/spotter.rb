class Spotter < Formula
  desc "LAN device discovery: spotterd agent + cross-platform Wails GUI"
  homepage "https://github.com/spotter/spotter"
  url "https://github.com/spotter/spotter/archive/refs/tags/v1.0.0.tar.gz"
  sha256 "REPLACE_AT_RELEASE"
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
    system bin/"spotter-client", "version"
  end
end
