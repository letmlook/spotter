package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/spotter/spotter/internal/protocol"
	"github.com/spotter/spotter/internal/registry"
)

// pollFailures tracks consecutive failures per device.
type pollFailures struct {
	mu        sync.Mutex
	counts    map[string]int
	threshold int
}

func newPollFailures(threshold int) *pollFailures {
	return &pollFailures{counts: map[string]int{}, threshold: threshold}
}

func (p *pollFailures) bump(deviceID string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.counts[deviceID]++
	return p.counts[deviceID]
}

func (p *pollFailures) reset(deviceID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.counts, deviceID)
}

// PollOnce performs one HTTP poll cycle against every registered device.
func (s *Scanner) PollOnce(ctx context.Context) error {
	entries := s.reg.List()
	if len(entries) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	for _, e := range entries {
		wg.Add(1)
		go func(e registry.Entry) {
			defer wg.Done()
			s.pollOne(ctx, e, s.failTrack)
		}(e)
	}
	wg.Wait()
	return nil
}

func (s *Scanner) pollOne(ctx context.Context, e registry.Entry, fails *pollFailures) {
	url := fmt.Sprintf("http://%s:%d/api/v1/info", e.IP, e.Port)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := s.opts.HTTPClient.Do(req)
	if err != nil {
		s.handlePollFailure(e, fails, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		// 4xx: incompatible — leave online state alone
		s.opts.Logger.Debug("poll 4xx", "device", e.DeviceID, "status", resp.StatusCode)
		return
	}
	if resp.StatusCode >= 500 {
		s.handlePollFailure(e, fails, fmt.Errorf("status %d", resp.StatusCode))
		return
	}
	var info protocol.DeviceInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		s.handlePollFailure(e, fails, err)
		return
	}
	fails.reset(e.DeviceID)
	s.mergeInfo("registry-poll", e.IP, e.Port, info)
}

func (s *Scanner) handlePollFailure(e registry.Entry, fails *pollFailures, cause error) {
	n := fails.bump(e.DeviceID)
	s.opts.Logger.Debug("poll failure",
		"device", e.DeviceID,
		"count", n,
		"err", cause.Error(),
	)
	if n >= fails.threshold {
		s.reg.Update(e.DeviceID, func(en *registry.Entry) { en.Online = false })
		s.emit(EventOffline{DeviceID: e.DeviceID})
	}
}

// pollLoop runs PollOnce every interval until ctx is done.
func (s *Scanner) pollLoop(ctx context.Context) {
	t := time.NewTicker(s.opts.PollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = s.PollOnce(ctx)
		}
	}
}
