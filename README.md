# FortiVPN Client

A native Linux desktop client for connecting to FortiGate IPsec dialup VPNs.
It does not implement IKE/IPsec itself — it wraps [strongSwan](https://strongswan.org)
(via `swanctl`) and adds FortiGate-shaped profile management, a GUI, secure
credential storage, and a system tray.

Supports:
- **IKEv1 Main mode** and **IKEv1 Aggressive mode** (PSK, optionally + XAuth)
- **IKEv2** (PSK + EAP-MSCHAPv2)
- The full strongSwan Diffie-Hellman group catalog (MODP 768–8192, the RFC
  5114 "s" variants, NIST and Brainpool ECP curves, Curve25519/Curve448),
  labeled with FortiGate's own numeric DH Group IDs in the picker

## Why this exists / architecture note

An earlier version of this client was written in Rust and had its GUI spawn
`pkexec` on every ~2-second status poll — every poll popped a polkit
password dialog, and the dialogs queued up faster than they could be
answered, effectively locking the user out of root/pkexec access. This
version fixes that architecturally rather than with retry logic:

- `fortivpn-daemon` is a root **D-Bus system service**, started on-demand by
  D-Bus activation (no password needed just to start it).
- `Status` is **not gated by polkit at all** — it's the method the GUI polls
  every ~2s, and it must be structurally incapable of producing a password
  prompt, no matter how often it's called.
- `ApplyProfile`/`Connect`/`Disconnect`/`RemoveProfile` each call
  `org.freedesktop.PolicyKit1.Authority.CheckAuthorization` themselves,
  gated by the `dev.fortivpn.helper.manage` polkit action
  (`auth_admin_keep`) — one prompt, then polkit's own ~5-minute cache covers
  further calls. No `pkexec` anywhere in this codebase.

This mirrors how NetworkManager and udisks2 do privilege separation on
Linux.

## Architecture

- `internal/profile` — profile/credentials model, DH group catalog,
  swanctl.conf/secrets rendering, `swanctl --list-sas` status parsing. No
  privileged I/O.
- `internal/daemon` — the privileged side: polkit `CheckAuthorization`
  client, `swanctl`/`systemctl` wrappers, the D-Bus method table.
- `internal/guiapp` — the unprivileged side: Fyne UI, tray icon (via Fyne's
  native `driver/desktop`, no separate systray dependency), D-Bus client,
  `go-keyring` credential storage.
- `internal/ipc` — the D-Bus bus name/path/interface/action constants shared
  by both binaries.
- `cmd/fortivpn-daemon`, `cmd/fortivpn-gui` — the two binaries.

Secrets (PSK, XAuth/EAP password) live in the freedesktop Secret Service via
`go-keyring` — never in `profiles.toml`, never in a D-Bus method's
introspectable signature (they're JSON payloads inside string arguments,
not logged).

## strongSwan prerequisite: IKEv1 (read this first)

FortiGate dialup VPN is **IKEv1** (usually Aggressive Mode + XAuth).
strongSwan 6.x builds `charon` **without IKEv1 by default** ("IKEv1 is now
disabled by default", strongSwan 6.0 NEWS), and Arch's/CachyOS's `strongswan`
package keeps it off. IKEv1 lives in charon's core, not in a plugin, so the
daemon must be rebuilt. On such a build every IKEv1 connection dies before
sending a packet, with `IKE version 1 not supported` in `journalctl -u strongswan`.
PSK + XAuth additionally needs the `xauth-generic` plugin (the client side of
XAuth), which those packages don't ship either.

On Arch, `packaging/strongswan-ikev1` builds the same strongSwan version with
Arch's configure flags plus `--enable-ikev1` (which also brings
`xauth-generic`). It `provides`/`conflicts` `strongswan`, so dependencies stay
satisfied and `pacman -Syu` won't swap it back:

```sh
sudo cp -a /etc/swanctl /etc/strongswan.conf /etc/strongswan.d /root/   # pacman replaces modified configs with .pacsave
cd packaging/strongswan-ikev1 && makepkg -si                          # confirm removing strongswan
# restore your /etc/swanctl/swanctl.conf etc. from /root if pacman left .pacsave files
sudo systemctl restart strongswan
```

`install.sh` and the daemon both check for this and say what to do instead
of failing silently.

## Build

Requires `strongswan` (with IKEv1 — see above), `polkit`, `dbus`, `gtk3`
(Fyne's GL/X11/Wayland backend), and a Go toolchain (`pacman -S go` on Arch).

```sh
go build ./...
go test ./...
```

## Install

**Arch/derivatives** (package-manager-tracked, supports clean `pacman -R` removal):

```sh
cd packaging
makepkg -si
```

**Any other systemd + D-Bus + polkit distro** (Debian/Ubuntu, Fedora, etc. —
checks for `go`/`gcc`/`pkg-config`/`swanctl`, builds, and installs with
`sudo install`):

```sh
./install.sh
```

Reverse with `./uninstall.sh` (matching `packaging/PKGBUILD`'s install
layout). Both routes install `fortivpn-gui` to `/usr/bin`, `fortivpn-daemon`
to `/usr/lib/fortivpn-client/` (D-Bus-activated only, not on PATH, never run
directly), the systemd unit, D-Bus service/policy files, the polkit action,
and a `.desktop` entry + icon.

## Manual dev run (without packaging)

```sh
# Terminal 1 — run the daemon manually as root (skips D-Bus activation):
sudo ./fortivpn-daemon

# Terminal 2:
./fortivpn-gui
```

For this to work without installing the D-Bus/polkit files first, the
daemon needs the `dev.fortivpn.Helper1` bus name policy and the
`dev.fortivpn.helper.manage` polkit action already installed — see
`packaging/dev.fortivpn.Helper1.conf` and
`packaging/dev.fortivpn.helper.policy`.

## Testing against a real FortiGate

There's no way to verify IKE/IPsec interop without a real FortiGate (or a
strongSwan responder configured to mimic one). Verified so far in
development: `go build ./...` / `go vet ./...` / `go test ./...` clean;
the D-Bus service activates on first call with zero polkit prompts for
repeated `Status` polling (confirmed via `busctl` over a 20s, 2s-interval
loop matching the GUI's actual poll rate); `Connect` triggers exactly one
polkit authorization and, once authorized, `swanctl` genuinely attempts an
IKE_SA_INIT exchange using the configured proposal (confirmed against a
non-routable test address — the DH group/cipher/integrity proposal was
accepted by strongSwan and used to build a real IKE packet, it just never
gets a response since nothing's listening there).

Still needed — a manual pass against a real FortiGate:

1. Launch the GUI, add a profile with the FortiGate's gateway address, IKE
   version/mode, PSK, and username/password (ask the FortiGate admin which
   mode they run — check the FortiGate's VPN > IPsec Wizard for "IKE
   Version" and whether XAuth or EAP is configured, and which DH groups the
   phase1/phase2 proposals allow).
2. Save — this triggers `ApplyProfile`, which should succeed even before
   connecting (it just loads the config into strongSwan; check
   `journalctl -u strongswan` if it doesn't).
3. Click Connect. Expect: status flips to "connected" within a few seconds,
   a virtual IP appears in the status line, and `ip a` shows a new tunnel
   interface with that address. **Also confirm**: leaving the GUI open and
   idle for several minutes produces no further password prompts (the
   regression test for the original bug), and the one Connect prompt only
   happens once even if you disconnect/reconnect within polkit's cache
   window.
4. Try reaching something on the far side of the tunnel (ping an internal
   IP).
5. Disconnect. Expect: status flips back, `ip a`/`ip route` no longer show
   the tunnel interface or its routes.
6. Try the tray icon's quick connect/disconnect and "Quit" actions.
7. The virtual IP is parsed from the bracket at the end of the IKE_SA's
   `local` line (`local '…' @ 192.168.1.118[4500] [10.212.134.200]`).

### Diagnosing a FortiGate that doesn't answer

Learned against a real FortiGate (2026-09-18):

- **Silence to every packet** usually means a phase-1 mismatch, not a
  network problem: a FortiGate drops IKEv1 requests that match no phase-1
  without replying. The fastest way to find its mode is to probe with
  throwaway `swanctl` connections (Main, Aggressive, IKEv2) that only send
  the first message — the variant it answers is the one it runs. In that
  test the gateway answered only Aggressive Mode and picked
  `aes128-sha256-modp2048`, which the profile didn't offer.
- **"IKE_SA … established" then XAuth asked three times and a DELETE** is
  the FortiGate rejecting the XAuth username/password (it re-prompts up to
  three times). Stop after that — repeated attempts can lock the account.
- The PSK is already validated at that point: the IKE_SA only establishes
  if the gateway's HASH checks out with the configured PSK.

## Known v1 limitations

- No certificate-based IKEv2 auth UI yet (only PSK + EAP-MSCHAPv2).
- Split tunneling is a raw CIDR-list text field, not a route editor.
- No FortiClient profile import.
- Status is polled every ~2s via `swanctl --list-sas` text parsing rather
  than streamed from the VICI socket — fine for a desktop client (and, per
  the architecture above, provably cheap to poll), just not instantaneous.
