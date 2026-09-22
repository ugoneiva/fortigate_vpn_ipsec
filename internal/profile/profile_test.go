package profile

import (
	"strings"
	"testing"
)

func TestConnNameMatchesFortivpnSlugShape(t *testing.T) {
	p := Profile{ID: "920ebb75-e04e-4bde-ae1d-6d0ce722e1ec"}
	got := p.ConnName()
	want := "fortivpn-920ebb75"
	if got != want {
		t.Fatalf("ConnName() = %q, want %q", got, want)
	}
}

func TestNewIDProducesValidConnName(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := NewID()
		p := Profile{ID: id}
		name := p.ConnName()
		if len(name) != len("fortivpn-")+8 {
			t.Fatalf("ConnName() = %q has unexpected length for id %q", name, id)
		}
	}
}

func TestRenderConfIkev1Aggressive(t *testing.T) {
	p := Profile{
		ID:         "aaaaaaaa-0000-0000-0000-000000000000",
		Gateway:    "203.0.113.1",
		IkeVersion: IkeV1,
		Ikev1Mode:  Ikev1Aggressive,
		AuthMode:   AuthPskXauth,
		Proposals:  Proposals{IKE: []string{"aes256-sha256-modp2048"}, ESP: []string{"aes256-sha256"}},
	}
	creds := Credentials{PSK: "secretpsk", Username: "bob", Password: "hunter2"}

	p.LocalID = "notebook-bob"
	p.SplitTunnel = SplitTunnel{Kind: SplitTunnelCustom, CIDRs: []string{"10.0.1.0/24", "10.0.2.0/24"}}
	conf := RenderConf(p, creds)
	for _, want := range []string{
		"version = 1", "aggressive = yes", "auth = xauth", `xauth_id = "bob"`,
		"vips = 0.0.0.0",
		// One child per network: see
		// TestRenderConfIkev1SplitTunnelGetsOneChildPerNetwork.
		"remote_ts = 10.0.1.0/24",
		"remote_ts = 10.0.2.0/24",
		`id = "notebook-bob"`,
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("RenderConf missing %q\n---\n%s", want, conf)
		}
	}

	// With no Peer ID configured the remote block must not pin an identity:
	// FortiGate dialup answers aggressive mode with the group name, not its
	// address, and pinning the address fails phase 1 with INVALID_ID.
	if strings.Contains(conf, "id = 203.0.113.1") {
		t.Errorf("RenderConf pinned the gateway address as remote id\n---\n%s", conf)
	}

	p.RemoteID = "fortigate-hq"
	if conf := RenderConf(p, creds); !strings.Contains(conf, `id = "fortigate-hq"`) {
		t.Errorf("RenderConf ignored RemoteID\n---\n%s", conf)
	}
	p.RemoteID = ""

	// The split-tunnel networks are what lives behind the gateway: they must be
	// the remote selector. As local_ts the FortiGate rejects phase 2. And no
	// hard-coded updown path (it does not exist on Arch).
	for _, bad := range []string{"local_ts", "updown"} {
		if strings.Contains(conf, bad) {
			t.Errorf("RenderConf must not contain %q\n---\n%s", bad, conf)
		}
	}

	secrets := RenderSecrets(p, creds)
	for _, want := range []string{`secret = "secretpsk"`, `id = "bob"`, `secret = "hunter2"`} {
		if !strings.Contains(secrets, want) {
			t.Errorf("RenderSecrets missing %q\n---\n%s", want, secrets)
		}
	}
}

func TestRenderConfIkev2Eap(t *testing.T) {
	p := Profile{
		ID:         "bbbbbbbb-0000-0000-0000-000000000000",
		Gateway:    "203.0.113.1",
		IkeVersion: IkeV2,
		AuthMode:   AuthEapMschapv2,
		Proposals:  Proposals{IKE: []string{"aes256-sha256-modp2048"}, ESP: []string{"aes256-sha256"}},
	}
	creds := Credentials{PSK: "secretpsk", Username: "alice", Password: "hunter2"}

	conf := RenderConf(p, creds)
	for _, want := range []string{"version = 2", "auth = eap-mschapv2", `eap_id = "alice"`} {
		if !strings.Contains(conf, want) {
			t.Errorf("RenderConf missing %q\n---\n%s", want, conf)
		}
	}
	if strings.Contains(conf, "aggressive") {
		t.Errorf("RenderConf should not set aggressive for IKEv2\n---\n%s", conf)
	}
}

func TestDHGroupCatalogKeywordsUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, g := range DHGroups {
		if seen[g.Keyword] {
			t.Errorf("duplicate DH group keyword %q", g.Keyword)
		}
		seen[g.Keyword] = true
	}
	if _, ok := DHGroupByKeyword("modp2048"); !ok {
		t.Errorf("expected modp2048 (Group 14) to be in the catalog")
	}
}

func TestParseListSASConnected(t *testing.T) {
	out := `fortivpn-920ebb75: #1, ESTABLISHED, IKEv2, deadbeef_i* cafebabe_r
  local  'eap-mschapv2' @ 198.51.100.5
  remote '203.0.113.1' @ 203.0.113.1
  AES_CBC-256/HMAC_SHA2_256_128/PRF_HMAC_SHA2_256/MODP_2048
  established 12s ago, rekeying in 3400s
  local vips: 10.212.134.5
    fortivpn-920ebb75: #1, reqid 1, INSTALLED, TUNNEL, ESP:AES_CBC-256/HMAC_SHA2_256_128
      installed 12s ago, rekeying in 3000s, expires in 3600s
      in  c1234567,    0 bytes,     0 packets
      out c1234567,    0 bytes,     0 packets
      local  0.0.0.0/0
      remote 0.0.0.0/0
`
	got := ParseListSAS(out, "fortivpn-920ebb75")
	if got.State != StateConnected {
		t.Errorf("State = %q, want connected", got.State)
	}
	if got.VirtualIP != "10.212.134.5" {
		t.Errorf("VirtualIP = %q, want 10.212.134.5", got.VirtualIP)
	}
}

// Real strongSwan layout: the virtual IP is bracketed at the end of the
// IKE_SA's "local" line, after the [port].
func TestParseListSASVirtualIPOnLocalLine(t *testing.T) {
	out := `fortivpn-920ebb75: #3, ESTABLISHED, IKEv1, 7a1b2c3d4e5f6a7b_i* 1a2b3c4d5e6f7a8b_r
  local  '192.168.1.118' @ 192.168.1.118[4500] [10.212.134.200]
  remote '203.0.113.1' @ 203.0.113.1[4500]
  AES_CBC-256/HMAC_SHA2_256_128/PRF_HMAC_SHA2_256/MODP_2048
  established 20s ago, reauth in 13967s
  fortivpn-920ebb75: #1, reqid 1, INSTALLED, TUNNEL-in-UDP, ESP:AES_CBC-256/HMAC_SHA2_256_128/MODP_1536
    local  10.212.134.200/32
    remote 10.0.1.0/24
`
	got := ParseListSAS(out, "fortivpn-920ebb75")
	if got.State != StateConnected {
		t.Errorf("State = %q, want connected", got.State)
	}
	if got.VirtualIP != "10.212.134.200" {
		t.Errorf("VirtualIP = %q, want 10.212.134.200", got.VirtualIP)
	}
}

func TestParseListSASNoMatchIsDisconnected(t *testing.T) {
	got := ParseListSAS("", "fortivpn-920ebb75")
	if got.State != StateDisconnected {
		t.Errorf("State = %q, want disconnected", got.State)
	}
}

// An established IKE_SA with no INSTALLED CHILD_SA is a dead VPN: the
// gateway answers DPD and the virtual IP is still assigned, but nothing is
// routed. Reporting that as "connected" is what made a stuck tunnel look
// healthy while Disconnect could not clear it.
func TestParseListSASIkeUpWithoutChildIsNotConnected(t *testing.T) {
	const out = `fortivpn-1f8397d0: #6, ESTABLISHED, IKEv1, 748781dd39a331eb_i* 5e3c35f27f4b3313_r
  local  'dialup-group' @ 198.51.100.20[4500] [10.212.134.200]
  remote '203.0.113.7' @ 203.0.113.7[4500]
  AES_CBC-256/HMAC_SHA2_256_128/PRF_HMAC_SHA2_256/MODP_1536
`
	st := ParseListSAS(out, "fortivpn-1f8397d0")
	if st.State != StateNoTunnel {
		t.Errorf("State = %q, want %q", st.State, StateNoTunnel)
	}
	if st.VirtualIP != "10.212.134.200" {
		t.Errorf("VirtualIP = %q, want 10.212.134.200", st.VirtualIP)
	}
}

