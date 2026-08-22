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
	"net"
	"os"
	"os/signal"
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
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "list", "ls":
		cmdList()
	case "scan":
		cmdScan(args)
	case "info":
		cmdInfo(args)
	case "version":
		fmt.Println("spotter-cli dev")
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: spotter-cli <list|scan|info|version> [...]")
	fmt.Fprintln(os.Stderr, "  list                 List devices from the local registry.")
	fmt.Fprintln(os.Stderr, "  scan [cidr]          Scan a subnet (or auto-pick the first RFC1918).")
	fmt.Fprintln(os.Stderr, "  info <device_id>     Fetch /api/v1/info from the device.")
	fmt.Fprintln(os.Stderr, "  version              Print version.")
}

// settingsPath returns the conventional location for settings.json.
func settingsPath() string {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s/Spotter/settings.json", cfgDir)
}

// registryPath returns the conventional location for devices.json.
func registryPath() string {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s/Spotter/devices.json", cfgDir)
}

func openStore() (*registry.Registry, *clientconfig.Store) {
	reg, err := registry.Open(registryPath())
	if err != nil {
		fmt.Fprintln(os.Stderr, "open registry:", err)
		os.Exit(1)
	}
	cfg, err := clientconfig.Open(settingsPath())
	if err != nil {
		fmt.Fprintln(os.Stderr, "open settings:", err)
		os.Exit(1)
	}
	return reg, cfg
}

func cmdList() {
	reg, _ := openStore()
	defer reg.Close()
	list := reg.List()
	if len(list) == 0 {
		fmt.Println("(no devices)")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, d := range list {
		online := "offline"
		if d.Online {
			online = "online"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s:%d\t%s\n",
			online, d.DeviceID, d.IP, d.Port, d.LastSource)
	}
	_ = w.Flush()
}

func cmdScan(args []string) {
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
		fmt.Fprintln(os.Stderr, "no subnet detected; pass --cidr")
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "scanning %s (timeout=%s)\n", target, *timeout)
	if err := s.ScanSubnet(ctx, target, *timeout); err != nil {
		fmt.Fprintln(os.Stderr, "scan:", err)
		os.Exit(1)
	}
}

// infoCmd prints the cached /api/v1/info response for a device from
// the local registry as JSON. Run `spotter-cli scan` first to
// refresh the cache; the CLI intentionally has no live-poll path
// (use the GUI for that).
func cmdInfo(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: spotter-cli info <device_id>")
		os.Exit(2)
	}
	id := args[0]
	reg, _ := openStore()
	defer reg.Close()
	fresh, ok := reg.Get(id)
	if !ok {
		fmt.Fprintln(os.Stderr, "device not in registry")
		os.Exit(1)
	}
	if fresh.LastInfo == nil {
		fmt.Fprintln(os.Stderr, "no cached info; run 'spotter-cli scan' first")
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(fresh.LastInfo)
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
