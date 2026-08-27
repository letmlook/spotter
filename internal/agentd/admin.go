// Package agentd's admin surface is a tiny read-only HTML
// view of the spotterd agent's own state. The endpoint
// /admin renders a single device's basics + metrics + audit
// log so operators can spot a misbehaving spotterd from any
// browser without needing the Wails client.
//
// Auth: when cfg.Auth.Token is non-empty the /admin pages
// require HTTP Basic auth with the token as the password.
// The user field is ignored. The same token gates /api/v1/*,
// so operators don't need a second secret.
//
// Why HTML and not JSON-only: the goal is "operator types
// `http://spotterd-host/admin` and sees something useful in
// 2 seconds". Plain HTML with no JS, no external fonts, no
// CDN dependencies. `curl http://spotterd-host/admin` and
// `lynx -dump` are first-class; SPA-grade UI lives in the
// spotter-client.
package agentd

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/spotter/spotter/internal/protocol"
)

//go:embed admin/templates/*
var adminFS embed.FS

type adminIndexData struct {
	Title       string
	Version     string
	GeneratedAt string
	Device      protocol.DeviceInfo
	Uptime      string
	LastSample  *adminSample
	HasMetrics  bool
	HasJetson   bool
	Audit       []adminAuditRow
	AuditError  string
}

type adminSample struct {
	At       string
	Ago      string
	CPU      string
	MemPct   string
	MemUsed  string
	Temp     string
	HaveCPU  bool
	HaveMem  bool
	HaveTemp bool
}

type adminAuditRow struct {
	At          string
	AtRFC3339   string
	Ago         string
	Action      string
	DryRun      bool
	RequestID   string
	RemoteAddr  string
	Result      string
	ResultClass string
}

// adminIndexTmpl is parsed once at init from the embedded
// FS. ParseFS creates one template per matched file using
// the file's base name (`index.html`); we expose that as
// `adminTmplName` for the ExecuteTemplate call. Using
// `template.New("name").ParseFS(...)` instead would create
// a second empty template with the given name, which then
// "wins" ExecuteTemplate lookup and reports itself as
// incomplete — the bug we already fixed once.
var (
	adminIndexTmpl  = template.Must(template.New("").Funcs(adminFuncMap).ParseFS(adminFS, "admin/templates/index.html"))
	adminTmplName   = "index.html"
)

var adminFuncMap = template.FuncMap{
	"safeHTML": func(s string) template.HTML { return template.HTML(s) },
}

// handleAdminIndex serves GET /admin.
func (a *Agent) handleAdminIndex(w http.ResponseWriter, r *http.Request) {
	if !a.checkAdminAuth(w, r) {
		return
	}
	if r.URL.Path != "/admin" && r.URL.Path != "/admin/" {
		http.NotFound(w, r)
		return
	}
	a.refreshForAdmin()

	now := time.Now().UTC()
	info := a.info // copy

	var sample *adminSample
	if a.metrics != nil {
		if snap := a.metrics.Snapshot(); len(snap) > 0 {
			last := snap[len(snap)-1]
			s := &adminSample{
				At:  last.At.Format(time.RFC3339),
				Ago: humanizeAgo(now.Sub(last.At)),
			}
			if last.CPUPercent != nil {
				s.CPU = fmt.Sprintf("%.1f%%", *last.CPUPercent)
				s.HaveCPU = true
			}
			if last.MemPercent != nil {
				s.MemPct = fmt.Sprintf("%.1f%%", *last.MemPercent)
				s.HaveMem = true
			}
			if last.MemUsedBytes != nil {
				s.MemUsed = humanizeBytes(*last.MemUsedBytes)
			}
			if last.TempCelsius != nil {
				s.Temp = fmt.Sprintf("%.1f°C", *last.TempCelsius)
				s.HaveTemp = true
			}
			sample = s
		}
	}

	audit, err := a.recentAudit(20)
	var auditRows []adminAuditRow
	var auditErr string
	if err != nil {
		auditErr = err.Error()
	}
	for _, e := range audit {
		auditRows = append(auditRows, adminAuditRow{
			At:          humanizeAgo(now.Sub(e.At)),
			AtRFC3339:   e.At.Format(time.RFC3339),
			Action:      e.Action,
			DryRun:      e.DryRun,
			RequestID:   e.RequestID,
			RemoteAddr:  e.RemoteAddr,
			Result:      e.Result,
			ResultClass: resultClass(e.Result),
		})
	}

	page := adminIndexData{
		Title:       "spotterd admin",
		Version:     a.cfg.AgentVersion,
		GeneratedAt: now.Format(time.RFC3339),
		Device:      info,
		Uptime:      humanizeUptime(info.Basic.UptimeSeconds),
		LastSample:  sample,
		HasMetrics:  sample != nil,
		HasJetson:   info.Jetson != nil,
		Audit:       auditRows,
		AuditError:  auditErr,
	}
	renderAdmin(w, adminTmplName, adminIndexTmpl, page)
}