func TestParseListSASWithInstalledChildIsConnected(t *testing.T) {
	const out = `fortivpn-1f8397d0: #6, ESTABLISHED, IKEv1, 748781dd39a331eb_i* 5e3c35f27f4b3313_r
  local  'dialup-group' @ 198.51.100.20[4500] [10.212.134.200]
  remote '203.0.113.7' @ 203.0.113.7[4500]
  fortivpn-1f8397d0: #1, reqid 1, INSTALLED, TUNNEL-in-UDP, ESP:AES_CBC-256/HMAC_SHA2_256_128
    in  cd079ae8, 4096 bytes, 32 packets
    out ef815286, 8192 bytes, 64 packets
    local  10.212.134.200/32
    remote 10.20.0.0/22 10.30.0.0/24
`
	st := ParseListSAS(out, "fortivpn-1f8397d0")
	if st.State != StateConnected {
		t.Errorf("State = %q, want %q", st.State, StateConnected)
	}
}

// IKEv1 Quick Mode negotiates one traffic-selector pair per CHILD_SA: a
// multi-network split tunnel listed on a single child makes the gateway
// narrow to the first network and silently drop the rest, which is exactly
// how 10.30.0.0/24 went missing from a working-looking tunnel.
func TestRenderConfIkev1SplitTunnelGetsOneChildPerNetwork(t *testing.T) {
	p := Profile{
		ID:          "cccccccc-0000-0000-0000-000000000000",
		Gateway:     "203.0.113.1",
		IkeVersion:  IkeV1,
		Ikev1Mode:   Ikev1Aggressive,
		AuthMode:    AuthPskXauth,
		Proposals:   Proposals{IKE: []string{"aes256-sha256-modp1536"}, ESP: []string{"aes256-sha256-modp1536"}},
		SplitTunnel: SplitTunnel{Kind: SplitTunnelCustom, CIDRs: []string{"10.20.0.0/22", "10.30.0.0/24"}},
	}
	conn := p.ConnName()
	conf := RenderConf(p, Credentials{})

	for _, want := range []string{
		"      " + conn + " {\n        remote_ts = 10.20.0.0/22\n",
		"      " + conn + "-2 {\n        remote_ts = 10.30.0.0/24\n",
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("RenderConf missing child block %q\n---\n%s", want, conf)
		}
	}
	if strings.Contains(conf, "remote_ts = 10.20.0.0/22,10.30.0.0/24") {
		t.Errorf("IKEv1 must not put both networks on one child\n---\n%s", conf)
	}
}

// IKEv2 carries several selectors in one CHILD_SA, so it stays a single child.
func TestRenderConfIkev2SplitTunnelStaysOneChild(t *testing.T) {
	p := Profile{
		ID:          "dddddddd-0000-0000-0000-000000000000",
		Gateway:     "203.0.113.1",
		IkeVersion:  IkeV2,
		AuthMode:    AuthEapMschapv2,
		Proposals:   Proposals{IKE: []string{"aes256-sha256-modp2048"}, ESP: []string{"aes256-sha256"}},
		SplitTunnel: SplitTunnel{Kind: SplitTunnelCustom, CIDRs: []string{"10.20.0.0/22", "10.30.0.0/24"}},
	}
	conf := RenderConf(p, Credentials{})
	if !strings.Contains(conf, "remote_ts = 10.20.0.0/22,10.30.0.0/24") {
		t.Errorf("IKEv2 should keep both networks on one child\n---\n%s", conf)
	}
	if strings.Contains(conf, p.ConnName()+"-2 {") {
		t.Errorf("IKEv2 should not split into several children\n---\n%s", conf)
	}
}

func TestChildNameNumbersFromSecond(t *testing.T) {
	if got := ChildName("fortivpn-1f8397d0", 0); got != "fortivpn-1f8397d0" {
		t.Errorf("ChildName(0) = %q, want the connection name itself", got)
	}
	if got := ChildName("fortivpn-1f8397d0", 1); got != "fortivpn-1f8397d0-2" {
		t.Errorf("ChildName(1) = %q, want fortivpn-1f8397d0-2", got)
	}
}
