package protocol

// DeviceInfo is the payload returned by GET /api/v1/info and embedded in
// HELLO-REPLY UDP packets. Field tags MUST match the wire contract in
// docs/superpowers/specs/2026-08-21-spotter-design.md §6.1.
type DeviceInfo struct {
	SchemaVersion  int             `json:"schema_version"`
	DeviceID       string          `json:"device_id"`
	CollectedAt    string          `json:"collected_at"`
	AgentVersion   string          `json:"agent_version"`
	Basic          BasicInfo       `json:"basic"`
	Network        NetworkInfo     `json:"network"`
	Jetson         *JetsonInfo     `json:"jetson"`               // nil means "not a Jetson" or probe failed
	Auth           *AuthInfo       `json:"auth,omitempty"`       // v2: present iff the agent has [auth] enabled
	LastHeartbeatAt LastHeartbeatAt `json:"last_heartbeat_at"`    // v3: timestamp of last heartbeat
}

type BasicInfo struct {
	Hostname      string `json:"hostname"`
	Username      string `json:"username"`
	OS            OSInfo `json:"os"`
	Kernel        string `json:"kernel"`
	Arch          string `json:"arch"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

type OSInfo struct {
	PrettyName string `json:"pretty_name"`
	ID         string `json:"id"`
	VersionID  string `json:"version_id"`
}

type NetworkInfo struct {
	PrimaryIP  string      `json:"primary_ip"`
	Interfaces []Interface `json:"interfaces"`
}

type Interface struct {
	Name  string   `json:"name"`
	MAC   string   `json:"mac"`
	Addrs []string `json:"addrs"` // CIDR notation
}

type JetsonInfo struct {
	Model    string `json:"model"`
	Jetpack  string `json:"jetpack"`
	L4T      string `json:"l4t"`
	CUDA     string `json:"cuda"`
	CUDNN    string `json:"cudnn"`
	TensorRT string `json:"tensorrt"`
	Python   string `json:"python"`
	Serial   string `json:"serial"`
}

// AuthInfo is embedded in DeviceInfo when the agent has auth enabled.
// Old (v1) clients ignore the field entirely; newer clients surface
// "Token required" in the device row and prompt for the token in
// Settings.
type AuthInfo struct {
	Required bool `json:"required"` // when true, /api/v1/{reboot,shutdown,logs} demand a Bearer token
}

// LastHeartbeatAt is the timestamp of the most recent agent heartbeat
// (a log line in v0.1, /api/v1/heartbeat on demand in v0.3 — see
// agentd/agent.go). Clients show "心跳 Xs 前" based on this field;
// values older than 5 min trigger the "agent idle" warning state.
type LastHeartbeatAt string
