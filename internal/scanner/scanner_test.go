package scanner_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/spotter/spotter/internal/protocol"
	"github.com/spotter/spotter/internal/registry"
	"github.com/spotter/spotter/internal/scanner"
)

func TestPollUpdatesOnline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/info" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(protocol.DeviceInfo{
			DeviceID: "d1",
			Basic:    protocol.BasicInfo{Hostname: "x"},
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	reg, _ := registry.Open(filepath.Join(dir, "devices.json"))
	_ = reg.Add(registry.Entry{
		DeviceID: "d1",
		IP:       srv.Listener.Addr().(*net.TCPAddr).IP.String(),
		Port:     srv.Listener.Addr().(*net.TCPAddr).Port,
	})

	var events []string
	sc := scanner.New(reg, scanner.WithOnEvent(func(e scanner.Event) {
		events = append(events, e.Tag())
	}))

	// Run a single poll synchronously.
	if err := sc.PollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	entry, _ := reg.Get("d1")
	if !entry.Online {
		t.Errorf("expected online, got %+v", entry)
	}
	if entry.LastInfo == nil || entry.LastInfo.Basic.Hostname != "x" {
		t.Errorf("expected info hostname x, got %+v", entry.LastInfo)
	}
	// Event "info-updated" should have been emitted.
	found := false
	for _, e := range events {
		if e == "info-updated" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected info-updated event, got %v", events)
	}
}

func TestPollOfflineAfter3Failures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	dir := t.TempDir()
	reg, _ := registry.Open(filepath.Join(dir, "devices.json"))
	_ = reg.Add(registry.Entry{
		DeviceID: "d1",
		IP:       srv.Listener.Addr().(*net.TCPAddr).IP.String(),
		Port:     srv.Listener.Addr().(*net.TCPAddr).Port,
		Online:   true,
	})

	var events []string
	sc := scanner.New(reg, scanner.WithOnEvent(func(e scanner.Event) {
		events = append(events, e.Tag())
	}))

	for i := 0; i < 3; i++ {
		if err := sc.PollOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	entry, _ := reg.Get("d1")
	if entry.Online {
		t.Error("expected offline after 3 failures")
	}
	found := false
	for _, e := range events {
		if e == "offline" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected offline event, got %v", events)
	}
}

func TestMcastCollectsReplies(t *testing.T) {
	// Set up a fake device on a loopback UDP socket.
	group := pickFreeUDPAddr(t)
	deviceInfo := protocol.DeviceInfo{
		DeviceID: "fake-device",
		Basic:    protocol.BasicInfo{Hostname: "fake"},
	}

	// Listener mimics a device that replies to HELLO.
	devConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer devConn.Close()
	devPort := devConn.LocalAddr().(*net.UDPAddr).Port
	devAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: devPort}

	go func() {
		buf := make([]byte, 64*1024)
		for {
			_ = devConn.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, src, err := devConn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			var hello protocol.HelloPacket
			if json.Unmarshal(buf[:n], &hello) != nil || hello.Type != "hello" {
				continue
			}
			reply := protocol.HelloReply{
				Type: "hello_reply", DeviceID: "fake-device", Info: deviceInfo,
			}
			b, _ := json.Marshal(reply)
			dst, _ := net.DialUDP("udp", nil, src)
			_, _ = dst.Write(b)
			dst.Close()
		}
	}()

	dir := t.TempDir()
	reg, _ := registry.Open(filepath.Join(dir, "devices.json"))
	var seenUnknown bool
	sc := scanner.New(reg,
		scanner.WithOnEvent(func(e scanner.Event) {
			if _, ok := e.(scanner.EventUnknownDeviceDiscovered); ok {
				seenUnknown = true
			}
		}),
	)

	// Drive a single mcast cycle. clientConn listens (rather than dials)
	// so we can WriteToUDP to devAddr and ReadFromUDP the reply.
	_ = group // reserved for future real-multicast parity
	clientConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer clientConn.Close()

	// Pretend the device lives at devAddr for the test (we'll send to
	// devAddr directly instead of broadcasting).
	hello := protocol.HelloPacket{Type: "hello", SenderID: "test", TS: "now"}
	b, _ := json.Marshal(hello)
	if _, err := clientConn.WriteToUDP(b, devAddr); err != nil {
		t.Fatal(err)
	}

	// Read reply at the client side (since the fake device replies to src).
	readBuf := make([]byte, 64*1024)
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := clientConn.ReadFromUDP(readBuf)
	if err != nil {
		t.Fatalf("client read: %v", err)
	}
	var reply protocol.HelloReply
	if err := json.Unmarshal(readBuf[:n], &reply); err != nil {
		t.Fatal(err)
	}
	// Manually trigger merge to simulate what mcast loop would do.
	// We can't easily test mcastLoop without OS multicast; instead
	// invoke the merge path with the captured info.
	sc.MergeForTest("mcast", "", 0, reply.Info)
	if !seenUnknown {
		t.Errorf("expected unknown-device event")
	}
}

func pickFreeUDPAddr(t *testing.T) string {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	addr := conn.LocalAddr().(*net.UDPAddr)
	conn.Close()
	return addr.String()
}

func TestSubnetScanFindsDevice(t *testing.T) {
	info := protocol.DeviceInfo{
		DeviceID: "scanned-device",
		Basic:    protocol.BasicInfo{Hostname: "scanme"},
	}
	// The scanner probes port 9999, so the server must listen there.
	ln, err := net.Listen("tcp", "127.0.0.1:9999")
	if err != nil {
		t.Skipf("port 9999 unavailable: %v", err)
	}
	srv := &httptest.Server{
		Listener: ln,
		Config: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/healthz":
				w.Write([]byte("ok"))
			case "/api/v1/info":
				_ = json.NewEncoder(w).Encode(info)
			default:
				http.NotFound(w, r)
			}
		})},
	}
	srv.Start()
	defer srv.Close()

	host := ln.Addr().(*net.TCPAddr).IP.String()

	dir := t.TempDir()
	reg, _ := registry.Open(filepath.Join(dir, "devices.json"))
	var unknownSeen bool
	sc := scanner.New(reg, scanner.WithOnEvent(func(e scanner.Event) {
		if _, ok := e.(scanner.EventUnknownDeviceDiscovered); ok {
			unknownSeen = true
		}
	}))

	// Scan /32 of the host's IP.
	cidr := host + "/32"
	if err := sc.ScanSubnet(context.Background(), cidr, 2*time.Second); err != nil {
		t.Fatalf("ScanSubnet: %v", err)
	}
	if !unknownSeen {
		t.Errorf("expected unknown-device event for scanned host")
	}
}

func TestSubnetScanRejectsLargeRange(t *testing.T) {
	dir := t.TempDir()
	reg, _ := registry.Open(filepath.Join(dir, "devices.json"))
	sc := scanner.New(reg)
	err := sc.ScanSubnet(context.Background(), "10.0.0.0/8", 1*time.Second)
	if err == nil {
		t.Error("expected error for range >4096 IPs")
	}
}