// handleAdminStatic serves /admin/static/<file>. The CSS lives
// inside the embedded FS so the binary is self-contained.
func (a *Agent) handleAdminStatic(w http.ResponseWriter, r *http.Request) {
	if !a.checkAdminAuth(w, r) {
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/admin/static/")
	full := path.Join("admin/templates", p)
	data, err := adminFS.ReadFile(full)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(p, ".css") {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	} else if strings.HasSuffix(p, ".html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

// handleAdminMetricsJSON serves the rolling history as JSON
// for `curl ... | jq '.samples[-1].cpu_percent'`.
func (a *Agent) handleAdminMetricsJSON(w http.ResponseWriter, r *http.Request) {
	if !a.checkAdminAuth(w, r) {
		return
	}
	if a.metrics == nil {
		http.Error(w, "metrics not started", http.StatusServiceUnavailable)
		return
	}
	samples := a.metrics.Snapshot()
	if len(samples) > 60 {
		samples = samples[len(samples)-60:]
	}
	w.Header().Set("Content-Type", "application/json")
	type sampleJSON struct {
		At           string   `json:"at"`
		CPUPercent   *float64 `json:"cpu_percent,omitempty"`
		MemPercent   *float64 `json:"mem_percent,omitempty"`
		MemUsedBytes *uint64  `json:"mem_used_bytes,omitempty"`
		TempCelsius  *float64 `json:"temp_celsius,omitempty"`
	}
	out := make([]sampleJSON, len(samples))
	for i, s := range samples {
		out[i] = sampleJSON{
			At:           s.At.UTC().Format(time.RFC3339Nano),
			CPUPercent:   s.CPUPercent,
			MemPercent:   s.MemPercent,
			MemUsedBytes: s.MemUsedBytes,
			TempCelsius:  s.TempCelsius,
		}
	}
	payload := map[string]any{
		"interval_seconds": int(metricsInterval.Seconds()),
		"samples":          out,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

// recentAudit returns the most recent N audit rows. Returns
// (nil, nil) if the audit logger isn't initialised.
func (a *Agent) recentAudit(n int) ([]AuditEntry, error) {
	if a.audit == nil {
		return nil, nil
	}
	return a.audit.Recent(n)
}

// checkAdminAuth gates /admin/* on cfg.Auth.Token. When the
// token is empty, access is open (typical for home / lab
// deployments where the LAN is the trust boundary).
func (a *Agent) checkAdminAuth(w http.ResponseWriter, r *http.Request) bool {
	if a.cfg.Auth.Token == "" {
		return true
	}
	hdr := r.Header.Get("Authorization")
	const prefix = "Basic "
	if !strings.HasPrefix(hdr, prefix) {
		writeAdminAuthChallenge(w)
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(hdr, prefix))
	if err != nil {
		writeAdminAuthChallenge(w)
		return false
	}
	_, password, ok := strings.Cut(string(raw), ":")
	if !ok || password != a.cfg.Auth.Token {
		writeAdminAuthChallenge(w)
		return false
	}
	return true
}

func writeAdminAuthChallenge(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="spotterd admin"`)
	http.Error(w, "authentication required", http.StatusUnauthorized)
}

func renderAdmin(w http.ResponseWriter, name string, tmpl *template.Template, data any) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, "template render failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(buf.Bytes())
}

// refreshForAdmin runs a one-shot collect so the admin page
// reflects the latest snapshot on every visit, not only on
// the 5s poll. Failure is logged at debug; the page still
// renders the cached snapshot. No-op when collect isn't wired.
func (a *Agent) refreshForAdmin() {
	if a.collect == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	info, err := a.collect(ctx)
	if err != nil {
		a.logger.Debug("admin refresh", slog.String("err", err.Error()))
		return
	}
	// Preserve AgentVersion + Auth + DeviceID; copy the rest.
	// Collectors set CollectedAt/Basic/Network/Jetson/Metrics
	// but may leave the long-lived fields alone.
	info.AgentVersion = a.info.AgentVersion
	if info.DeviceID == "" {
		info.DeviceID = a.info.DeviceID
	}
	if info.Auth == nil {
		info.Auth = a.info.Auth
	}
	if info.SchemaVersion == 0 {
		info.SchemaVersion = a.info.SchemaVersion
	}
	a.info = info
}

func humanizeAgo(d time.Duration) string {
	if d < 0 {
		return "0s ago"
	}
	sec := int(d.Seconds())
	switch {
	case sec < 60:
		return fmt.Sprintf("%ds ago", sec)
	case sec < 3600:
		return fmt.Sprintf("%dm ago", sec/60)
	case sec < 86400:
		return fmt.Sprintf("%dh ago", sec/3600)
	default:
		return fmt.Sprintf("%dd ago", sec/86400)
	}
}

func humanizeUptime(sec int64) string {
	if sec <= 0 {
		return "—"
	}
	d := time.Duration(sec) * time.Second
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	default:
		return fmt.Sprintf("%dm", mins)
	}
}

func humanizeBytes(b uint64) string {
	const (
		KiB = 1 << 10
		MiB = 1 << 20
		GiB = 1 << 30
		TiB = 1 << 40
	)
	switch {
	case b >= TiB:
		return fmt.Sprintf("%.1f TiB", float64(b)/TiB)
	case b >= GiB:
		return fmt.Sprintf("%.1f GiB", float64(b)/GiB)
	case b >= MiB:
		return fmt.Sprintf("%.1f MiB", float64(b)/MiB)
	case b >= KiB:
		return fmt.Sprintf("%.1f KiB", float64(b)/KiB)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func resultClass(result string) string {
	switch result {
	case "scheduled", "would_execute", "delayed-executed":
		return "ok"
	case "error":
		return "error"
	case "cancelled":
		return "cancelled"
	case "running":
		return "running"
	}
	return ""
}
