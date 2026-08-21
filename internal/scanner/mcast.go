package scanner

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
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

func (s *Scanner) mcastOnce(ctx context.Context) {
	addr, err := net.ResolveUDPAddr("udp", s.opts.MulticastGroup)
	if err != nil {
		s.opts.Logger.Debug("resolve mcast", "err", err.Error())
		return
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		s.opts.Logger.Debug("dial mcast", "err", err.Error())
		return
	}
	defer conn.Close()

	hello := protocol.HelloPacket{
		Type:     "hello",
		SenderID: s.opts.ClientSenderID,
		TS:       time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(hello)
	if _, err := conn.Write(data); err != nil {
		return
	}

	// Collect replies on a separate listening socket.
	listenConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return
	}
	defer listenConn.Close()
	_ = listenConn.SetReadDeadline(time.Now().Add(1 * time.Second))

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 64*1024)
		for {
			n, src, err := listenConn.ReadFromUDP(buf)
			if err != nil {
				var ne net.Error
				if errors.As(err, &ne) && ne.Timeout() {
					return
				}
				return
			}
			var reply protocol.HelloReply
			if json.Unmarshal(buf[:n], &reply) != nil || reply.Type != "hello_reply" {
				continue
			}
			s.mergeInfo("mcast", src.IP.String(), 0, reply.Info)
		}
	}()
	wg.Wait()
}
