# Contributing to Spotter

Thanks for your interest in making Spotter better. Spotter has two surfaces:
the Linux device-side agent (`spotterd`, Go) and the cross-platform desktop
client (`spotter-client`, Wails + React). They share the protocol package
but otherwise evolve on slightly different cadences.

## Ground rules

- All interactions are covered by our [Code of Conduct](CODE_OF_CONDUCT.md).
- By submitting a contribution you agree to license it under the project's
  [MIT License](LICENSE).
- Discuss non-trivial changes in an issue before opening a PR.
- Keep PRs focused — one fix or feature per PR. Squash noisy commits before merge.

## Development setup

| Tool         | Min version | Why                                          |
| ------------ | ----------- | -------------------------------------------- |
| Go           | 1.25        | `go.mod` declares `go 1.25.0`.               |
| Node.js      | 20 LTS      | Required by Wails / Vite.                    |
| npm          | 10+         | Frontend dependency management.              |
| Wails CLI    | v2.15+      | `go install github.com/wailsapp/wails/v2/cmd/wails@latest` |

The Makefile is the only sanctioned build entry point.

```bash
git clone https://github.com/spotter/spotter.git
cd spotter

# Run the full Go test suite with race detection.
make test

# Build the agent for the host OS/arch (handy for `go run`).
make agent

# Build the desktop client for the active GOOS.
make client
```

For UI iteration without rebuilding the binary, run the Vite dev server and
let Wails serve it (`wails dev`).

## Coding conventions

- **Go**: standard `gofmt` formatting, `go vet` clean, race detector clean.
  Run `golangci-lint run ./...` before pushing — see `.golangci.yml`.
- **Go tests**: keep tests next to the code (`foo_test.go`), use table-driven
  tests where natural, prefer real implementations over mocks for protocol
  packets, and never introduce timing-dependent assertions (use the existing
  injected `time.Sleep` / backoff helpers in `internal/scanner`).
- **TypeScript / React**: strict mode already on in `frontend/tsconfig.json`.
  Components live under `frontend/src/components/`; shared state under
  `frontend/src/state/`; i18n strings live in
  `frontend/src/i18n/dictionaries.ts` — never inline a user-facing string.
- **Shell / PowerShell scripts**: `set -euo pipefail` on bash, `$ErrorActionPreference = 'Stop'` on PowerShell; match the style of `scripts/deploy.sh` / `scripts/deploy.ps1`.

## Project layout

```
cmd/agent/                 spotterd entry point (Linux only)
internal/agentd/           HTTP + UDP loop
internal/collector/        OS-specific collectors (basic / jetson / network)
internal/protocol/         Wire format shared by agent & client
internal/registry/         Local device registry (JSON on disk)
internal/scanner/          Discovery merge pipeline (mcast + poll + subnet)
main.go + frontend/        spotter-client (Wails)
scripts/                   install / uninstall / deploy / build-all
docs/                      End-user documentation
docs/superpowers/          Internal design specs & plans
```

## Adding a new collector

Collectors live under `internal/collector/`. To add one:

1. Drop a new `<name>_<os>.go` file with a `Collect(context.Context) (X, error)` function.
2. Register it from `internal/collector/collector.go`'s `Collect(...)` shim with a
   platform build tag (or a runtime guard) so cross-compiles stay lean.
3. Extend `internal/protocol/info.go` if the new payload needs a wire field —
   bump `schemaVersion` in `internal/protocol/schema_version.go` and update
   the rolling decoder in `cmd/agent`.
4. Add a unit test under `internal/collector/<name>_<os>_test.go` that runs
   against a `t.TempDir()` fixture (see `basic_linux_test.go` for the pattern).

## Adding a UI component

1. Create `frontend/src/components/MyThing.tsx` plus a sibling `.module.css`
   if scoped styles are needed. Top-level `.css` only for global rules.
2. Reuse the `DeviceContext` for registry access; `useWailsEvents` for live
   scanner events; `useDeviceActions` for backend calls.
3. Add any new user-facing string to both languages in
   `frontend/src/i18n/dictionaries.ts` (look at the existing entries for the
   dictionary schema).

## Commit messages

Follow Conventional Commits:

```
feat(agent): add DHCP-discovered subnet hint to /api/v1/info
fix(client): handle empty registry list without crashing About dialog
docs(readme): note that spotterd requires systemd 240+
```

Scopes: `agent`, `client`, `frontend`, `protocol`, `scanner`, `docs`,
`ci`, `scripts`, `build`. Use `chore` for tooling changes.

## Submitting a pull request

1. Fork and create a feature branch off `master`.
2. Run `make test` and the relevant lint commands locally; both must pass.
3. Update `CHANGELOG.md` under the "Unreleased" section if the change is
   user-facing.
4. Open a PR using the [PR template](.github/PULL_REQUEST_TEMPLATE.md).
   Link any issue it closes (`Closes #123`).
5. Wait for CI to go green; reviewers may request changes — address them as
   follow-up commits on the same branch.

## Reporting bugs / requesting features

Use the issue templates in `.github/ISSUE_TEMPLATE/`. For security
vulnerabilities, follow [SECURITY.md](SECURITY.md) instead — please do not
file a public issue.
