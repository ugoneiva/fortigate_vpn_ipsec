// Package profile holds the VPN profile data model, the strongSwan
// swanctl.conf rendering logic, and status parsing — shared by the
// privileged daemon and the unprivileged GUI.
package profile

// DHGroup is one Diffie-Hellman / key-exchange group strongSwan accepts in
// proposal strings. Labeled with FortiGate's own numeric "DH Group" IDs
// (IANA IKEv2 transform registry) since that's what a FortiGate admin
// actually has in front of them in the FortiOS GUI — strongSwan itself has
// no numeric aliases, only the keyword.
type DHGroup struct {
	FortiGateGroup int
	Keyword        string
	Label          string
}

var DHGroups = []DHGroup{
	{1, "modp768", "Group 1 — MODP 768"},
	{2, "modp1024", "Group 2 — MODP 1024"},
	{5, "modp1536", "Group 5 — MODP 1536"},
	{14, "modp2048", "Group 14 — MODP 2048"},
	{15, "modp3072", "Group 15 — MODP 3072"},
	{16, "modp4096", "Group 16 — MODP 4096"},
	{17, "modp6144", "Group 17 — MODP 6144"},
	{18, "modp8192", "Group 18 — MODP 8192"},
	{19, "ecp256", "Group 19 — ECP 256"},
	{20, "ecp384", "Group 20 — ECP 384"},
	{21, "ecp521", "Group 21 — ECP 521"},
	{22, "modp1024s160", "Group 22 — MODP 1024/160"},
	{23, "modp2048s224", "Group 23 — MODP 2048/224"},
	{24, "modp2048s256", "Group 24 — MODP 2048/256"},
	{25, "ecp192", "Group 25 — ECP 192"},
	{26, "ecp224", "Group 26 — ECP 224"},
	{27, "ecp224bp", "Group 27 — Brainpool 224"},
	{28, "ecp256bp", "Group 28 — Brainpool 256"},
	{29, "ecp384bp", "Group 29 — Brainpool 384"},
	{30, "ecp512bp", "Group 30 — Brainpool 512"},
	{31, "curve25519", "Group 31 — Curve25519"},
	{32, "curve448", "Group 32 — Curve448"},
}

func DHGroupByKeyword(keyword string) (DHGroup, bool) {
	for _, g := range DHGroups {
		if g.Keyword == keyword {
			return g, true
		}
	}
	return DHGroup{}, false
}
