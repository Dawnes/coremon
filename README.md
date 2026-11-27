# Coremon

Self-hosted update monitoring for a fleet of Linux devices (Debian and Ubuntu
with apt; Rocky Linux, AlmaLinux and RHEL with dnf; SLES and openSUSE with
zypper; Alpine with apk): VMs, LXC containers or bare metal, with systemd or
OpenRC. A web dashboard shows which devices have
updates, security updates, pending reboots and known vulnerabilities in their
applications. You choose what to install and when; nothing is ever upgraded
automatically.

Coremon has two parts: the **master** (the website, one per environment) and
the **agent** (one per device). Agents connect out to the master, so no open
ports and no SSH are needed on your devices.

Devices without an agent, such as routers, switches and printers, can be added
as **agentless** devices: the master itself pings them and checks their services
(TCP ports such as SSH, HTTPS or SMTP) to show whether they are up.

## Features

* **Dashboard**: all devices at a glance, in your own groups, with filters.
  Every device has one state, the same for agent and agentless devices:
  **OK**, **Attention** (updates waiting, reboot required, a check failing but
  not yet down, a newer agent), **Problem** (a check or service down,
  security updates, a serious CVE, failed systemd services) or **Down**, with
  the reasons on its tile. The Overview counts the devices that are OK; the
  Devices list has a State column and filters for Problem, Attention and Down.
* **OS support**: a service on every agent device, down once its OS release no
  longer gets security support (or a set number of days before). Decided by the
  master from the support dates; it cannot be removed, but pausing it mutes it.
  For RHEL and SLES the end of normal support counts, not the paid extended
  support; openSUSE Tumbleweed is rolling and has no end date.
* **Updates**: check, install selected packages or upgrade all, with live output,
  with apt, dnf, zypper or apk. With dnf an update is a security update when a
  security advisory lists it; "reboot required" comes from
  `dnf needs-restarting` and a newer installed kernel. With zypper: when a
  needed security patch replaces the installed version; "Upgrade all" is `zypper update`, on
  Tumbleweed `zypper dup`; a warning shows when no package sources are enabled
  (such as an unregistered SLES). apk and Tumbleweed mark no security updates:
  an update counts as one when it fixes a known CVE of a tracked application on
  that device. The System tab shows the package manager.
* **Applications**: 43 common applications (Apache, ClamAV, Docker, Dovecot,
  HAProxy, MariaDB, Nginx, OpenSSL, PostgreSQL, Proxmox VE, Proxmox Backup
  Server, the SSH server and more) with version, status and ports,
  per device and as one list for the whole environment.
* **Vulnerabilities**: known CVEs of those applications, with severity, fixes
  and actively exploited ones highlighted. Proxmox VE and Proxmox Backup Server
  are checked against NVD, the others against the distribution's own data
  (Debian, Ubuntu, Rocky Linux, AlmaLinux, openSUSE Leap and Tumbleweed, SLES
  16 and Alpine; not yet RHEL and SLES 15, whose data is split per module).
  Rocky Linux and AlmaLinux rate a whole advisory (Critical, Important,
  Moderate, Low); none of those counts as minor. A CVE the distribution rates minor never counts as serious, even when it is
  actively exploited. An admin can accept a CVE for all devices, with a note:
  it stays listed but no longer counts, until the acceptance is undone or no
  device has the CVE any more.
