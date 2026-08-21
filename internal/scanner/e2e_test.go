package scanner_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/spotter/spotter/internal/protocol"
	"github.com/spotter/spotter/internal/registry"
	"github.com/spotter/spotter/internal/scanner"
)

// startFakeSpotterd spins up an HTTP handler returning a fixed
// DeviceInfo plus a UDP listener that replies to HELLO packets with
// the same info. Returns a cleanup func and the HTTP base URL.
func startFakeSpotterd(t *testing.T, info protocol.DeviceInfo) (string, string, func()) {
	t.Helper()

	// --- HTTP server on a random port ---
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/api/v1/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(info)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	httpSrv := &http.Server{Handler: mux}
	go func() { _ = httpSrv.Serve(ln) }()
	httpAddr := ln.Addr().String()

	// --- UDP listener (loopback only; not real multicast) ---
	udpLn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		_ = httpSrv.Close()
		t.Fatalf("udp listen: %v", err)
	}
	udpAddr := udpLn.LocalAddr().String()

	go func() {
		buf := make([]byte, 64*1024)
		for {
			n, src, err := udpLn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			t.Logf("fake spotterd received %d bytes from %s: %s", n, src.String(), string(buf[:n]))
			var hello protocol.HelloPacket
			if json.Unmarshal(buf[:n], &hello) != nil || hello.Type != "hello" {
				continue
			}
			reply := protocol.HelloReply{
				Type:     "hello_reply",
				DeviceID: info.DeviceID,
				Info:     info,
			}
			data, _ := json.Marshal(reply)
			conn, err := net.DialUDP("udp", nil, src)
			if err != nil {
				t.Logf("fake dial back: %v", err)
				continue
			}
			n2, err := conn.Write(data)
			t.Logf("fake reply wrote %d bytes to %s, err=%v", n2, src.String(), err)
			_ = conn.Close()
		}
	}()

	cleanup := func() {
		_ = httpSrv.Close()
		_ = udpLn.Close()
	}
	return httpAddr, udpAddr, cleanup
}

// TestE2E_UnknownDevicePayload verifies the unknown-device event
// carries both Info and the source IP, so the UI can prompt the user
// with a working IP even when multicast was the discovery path.
func TestE2E_UnknownDevicePayload(t *testing.T) {
	wantInfo := protocol.DeviceInfo{
		SchemaVersion: protocol.SchemaVersion,
		DeviceID:      "e2e-test-uuid",
		Basic:         protocol.BasicInfo{Hostname: "e2e-host"},
		Jetson:        &protocol.JetsonInfo{Model: "Fake Jetson"},
	}
	httpAddr, udpAddr, cleanup := startFakeSpotterd(t, wantInfo)
	defer cleanup()

	// Pull IP/port from httpAddr
	tcpIP, tcpPort, _ := net.SplitHostPort(httpAddr)
	_, _ = tcpIP, tcpPort

	reg, err := registry.Open(t.TempDir() + "/devices.json")
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}

	var (
		mu     sync.Mutex
		events []scanner.Event
	)
	capture := func(e scanner.Event) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
	}

	s := scanner.New(reg, scanner.WithOnEvent(capture), func(o *scanner.Options) {
		o.HTTPClient = &http.Client{Timeout: 2 * time.Second}
		o.MulticastGroup = udpAddr // point HELLO at our loopback UDP "multicast"
		o.PollInterval = time.Hour
		o.McastInterval = time.Hour // we'll trigger manually
		o.DevicePort = 9999         // not probed by mcast, but carried in event payload
	})

	// Trigger one mcast cycle directly.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Sanity: can we send a UDP packet to fake server at all?
	probe, err := net.DialUDP("udp", nil, mustResolve(t, udpAddr))
	if err != nil {
		t.Fatalf("probe dial: %v", err)
	}
	_, _ = probe.Write([]byte("PROBE"))
	t.Logf("PROBE sent to %s", udpAddr)
	_ = probe.Close()

	s.McastOnceForTest(ctx)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(events)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) == 0 {
		t.Fatalf("expected at least one event from mcast, got none")
	}

	// Find the unknown-device event.
	var unk *scanner.EventUnknownDeviceDiscovered
	for _, e := range events {
		if u, ok := e.(scanner.EventUnknownDeviceDiscovered); ok {
			unk = &u
			break
		}
	}
	if unk == nil {
		t.Fatalf("expected EventUnknownDeviceDiscovered, got %T: %+v", events[0], events)
	}
	if unk.Info.DeviceID != wantInfo.DeviceID {
		t.Errorf("Info.DeviceID = %q, want %q", unk.Info.DeviceID, wantInfo.DeviceID)
	}
	if unk.Info.Basic.Hostname != wantInfo.Basic.Hostname {
		t.Errorf("Info.Basic.Hostname = %q, want %q", unk.Info.Basic.Hostname, wantInfo.Basic.Hostname)
	}
	if unk.IP == "" {
		t.Errorf("event IP is empty; UI would have no address to prompt with")
	}
	if unk.Port != 9999 {
		t.Errorf("event Port = %d, want 9999", unk.Port)
	}
}

