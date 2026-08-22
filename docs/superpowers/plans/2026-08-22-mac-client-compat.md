# Mac Client Compatibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable `spotter-client` GUI to be built and launched on macOS, producing a working `.app` bundle with no UI changes versus the existing Windows build.

**Architecture:** Single-file modification to `main.go` (add `Mac` + `Linux` fields to the existing `wails.Run` `options.App`), `Makefile` enhancement (detect `wails` CLI and fall back to `go build`), README addendum. No new files in the Go tree. No frontend changes. No build tags.

**Tech Stack:** Go 1.27, Wails v2.15, React 18 + Vite 5 + Ant Design 5 (frontend, unchanged), Make, Bash.

**Spec:** [`docs/superpowers/specs/2026-08-22-mac-client-compat.md`](../specs/2026-08-22-mac-client-compat.md)

---

## Global Constraints

- Wails v2.15 (already in `go.mod`); `mac.Options`/`linux.Options` fields as defined in Wails v2.15 — verify each field name matches v2.15 before committing
- All edits to `main.go` must compile on **macOS, Linux, and Windows** (no build tags; platform sub-packages are pure Go)
- `internal/**` and `frontend/**` are **unchanged**
- Each task ends with a clean `go build ./...` and `go vet ./...`
- No new unit tests (config-only changes; verification is end-to-end build)
- Commit style: imperative mood, ≤72 char subject; reference spec section when relevant

---

## File Structure

| File | Status | Responsibility |
|------|--------|----------------|
| `main.go` | modify | Add `Mac` and `Linux` platform options to the `wails.Run` options struct |
| `Makefile` | modify | Detect `wails` CLI; use it when present, fall back to `go build` with warning |
| `README.md` | modify | Add a "Build on macOS" subsection under existing "Build" |
| `docs/superpowers/plans/2026-08-22-mac-client-compat.md` | create | This plan |

No new Go source files. No new frontend files. No new test files.

---

## Task 1: Add Mac and Linux options to `main.go`

**Files:**
- Modify: `/Users/letmlook/spotter/main.go`

**Goal:** Make the `wails.Run` call cross-platform so `go build` succeeds on macOS and Linux without losing existing Windows behaviour.

**Why this is one task:** A single PR-worth diff; only testable as a unit via `go build` (no behavioural logic added).

### Steps

- [ ] **Step 1.1: Read current `main.go` imports section**

Confirm current import block includes `"github.com/wailsapp/wails/v2/pkg/options/windows"`. Expected: yes (line ~19).

- [ ] **Step 1.2: Add two new imports**

In the import block, immediately after the `windows` import line, add:

```go
"github.com/wailsapp/wails/v2/pkg/options/mac"
"github.com/wailsapp/wails/v2/pkg/options/linux"
```

- [ ] **Step 1.3: Replace the `wails.Run` call**

Replace the existing block (the call passing `&options.App{...}` into `wails.Run`) with the following. Keep every existing field exactly as it is; only add two new fields and the mac/linux option structs.

```go
app := NewApp(reg, logger)

err = wails.Run(&options.App{
    Title:  "Spotter",
    Width:  1200,
    Height: 800,
    AssetServer: &assetserver.Options{
        Assets: uiFS,
    },
    OnStartup: app.OnStartup,
    Windows: &windows.Options{
        WebviewIsTransparent: false,
        DisableWindowIcon:    true,
    },
    Mac: &mac.Options{
        TitleBar: &mac.TitleBar{
            TitlebarAppearsTransparent: true,
            HideTitle:                  true,
            HideTitleBar:               false,
            FullSizeContent:            false,
        },
        Appearance:           mac.NSAppearanceNameDarkAqua,
        WebviewIsTransparent: false,
    },
    Linux: &linux.Options{
        ProgramName: "Spotter",
    },
    Frameless: true,
    Bind: []interface{}{
        app,
    },
})
```