* **System and network**: per device the OS, platform (such as "Proxmox VE
  9.0.11"), uptime and health, and a Network box with the main network card
  and its MAC address (a bridge, bond or VLAN is followed to the physical card),
  every IP address marked static, DHCP or SLAAC, the gateway and DNS servers.
  A Proxmox VE host gets its own Proxmox tab.
* **Agentless devices**: devices without an agent, checked by the master with a
  ping and per-service checks (heartbeat, retries, timeout): TCP (optionally
  waiting for a greeting, or in reverse mode), HTTPS (hostname, path, accepted
  status codes, valid certificate, certificate valid for at least N days) or a
  DNS query to the device's own DNS server (A, AAAA, MX, TXT, CNAME or NS, an
  optional expected value and a maximum response time), or **Certificate
  Validity**: the TLS certificate of a web address (name and optional port), on
  the device's address, with a valid chain, valid in time and not expiring within N days (default 10), checked
  once an hour; "Show" lists the full certificate, its chain and every valid
  name. Each check shows which step failed. With a 24-hour history and a graph of the
  ping's round-trip time. For a network
  the master cannot reach, an agent there can do the checks instead ("Checked by").
  The address is an IP address or a name, such as a website or a server known
  only by its name: the system that checks looks the name up before every
  check with its own DNS setup (IPv4 first, IPv6 only when the name has no
  IPv4 address and that system has IPv6 connectivity). A failed lookup fails
  the check at the step "Name lookup"; the device page shows the address the
  last check went to, and the device that has it when Coremon knows it.
* **Services of agent devices**: the same service checks (and optionally a ping)
  on a device with an agent, on its Services tab: checked from outside by the
  master or by another agent, on one of its own addresses or another one.
* **Disk, memory and load**: optional services of an agent device, measured by
  its own agent every minute: disk space per mount point (% used or GB free),
  memory in use and the 1-minute load per core, each down after a set number of
  readings in a row past its limit. Each one has a 24-hour graph on the
  device's System tab, and a D, M or C letter on its dashboard tile and on the
  Devices page (green, red, or grey when unknown).
* **Network dependencies**: each device can be **connected via** another device,
  such as a switch or the host of a VM. When that device is down or in
  maintenance, the devices behind it show as unreachable instead of down: the
  cause is shown once, and their checks keep running. A **virtual** device (only
  a name, a group and a Connected via) stands for something Coremon cannot
  check, such as an unmanaged switch: devices can be connected via it, it is
  drawn on the network map, shown on the dashboard and on the Devices page, and
  it passes on the state of the devices above it. Its own page lists the
  devices connected via it.
* **Upstream**: Settings → Upstream adds the internet connection as a special
  virtual device. The master pings three addresses every minute or at your own
  interval of 30 seconds to an hour (by default
  the public DNS servers 9.9.9.9, 86.54.11.100 and 1.1.1.1): online when all
  answer, partly online when some do, offline when none do. It has its own
  tile on the Overview and is the top of the network map. The device that is
  the default gateway of most agents is suggested as the router to connect via it.
* **Map**: a live drawing of every device, as the network (who is connected via
  whom, with the break where a device is down), as monitoring (the master or a
  remote probe) or both together, coloured green, red, unreachable or grey.
  With Upstream on, the network views start at a cloud in its state colour.
  Healthy lines carry a slow pulse toward the master, and the map zooms and
  pans (it opens fitted to the screen).
* **Notifications**: push messages with ntfy (ntfy.sh or your own server, with
  an optional access token or username and password) and Pushover (an
  application token and a user or group key, checked with Pushover when
  saved; optionally only to some devices), each switched on and off with its
  own switch and with or without a link. Right away when a device goes down and is back, a service
  fails and recovers, Upstream goes offline, or a new device waits for
  approval; once a day a summary of new security updates, serious CVEs,
  reboots, ending OS support and new releases. Each event has its own
  priority; quiet hours let only Urgent messages through; at most X messages
  in Y minutes per device; tapping a message opens the device when a public
  address is set. Messages are kept for up to 24 hours while the server
  cannot be reached, followed by a note that they were late. Activity →
  Notifications lists what was sent, held back or failed.
* **Maintenance**: pause all checks of a device for 5 minutes up to 4 hours, or
  until ended by hand; no automatic jobs run and no failures are shown meanwhile.
