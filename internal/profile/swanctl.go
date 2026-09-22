package profile

import (
	"fmt"
	"strings"
)

// RenderConf produces a swanctl.conf `connections` block for
// /etc/swanctl/conf.d/<ConnName>.conf, shaped for FortiGate dialup VPN:
//
//   - `vips = 0.0.0.0`: the FortiGate hands the client its tunnel address via
//     mode-config; without asking for it the client has no inner source IP.
//   - the split-tunnel networks are the REMOTE traffic selector (what lives
//     behind the gateway). The local side is left as strongSwan's default,
//     `dynamic`, which becomes the assigned virtual IP.
//   - the gateway's IKE identity is its address, as FortiGate presents it.
//   - no `updown` script: charon installs the routes itself (kernel-netlink,
//     table 220), and a hard-coded script path breaks on distros that ship it
//     elsewhere (Arch: /usr/lib/strongswan/_updown, not /usr/lib/ipsec).
//
// XAuth as a client additionally needs strongSwan's `xauth-generic` plugin,
// which Arch's `strongswan` package does not build — see
// packaging/strongswan-xauth-generic.
func RenderConf(p Profile, creds Credentials) string {
	connName := p.ConnName()
	var b strings.Builder

	fmt.Fprintf(&b, "connections {\n")
	fmt.Fprintf(&b, "  %s {\n", connName)

	version := "2"
	if p.IkeVersion == IkeV1 {
		version = "1"
	}
	fmt.Fprintf(&b, "    version = %s\n", version)
	if p.IkeVersion == IkeV1 && p.Ikev1Mode == Ikev1Aggressive {
		fmt.Fprintf(&b, "    aggressive = yes\n")
	}
	fmt.Fprintf(&b, "    remote_addrs = %s\n", p.Gateway)
	fmt.Fprintf(&b, "    proposals = %s\n", strings.Join(p.Proposals.IKE, ","))
	if p.DPDDelay != "" {
		fmt.Fprintf(&b, "    dpd_delay = %s\n", p.DPDDelay)
	}
	fmt.Fprintf(&b, "    send_certreq = no\n")
	fmt.Fprintf(&b, "    vips = 0.0.0.0\n")

	writeAuthRounds(&b, p, creds)

	fmt.Fprintf(&b, "    children {\n")
	for i, ts := range childRemoteTS(p) {
		fmt.Fprintf(&b, "      %s {\n", ChildName(connName, i))
		fmt.Fprintf(&b, "        remote_ts = %s\n", ts)
		fmt.Fprintf(&b, "        esp_proposals = %s\n", strings.Join(p.Proposals.ESP, ","))
		fmt.Fprintf(&b, "        dpd_action = clear\n")
		fmt.Fprintf(&b, "      }\n")
	}
	fmt.Fprintf(&b, "    }\n")

	fmt.Fprintf(&b, "  }\n")
	fmt.Fprintf(&b, "}\n")
	return b.String()
}

func writeAuthRounds(b *strings.Builder, p Profile, creds Credentials) {
	localID := ""
	if p.LocalID != "" {
		localID = fmt.Sprintf("      id = %q\n", p.LocalID)
	}
	// No remote id unless one was configured: strongSwan then defaults to
	// %any. FortiGate dialup in aggressive mode identifies itself with the
	// phase-1 (group) name, not its address, so requiring the address here
	// aborts an otherwise good phase 1 with INVALID_ID. FortiClient leaves
	// its equivalent "Peer ID" field empty for the same reason.
	remoteID := ""
	if p.RemoteID != "" {
		remoteID = fmt.Sprintf("      id = %q\n", p.RemoteID)
	}
	remote := fmt.Sprintf("    remote {\n      auth = psk\n%s    }\n", remoteID)

	switch p.AuthMode {
	case AuthPsk:
		fmt.Fprintf(b, "    local {\n      auth = psk\n%s    }\n", localID)
		b.WriteString(remote)
	case AuthPskXauth:
		fmt.Fprintf(b, "    local1 {\n      auth = psk\n%s    }\n", localID)
		fmt.Fprintf(b, "    local2 {\n      auth = xauth\n      xauth_id = %q\n    }\n", creds.Username)
		b.WriteString(remote)
	case AuthEapMschapv2:
		fmt.Fprintf(b, "    local {\n      auth = eap-mschapv2\n      eap_id = %q\n    }\n", creds.Username)
		b.WriteString(remote)
	}
}

// remoteTrafficSelectors are the networks reached through the tunnel: the
// custom split-tunnel list, or everything.
func remoteTrafficSelectors(st SplitTunnel) []string {
	if st.Kind == SplitTunnelCustom && len(st.CIDRs) > 0 {
		return st.CIDRs
	}
	return []string{"0.0.0.0/0"}
}

// ChildName is the swanctl child-SA name for the i-th traffic selector.
// The first keeps the connection's own name so single-network profiles
// (and anything addressing them) are unaffected.
func ChildName(connName string, i int) string {
	if i == 0 {
		return connName
	}
	return fmt.Sprintf("%s-%d", connName, i+1)
}

// childRemoteTS returns the remote_ts value for each child SA to define.
//
// IKEv1 Quick Mode negotiates exactly ONE pair of traffic selectors per
// CHILD_SA, so several networks listed on a single child make the gateway
// narrow the proposal to the first one and silently drop the rest — the
// tunnel comes up looking healthy with only part of the split tunnel
// reachable. One child per network is the only way to get them all. IKEv2
// carries multiple selectors in one CHILD_SA, so there it stays a single
// child.
func childRemoteTS(p Profile) []string {
	ts := remoteTrafficSelectors(p.SplitTunnel)
	if p.IkeVersion == IkeV1 {
		return ts
	}
	return []string{strings.Join(ts, ",")}
}

// RenderSecrets produces the matching secrets.conf, written 0600 by the
// caller — never embedded in the world-readable conf.d entry.
func RenderSecrets(p Profile, creds Credentials) string {
	connName := p.ConnName()
	var b strings.Builder

	fmt.Fprintf(&b, "secrets {\n")
	fmt.Fprintf(&b, "  ike-%s {\n", connName)
	fmt.Fprintf(&b, "    id = %s\n", p.Gateway)
	fmt.Fprintf(&b, "    secret = %q\n", creds.PSK)
	fmt.Fprintf(&b, "  }\n")

	switch p.AuthMode {
	case AuthPskXauth:
		fmt.Fprintf(&b, "  xauth-%s {\n", connName)
		fmt.Fprintf(&b, "    id = %q\n", creds.Username)
		fmt.Fprintf(&b, "    secret = %q\n", creds.Password)
		fmt.Fprintf(&b, "  }\n")
	case AuthEapMschapv2:
		fmt.Fprintf(&b, "  eap-%s {\n", connName)
		fmt.Fprintf(&b, "    id = %q\n", creds.Username)
		fmt.Fprintf(&b, "    secret = %q\n", creds.Password)
		fmt.Fprintf(&b, "  }\n")
	}

	fmt.Fprintf(&b, "}\n")
	return b.String()
}
