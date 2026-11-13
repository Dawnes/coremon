# Coremon

Self-hosted update monitoring for a fleet of Debian and Ubuntu systems: VMs,
LXC containers or bare metal. A web dashboard shows which systems have
updates, security updates, pending reboots and known vulnerabilities in their
applications. You choose what to install and when; nothing is ever upgraded
automatically.

Coremon has two parts: the **master** (the website, one per environment) and
the **agent** (one per system). Agents connect out to the master, so no open
ports and no SSH are needed on your systems.

## Features

* **Dashboard**: all systems at a glance, in your own groups, with filters.
* **Updates**: check, install selected packages or upgrade all, with live output.
* **Applications**: 23 common applications (Apache, Docker, MariaDB, Nginx,
  OpenSSL, PostgreSQL, the SSH server and more) with version, status and ports.
* **Vulnerabilities**: known CVEs of those applications, with severity, fixes
  and actively exploited ones highlighted.
* **Releases**: update the master and the agents from the website.
* **Users**: administrators and read-only users, optional two-factor sign-in
  with an authenticator app, and accounts can be locked.
* **History**: every job and an audit log of who did what.

## Security

* Every master and agent release is signed; unsigned or changed binaries are refused.
* All traffic between master and agents is encrypted and signed.
* For normal use you never need to log in to the server or your systems:
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

The master needs outbound HTTPS to GitHub (releases), Debian (support dates)
and OSV.dev, NVD, CISA and FIRST (vulnerability data).

## Add a system

1. **Settings → Systems → Add a system → Generate install command.**
2. Run the command as root on the system (it needs `curl`).
3. **Approve** the new system in the website and pick its group.

## Updating

**Settings → Releases** shows the available versions. Update the master with
one button, and the agents per system or per group.

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

Coremon includes the Go standard library; see [THIRD-PARTY-NOTICES](THIRD-PARTY-NOTICES).

© 2026 Dawnes, the Developer
