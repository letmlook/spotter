// spotter-cli is a small terminal client that reuses the scanner
// to list devices, scan a subnet, and stream the execution log of a
// specific device. It's the answer to FAQ "what if I only have
// SSH?" — operators can drive the system without the Wails GUI.
//
//go:build linux || darwin || windows

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spotter/spotter/internal/clientconfig"
	"github.com/spotter/spotter/internal/registry"
	"github.com/spotter/spotter/internal/scanner"
)

func main() {
	code := run(os.Args[1:], os.Stdout, os.Stderr)
	os.Exit(code)
}

// run is the testable entry point. It dispatches to the right
// sub-command against the provided stdout/stderr and returns the
// intended exit code without calling os.Exit. Tests drive this
// directly so they can observe side effects on captured streams.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: spotter-cli <list|scan|info|version> [...]")
		fmt.Fprintln(stderr, "  list                 List devices from the local registry.")
		fmt.Fprintln(stderr, "  scan [cidr]          Scan a subnet (or auto-pick the first RFC1918).")
		fmt.Fprintln(stderr, "  info <device_id>     Fetch /api/v1/info from the device.")
		fmt.Fprintln(stderr, "  version              Print version.")
		return 2
	}
	cmd, args := args[0], args[1:]
	switch cmd {
	case "list", "ls":
		return cmdList(stdout, stderr)
	case "scan":
		return cmdScan(args, stdout, stderr)
	case "info":
		return cmdInfo(args, stdout, stderr)
	case "version":
		fmt.Fprintln(stdout, "spotter-cli dev")
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", cmd)
		fmt.Fprintln(stderr, "usage: spotter-cli <list|scan|info|version> [...]")
		return 2
	}
}

// userDataDir returns the directory under which the CLI keeps its
// settings.json and devices.json. We honour $XDG_CONFIG_HOME
// (Linux), $HOME + Library/Application Support (macOS), and
// %AppData% (Windows). Test harness relies on $XDG_CONFIG_HOME /
// $HOME being set, so we resolve both branches.
func userDataDir() (string, error) {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "Spotter"), nil
	}
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfgDir, "Spotter"), nil
}

// settingsPath returns the conventional location for settings.json.
func settingsPath() string {
	dir, err := userDataDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "settings.json")
}

// registryPath returns the conventional location for devices.json.
func registryPath() string {
	dir, err := userDataDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "devices.json")
}

// openStoreSilent is the test-friendly variant: it does NOT exit on
// error; the caller inspects the returned error. Production code
// uses openStore which does os.Exit.
func openStoreSilent() (reg *registry.Registry, cfg *clientconfig.Store, err error) {
	reg, rerr := registry.Open(registryPath())
	if rerr != nil {
		return nil, nil, rerr
	}
	cfg, serr := clientconfig.Open(settingsPath())
	if serr != nil {
		reg.Close()
		return nil, nil, serr
	}
	return reg, cfg, nil
}

// openStore is the production path used by cmdList / cmdScan /
// cmdInfo. It logs to stderr and exits non-zero on failure so
// the binary's UX matches traditional CLI expectations.
func openStore() (*registry.Registry, *clientconfig.Store) {
	reg, cfg, err := openStoreSilent()
	if err != nil {
		fmt.Fprintln(os.Stderr, "open store:", err)
		os.Exit(1)
	}
	return reg, cfg
}

func cmdList(stdout, stderr io.Writer) int {
	reg, _ := openStore()
	defer reg.Close()
	list := reg.List()
	if len(list) == 0 {
		fmt.Fprintln(stdout, "(no devices)")
		return 0
	}
	w := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	for _, d := range list {
		online := "offline"
		if d.Online {
			online = "online"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s:%d\t%s\n",
			online, d.DeviceID, d.IP, d.Port, d.LastSource)
	}
	_ = w.Flush()
	return 0
}

func cmdScan(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	cidr := fs.String("cidr", "", "CIDR to scan (default: auto)")
	timeout := fs.Duration("timeout", 30*time.Second, "overall timeout")
	_ = fs.Parse(args)
	reg, cfg := openStore()
	defer reg.Close()
	opts := []func(*scanner.Options){
		scanner.WithMulticastGroup(cfg.Get().MulticastGroup),
		scanner.WithDevicePort(cfg.Get().DevicePort),
	}
	s := scanner.New(reg, opts...)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	target := *cidr
	if target == "" {
		subs := mainpkgLocalSubnets()
		if len(subs) > 0 {
			target = subs[0]
		}
	}
	if target == "" {
		fmt.Fprintln(stderr, "no subnet detected; pass --cidr")
		return 1
	}
	fmt.Fprintf(stderr, "scanning %s (timeout=%s)\n", target, *timeout)
	if err := s.ScanSubnet(ctx, target, *timeout); err != nil {
		fmt.Fprintln(stderr, "scan:", err)
		return 1
	}
	return 0
}

func cmdInfo(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: spotter-cli info <device_id>")
		return 2
	}
	id := args[0]
	reg, _ := openStore()
	defer reg.Close()
	fresh, ok := reg.Get(id)
	if !ok {
		fmt.Fprintln(stderr, "device not in registry")
		return 1
	}
	if fresh.LastInfo == nil {
		fmt.Fprintln(stderr, "no cached info; run 'spotter-cli scan' first")
		return 1
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(fresh.LastInfo)
	return 0
}

// mainpkgLocalSubnets re-implements main.go's LocalSubnets inline
// so the CLI binary doesn't depend on package main.
func mainpkgLocalSubnets() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var cidrs []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil {
				continue
			}
			ip := ipnet.IP.To4()
			if ip[0] == 169 && ip[1] == 254 {
				continue
			}
			ones, _ := ipnet.Mask.Size()
			cidrs = append(cidrs, fmt.Sprintf("%s/%d", ip.Mask(ipnet.Mask), ones))
		}
	}
	sort.SliceStable(cidrs, func(i, j int) bool { return rfc1918Rank(cidrs[i]) < rfc1918Rank(cidrs[j]) })
	return cidrs
}

func rfc1918Rank(cidr string) int {
	ip := net.ParseIP(cidr[:strings.IndexByte(cidr, '/')])
	if ip == nil {
		return 1
	}
	// net.ParseIP returns IPv4 addresses in 16-byte form (mapped);
	// the IPv4 bytes live at indices [12..15]. Take To4 so we
	// always inspect the right slot regardless of input form.
	v4 := ip.To4()
	if v4 == nil {
		return 1
	}
	switch {
	case v4[0] == 10:
		return 0
	case v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31:
		return 0
	case v4[0] == 192 && v4[1] == 168:
		return 0
	}
	return 1
}
