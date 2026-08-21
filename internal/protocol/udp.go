package protocol

// HelloPacket is sent by the client to the multicast group to discover
// devices on the same L2 network.
type HelloPacket struct {
	Type     string `json:"type"`      // always "hello"
	SenderID string `json:"sender_id"` // client UUID for logging/diagnostics
	TS       string `json:"ts"`        // RFC3339 timestamp
}

// HelloReply is the unicast response sent by a device to the Hello source.
type HelloReply struct {
	Type     string     `json:"type"` // always "hello_reply"
	DeviceID string     `json:"device_id"`
	Info     DeviceInfo `json:"info"`
}
