package agentd

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/spotter/spotter/internal/protocol"
)

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
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer conn.Close()
		a.udpReadLoop(ctx, conn)
	}()
	go func() {
		<-ctx.Done()
		wg.Wait()
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
		_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
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
