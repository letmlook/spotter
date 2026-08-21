//go:build linux

package collector

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spotter/spotter/internal/protocol"
)

// collectJetson runs on the host. Returns nil if no Jetson signals are
// found. The four steps are independent and additive — each step may
// fill any subset of fields. A partial JetsonInfo (e.g. only serial) is
// still returned (not nil) so clients know "this *is* a Jetson".
func collectJetson(ctx context.Context) *protocol.JetsonInfo {
	return collectJetsonFromRoot("/")
}

// collectJetsonFromRoot is the testable form: takes a filesystem root
// instead of hardcoded paths.
func collectJetsonFromRoot(root string) *protocol.JetsonInfo {
	info := &protocol.JetsonInfo{}
	found := false

	// Step 1: jetson_release -v (with hermetic PATH that prepends
	// root-scoped bin dirs so a root-scoped jetson_release wins over
	// the host's, but without polluting the process environment).
	if j, err := probeJetsonRelease(root); err == nil && j != nil {
		mergeJetson(info, *j)
		found = true
	}

	// Step 2: nv_tegra_release + device-tree model
	if l4t := readFile(root + "/etc/nv_tegra_release"); l4t != "" {
		info.L4T = parseL4T(l4t)
		found = true
	}
	if model := readFile(root + "/proc/device-tree/model"); model != "" {
		info.Model = model
		found = true
	}

	// Step 3: serial
	if serial := readFile(root + "/sys/firmware/devicetree/base/serial-number"); serial != "" {
		info.Serial = strings.TrimRight(serial, "\n")
		found = true
	}

	// Step 4: CUDA/cuDNN/TensorRT from root-scoped /usr/local/cuda
	if c := readFile(root + "/usr/local/cuda/version.json"); c != "" {
		// best-effort; just mark found=true if file exists
		found = true
		_ = c
	}

	if !found {
		return nil
	}
	return info
}

// probeJetsonRelease runs `jetson_release -v` with a hermetic PATH so
// that a root-scoped /usr/bin/jetson_release wins over the host's
// without mutating the agent's process environment.
func probeJetsonRelease(root string) (*protocol.JetsonInfo, error) {
	// PATH separator is ':' on Linux. We prepend root-scoped bin dirs
	// in front of the inherited PATH so the probe resolves a
	// root-scoped binary first.
	var extraParts []string
	if _, err := os.Stat(root + "/usr/bin"); err == nil {
		extraParts = append(extraParts, root+"/usr/bin")
	}
	if _, err := os.Stat(root + "/usr/local/bin"); err == nil {
		extraParts = append(extraParts, root+"/usr/local/bin")
	}
	cmd := exec.Command("jetson_release", "-v")
	if len(extraParts) > 0 {
		extraPath := strings.Join(extraParts, ":") + ":" + os.Getenv("PATH")
		cmd.Env = overrideEnvPath(os.Environ(), extraPath)
	} else {
		cmd.Env = os.Environ()
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	text := string(out)
	if text == "" {
		return nil, exec.ErrNotFound
	}
	j := &protocol.JetsonInfo{}
	for _, line := range strings.Split(text, "\n") {
		kv := strings.SplitN(line, ":", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.TrimSpace(kv[0])
		v := strings.TrimSpace(kv[1])
		switch k {
		case "Model":
			j.Model = v
		case "Jetpack":
			j.Jetpack = v
		case "L4T":
			j.L4T = v
		case "CUDA":
			j.CUDA = v
		case "cuDNN":
			j.CUDNN = v
		case "TensorRT":
			j.TensorRT = v
		case "Python":
			j.Python = v
		}
	}
	return j, nil
}

// overrideEnvPath returns a copy of env with PATH replaced by newPath.
// If env has no PATH entry, appends one.
func overrideEnvPath(env []string, newPath string) []string {
	out := make([]string, 0, len(env))
	replaced := false
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			out = append(out, "PATH="+newPath)
			replaced = true
			continue
		}
		out = append(out, e)
	}
	if !replaced {
		out = append(out, "PATH="+newPath)
	}
	return out
}

func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		// also try resolving symlinks (e.g. /proc/device-tree/model -> ../model)
		if resolved, err2 := filepath.EvalSymlinks(path); err2 == nil && resolved != path {
			if b, err = os.ReadFile(resolved); err != nil {
				return ""
			}
		} else {
			return ""
		}
	}
	return string(bytes.TrimRight(b, "\x00\n"))
}

// parseL4T parses the nv_tegra_release header line, e.g.
//
//	"# R35 (release), REVISION: 5.0, GCID: 35550185 ..."
func parseL4T(text string) string {
	// The version is encoded as "R<MAJOR> (release), REVISION: <MINOR>..."
	// e.g. "R35 (release), REVISION: 5.0" -> "35.5.0"
	re := regexp.MustCompile(`R(\d+)\s*\(release\),\s*REVISION:\s*([\d.]+)`)
	m := re.FindStringSubmatch(text)
	if len(m) != 3 {
		return ""
	}
	return m[1] + "." + m[2]
}

func mergeJetson(dst *protocol.JetsonInfo, src protocol.JetsonInfo) {
	if src.Model != "" {
		dst.Model = src.Model
	}
	if src.Jetpack != "" {
		dst.Jetpack = src.Jetpack
	}
	if src.L4T != "" {
		dst.L4T = src.L4T
	}
	if src.CUDA != "" {
		dst.CUDA = src.CUDA
	}
	if src.CUDNN != "" {
		dst.CUDNN = src.CUDNN
	}
	if src.TensorRT != "" {
		dst.TensorRT = src.TensorRT
	}
	if src.Python != "" {
		dst.Python = src.Python
	}
}
