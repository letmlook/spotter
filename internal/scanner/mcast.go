package scanner

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"time"

	"github.com/spotter/spotter/internal/protocol"
)

func (s *Scanner) mcastLoop(ctx context.Context) {
	t := time.NewTicker(s.opts.McastInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.mcastOnce(ctx)
		}
	}
}

// mcastOnce sends a single HELLO and collects replies on one UDP
// socket. Windows disallows having a separate ListenUDP and DialUDP
// bound to the same LocalAddr in the same process, so we use a single
// ListenUDP and call WriteToUDP to send the HELLO and ReadFromUDP to
// receive replies on the same socket. The source address that devices
// see is this socket's LocalAddr, so their unicast replies (which
// they direct back at the HELLO source) arrive at our listener.
func (s *Scanner) mcastOnce(ctx context.Context) {
	addr, err := net.ResolveUDPAddr("udp", s.opts.MulticastGroup)
	if err != nil {
		s.opts.Logger.Debug("resolve mcast", "err", err.Error())
		return
	}

	// Bind to the same family as the dial target. For real multicast,
	// zero IP works (the kernel picks the right interface). For
	// unicast loopback tests, bind to the same IP so replies sent to
	// (our IP, our port) reach us (Windows rejects loopback packets
	// to 0.0.0.0 listener).
	listenIP := net.IPv4zero
	if !addr.IP.IsUnspecified() && !addr.IP.IsMulticast() {
		listenIP = addr.IP
	}
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: listenIP, Port: 0})
	if err != nil {
		s.opts.Logger.Debug("mcast listen", "err", err.Error())
		return
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
		s.opts.Logger.Debug("mcast set read deadline", "err", err.Error())
	}

	hello := protocol.HelloPacket{
		Type:     "hello",
		SenderID: s.opts.ClientSenderID,
		TS:       time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(hello)
	if err != nil {
		slog.Default().Debug("mcast marshal hello", "err", err.Error())
		return
	}
	if _, err := conn.WriteToUDP(data, addr); err != nil {
		s.opts.Logger.Debug("mcast write", "err", err.Error())
		return
	}

	buf := make([]byte, 64*1024)
	for {
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				return
			}
			s.opts.Logger.Debug("mcast read", "err", err.Error())
			return
		}
		var reply protocol.HelloReply
		if json.Unmarshal(buf[:n], &reply) != nil || reply.Type != "hello_reply" {
			continue
		}
		s.mergeInfo("mcast", src.IP.String(), s.opts.DevicePort, reply.Info)
	}
}
