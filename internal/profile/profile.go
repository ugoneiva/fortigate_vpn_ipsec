package profile

import (
	"crypto/rand"
	"fmt"
	"strings"
)

type IkeVersion string

const (
	IkeV1 IkeVersion = "v1"
	IkeV2 IkeVersion = "v2"
)

// Ikev1Mode only applies when IkeVersion == IkeV1: Main is the default,
// Aggressive is the mode most FortiGate dialup IPsec templates actually use
// (it trades one round trip and PSK-cracking resistance for compatibility
// with clients behind NAT / dynamic IPs).
type Ikev1Mode string

const (
	Ikev1Main       Ikev1Mode = "main"
	Ikev1Aggressive Ikev1Mode = "aggressive"
)

// AuthMode picks the local (client-side) authentication round(s). PSK alone
// authenticates the tunnel but not the individual user; XAuth (IKEv1) and
// EAP-MSCHAPv2 (IKEv2) add a username/password round on top of the PSK,
// which is how FortiGate dialup VPN normally does per-user login.
type AuthMode string

const (
	AuthPsk         AuthMode = "psk"          // IKEv1, PSK only
	AuthPskXauth    AuthMode = "psk_xauth"    // IKEv1, PSK + XAuth
	AuthEapMschapv2 AuthMode = "eap_mschapv2" // IKEv2, PSK (gateway) + EAP-MSCHAPv2 (user)
)

type SplitTunnelKind string

const (
	SplitTunnelFull   SplitTunnelKind = "full"   // route 0.0.0.0/0 through the tunnel
	SplitTunnelCustom SplitTunnelKind = "custom" // only the listed CIDRs
)

type SplitTunnel struct {
	Kind  SplitTunnelKind `toml:"kind"`
	CIDRs []string        `toml:"cidrs,omitempty"` // used when Kind == SplitTunnelCustom
}

// Proposals are raw swanctl proposal strings ("aes256-sha256-modp2048"),
// not a structured cipher/integrity/group triple — this is the format
// swanctl.conf actually consumes, and keeping it as-is means the GUI's
// cipher/integrity/DH-group picker just assembles strings rather than the
// data model needing to round-trip through a second representation.
type Proposals struct {
	IKE []string `toml:"ike"`
	ESP []string `toml:"esp"`
}

type Profile struct {
	ID      string `toml:"id" json:"id"`
	Name    string `toml:"name" json:"name"`
	Gateway string `toml:"gateway" json:"gateway"`
	// LocalID is the IKE identity this client presents ("Local ID" in
	// FortiClient). Optional: FortiGate dialup in aggressive mode often
	// selects the phase-1 by it; empty means strongSwan's default (our IP).
	LocalID string `toml:"local_id,omitempty" json:"local_id,omitempty"`
	// RemoteID is the identity the gateway is required to present ("Peer ID"
	// in FortiClient, where it is normally left blank). Empty means accept
	// whatever it sends, which is what FortiGate dialup needs: in aggressive
	// mode it answers with the phase-1/group name rather than its address,
	// so pinning this to the gateway address fails with INVALID_ID.
	RemoteID    string      `toml:"remote_id,omitempty" json:"remote_id,omitempty"`
	IkeVersion  IkeVersion  `toml:"ike_version" json:"ike_version"`
	Ikev1Mode   Ikev1Mode   `toml:"ikev1_mode,omitempty" json:"ikev1_mode,omitempty"`
	AuthMode    AuthMode    `toml:"auth_mode" json:"auth_mode"`
	Proposals   Proposals   `toml:"proposals" json:"proposals"`
	SplitTunnel SplitTunnel `toml:"split_tunnel" json:"split_tunnel"`
	DPDDelay    string      `toml:"dpd_delay" json:"dpd_delay"`
}

// ConnName is the deterministic swanctl connection name and conf.d file
// stem: "fortivpn-" + the first 8 hex chars of ID. The daemon treats
// connection names as root-owned file paths, so this shape
// (`fortivpn-<8 lowercase hex>`) must stay in sync with
// internal/daemon's validateConnName.
func (p Profile) ConnName() string {
	id := strings.ToLower(strings.ReplaceAll(p.ID, "-", ""))
	for len(id) < 8 {
		id += "0"
	}
	return "fortivpn-" + id[:8]
}

// Credentials never gets written to profiles.toml — it lives in the Secret
// Service via keyring, keyed by ConnName. Passwords/PSK reach the daemon
// only as part of an ApplyProfile call, never logged or stored in conf.d
// (which is why RenderConf/RenderSecrets are separate: conf.d is
// world-readable-ish config, secrets.conf is 0600).
type Credentials struct {
	PSK      string `json:"psk"`
	Username string `json:"username,omitempty"` // XAuth identity (v1) or EAP identity (v2)
	Password string `json:"password,omitempty"`
}

// NewID mints an ID that's shaped like a UUIDv4 (so it's unambiguous in
// profiles.toml and unlikely to collide), even though only its first 8 hex
// characters are load-bearing (they seed ConnName).
func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
