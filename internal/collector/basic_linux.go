package collector

import (
	"bufio"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/spotter/spotter/internal/protocol"
)

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func collectBasic() protocol.BasicInfo {
	u, _ := user.Current()
	username := ""
	if u != nil {
		username = u.Username
	}
	return protocol.BasicInfo{
		Hostname:      readHostname(),
		Username:      username,
		OS:            readOSRelease("/etc"),
		Kernel:        readKernel(),
		Arch:          readArch(),
		UptimeSeconds: readUptime(),
	}
}

func readHostname() string {
	b, err := os.ReadFile("/proc/sys/kernel/hostname")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readKernel() string {
	// uname -r equivalent: /proc/version has "kernel version string"
	b, err := os.ReadFile("/proc/version")
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(b))
	if len(fields) >= 3 {
		return fields[2]
	}
	return ""
}

func readArch() string {
	b, err := os.ReadFile("/proc/sys/kernel/arch")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readUptime() int64 {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(b))
	if len(fields) < 1 {
		return 0
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return int64(secs)
}

// readOSRelease parses /etc/os-release. If `dir` is empty, uses /etc.
// Returns an empty struct on any failure.
func readOSRelease(dir string) protocol.OSInfo {
	if dir == "" {
		dir = "/etc"
	}
	path := dir + "/os-release"
	f, err := os.Open(path)
	if err != nil {
		// fallback: /etc/lsb-release
		if dir == "/etc" {
			f, err = os.Open("/etc/lsb-release")
			if err != nil {
				return protocol.OSInfo{}
			}
		} else {
			return protocol.OSInfo{}
		}
	}
	defer f.Close()

	out := protocol.OSInfo{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		k := line[:eq]
		v := strings.Trim(line[eq+1:], `"'`)
		switch k {
		case "PRETTY_NAME":
			out.PrettyName = v
		case "ID":
			out.ID = v
		case "VERSION_ID":
			out.VersionID = v
		}
	}
	return out
}
