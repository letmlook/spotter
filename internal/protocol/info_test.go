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

func TestSchemaVersion_IsTwo(t *testing.T) {
	if protocol.SchemaVersion != 2 {
		t.Fatalf("SchemaVersion = %d, want 2 (v0.3 bumped for AuthInfo)", protocol.SchemaVersion)
	}
}

func TestDeviceInfoAuth_OmitEmptyWhenDisabled(t *testing.T) {
	info := protocol.DeviceInfo{
		SchemaVersion: protocol.SchemaVersion,
		DeviceID:      "no-auth",
		// Auth left nil: must NOT appear in JSON.
	}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if contains(string(data), `"auth"`) {
		t.Errorf("expected auth field omitted when nil, got: %s", string(data))
	}
}

func TestDeviceInfoAuth_RoundTripWhenRequired(t *testing.T) {
	info := protocol.DeviceInfo{
		SchemaVersion: protocol.SchemaVersion,
		DeviceID:      "with-auth",
		Auth:          &protocol.AuthInfo{Required: true},
	}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	var got protocol.DeviceInfo
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Auth == nil || !got.Auth.Required {
		t.Fatalf("expected Auth.Required=true after roundtrip, got %+v", got.Auth)
	}
}

func TestDeviceInfoAuth_BackCompatOldClientIgnoresField(t *testing.T) {
	// Simulate a v1-only client reading a v2 wire payload. v1
	// schema only knows schema_version/device_id/basic/network/jetson;
	// the `auth` field must be ignored by Go's default decode.
	const v2payload = `{"schema_version":2,"device_id":"x","auth":{"required":true}}`
	var got protocol.DeviceInfo
	if err := json.Unmarshal([]byte(v2payload), &got); err != nil {
		t.Fatal(err)
	}
	if got.DeviceID != "x" {
		t.Errorf("DeviceID: %q", got.DeviceID)
	}
	if got.Auth == nil || !got.Auth.Required {
		t.Errorf("expected Auth.Required true; got %+v", got.Auth)
	}
}
