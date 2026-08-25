package agentd

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"time"

	"github.com/spotter/spotter/internal/protocol"
	"github.com/spotter/spotter/internal/timefmt"

	"github.com/spotter/spotter/internal/looputil"
)

// helloInterval is the default cadence at which the agent proactively
// emits a HELLO packet on the multicast group. Short enough that the
// client's GUI sees online transitions within ~one interval even when
// HTTP polls are blocked (firewall, host down, etc.); long enough that
// the multicast traffic stays below noise levels on busy LANs.
const helloInterval = 5 * time.Second

// StartUDP begins listening on the configured multicast group. Returns
// once the listener is up; the read loop runs in the background until
// ctx is cancelled.
func (a *Agent) StartUDP(ctx context.Context) error {
	addr, err := net.ResolveUDPAddr("udp", a.cfg.MulticastGroup)
	if err != nil {
		return err
	}
	// If the address is a real multicast group, join it. For loopback
	// test addresses (127.0.0.1), skip join.
	conn, err := net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		// Fallback for non-multicast loopback addresses (tests).
		conn, err = net.ListenUDP("udp", addr)
		if err != nil {
			return err
		}
	}
	if err := conn.SetReadBuffer(64 * 1024); err != nil {
		a.logger.Warn("set read buffer", slog.String("err", err.Error()))
	}

	a.logger.Info("udp listening", slog.String("addr", a.cfg.MulticastGroup))
	go func() {
		defer conn.Close()
		a.udpReadLoop(ctx, conn)
	}()
	return nil
}

func (a *Agent) udpReadLoop(ctx context.Context, conn *net.UDPConn) {
	buf := make([]byte, 64*1024)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := conn.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
			a.logger.Debug("set read deadline", slog.String("err", err.Error()))
		}
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			a.logger.Debug("udp read", slog.String("err", err.Error()))
			continue
		}
		var hello protocol.HelloPacket
		if err := json.Unmarshal(buf[:n], &hello); err != nil {
			a.logger.Debug("udp decode hello", slog.String("err", err.Error()))
			continue
		}
		if hello.Type != "hello" {
			continue
		}
		a.replyToHELLO(src)
	}
}

func (a *Agent) replyToHELLO(src *net.UDPAddr) {
	reply := protocol.HelloReply{
		Type:     "hello_reply",
		DeviceID: a.cfg.DeviceID,
		Info:     a.Info(),
	}
	data, err := json.Marshal(reply)
	if err != nil {
		a.logger.Error("marshal reply", slog.String("err", err.Error()))
		return
	}
	conn, err := net.DialUDP("udp", nil, src)
	if err != nil {
		a.logger.Debug("dial src", slog.String("err", err.Error()))
		return
	}
	defer conn.Close()
	if _, err := conn.Write(data); err != nil {
		a.logger.Debug("write reply", slog.String("err", err.Error()))
	}
}

// helloEmitInterval returns the configured emit cadence, falling back
// to the package default when zero.
func (a *Agent) helloEmitInterval() time.Duration {
	if a.cfg.HelloInterval > 0 {
		return a.cfg.HelloInterval
	}
	return helloInterval
}

// runHelloEmit proactively broadcasts a HELLO packet to the multicast
// group on a fixed cadence so the client can detect online transitions
// without waiting for its own slower poll cycle. Exits when ctx is
// cancelled. Errors are logged at Debug so a transient network blip
// doesn't spam the journal.
func (a *Agent) runHelloEmit(ctx context.Context) {
	addr, err := net.ResolveUDPAddr("udp", a.cfg.MulticastGroup)
	if err != nil {
		a.logger.Error("resolve mcast", slog.String("err", err.Error()))
		return
	}
	// Dial once and reuse the connection for every emit; the kernel
	// handles the rest. This is a write-only socket — we never read
	// from it (the listener in StartUDP owns that role).
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		a.logger.Error("dial mcast", slog.String("err", err.Error()))
		return
	}
	defer conn.Close()

	interval := a.helloEmitInterval()
	a.logger.Info("hello emit started",
		slog.String("group", a.cfg.MulticastGroup),
		slog.Duration("interval", interval),
	)
	// Emit once immediately so a freshly-started agent shows up in the
	// client before the first interval tick.
	a.emitHello(conn)

	looputil.RunTicker(ctx, interval, func() {
		a.emitHello(conn)
	})
}

// emitHello marshals and writes a single HELLO packet to conn. Called
// from runHelloEmit; factored out so the test surface stays small.
func (a *Agent) emitHello(conn *net.UDPConn) {
	pkt := protocol.HelloPacket{
		Type:     "hello",
		SenderID: a.cfg.DeviceID,
		TS:       timefmt.NowUTC(),
	}
	data, err := json.Marshal(pkt)
	if err != nil {
		a.logger.Debug("marshal hello", slog.String("err", err.Error()))
		return
	}
	if _, err := conn.Write(data); err != nil {
		a.logger.Debug("write hello", slog.String("err", err.Error()))
	}
}