// TestE2E_PollOnceUpdatesEntry verifies that after a device is in the
// registry, PollOnce populates LastInfo and flips Online=true —
// i.e. the round-trip the GUI's "Refresh" button relies on.
func TestE2E_PollOnceUpdatesEntry(t *testing.T) {
	wantInfo := protocol.DeviceInfo{
		SchemaVersion: protocol.SchemaVersion,
		DeviceID:      "e2e-poll-uuid",
		Basic:         protocol.BasicInfo{Hostname: "e2e-poll-host"},
	}
	httpAddr, _, cleanup := startFakeSpotterd(t, wantInfo)
	defer cleanup()

	reg, err := registry.Open(t.TempDir() + "/devices.json")
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	if err := reg.Add(registry.Entry{
		DeviceID:   wantInfo.DeviceID,
		IP:         "127.0.0.1",
		Port:       mustPort(t, httpAddr),
		Username:   "fitow",
		DeployedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("reg.Add: %v", err)
	}

	var (
		mu     sync.Mutex
		events []scanner.Event
	)
	s := scanner.New(reg, scanner.WithOnEvent(func(e scanner.Event) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
	}), func(o *scanner.Options) {
		o.HTTPClient = &http.Client{Timeout: 2 * time.Second}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.PollOnce(ctx); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	got, ok := reg.Get(wantInfo.DeviceID)
	if !ok {
		t.Fatal("entry missing after poll")
	}
	if !got.Online {
		t.Errorf("Online = false, want true")
	}
	if got.LastInfo == nil || got.LastInfo.Basic.Hostname != "e2e-poll-host" {
		t.Errorf("LastInfo not populated: %+v", got.LastInfo)
	}
	if got.LastSource != "registry-poll" {
		t.Errorf("LastSource = %q, want registry-poll", got.LastSource)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) == 0 {
		t.Fatal("expected EventInfoUpdated from poll")
	}
	if _, ok := events[0].(scanner.EventInfoUpdated); !ok {
		t.Errorf("first event = %T, want EventInfoUpdated", events[0])
	}
}

// TestE2E_SubnetScanFindsDevice verifies the third path (manual
// subnet scan) finds our fake spotterd and emits unknown-device.
func TestE2E_SubnetScanFindsDevice(t *testing.T) {
	wantInfo := protocol.DeviceInfo{
		SchemaVersion: protocol.SchemaVersion,
		DeviceID:      "e2e-subnet-uuid",
		Basic:         protocol.BasicInfo{Hostname: "e2e-subnet-host"},
	}
	httpAddr, _, cleanup := startFakeSpotterd(t, wantInfo)
	defer cleanup()

	ip, portStr, _ := net.SplitHostPort(httpAddr)
	_ = portStr
	cidr := fmt.Sprintf("%s/32", ip)

	reg, err := registry.Open(t.TempDir() + "/devices.json")
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}

	var (
		mu     sync.Mutex
		events []scanner.Event
	)
	s := scanner.New(reg, scanner.WithOnEvent(func(e scanner.Event) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
	}), func(o *scanner.Options) {
		o.HTTPClient = &http.Client{Timeout: 2 * time.Second}
		o.DevicePort = mustPort(t, httpAddr)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.ScanSubnet(ctx, cidr, 3*time.Second); err != nil {
		t.Fatalf("ScanSubnet: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) == 0 {
		t.Fatal("expected EventUnknownDeviceDiscovered from subnet scan")
	}
	unk, ok := events[0].(scanner.EventUnknownDeviceDiscovered)
	if !ok {
		t.Fatalf("first event = %T, want EventUnknownDeviceDiscovered", events[0])
	}
	if unk.Info.DeviceID != wantInfo.DeviceID {
		t.Errorf("DeviceID = %q, want %q", unk.Info.DeviceID, wantInfo.DeviceID)
	}
	if unk.IP != ip {
		t.Errorf("IP = %q, want %q", unk.IP, ip)
	}
	if unk.Port != mustPort(t, httpAddr) {
		t.Errorf("Port = %d, want %d", unk.Port, mustPort(t, httpAddr))
	}
}

func mustPort(t *testing.T, addr string) int {
	t.Helper()
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split host/port: %v", err)
	}
	var port int
	if _, err := fmt.Sscanf(p, "%d", &port); err != nil {
		t.Fatalf("parse port %q: %v", p, err)
	}
	return port
}

func mustResolve(t *testing.T, addr string) *net.UDPAddr {
	t.Helper()
	a, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return a
}
