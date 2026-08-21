package agentd_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/spotter/spotter/internal/agentd"
	"github.com/spotter/spotter/internal/protocol"
)

// TestUDPHelloReply runs an Agent against a loopback UDP socket (not
// real multicast, to keep tests hermetic).
func TestUDPHelloReply(t *testing.T) {
	// Pick a free UDP port for "multicast" group.
	group := pickFreeUDPAddr(t)

	a, err := agentd.New(agentd.Config{
		DeviceID:       "test-uuid",
		ListenAddr:     "127.0.0.1:0",
		MulticastGroup: group,
		AgentVersion:   "0.1.0",
	}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	a.SetInfo(protocol.DeviceInfo{
		DeviceID: "test-uuid",
		Basic:    protocol.BasicInfo{Hostname: "loopback-host"},
	})

	// Start the UDP listener.
	udpCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := a.StartUDP(udpCtx); err != nil {
		t.Fatalf("StartUDP: %v", err)
	}

	// Send a HELLO to the group.
	conn, err := net.Dial("udp", group)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}

	hello := protocol.HelloPacket{Type: "hello", SenderID: "client-test", TS: "2026-08-21T10:00:00Z"}
	b, _ := json.Marshal(hello)
	if _, err := conn.Write(b); err != nil {
		t.Fatal(err)
	}
	// Capture source port and close conn so we can rebind to it for
	// receiving the agent's unicast reply.
	srcAddr := conn.LocalAddr().(*net.UDPAddr)
	conn.Close()

	// Read reply on a separate socket bound to the same source port.
	// We listen on a fresh UDP socket and read until we get hello_reply.
	readConn, err := net.ListenUDP("udp", srcAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer readConn.Close()
	if err := readConn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 64*1024)
	n, _, err := readConn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	reply := protocol.HelloReply{}
	if err := json.Unmarshal(buf[:n], &reply); err != nil {
		t.Fatalf("unmarshal reply: %v", err)
	}
	if reply.Type != "hello_reply" {
		t.Errorf("type: %q", reply.Type)
	}
	if reply.DeviceID != "test-uuid" {
		t.Errorf("device_id: %q", reply.DeviceID)
	}
	if reply.Info.Basic.Hostname != "loopback-host" {
		t.Errorf("info.basic.hostname: %q", reply.Info.Basic.Hostname)
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
