package scanner

import (
	"github.com/spotter/spotter/internal/protocol"
	"github.com/spotter/spotter/internal/registry"
)

// mergeInfo applies incoming DeviceInfo to the registry entry. If
// device_id is unknown, emits unknown-device.
func (s *Scanner) mergeInfo(src string, ip string, port int, info protocol.DeviceInfo) {
	// Try to find by device_id first.
	if _, ok := s.reg.Get(info.DeviceID); ok {
		s.reg.Update(info.DeviceID, func(e *registry.Entry) {
			if ip != "" {
				e.IP = ip
			}
			if port > 0 {
				e.Port = port
			}
			e.LastSeenAt = timeNowUTC()
			e.LastSource = src
			e.Online = true
			e.LastInfo = &info
		})
		updated, _ := s.reg.Get(info.DeviceID)
		s.emit(EventInfoUpdated{Entry: updated})
		return
	}
	// Not in registry.
	s.emit(EventUnknownDeviceDiscovered{Info: info, IP: ip, Port: port})
}
