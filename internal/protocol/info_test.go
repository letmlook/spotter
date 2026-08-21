package protocol_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/spotter/spotter/internal/protocol"
)

func TestDeviceInfoRoundTrip(t *testing.T) {
	original := protocol.DeviceInfo{
		SchemaVersion: protocol.SchemaVersion,
		DeviceID:      "5f3a1c9b-1234-5678-9abc-def012345678",
		CollectedAt:   time.Now().UTC().Format(time.RFC3339),
		AgentVersion:  "0.1.0",
		Basic: protocol.BasicInfo{
			Hostname: "jetson-01",
			Username: "nvidia",
			OS: protocol.OSInfo{
				PrettyName: "Ubuntu 22.04.4 LTS",
				ID:         "ubuntu",
				VersionID:  "22.04",
			},
			Kernel:        "5.15.122-tegra",
			Arch:          "aarch64",
			UptimeSeconds: 1234567,
		},
		Network: protocol.NetworkInfo{
			PrimaryIP: "10.0.5.23",
			Interfaces: []protocol.Interface{
				{Name: "eth0", MAC: "aa:bb:cc:dd:ee:ff", Addrs: []string{"10.0.5.23/24"}},
			},
		},
		Jetson: &protocol.JetsonInfo{
			Model:    "NVIDIA Jetson Orin Nano",
			Jetpack:  "5.1.3",
			L4T:      "35.5.0",
			CUDA:     "11.4",
			CUDNN:    "8.6",
			TensorRT: "8.5",
			Python:   "3.8.10",
			Serial:   "1420921088123",
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded protocol.DeviceInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.DeviceID != original.DeviceID {
		t.Errorf("DeviceID: got %q want %q", decoded.DeviceID, original.DeviceID)
	}
	if decoded.Jetson == nil {
		t.Fatal("Jetson should not be nil")
	}
	if decoded.Jetson.Model != original.Jetson.Model {
		t.Errorf("Jetson.Model: got %q want %q", decoded.Jetson.Model, original.Jetson.Model)
	}
}

func TestDeviceInfoJetsonNullable(t *testing.T) {
	info := protocol.DeviceInfo{
		SchemaVersion: protocol.SchemaVersion,
		DeviceID:      "no-jetson",
		Jetson:        nil,
	}
	data, _ := json.Marshal(info)

	if got := string(data); !contains(got, `"jetson":null`) {
		t.Errorf("expected jetson:null in JSON, got: %s", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}