package deployer

import "embed"

//go:embed scripts/*.sh scripts/spotterd.service
var scriptsFS embed.FS

// script names we expect to find. Used by deploy/uninstall.
const (
	InstallScript   = "scripts/install.sh"
	UninstallScript = "scripts/uninstall.sh"
	CleanupScript   = "scripts/cleanup.sh"
	UnitFile        = "scripts/spotterd.service"
)
