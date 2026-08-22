# Security Policy

## Supported versions

Spotter is in pre-1.0 development. The following versions receive security
fixes:

| Version  | Supported           |
| -------- | ------------------- |
| `master` | ✅ Active development |
| `0.1.x`  | ✅ Best-effort         |
| `<0.1.0` | ❌ End of life         |

Pre-1.0 minor versions may break the wire protocol (`/api/v1/info`,
`/healthz`, UDP multicast packet) without notice — operators pinning a minor
version should expect to redeploy agents when upgrading the client across a
protocol-bump boundary.

## Reporting a vulnerability

**Please do not open a public GitHub issue for security bugs.** Use one of
these private channels instead:

- **Email**: spotter-security@example.com (replace with the real maintainer
  address before publishing) — GPG key to be added here once a key is
  chosen.
- **GitHub Security Advisories**: **Repository → Security → Advisories →
  "New draft security advisory"**. This is the preferred channel for code
  paths and reproduction steps because it keeps the report private until a
  fix is published.

We aim to acknowledge new reports within **3 business days** and to publish a
fix or mitigation within **30 days** for issues with a clear path to
remediation. We follow [coordinated disclosure][cd]: please give us a
reasonable window before going public.

[cd]: https://en.wikipedia.org/wiki/Coordinated_vulnerability_disclosure

## What to include

A useful report has:

1. The component affected (`spotterd` agent, `spotter-client` GUI, UDP
   multicast packet, `/api/v1/info` HTTP API, install/deploy scripts, etc.).
2. A concrete reproduction: device (`uname -a`, `cat /etc/os-release`),
   agent version (`spotterd -V` or `agent_version` field in the registry),
   client version (Help → About in the GUI), and exact commands run.
3. The expected vs. actual behavior, plus any logs (`journalctl -u
   spotterd`, `~/.config/Spotter/logs/spotter.log`).
4. Why it matters — what asset is at risk (the device itself, the LAN, the
   client host).

## Threat model and out-of-scope reports

Spotter is designed for **trusted LANs**. The following are out of scope and
will not be treated as security bugs:

- HTTP endpoints (`/api/v1/info`, `/healthz`) being reachable without
  authentication — by design. Run on a private network only.
- Discovery of devices by other LAN participants — by design.
- Lack of signing or integrity protection on the UDP multicast HELLO
  packet — tracked separately as a hardening item, not a CVE-worthy flaw.

If you are deploying Spotter on an untrusted network, **do not**. Put it
behind a VLAN or firewall until authentication is added.

## Hardening checklist for operators

Even within a trusted LAN, the following baseline reduces blast radius:

1. Restrict `spotterd`'s listen port (`listen_addr` in `/etc/spotterd/agent.toml`)
   to the management VLAN via `systemd`'s `IPAddressAllow=` + nftables.
2. Run `make release` outputs through a checksum check; the `SHA256SUMS` file
   in each `dist/` artefact is signed by the release workflow.
3. Subscribe to GitHub Releases (Watch → Custom → Releases) so advisory
   notes reach you in mail.
4. SSH keys only (no password mode) for `scripts/deploy.sh` — password mode
   lingers in shell history.
