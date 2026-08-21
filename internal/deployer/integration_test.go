//go:build integration
// +build integration

// Integration test skeleton for the deployer package.
//
// IMPORTANT: This file uses the stock `ubuntu:22.04` image for illustration,
// but that image does NOT run sshd out of the box. The test as written will
// fail at the SSH dial step on the stock image.
//
// To make this test runnable end-to-end, build and publish a pre-baked image
// that:
//   - installs openssh-server
//   - sets a known root password
//   - exposes port 22
//   - runs sshd as PID 1 (or under an init that brings sshd up)
//
// Then replace the `pool.Run("ubuntu", "22.04", ...)` call below with
// `pool.Run("your-registry/spotter-test", "tag", ...)`. The pool.Skip-on-
// no-docker logic still applies, so the test will be skipped in environments
// without a Docker daemon.
//
// Run with:
//
//	go test -tags integration -run TestDeployEndToEnd ./internal/deployer/...
//
// Or run all integration tests:
//
//	go test -tags integration ./...
package deployer_test

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/spotter/spotter/internal/deployer"
)

// TestDeployEndToEnd spins up a container with SSH, deploys spotterd via the
// embed.FS-backed installer, parses the resulting DEVICE_ID, and finally
// uninstalls.
//
// This is a skeleton: see the package doc comment above for caveats about
// the stock ubuntu image. The test gracefully skips when Docker is
// unavailable so CI without a Docker daemon still passes.
func TestDeployEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}

	resource, err := pool.Run("ubuntu", "22.04", []string{
		"PASSWORD=spotterpass",
	})
	if err != nil {
		t.Fatalf("start container: %v", err)
	}
	// Best-effort cleanup. Purge is a no-op if the container is already gone.
	defer func() {
		if err := pool.Purge(resource); err != nil {
			t.Logf("purge resource: %v", err)
		}
	}()

	// NOTE: stock ubuntu:22.04 does not run sshd. Replace this image with a
	// pre-baked one that does, or `docker exec` an sshd setup here.
	time.Sleep(10 * time.Second)

	hp := resource.GetHostPort("22/tcp")
	if hp == "" {
		t.Fatal("no host:port for 22/tcp")
	}
	host, port, err := splitHostPort(hp)
	if err != nil {
		t.Fatalf("parse host:port %q: %v", hp, err)
	}

	password := os.Getenv("SPOTTER_TEST_PASSWORD")
	if password == "" {
		password = "spotterpass"
	}

	d := &deployer.Deployer{}

	deployReq := deployer.DeployRequest{
		IP:       host,
		SSHPort:  port,
		Username: "root",
		Password: password,
	}

	res, err := d.Deploy(context.Background(), deployReq)
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if res == nil || res.DeviceID == "" {
		t.Fatal("empty device_id in deploy result")
	}
	t.Logf("deployed device_id=%s", res.DeviceID)

	if err := d.Uninstall(context.Background(), deployReq); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
}

// splitHostPort parses strings of the form "host:port". It is intentionally
// minimal and only supports IPv4 / DNS hosts with a numeric port — sufficient
// for the addresses returned by dockertest's GetHostPort.
func splitHostPort(hp string) (string, int, error) {
	idx := strings.LastIndex(hp, ":")
	if idx < 0 {
		return hp, 22, nil
	}
	port, err := strconv.Atoi(hp[idx+1:])
	if err != nil {
		return "", 0, err
	}
	return hp[:idx], port, nil
}