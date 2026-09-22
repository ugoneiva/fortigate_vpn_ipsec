// Package daemon implements the privileged side of fortivpn-client: writing
// swanctl.conf, driving swanctl/systemctl, and the D-Bus service that
// exposes all of that to the unprivileged GUI. Ported 1:1 in behavior from
// the earlier Rust fortivpn-helper, minus the pkexec-per-call transport that
// caused the original bug.
package daemon

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"fortivpn-client/internal/profile"
)

const (
	confD       = "/etc/swanctl/conf.d"
	swanctlConf = "/etc/swanctl/swanctl.conf"
)

// Unit name varies by distro: e.g. Arch masks `strongswan.service` in favor
// of `strongswan-starter.service`, while other distros ship the plain name.
var strongswanUnits = []string{"strongswan.service", "strongswan-starter.service"}

// connNameRE matches the deterministic slug profile.Profile.ConnName()
// produces. Connection names arrive over D-Bus from an unprivileged caller
// and become root-owned file paths, so anything not matching this shape is
// rejected outright.
var connNameRE = regexp.MustCompile(`^fortivpn-[0-9a-f]{8}$`)

func validateConnName(name string) error {
	if !connNameRE.MatchString(name) {
		return fmt.Errorf("invalid connection name: %q", name)
	}
	return nil
}

func confPath(connName string) string {
	return filepath.Join(confD, connName+".conf")
}

func secretsPath(connName string) string {
	return filepath.Join(confD, connName+"-secrets.conf")
}

func applyProfile(p profile.Profile, creds profile.Credentials) error {
	if err := validateConnName(p.ConnName()); err != nil {
		return err
	}
	if err := ensureConfDIncluded(); err != nil {
		return err
	}
	if err := os.MkdirAll(confD, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", confD, err)
	}

	if p.IkeVersion == profile.IkeV1 {
		if err := requireIkev1(p.AuthMode == profile.AuthPskXauth); err != nil {
			return err
		}
	}

	conf := profile.RenderConf(p, creds)
	if err := os.WriteFile(confPath(p.ConnName()), []byte(conf), 0o644); err != nil {
		return fmt.Errorf("writing connection conf: %w", err)
	}

	secrets := profile.RenderSecrets(p, creds)
	secPath := secretsPath(p.ConnName())
	if err := os.WriteFile(secPath, []byte(secrets), 0o600); err != nil {
		return fmt.Errorf("writing secrets conf: %w", err)
	}

	return loadAll()
}

func removeProfile(connName string) error {
	if err := validateConnName(connName); err != nil {
		return err
	}
	_ = os.Remove(confPath(connName))
	_ = os.Remove(secretsPath(connName))
	return loadAll()
}

