# Coremon

Self-hosted update monitoring and control for a fleet of Debian/Ubuntu
systems — VMs, LXC containers, bare metal, anything with systemd. A dark web
dashboard shows which systems have updates (security updates and pending
reboots highlighted); you pick packages and install them on demand. Nothing is
ever upgraded automatically.

```
 GitHub ── signed versions.json ──► Master (LXC) ◄──HTTPS (sealed)── coremon-agent
 (CI builds + signs on tag)         web UI, version store             (each system, all functions built in)
```

* **coremon-agent** — installed once per system by you. Connects *out* to the
  master; no open ports, no SSH. Every function (starting with package
  updates) is built into this one binary.
* **coremon-master** — web UI, job runner and a local store of the signed
  agent versions. Agents only ever talk to the master.

Go standard library only, plain JS/CSS frontend, JSON storage — no
third-party dependencies to patch.

## Versions and releases

Master and agent are released and versioned separately:

| Track | Tag | Contains | Rolled out by |
|---|---|---|---|
| master | `master-vX.Y.Z` | server + website, installer | *Releases → Update master* (never downgrades) |
| agent | `agent-vX.Y.Z` | agent binaries | per system: version dropdown (up- or downgrade) |

Each release workflow builds, tests and signs the binaries, publishes a
GitHub release, and adds the release to the **version index**: one signed
file, `versions.json`, in the fixed GitHub release `index`. It lists the
newest master and the newest **3** agent versions.

The master downloads only that file (every 6 hours, or *Releases → Check
now*), verifies its signature, then downloads and verifies every listed
binary. It keeps exactly the 3 listed agent versions and deletes older ones.
No GitHub API is used and nothing is rolled out automatically.

On the dashboard each system shows its agent version: **green** when it runs
the latest, **orange** otherwise.

## Security model

| Concern | How it is handled |
|---|---|
| Code authenticity | Every agent and master binary is ed25519-signed in CI; signatures bind kind, version and platform. The version index is signed too. Agents and the master refuse anything else. |
| Signing key | Only in the `release` environment secret on GitHub. Add a required reviewer to that environment to approve each release by hand. |
| Master's trust anchor | `/etc/coremon-master/signing.pub`, root-owned, written by the installer. The master service runs unprivileged and cannot change it. |
| Master self-update | The unprivileged master can only *request* an update; a root systemd unit verifies the signature against the root-owned key and refuses anything not newer. |
| Agent ↔ master | HTTPS only carries the traffic (self-signed; agents do not rely on it). Every message is encrypted end to end and signed with the master's **payload key** (ed25519 + X25519, AES-256-GCM), which agents trust in two key slots; replies are bound to their request (no replay). The install command carries the sha256 of the install script, which carries the payload key and the agent's checksum. |
| Agent identity | Single-use enrollment token; each agent then has its **own key pair** and signs every request — no shared secret on the wire. New systems stay **pending** until approved. Agents before 0.3.0 (pinned HTTPS + shared secret) are no longer supported: run a new install command on such a system. |
| What agents can do | Fixed actions only; package names validated (by the agent and the master); no arbitrary shell. |
| What agents send | Checked by the master as if hostile: text cleaned (no control, invisible or shell/markup characters), update results validated (at most 5,000 packages, 2 MB), times in the future set to now, one open heartbeat per agent, about 10 requests per second per agent. |
| Input | Names (systems 64, groups 48, notes 64 characters) may not contain control or invisible characters or ``< > " ' ` $ \ ; \| &`` — refused by the master, not only the website. Passwords: 5–40 characters when set. |
| Search engines | `X-Robots-Tag: noindex` on the website, install script and downloads, plus a robots meta tag. |
| UI | User accounts with roles (Administrator, Read-only) checked by the master on every request; PBKDF2 passwords (5–40 characters), HttpOnly/Secure/SameSite=Strict session, anti-CSRF header, login rate limiting, strict CSP, audit log per user. Read-only users cannot see enrollment tokens. |

Keep the master reachable only from your LAN/VPN.

## One-time GitHub setup

1. **Signing key** — on your PC, with Go installed:
   ```bash
   go run ./cmd/coremon-sign keygen -key keys/signing.key -pub keys/signing.pub
   ```
   Store `keys/signing.key` safely offline (password manager / backup).
   `keys/` is git-ignored — never commit it.
2. **Environment**: *Settings → Environments → New environment* `release`,
   with *Environment secret* `COREMON_SIGNING_KEY` = contents of
   `keys/signing.key`. Optional: *Required reviewers* (approve each release).
