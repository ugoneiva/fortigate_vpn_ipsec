package guiapp

import (
	"time"

	"fyne.io/fyne/v2"

	"fortivpn-client/internal/profile"
)

// startStatusPolling ticks every ~2s and calls DaemonClient.Status. This is
// the exact call pattern that used to spawn pkexec once per tick in the old
// Rust version; here Status has no polkit check in the daemon at all, so this
// loop can run indefinitely without ever producing a password prompt.
//
// Besides the selected profile (which feeds the status card), the others are
// polled at a slower pace just to keep the list dots honest: an indicator that
// never changes is worse than no indicator at all.
func (a *App) startStatusPolling() {
	ticker := time.NewTicker(2 * time.Second)
	go func() {
		tick := 0
		for range ticker.C {
			a.pollSelected()
			// The unselected ones every 5 ticks (~10s): this is a supporting
			// indicator, it does not need the same cadence.
			if tick%5 == 0 {
				a.pollOthers()
			}
			tick++
		}
	}()
}

func (a *App) pollSelected() {
	chosen := make(chan *profile.Profile, 1)
	fyne.Do(func() {
		if p, ok := a.selectedProfile(); ok {
			chosen <- &p
			return
		}
		a.card.update("", profile.ConnectionStatus{}, false)
		chosen <- nil
	})
	sel := <-chosen
	if sel == nil {
		return
	}
	p := *sel
	st, err := a.client.Status(p.ConnName())
	if err != nil {
		fyne.Do(func() { a.card.showError(err.Error()) })
		return
	}
	fyne.Do(func() {
		a.states[p.ConnName()] = st.State
		a.card.update(p.Name, st, true)
		a.reflectState(st.State)
		a.list.Refresh()
	})
}

// pollOthers queries the profiles that are not selected. The `states` map is
// read by the list on the UI thread, so it is only read and written inside
// fyne.Do; the goroutine works on copies.
func (a *App) pollOthers() {
	ready := make(chan []string, 1)
	fyne.Do(func() {
		sel, temSel := a.selectedProfile()
		names := make([]string, 0, len(a.store.Profiles))
		for _, p := range a.store.Profiles {
			if temSel && p.ID == sel.ID {
				continue
			}
			names = append(names, p.ConnName())
		}
		ready <- names
	})

	fresh := map[string]profile.ConnState{}
	for _, conn := range <-ready {
		st, err := a.client.Status(conn)
		if err != nil {
			continue
		}
		fresh[conn] = st.State
	}

	fyne.Do(func() {
		changed := false
		for conn, st := range fresh {
			if a.states[conn] != st {
				a.states[conn] = st
				changed = true
			}
		}
		if changed {
			a.list.Refresh()
		}
	})
}
