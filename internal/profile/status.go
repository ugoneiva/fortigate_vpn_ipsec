package profile

import (
	"regexp"
	"strings"
)

type ConnState string

const (
	StateDisconnected ConnState = "disconnected"
	StateConnecting   ConnState = "connecting"
	StateConnected    ConnState = "connected"
	// StateNoTunnel is an established IKE_SA (phase 1) with no INSTALLED
	// CHILD_SA: the gateway still answers DPD, the virtual IP is still
	// assigned, but no traffic is routed into the tunnel. Reporting this as
	// "connected" hides a dead VPN behind a healthy-looking status.
	StateNoTunnel ConnState = "no_tunnel"
)

type ConnectionStatus struct {
	State     ConnState `json:"state"`
	VirtualIP string    `json:"virtual_ip,omitempty"`
	Detail    string    `json:"detail,omitempty"` // raw IKE_SA header line, for troubleshooting
}

// strongSwan prints the virtual IP in brackets at the end of the IKE_SA's
// "local" line: `  local  'user' @ 192.168.1.118[4500] [10.212.134.200]`. The
// first bracket is the port; any bracketed token after it is a virtual IP.
// The "vips:" form is kept for older/other output layouts.
var (
	vipRE      = regexp.MustCompile(`vips?:\s*([0-9a-fA-F.:]+)`)
	localVipRE = regexp.MustCompile(`\]\s+\[([0-9a-fA-F.:]+)\]`)
)

// ParseListSAS scans `swanctl --list-sas` text output for the IKE_SA block
// belonging to connName. strongSwan prints one unindented
// "<conn>: #N, STATE, ..." header per SA, followed by indented detail
// lines (identities, proposal, virtual IP, child SAs) until the next
// unindented header or EOF.
func ParseListSAS(output, connName string) ConnectionStatus {
	prefix := connName + ":"
	status := ConnectionStatus{State: StateDisconnected}
	inBlock := false
	childInstalled := false

	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			inBlock = strings.HasPrefix(line, prefix)
			if inBlock {
				status.Detail = strings.TrimSpace(line)
				switch {
				case strings.Contains(line, "ESTABLISHED"):
					status.State = StateConnected
				case strings.Contains(line, "CONNECTING"):
					status.State = StateConnecting
				}
			}
			continue
		}
		if !inBlock {
			continue
		}
		trimmed := strings.TrimSpace(line)
		// The CHILD_SA appears as an indented "<name>: #N, reqid N,
		// INSTALLED, TUNNEL..." line inside the IKE_SA's block.
		if strings.Contains(trimmed, "INSTALLED") {
			childInstalled = true
		}
		if strings.HasPrefix(trimmed, "local ") && strings.Contains(trimmed, "@") {
			if m := localVipRE.FindStringSubmatch(trimmed); m != nil {
				status.VirtualIP = m[1]
			}
		} else if m := vipRE.FindStringSubmatch(line); m != nil && status.VirtualIP == "" {
			status.VirtualIP = m[1]
		}
	}

	if status.State == StateConnected && !childInstalled {
		status.State = StateNoTunnel
	}

	return status
}