3. **Index release** — create it once (as a pre-release, so it never counts
   as "latest"):
   ```bash
   gh release create index --prerelease --title "Version index" --notes "Signed versions.json, maintained by the release workflows."
   ```
4. **Tag protection** (recommended): *Settings → Rules → Rulesets* → protect
   tags `master-v*`, `agent-v*` and `index` so only you can create them.

## Releasing

```bash
git tag agent-v0.2.1 && git push origin agent-v0.2.1     # agent
git tag master-v0.2.1 && git push origin master-v0.2.1   # master
```

A minute later the release is on GitHub and in the version index; masters
pick it up on their next check.

## Install the master (dedicated LXC)

Create a Debian 12/13 or Ubuntu LXC (1 core, 256 MB RAM, 2 GB disk is
plenty) with the **nesting** feature enabled (default on new Proxmox CTs).
As root:

```bash
apt-get install -y curl
curl -fsSL https://github.com/Dawnes/coremon/releases/latest/download/install-master.sh | sh
```

The script downloads the newest master release, checks its sha256, pins the
release signing key, creates the unprivileged `coremon` user and systemd
units, and prints the UI address and initial password. Open
`https://<master>:8443`, accept the self-signed certificate, sign in, and
change the password under **Settings**. Within a minute the master has
downloaded the agent versions (see **Releases**).

