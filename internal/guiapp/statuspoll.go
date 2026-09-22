package guiapp

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"

	"fortivpn-client/internal/profile"
)

// startStatusPolling ticks every ~2s and calls DaemonClient.Status for the
// selected profile. This is the exact call pattern that used to spawn
// pkexec once per tick in the old Rust version; here Status has no polkit
// check in the daemon at all, so this loop can run indefinitely without
// ever producing a password prompt.
func (a *App) startStatusPolling() {
	ticker := time.NewTicker(2 * time.Second)
	go func() {
		for range ticker.C {
			p, ok := a.selectedProfile()
			if !ok {
				fyne.Do(func() { a.statusLabel.SetText("No profile selected") })
				continue
			}
			st, err := a.client.Status(p.ConnName())
			if err != nil {
				fyne.Do(func() { a.statusLabel.SetText(fmt.Sprintf("Status error: %v", err)) })
				continue
			}
			text := fmt.Sprintf("%s: %s", p.Name, stateLabel(st.State))
			if st.VirtualIP != "" {
				text += fmt.Sprintf(" (%s)", st.VirtualIP)
			}
			fyne.Do(func() { a.statusLabel.SetText(text) })
		}
	}()
}

// stateLabel spells out the states whose bare identifier would not tell the
// user what to do about it.
func stateLabel(s profile.ConnState) string {
	if s == profile.StateNoTunnel {
		return "phase 1 up, no tunnel"
	}
	return string(s)
}