**Field-name verification before committing:** Run `go doc github.com/wailsapp/wails/v2/pkg/options/mac.Options` and `go doc github.com/wailsapp/wails/v2/pkg/options/mac.TitleBar` to confirm the field names exist in v2.15. If any field name has changed (e.g. `NSAppearanceNameDarkAqua` vs `AppearanceDark`), use whatever `go doc` returns.

- [ ] **Step 1.4: Build to verify it compiles**

Run: `go build ./...`
Expected: exit code 0, no output, no errors.

If `go build` reports an unknown field (e.g. `unknown field HideTitleBar`), consult `go doc` for the actual name and update the code. Common substitutes seen across Wails minor versions: `HideTitleBar` may be `HideTitlebar` or absent (omitting it is fine — it's optional). Re-run `go build` until it passes.

- [ ] **Step 1.5: Run `go vet`**

Run: `go vet ./...`
Expected: exit code 0, no output.

- [ ] **Step 1.6: Confirm `main.go` is gofmt-clean**

Run: `gofmt -l main.go`
Expected: no output (empty).

If non-empty, run `gofmt -w main.go` and re-verify.

- [ ] **Step 1.7: Commit**

```bash
git add main.go
git commit -m "feat(client): add Mac and Linux options to wails.Run

Cross-platform compilation: main.go now fills Mac (TitleBar hidden,
DarkAqua appearance) and Linux (ProgramName) options alongside the
existing Windows options block. No UI changes; spec §4.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 2: Update `Makefile` to prefer `wails build` when available

**Files:**
- Modify: `/Users/letmlook/spotter/Makefile`

**Goal:** On macOS (and Windows), `make client` should use `wails build` to produce a proper `.app` / `.exe` bundle when the Wails CLI is installed; otherwise fall back to `go build` with a clear warning.

**Why this is one task:** Single file, isolated logic; can be verified by toggling `PATH`.

### Steps

- [ ] **Step 2.1: Read current `Makefile` `client` target**

Expected content (lines 18-19):

```makefile
client:
	$(GO) build $(GOFLAGS) -o bin/spotter-client .
```

- [ ] **Step 2.2: Replace the `client` target**

Replace the two-line block above with:

```makefile
WAILS := $(shell command -v wails 2>/dev/null)

client:
ifneq ($(WAILS),)
	$(WAILS) build
else
	@echo "warning: wails CLI not found; falling back to 'go build' (will NOT produce a .app bundle on macOS)" >&2
	$(GO) build $(GOFLAGS) -o bin/spotter-client .
endif
```

**Critical:** Use `ifneq ($(WAILS),)` — **NOT** `ifdef WAILS`. The latter treats an empty string as "defined" and would expand to ` build` (error).

- [ ] **Step 2.3: Test the fallback path (no wails on PATH)**

Run:
```bash
PATH=/usr/bin:/bin make client
```
Expected:
- `go build` runs
- `bin/spotter-client` is created
- The "warning: wails CLI not found…" line is printed to stderr
- Exit code 0

Verify: `ls -la bin/spotter-client` exists.

- [ ] **Step 2.4: Test the wails path (if wails is already installed)**

Check whether wails is installed:
```bash
command -v wails || echo "wails not installed yet"
```

If installed, run:
```bash
make client
```
Expected: `wails build` runs and produces `build/bin/Spotter.app` (Mac) or `build/bin/spotter-client.exe` (Windows).

If `wails build` fails because frontend deps are not installed, run `cd frontend && npm install && cd ..` first, then retry. (Task 4 will repeat this in a controlled sequence.)

If wails is **not** installed yet, skip Step 2.4 — it will be exercised in Task 4 (which installs wails as part of the end-to-end run).

- [ ] **Step 2.5: Verify `make test`, `make agent`, `make agent-linux-arm64`, `make clean` are unaffected**

Run:
```bash
make test
make agent
make agent-linux-arm64
make clean
make build
```
Expected: all exit 0; behaviour identical to pre-change Makefile.

- [ ] **Step 2.6: Commit**

```bash
git add Makefile
git commit -m "feat(makefile): prefer wails build when CLI is available

make client now checks for the wails CLI and invokes 'wails build'
when present (producing a .app bundle on macOS or a packaged .exe on
Windows). Falls back to 'go build' with a stderr warning otherwise.

Use 'ifneq (\$(WAILS),)' rather than 'ifdef' — the latter treats an
empty string as defined. Spec §5.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 3: Add macOS build instructions to README

**Files:**
- Modify: `/Users/letmlook/spotter/README.md`

**Goal:** Document the macOS build path so other developers don't have to read the spec to figure it out.

**Why this is one task:** Documentation-only; no code coupling.

### Steps

- [ ] **Step 3.1: Locate the existing "Build" section in `README.md`**

The "Build" heading is around line 16. The existing section ends with:

```markdown
# Or, equivalently, use the Wails CLI directly:
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails build
```

- [ ] **Step 3.2: Append the macOS subsection**

Immediately after the `wails build` line above, append a blank line followed by:

```markdown
### Build on macOS

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
cd frontend && npm install && cd ..
make client          # produces build/bin/Spotter.app
open build/bin/Spotter.app
```

If `wails` is not on PATH, `make client` falls back to `go build` and
produces a bare `bin/spotter-client` Mach-O binary (no .app bundle).
The `.app` bundle is the recommended way to launch on macOS.
```

> Note: the inner triple-backtick `bash` block is literal text in the README. Verify the README renders correctly by viewing it in a Markdown preview.

- [ ] **Step 3.3: Commit**

```bash
git add README.md
git commit -m "docs: macOS build instructions for spotter-client

Adds a 'Build on macOS' subsection covering wails CLI install,
frontend deps, and the fallback warning. Spec §6.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 4: End-to-end build & launch verification on macOS

**Files:** none modified — this is a verification gate.

**Goal:** Confirm `make client` produces a launchable `.app` and the GUI starts cleanly with the custom TitleBar visible and no native-macOS title bar stacking.

**Why this is one task:** Spec §1.3 requires actual build + launch on a real Mac; this is the gate that proves the previous three tasks compose correctly.

### Steps

- [ ] **Step 4.1: Confirm macOS environment**

Run:
```bash
uname -srm
sw_vers
go version
```
Expected: `Darwin … arm64` (or x86_64); `ProductVersion: 14.x` or later; Go ≥ 1.25.

- [ ] **Step 4.2: Install Wails CLI (if not already)**

Run:
```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```
Expected: success within ~30s. Verify:
```bash
command -v wails
```
Should print the path (typically `~/go/bin/wails`).

If `~/go/bin` is not on `PATH`, add it for this session:
```bash
export PATH="$HOME/go/bin:$PATH"
```
(Persist in `~/.zshrc` if desired, but not required for this task.)

- [ ] **Step 4.3: Install frontend dependencies (if not already)**

Run:
```bash
cd frontend && npm install && cd ..
```
Expected: success; `frontend/node_modules/` exists.

- [ ] **Step 4.4: Run `make test`**

Run:
```bash
make test
```
Expected: all packages pass with `-race -count=1`. Exit code 0.

If any test fails, do **not** proceed. Fix the regression first (it would be unrelated to this spec but must be cleared before claiming task completion).

- [ ] **Step 4.5: Run `go vet ./...`**

Run:
```bash
go vet ./...
```
Expected: exit code 0, no output.

- [ ] **Step 4.6: Run `make client`**

Run:
```bash
rm -rf build/bin && make client
```
Expected: `wails build` runs; output mentions `Spotter.app`; exit code 0.

If `wails build` fails:
- Check stderr for missing XCode CLT: `xcode-select --install`
- Check Go version mismatch: `wails doctor` reports issues
- Check `frontend/dist` build errors (read wails output carefully)

- [ ] **Step 4.7: Verify the `.app` bundle structure**

Run:
```bash
ls -la build/bin/Spotter.app/Contents/MacOS/
```
Expected: contains an executable named `Spotter`.

Run:
```bash
file build/bin/Spotter.app/Contents/MacOS/Spotter
```
Expected: `Mach-O 64-bit executable arm64` (or x86_64).

- [ ] **Step 4.8: Launch the app**

Run:
```bash
open build/bin/Spotter.app
```
Expected: a window titled "Spotter" appears. The window should:
- Show a dark background
- Display the custom `Spotter` TitleBar with radar-style logo (left), drag region (middle), and minimize/maximize/close icons (right)
- **Not** show a macOS-native title bar stacked above the custom TitleBar (this is the `HideTitle: true` check)
- Display a sidebar with "Deploy", "Scan", "Add" buttons and a "Clear registry" button

If the app does not appear, run it directly to see stderr:
```bash
./build/bin/Spotter.app/Contents/MacOS/Spotter
```

- [ ] **Step 4.9: Click each action button to confirm no JS panic**

Click in order:
1. `Deploy` button — form appears (inline), no error
2. `Scan` button — form appears
3. `Add` button — form appears
4. `Clear registry` — Popconfirm appears

Expected: no JavaScript console errors, no GUI freeze. (The forms may not actually deploy anything because there are no real devices, but the UI must render and not crash.)

- [ ] **Step 4.10: Close the app and confirm the process exits**

Click the close button (top-right of custom TitleBar). Then run:
```bash
pgrep -lf Spotter
```
Expected: no process matches `Spotter` (the GUI process exited cleanly).

- [ ] **Step 4.11: Test the Makefile fallback path explicitly**

Run:
```bash
PATH=/usr/bin:/bin make client
```
Expected: stderr line `warning: wails CLI not found…` and `bin/spotter-client` is produced.

Verify:
```bash
ls -la bin/spotter-client
file bin/spotter-client
```
Expected: a Mach-O executable (may launch via `./bin/spotter-client &`, but won't have a Dock icon or `.app` integration).

- [ ] **Step 4.12: Final commit (verification log)**

If any small fixes were needed during the verification (e.g. README typo, Makefile formatting), commit them:

```bash
git status
git add -A
git diff --cached --stat
git commit -m "fix: minor adjustments from macOS build verification

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

(Skip this step if `git status` is clean.)

---

## Self-Review

### Spec coverage

| Spec section | Covered by |
|--------------|-----------|
| §1.1 Goals | Tasks 1-4 |
| §1.2 Out of scope | Tasks 1-4 (explicitly avoided) |
| §1.3 Acceptance criteria A-E | Task 4.4, 4.7, 4.8, 4.9, 4.10 |
| §3.2 File changes | Tasks 1, 2, 3 |
| §4 main.go details | Task 1 |
| §5 Makefile details | Task 2 |
| §6 README addendum | Task 3 |
| §7 Verification steps | Task 4 |

### Placeholder scan

No "TBD", "TODO", "implement later", or vague phrases. Every code block contains real code. Every command is concrete and runnable.

### Type / field-name consistency

The same `mac.TitleBar`, `mac.NSAppearanceNameDarkAqua`, `linux.ProgramName`, and `windows.{WebviewIsTransparent,DisableWindowIcon}` field names appear in both Task 1's code and the spec. The Makefile uses `WAILS` and `ifneq` consistently in Task 2.

### Open risks surfaced (not blockers)

- Wails field names verified at execution time via `go doc` in Task 1.3 — if Wails renamed a field between v2.15 minor versions, the implementer adjusts inline.
- The wails CLI install in Step 4.2 may require `~/go/bin` to be on PATH — explicitly handled.
- macOS may require XCode CLT (`xcode-select --install`) for the cgo pieces Wails uses — surfaced in Step 4.6 troubleshooting.