Re-running the same command upgrades an existing master and keeps its data.
The master needs outbound HTTPS to `github.com`, `salsa.debian.org` (OS support dates from Debian's distro-info-data, looked up every 3/14/30/90 days — Settings → General) and
`release-assets.githubusercontent.com` / `objects.githubusercontent.com`.

## Add systems

1. **Settings → Systems → Add a system → Generate install command.**
2. Run it as root on the target (needs `curl`). Do this on the master
   container too if you want it monitored.
3. **Approve** the pending system and pick its group. The first update check
   runs right away.

## Day-to-day

* **Dashboard → Overview** — systems in your groups and order (drag cards and group
  headers to arrange; fold groups; select a whole group). Cards show small badges for failed services, an expired (or soon expiring) OS and automatic updates. Filters for updates,
  security, reboot, offline, pending and outdated agents; bulk *Check now*,
  *Upgrade all packages* and *Agent → latest*. *Check now* checks every system, *Check selected* the selected ones.
* **Dashboard → List** — every system on one line (status, name, group, IP, OS, updates, reboot, agent, health, last check), grouped and sorted A–Z; click a column header to sort, again to reverse. View only.
* **System page** (`#/system/<id>`, for operating a system) — *Ping*, *Report only* and *Reboot*.
  *Ping* (every user, agents 0.3.1+) asks the agent to answer at once without
  doing anything: it shows the round trip, the agent version, which payload
  key the connection used, the agent's key slots and its clock difference —
  or, without an answer within 15 s, the last contact. Pings are listed under
  Jobs, not in the audit log.
  *Updates*: select packages (or *Security only*) and *Install selected*, or
  *Upgrade all*; output streams live. *Agent*: install any of the 3 kept agent
  versions. *System*: host facts — OS support status, failed services, automatic updates (unattended-upgrades), last upgrade (from apt's history: when, how many packages, started by whom) — and the IPv4 gateway (default route with the lowest metric) and the IPv4 DNS servers in use (the real servers when systemd-resolved is used).
* **Releases** — version index status, master update, agent versions and a
  per-system version table.
* **History** — *Jobs* (everything sent to agents, with output) and *Audit
  log* (who did what; filter by user and system; *Show output* opens the job
  an action started).
* **Report only** (per system): pure reporting — no update checks, installs
  or reboots until you *Allow changes* again. Enforced twice: the master
  refuses those jobs, and the agent (0.2.3 and later) stores the mode on the
  system and refuses them itself. Agent version changes and uninstall stay
  possible. Switching to report only cancels queued jobs.
* **Settings** — *General* (timers, retention, website port, install URL,
  GitHub repository), *Systems* (enrollment, approvals, rename, group and
  remove systems; create, rename, delete and reorder groups), *Users*
  (administrators: add users, set role, reset password, remove), *Login*
  (your own password) and *Identity* (payload key rotation and the key status of every system; key fingerprints are shown to administrators only). The *Master URL* used in install commands is automatic (the address your browser uses, with the agent port); administrators can *Change* it, e.g. for a DNS name agents use.
* **Users** — sign in with user name and password; the first user is
  `admin`. *Read-only* users see everything (except enrollment tokens) and
  change nothing but their own password. The last administrator cannot be
  removed or demoted.
* The dashboard's **Oldest information** tile shows the oldest data on
  screen: last contact of offline systems, last update check of online ones
  (report-only and pending systems excluded). When every system is online and checked within the check interval, it shows *All systems current*; an offline system is never current. Agent versions are changed only
  under **Releases**.

Entries, jobs and job output older than the retention period (1–30 days,
default 3) are removed by a cleanup that runs every full hour. After you
change the period, the first cleanup is the next full hour, or the hour after
that if it is less than 3 minutes away; Settings shows when and what will be
removed before you save.

**Lowest allowed agent version** (Settings → General): one of the agent
versions stored on the master (at least the oldest one; raised automatically
when that version is no longer stored). Older agents can still connect, but
only to be updated: the master stores and shows none of their data, and runs
nothing else on them (only agent update, uninstall and key-slot changes). Their
card shows *Update agent*. Agents before 0.3.0 cannot connect at all; they are
only listed under Settings → Systems, to be removed or reinstalled.

**Website port**: 8443 (default) or 443. With 443 the website is reachable
at `https://<master>` as well; agents always keep using port 8443.

### Timers

* **Heartbeat** (Settings, default 30 s): the longest time between two agent
  contacts. A system is shown offline after 3 heartbeats without contact.
  Commands such as *Check now* are delivered instantly regardless: the agent
  keeps a request open that the master answers as soon as there is work.
* **Check interval** (default 6 h): how often the master asks each agent to
  refresh its package lists and report updates.

## Adding a function to the agent

Functions live in `internal/feature/<name>` and implement
`feature.Feature` (`Info()` with the actions, `Call()` to run one). Register
it in `features()` in `internal/agent/agent.go`. A result with a `"state"`
key is stored by the master as that function's state for the system; a
`check` action is called on the automatic interval; mark system-changing
actions `Mutating`.

## Operations

| Task | Command |
|---|---|
| Master logs | `journalctl -u coremon-master -f` |
| Master update logs | `journalctl -u coremon-master-update` |
| Agent logs | `journalctl -u coremon-agent -f` |
| Upgrade master by hand | re-run the `install-master.sh` one-liner |
| Reset a password | `systemctl stop coremon-master && sudo -u coremon coremon-master reset-password -user admin && systemctl start coremon-master` (creates the user as administrator if it does not exist) |
| Back up the master | `/var/lib/coremon-master` and `/etc/coremon-master` |
| Reverse proxy | add `-http 127.0.0.1:8080` to the master's `ExecStart` (loopback-only plain HTTP for a local proxy); agents keep using the agent port |

**Payload key rotation** (Settings → Identity, agents 0.3.0+): *Generate new
key* — every agent receives it in a message signed with the current key and
stores it in its second key slot; the page shows both slots of every system.
*Switch to new key* — the master uses the new key and agents drop the old
one. If systems are missing the new key, the switch warns (default: don't
switch). *Cancel* discards the new key. The HTTPS certificate is separate and
not touched by this.

**HTTPS certificate** (Settings → Identity): self-signed, valid for 1 year
and renewed automatically 10 days before it expires; *Generate new
certificate* replaces it at once. Browsers ask once to accept the new one;
agents are not affected.

If the payload key is ever lost (`/var/lib/coremon-master/payload.key`),
agents reject the new one by design. Run a new install command on each
system: it updates the stored master address and keys (keeping the
registration, or registering again if the master no longer knows the system).

## Development

```bash
go test ./...
./build.sh agent 0.0.1                        # signed agent build in dist/ (keys/signing.key)
./build.sh master 0.0.1                       # signed master package in dist/
go build -tags fake ./cmd/coremon-agent       # simulated apt; enable with COREMON_FAKE=1
```

Layout: `cmd/` binaries · `internal/agent` agent · `internal/feature`
built-in functions · `internal/master` server + embedded `web/` ·
`internal/release` version index · `internal/proto` wire types ·
`internal/sign` · `deploy/` installer, units, index update ·
`.github/workflows/` CI and release.

## License

Coremon is licensed under the [PolyForm Noncommercial License 1.0.0](LICENSE):
free for personal, hobby, educational and non-profit use. Commercial use
requires permission from Dawnes, the Developer ([github.com/Dawnes](https://github.com/Dawnes)).

Coremon includes the Go standard library; see [THIRD-PARTY-NOTICES](THIRD-PARTY-NOTICES).

© 2026 Dawnes, the Developer
