package deployer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// DeployRequest holds the parameters for deploying spotterd to a remote
// device.
type DeployRequest struct {
	IP       string
	SSHPort  int // default 22
	Username string
	Password string
}

// DeployResult is the parsed output from install.sh.
type DeployResult struct {
	DeviceID string
}

// Deployer performs SSH-based deploys and uninstalls.
type Deployer struct {
	AgentBinary []byte // contents of spotterd-linux-arm64; nil = read from default path
}

// Deploy connects, uploads agent + unit + install.sh, runs install.sh,
// parses DEVICE_ID.
func (d *Deployer) Deploy(ctx context.Context, req DeployRequest) (*DeployResult, error) {
	if req.SSHPort == 0 {
		req.SSHPort = 22
	}
	agentBytes, err := d.loadAgentBinary()
	if err != nil {
		return nil, err
	}
	unitBytes, err := scriptsFS.ReadFile(UnitFile)
	if err != nil {
		return nil, fmt.Errorf("read embedded unit: %w", err)
	}
	installBytes, err := scriptsFS.ReadFile(InstallScript)
	if err != nil {
		return nil, fmt.Errorf("read embedded install.sh: %w", err)
	}

	client, err := dialSSH(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("ssh dial: %w", err)
	}
	defer client.Close()
	sftpClient, err := sftpClient(client)
	if err != nil {
		return nil, fmt.Errorf("sftp: %w", err)
	}
	defer sftpClient.Close()

	if err := upload(sftpClient, "/tmp/spotterd", agentBytes, 0755); err != nil {
		return nil, err
	}
	if err := upload(sftpClient, "/tmp/spotterd.service", unitBytes, 0644); err != nil {
		return nil, err
	}
	if err := upload(sftpClient, "/tmp/install.sh", installBytes, 0755); err != nil {
		return nil, err
	}

	stdout, err := runRemote(ctx, client, "SPOTTER_AGENT_VERSION=0.1.0 bash /tmp/install.sh")
	if err != nil {
		// Best-effort cleanup
		_ = runRemoteQuiet(ctx, client, "bash /tmp/cleanup.sh")
		return nil, fmt.Errorf("install.sh: %w (stdout: %s)", err, stdout)
	}

	id, err := parseDeviceID(stdout)
	if err != nil {
		return nil, err
	}
	return &DeployResult{DeviceID: id}, nil
}

func (d *Deployer) loadAgentBinary() ([]byte, error) {
	if d.AgentBinary != nil {
		return d.AgentBinary, nil
	}
	for _, p := range []string{"bin/spotterd-linux-arm64", "../bin/spotterd-linux-arm64"} {
		if b, err := os.ReadFile(p); err == nil {
			return b, nil
		}
	}
	return nil, fmt.Errorf("agent binary not found; run `make agent-linux-arm64` first")
}

func dialSSH(ctx context.Context, req DeployRequest) (*ssh.Client, error) {
	_ = ctx // ssh.Dial does not accept context; we rely on the Timeout below
	cfg := &ssh.ClientConfig{
		User:            req.Username,
		Auth:            []ssh.AuthMethod{ssh.Password(req.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // MVP: trust on first use
		Timeout:         10 * time.Second,
	}
	addr := fmt.Sprintf("%s:%d", req.IP, req.SSHPort)
	return ssh.Dial("tcp", addr, cfg)
}

func sftpClient(c *ssh.Client) (*sftp.Client, error) {
	return sftp.NewClient(c)
}

func upload(c *sftp.Client, path string, data []byte, mode os.FileMode) error {
	f, err := c.Create(path)
	if err != nil {
		return fmt.Errorf("sftp create %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("sftp write %s: %w", path, err)
	}
	if err := f.Chmod(mode); err != nil {
		return fmt.Errorf("sftp chmod %s: %w", path, err)
	}
	return nil
}

func runRemote(ctx context.Context, c *ssh.Client, cmd string) (string, error) {
	_ = ctx // ssh.Client.NewSession does not accept context; cmd timeout relies on caller
	sess, err := c.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	var out bytes.Buffer
	sess.Stdout = &out
	sess.Stderr = &out
	if err := sess.Run(cmd); err != nil {
		return out.String(), err
	}
	return out.String(), nil
}

func runRemoteQuiet(ctx context.Context, c *ssh.Client, cmd string) error {
	_, err := runRemote(ctx, c, cmd)
	return err
}

var deviceIDRegex = regexp.MustCompile(`DEVICE_ID=([0-9a-fA-F-]+)`)

func parseDeviceID(stdout string) (string, error) {
	m := deviceIDRegex.FindStringSubmatch(stdout)
	if len(m) != 2 {
		return "", fmt.Errorf("could not parse DEVICE_ID from output: %q", stdout)
	}
	return m[1], nil
}
