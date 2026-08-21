package deployer

import (
	"context"
	"fmt"
)

// Uninstall runs the embedded uninstall.sh on the target. The caller
// must re-supply credentials (they are not persisted).
func (d *Deployer) Uninstall(ctx context.Context, req DeployRequest) error {
	if req.SSHPort == 0 {
		req.SSHPort = 22
	}
	client, err := dialSSH(ctx, req)
	if err != nil {
		return fmt.Errorf("ssh dial: %w", err)
	}
	defer client.Close()

	sftpClient, err := sftpClient(client)
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	uninstallBytes, err := scriptsFS.ReadFile(UninstallScript)
	if err != nil {
		return fmt.Errorf("read embedded uninstall.sh: %w", err)
	}
	if err := upload(sftpClient, "/tmp/uninstall.sh", uninstallBytes, 0755); err != nil {
		return err
	}
	if _, err := runRemote(ctx, client, "bash /tmp/uninstall.sh"); err != nil {
		return fmt.Errorf("uninstall.sh: %w", err)
	}
	return nil
}
