//go:build linux

package collector

import (
	"bytes"
	"context"
	"encoding/json"
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
	// version.json. The schema is documented at NVIDIA's CUDA repo;
	// we parse just the version strings since those are what the GUI
	// surfaces.
	if c := readFile(root + "/usr/local/cuda/version.json"); c != "" {
		var v struct {
			CUDA     struct{ Version string `json:"version"` } `json:"cuda"`
			CUDNN    struct{ Version string `json:"version"` } `json:"cudnn"`
			TensorRT struct{ Version string `json:"version"` } `json:"tensorrt"`
		}
		if err := json.Unmarshal([]byte(c), &v); err == nil {
			if v.CUDA.Version != "" {
				info.CUDA = v.CUDA.Version
				found = true
			}
			if v.CUDNN.Version != "" {
				info.CUDNN = v.CUDNN.Version
				found = true
			}
			if v.TensorRT.Version != "" {
				info.TensorRT = v.TensorRT.Version
				found = true
			}
		}
	}

	if !found {
		return nil
	}
	return info
}

// probeJetsonRelease runs `jetson_release -v` with a hermetic PATH so
// that a root-scoped /usr/bin/jetson_release wins over the host's
// without mutating the agent's process environment.
//
// The output format changed in jetpack-stats 7.x from a flat list of
// "Key: Value" lines to grouped indented sections. Lines also contain
// ANSI SGR colour codes; we strip those before parsing so a colour-
// wrapped key still matches.
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
	text := stripANSI(string(out))
	if strings.TrimSpace(text) == "" {
		return nil, exec.ErrNotFound
	}
	return parseJetsonRelease(text), nil
}

// ansiSeq matches a single ANSI SGR escape sequence (e.g. "\x1b[1m",
// "\x1b[0m", "\x1b[92;101m").
var ansiSeq = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// stripANSI removes ANSI SGR escape sequences from s.
func stripANSI(s string) string { return ansiSeq.ReplaceAllString(s, "") }

// modelJetpackRe extracts an embedded Jetpack label from a Model line.
// Matches "Jetpack 7.2 GA", "Jetpack 5.1.3", "Jetpack 6.0 DP" etc.
var modelJetpackRe = regexp.MustCompile(`Jetpack\s+([A-Za-z0-9]+(?:\.[A-Za-z0-9]+)*(?:[ -](?:GA|EA|DP|RC|Production))?)`)

// modelL4TRe extracts an L4T version from "[L4T 39.2.0]" inline.
var modelL4TRe = regexp.MustCompile(`\[L4T\s+([0-9]+(?:\.[0-9]+){1,2})\]`)

// indentedItemRe matches " - Key: Value" lines (jetpack-stats 7.x style).
// Leading whitespace is tolerated.
var indentedItemRe = regexp.MustCompile(`^\s*-\s*([A-Za-z][A-Za-z0-9 _]*?)\s*:\s*(.+?)\s*$`)

// parseJetsonRelease parses the (ANSI-stripped) text of `jetson_release
// -v`. Supports both the legacy flat "Key: Value" layout (jetpack-stats
// <= 4.x) and the grouped indented layout (jetpack-stats 7.x). Always
// returns a non-nil JetsonInfo; callers decide what an empty result
// means in context.
func parseJetsonRelease(text string) *protocol.JetsonInfo {
	j := &protocol.JetsonInfo{}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "Model:"):
			// First-line layout: "Model: <name> - Jetpack X [L4T Y]"
			// Strip trailing annotations to keep the model name clean,
			// and harvest the embedded Jetpack / L4T as a fallback when
			// they aren't listed elsewhere in the report.
			name := strings.TrimSpace(strings.TrimPrefix(line, "Model:"))
			if i := strings.Index(name, " - "); i >= 0 {
				name = strings.TrimSpace(name[:i])
			}
			if name != "" {
				j.Model = name
			}
			if m := modelJetpackRe.FindStringSubmatch(line); len(m) >= 2 && j.Jetpack == "" {
				j.Jetpack = strings.TrimSpace(m[1])
			}
			if m := modelL4TRe.FindStringSubmatch(line); len(m) >= 2 && j.L4T == "" {
				j.L4T = strings.TrimSpace(m[1])
			}
		case strings.HasPrefix(line, "-"):
			// Indented " - Key: Value" line from jetpack-stats 7.x.
			// Indents may be tabs or spaces — the regex tolerates both.
			m := indentedItemRe.FindStringSubmatch(line)
			if len(m) != 3 {
				continue
			}
			assignJetsonField(j, strings.TrimSpace(m[1]), strings.TrimSpace(m[2]))
		default:
			// Legacy flat "Key: Value" line, e.g. "Model: ...", "L4T: ...".
			// Section headers like "Hardware:" / "Libraries:" have no
			// value (SplitN returns len=1) and naturally fall through.
			kv := strings.SplitN(line, ":", 2)
			if len(kv) != 2 {
				continue
			}
			assignJetsonField(j, strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1]))
		}
	}
	return j
}

// assignJetsonField writes v into the matching JetsonInfo field for key.
// Distinct-but-adjacent keys (e.g. "CUDA" vs "CUDA Arch BIN") are kept
// separate by exact match — only the canonical names below trigger a
// write.
func assignJetsonField(j *protocol.JetsonInfo, key, value string) {
	switch key {
	case "Model":
		j.Model = value
	case "Jetpack":
		j.Jetpack = value
	case "L4T":
		j.L4T = value
	case "CUDA":
		j.CUDA = value
	case "cuDNN":
		j.CUDNN = value
	case "TensorRT":
		j.TensorRT = value
	case "Python":
		j.Python = value
	}
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