* **Releases**: update the master and the agents from the website.
* **Users**: administrators and read-only users, optional two-factor sign-in
  with an authenticator app (optionally not asked from trusted networks), and
  accounts can be locked.
* **Activity**: every job, the upcoming scheduled tasks, the requests remote agents
  have not answered yet (optionally also the check results remote probes just sent),
  and an audit log of who did what.

## Security

* Every master and agent release is signed; unsigned or changed binaries are refused.
* All traffic between master and agents is encrypted and signed.
* For normal use you never need to log in to the server or your devices:
  everything is done from the website.

## Install the master

Create a Debian or Ubuntu LXC (1 core, 256 MB RAM, 2 GB disk is plenty) with
the **nesting** feature enabled. As root:

```bash
apt-get install -y curl
curl -fsSL https://github.com/Dawnes/coremon/releases/latest/download/install-master.sh | sh
```

The script prints the website address and an initial password. Open
`https://<master>:8443`, accept the self-signed certificate, sign in as
`admin` and choose a new password. Running the same command again upgrades
the master and keeps its data.

The master needs outbound HTTPS to GitHub (releases), Debian and
endoflife.date (support dates) and OSV.dev, NVD, CISA and FIRST
(vulnerability data), and to your ntfy server or api.pushover.net when
those notifications are on. The master itself runs on Debian or Ubuntu; agents also
run on Rocky Linux, AlmaLinux, RHEL, SLES, openSUSE and Alpine (with OpenRC:
the agent then runs under supervise-daemon).

## Add a device

1. **Dashboard → Devices → Add device → Agent → Generate install command.**
2. Run the command as root on the device (it needs `curl`).
3. **Approve** the new device and pick its group: the Overview shows a bar with
   **Review** while a device is waiting, and the Devices page lists them at the top.

Groups are made, renamed and ordered with **Groups** on the Devices page. A
device's name, group and Connected via are changed with **More → Edit** on its
own page, where **More** also has Maintenance and Remove.

For a device without an agent choose **Agentless** instead and enter its name and its IP
address or DNS name. Settings → General → Ping sets how often it is pinged; its services are added
on the device's Services tab. **Checked by** chooses who checks it: the master, or
an agent (0.3.9 or later) on another system that can reach it. Only agents an admin
switched on with **Act as remote probe** (Remote probe section of the agent's device
page; off by default) are offered; it can be switched off again once no device is
checked by it. A device whose agent is offline shows as unknown, not down.

A device with an agent can have services too (its **Services** tab): they are
checked from outside like those of an agentless device, by the master or by
another agent, never by its own agent. Its own status still comes from its agent.
The exceptions are **Disk space**, **Memory** and **CPU load** (at the top of
Add service): its own agent measures them every minute and sends the readings
with its regular contact. The defaults follow common monitoring practice: disk
more than 90% used for 3 readings, memory more than 90% for 5, and load more than
1.5 per core for 5. While the agent is offline they show as unknown.

## Updating

**Settings → Releases** shows the available versions. Update the master with
one button, and the agents per device or per group.

## Forgot a password?

As root on the master:

```bash
coremon-reset                 # new password for admin
coremon-reset -user <name>    # for another user
```

The new password is shown once and must be changed at the next sign-in.

## Building from source

Not needed for normal use: the install command above uses the signed releases.

```bash
go test ./...
./build.sh agent 0.0.1     # signed agent build in dist/ (needs keys/signing.key)
./build.sh master 0.0.1    # signed master package in dist/
```

## License

Coremon is licensed under the [PolyForm Noncommercial License 1.0.0](LICENSE):
free for personal, hobby, educational and non-profit use. Commercial use
requires permission from Dawnes, the Developer ([github.com/Dawnes](https://github.com/Dawnes)).

Coremon includes the Go standard library and the fonts Poppins, Geist and
Geist Mono; see [THIRD-PARTY-NOTICES](THIRD-PARTY-NOTICES).

© 2026 Dawnes, the Developer
