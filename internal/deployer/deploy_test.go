package deployer

import "testing"

func TestParseDeviceID(t *testing.T) {
	cases := []struct {
		in   string
		want string
		err  bool
	}{
		{"DEVICE_ID=5f3a1c9b-1234-5678-9abc-def012345678\n", "5f3a1c9b-1234-5678-9abc-def012345678", false},
		{"prefix\nDEVICE_ID=abc\nsuffix\n", "abc", false},
		{"nothing here", "", true},
	}
	for _, c := range cases {
		got, err := parseDeviceID(c.in)
		if c.err {
			if err == nil {
				t.Errorf("input %q: expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("input %q: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("input %q: got %q want %q", c.in, got, c.want)
		}
	}
}