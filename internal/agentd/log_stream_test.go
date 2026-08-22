package agentd

import "testing"

func TestParseLogTail(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"", 100},      // empty -> default
		{"0", 100},     // 0 -> default
		{"-5", 100},    // negative -> default
		{"abc", 100},   // non-numeric -> default
		{"50", 50},     // valid
		{"2000", 1000}, // clamp to max
		{"1", 1},       // boundary
		{"1000", 1000}, // boundary max
	}
	for _, c := range cases {
		got := parseLogTail(c.raw, 100)
		if got != c.want {
			t.Errorf("parseLogTail(%q) = %d, want %d", c.raw, got, c.want)
		}
	}
}
