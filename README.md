# Coremon

Self-hosted update monitoring and control for a fleet of Debian/Ubuntu
systems — VMs, LXC containers, bare metal, anything with systemd. A dark web
dashboard shows which systems have updates (security updates and pending
reboots highlighted); you pick packages and install them on demand. Nothing is
ever upgraded automatically.

```
 GitHub ── signed releases ──► Master (LXC) ◄──HTTPS (pinned)── coremon-agent ─┬─ updates module
 (CI builds + signs on tag)    web UI, mirror                    (each system)  └─ future modules
```

* **coremon-agent** (agent-core) — installed once per system. Connects *out*
  to the master; no open ports, no SSH. Runs modules for the master.
* **modules** — all functionality (starting with `updates`) ships as separate
  signed binaries deployed through the agent.
* **coremon-master** — web UI, job runner, and a local mirror of the signed
  GitHub releases. Agents only ever talk to the master.

Go standard library only, plain JS/CSS frontend, JSON storage — no
third-party dependencies to patch.

## How releases flow

1. You push a tag `vX.Y.Z` to GitHub.
2. The **release** workflow builds all binaries, signs them with the release
   key (a secret in a protected GitHub environment), and publishes a GitHub
   release with `coremon-X.Y.Z.tar.gz`, `install-master.sh` and `SHA256SUMS`.
3. The master checks GitHub every 6 hours (or on *Check GitHub*), downloads
   the new release, **verifies every signature** against the key pinned on the
   master, and adds it to its local repository (auto-import can be turned off).
4. You roll out: **Modules** page → update agent-core / modules per system or
   in bulk, and *Update master* for the master itself. Nothing is rolled out
   automatically.

## Security model

| Concern | How it is handled |
|---|---|
| Code authenticity | Every agent-core, module and master binary is ed25519-signed in CI. Signatures bind kind, name, version and platform. Agents and the master refuse anything else. |
| Signing key | Only in the `release` environment secret. Require your approval on that environment so no tag is signed without you. |
| Master's trust anchor | `/etc/coremon-master/signing.pub`, root-owned, written by the installer. The master service runs unprivileged and cannot change it. The installer refuses a different key unless `--force-key`. |
| Master self-update | The unprivileged master can only *request* an update; a root systemd unit verifies the signature against the root-owned key before replacing the binary. |
| Agent ↔ master | TLS with the master's public key pinned in each agent. Install script fetched with `curl --pinnedpubkey`, binary checked by sha256. |
| Agent identity | Single-use enrollment token → per-agent secret (only a hash is stored). New systems stay **pending** until approved. |
| Local opt-out | `--allow-modules` at agent install limits what may be deployed on that system. |
| What modules can do | Fixed actions only; package names validated; no arbitrary shell. |
| UI | PBKDF2 password, HttpOnly/Secure/SameSite=Strict session, anti-CSRF header, login rate limiting, strict CSP, audit log. |

Keep the master reachable only from your LAN/VPN.

## One-time GitHub setup

1. **Repository**: push this code to your GitHub repository (public).
2. **Signing key** — on your PC, with Go installed:
   ```bash
   go run ./cmd/coremon-sign keygen -key keys/signing.key -pub keys/signing.pub
   ```
   Store `keys/signing.key` safely offline (password manager / backup).
   `keys/` is git-ignored — never commit it.
3. **Environment**: *Settings → Environments → New environment* `release`.
   * *Required reviewers*: add yourself.
   * *Deployment branches and tags*: *Selected* → add tag rule `v*`.
   * *Environment secrets*: `COREMON_SIGNING_KEY` = contents of `keys/signing.key`.
4. **Tag protection** (recommended): *Settings → Rules → Rulesets* → protect
   tags `v*` so only you can create them.

## Releasing

```bash
git tag v0.2.0
git push origin v0.2.0
```

Approve the waiting **release** job under *Actions*. A minute later the
release is on GitHub; the master picks it up on its next check.

Module versions are independent: bump `modules/<name>/VERSION` when a module
changes. agent-core and master use the tag version.

## Install the master (dedicated LXC)

Create a Debian 12 / Ubuntu LXC (1 core, 256 MB RAM, 2 GB disk is plenty) with
the **nesting** feature enabled (default on new Proxmox CTs). As root:

```bash
apt-get install -y curl
curl -fsSL https://github.com/Dawnes/coremon/releases/latest/download/install-master.sh | sh
```

The script downloads the matching release, checks its sha256, pins the release
signing key, creates the unprivileged `coremon` user and systemd units, and
prints the UI address and initial password. Open `https://<master>:8443`,
accept the self-signed certificate, sign in, and change the password under
**Settings**. The GitHub repository is preconfigured from the installer.

The master needs outbound HTTPS to `api.github.com`, `github.com` and
`objects.githubusercontent.com`.

## Add systems

1. **Systems → Add a system → Generate install command.**
2. Run it as root on the target (needs `curl`). Do this on the master
   container too if you want it monitored.
3. **Approve** the pending system; name it, tag it, pick modules (`updates`
   is preselected).

Restrict modules on a particular system: append `--allow-modules updates`
(or `none`) to the install command.

## Day-to-day

* **Dashboard** — filter by updates / security / reboot / offline / tag;
  select systems for bulk *Check now* or *Upgrade all packages*.
* **System page → Updates** — select packages (or *Security only*) and
  *Install selected*, or *Upgrade all*. Output streams live.
* **Modules** — GitHub releases, master update, and per-system rollout of
  agent-core and modules.
* **Jobs / Audit log** — full history.

## Writing a new module

A module is an executable that answers:

```
<module> manifest          # JSON {name, version, title, description, actions[]}
<module> run <action>      # params JSON on stdin; JSON lines on stdout:
                           #   {"t":"log","d":"..."}  then  {"t":"result","ok":true,"data":{...}}
```

Use `internal/modkit` (see `modules/updates`). Add `modules/<name>/` with a
`VERSION` file; CI builds, signs and ships it automatically. Conventions: a
result with `"state"` is stored as the module's state for that system; a
`check` action is called on the automatic interval; mark system-changing
actions `"mutating": true`.

## Operations

| Task | Command |
|---|---|
| Master logs | `journalctl -u coremon-master -f` |
| Master update logs | `journalctl -u coremon-master-update` |
| Agent logs | `journalctl -u coremon-agent -f` |
| Upgrade master by hand | re-run the `install-master.sh` one-liner |
| Reset admin password | `sudo -u coremon coremon-master reset-password`, then `systemctl restart coremon-master` |
| Back up the master | `/var/lib/coremon-master` and `/etc/coremon-master` |
| Reverse proxy | add `-http 127.0.0.1:8080` to the master's `ExecStart` (loopback-only plain HTTP for a local proxy); agents keep using the pinned HTTPS port |

If the master's TLS key is lost, agents reject the new one by design and must
be reinstalled.

## Development

```bash
go test ./...
./build.sh                                   # local signed release in dist/ (keys/signing.key)
go build -tags fake ./modules/updates        # simulated apt; enable with COREMON_FAKE=1
```

Windows: `.\build.ps1 -Repo owner/name`.

Layout: `cmd/` binaries · `internal/agent` agent-core · `internal/master`
server + embedded `web/` · `internal/proto` wire types · `internal/sign` ·
`internal/modkit` module SDK · `modules/` · `deploy/` installer and units ·
`.github/workflows/` CI and release.