func connect(connName string) error {
	if err := validateConnName(connName); err != nil {
		return err
	}
	if err := ensureStrongswanRunning(); err != nil {
		return err
	}
	if conf, err := os.ReadFile(confPath(connName)); err == nil && strings.Contains(string(conf), "version = 1") {
		if err := requireIkev1(strings.Contains(string(conf), "auth = xauth")); err != nil {
			return err
		}
	}
	// One --initiate per child config: an IKEv1 split tunnel is one CHILD_SA
	// per network, and swanctl brings up a single child at a time. The
	// failures are gathered rather than returned on the first one, so a
	// network the gateway refuses does not keep the others down.
	var errs []error
	for _, child := range childNamesFromConf(connName) {
		if err := runOk(exec.Command("swanctl", "--initiate", "-c", child)); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// childNameRE matches a child-SA block header in a conf.d entry this daemon
// wrote: six spaces of indent, inside the `children {` section.
var childNameRE = regexp.MustCompile(`(?m)^      (\S+) \{$`)

// childNamesFromConf lists the child SAs defined for a connection. Falling
// back to the connection's own name keeps older single-child conf.d entries
// (and anything this daemon did not write) working.
func childNamesFromConf(connName string) []string {
	data, err := os.ReadFile(confPath(connName))
	if err != nil {
		return []string{connName}
	}
	text := string(data)
	start := strings.Index(text, "    children {")
	if start < 0 {
		return []string{connName}
	}
	var names []string
	for _, m := range childNameRE.FindAllStringSubmatch(text[start:], -1) {
		names = append(names, m[1])
	}
	if len(names) == 0 {
		return []string{connName}
	}
	return names
}

// disconnect tears down the IKE_SA (-i), not just the CHILD_SA (-c).
// Terminating the IKE_SA takes its children with it, whereas -c leaves
// phase 1 up: once the CHILD_SA is gone on its own (rekey failure, a
// gateway-side DELETE), -c reports "no matching SAs to terminate found" and
// the connection can never be torn down from the GUI again, while Status
// still reports it as up. Having nothing left to terminate is the state
// Disconnect is asked to reach, so it counts as success.
func disconnect(connName string) error {
	if err := validateConnName(connName); err != nil {
		return err
	}
	err := runOk(exec.Command("swanctl", "--terminate", "-i", connName))
	if err != nil && strings.Contains(err.Error(), "no matching SAs to terminate") {
		return nil
	}
	return err
}

func status(connName string) (profile.ConnectionStatus, error) {
	if err := validateConnName(connName); err != nil {
		return profile.ConnectionStatus{}, err
	}
	out, err := exec.Command("swanctl", "--list-sas").Output()
	if err != nil {
		return profile.ConnectionStatus{}, fmt.Errorf("swanctl --list-sas: %w", err)
	}
	return profile.ParseListSAS(string(out), connName), nil
}

func ensureStrongswanRunning() error {
	for _, unit := range strongswanUnits {
		if exec.Command("systemctl", "is-active", "--quiet", unit).Run() == nil {
			return nil
		}
	}

	for _, unit := range strongswanUnits {
		out, _ := exec.Command("systemctl", "is-enabled", unit).Output()
		if strings.TrimSpace(string(out)) == "masked" {
			continue
		}
		if runOk(exec.Command("systemctl", "start", unit)) == nil {
			return nil
		}
	}

	return fmt.Errorf("could not start strongSwan (tried units: %v)", strongswanUnits)
}

// libcharon paths across distros; the first one that exists is inspected.
var libcharonPaths = []string{
	"/usr/lib/ipsec/libcharon.so.0",
	"/usr/lib/x86_64-linux-gnu/ipsec/libcharon.so.0",
	"/usr/lib64/ipsec/libcharon.so.0",
	"/usr/libexec/strongswan/libcharon.so.0",
}

// ikev1Marker is a log string that only exists in libcharon when it was built
// with IKEv1 support (it comes from the IKEv1 XAuth task).
const ikev1Marker = "XAuth authentication of"

// requireIkev1 fails early, with a message that says what to install, when an
// IKEv1 profile is used on a strongSwan that cannot do it. Two separate gaps,
// both found against a real FortiGate:
//
//   - strongSwan 6.x builds charon WITHOUT IKEv1 by default ("IKEv1 is now
//     disabled by default"), and Arch's strongswan package keeps it off. The
//     connection then dies before sending a packet ("IKE version 1 not
//     supported" in the charon log).
//   - PSK+XAuth also needs the xauth-generic plugin to answer the gateway's
//     XAuth request as a client; without it authentication stalls.
func requireIkev1(xauth bool) error {
	for _, path := range libcharonPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if !strings.Contains(string(data), ikev1Marker) {
			return fmt.Errorf("this strongSwan was built without IKEv1 support (strongSwan 6.x disables it by default), " +
				"so IKEv1 profiles cannot connect. Install a build with --enable-ikev1 " +
				"(Arch: packaging/strongswan-ikev1, `makepkg -si`) and restart strongswan")
		}
		break
	}

	if !xauth {
		return nil
	}
	if err := ensureStrongswanRunning(); err != nil {
		return err
	}
	out, err := exec.Command("swanctl", "--stats").Output()
	if err != nil || strings.Contains(string(out), "xauth-generic") {
		// Loaded, or we can't tell; let the attempt report whatever happens.
		return nil
	}
	return fmt.Errorf("strongSwan is missing the xauth-generic plugin, required for PSK + XAuth " +
		"(FortiGate IKEv1 dialup). Arch: packaging/strongswan-ikev1 includes it; " +
		"Debian/Ubuntu: libcharon-extra-plugins. Then restart strongswan")
}

func loadAll() error {
	if err := runOk(exec.Command("swanctl", "--load-all", "--noprompt")); err != nil {
		return fmt.Errorf("swanctl --load-all failed: %w", err)
	}
	return nil
}

func ensureConfDIncluded() error {
	existing, err := os.ReadFile(swanctlConf)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", swanctlConf, err)
	}
	if strings.Contains(string(existing), "conf.d") {
		return nil
	}

	updated := string(existing)
	if updated != "" && !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	updated += "include conf.d/*.conf\n"

	if err := os.MkdirAll(filepath.Dir(swanctlConf), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(swanctlConf), err)
	}
	if err := os.WriteFile(swanctlConf, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("updating %s: %w", swanctlConf, err)
	}
	return nil
}

func runOk(cmd *exec.Cmd) error {
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %w: %s", cmd.Args, err, strings.TrimSpace(string(out)))
	}
	return nil
}
