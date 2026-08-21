package protocol_test

import (
	"encoding/json"
	"testing"

	"github.com/spotter/spotter/internal/protocol"
)

func TestHelloPacketRoundTrip(t *testing.T) {
	original := protocol.HelloPacket{
		Type:     "hello",
		SenderID: "client-uuid-1234",
		TS:       "2026-08-21T10:00:00Z",
	}
	data, _ := json.Marshal(original)
	var got protocol.HelloPacket
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != original {
		t.Errorf("round-trip mismatch: got %+v want %+v", got, original)
	}
}

func TestHelloReplyIncludesInfo(t *testing.T) {
	reply := protocol.HelloReply{
		Type:     "hello_reply",
		DeviceID: "device-uuid-5678",
		Info: protocol.DeviceInfo{
			DeviceID:      "device-uuid-5678",
			SchemaVersion: protocol.SchemaVersion,
		},
	}
	data, _ := json.Marshal(reply)
	var got protocol.HelloReply
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.DeviceID != reply.DeviceID || got.Info.DeviceID != reply.Info.DeviceID {
		t.Errorf("reply mismatch: %+v", got)
	}
}
