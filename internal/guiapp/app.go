package guiapp

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"fortivpn-client/internal/profile"
)

type App struct {
	fyneApp fyne.App
	window  fyne.Window
	client  *DaemonClient

	store    profile.Store
	selected int // index into store.Profiles, -1 if none

	list          *widget.List
	statusLabel   *widget.Label
	connectBtn    *widget.Button
	disconnectBtn *widget.Button
	editBtn       *widget.Button
	removeBtn     *widget.Button
}

func Run() error {
	client, err := NewDaemonClient()
	if err != nil {
		return fmt.Errorf("connecting to fortivpn-daemon: %w", err)
	}

	store, err := profile.LoadStore()
	if err != nil {
		client.Close()
		return fmt.Errorf("loading profiles: %w", err)
	}

	a := &App{
		fyneApp:  app.NewWithID("dev.fortivpn.client"),
		client:   client,
		store:    store,
		selected: -1,
	}
	a.fyneApp.SetIcon(appIcon)

	a.window = a.fyneApp.NewWindow("FortiVPN Client")
	a.window.SetIcon(appIcon)
	a.buildUI()
	a.setupTray()
	a.startStatusPolling()

	// Hide to tray instead of quitting on window close — the tray menu's
	// "Quit" is the only way out, matching a normal desktop VPN client.
	a.window.SetCloseIntercept(func() { a.window.Hide() })

	a.window.Resize(fyne.NewSize(560, 460))
	a.window.ShowAndRun()

	client.Close()
	return nil
}

func (a *App) buildUI() {
	a.list = a.newProfileList()
	a.statusLabel = widget.NewLabel("No profile selected")

	addBtn := widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), func() { a.showProfileForm(nil) })
	a.editBtn = widget.NewButtonWithIcon("Edit", theme.DocumentCreateIcon(), a.editSelected)
	a.removeBtn = widget.NewButtonWithIcon("Remove", theme.DeleteIcon(), a.removeSelected)
	a.connectBtn = widget.NewButtonWithIcon("Connect", theme.MediaPlayIcon(), a.connectSelected)
	a.disconnectBtn = widget.NewButtonWithIcon("Disconnect", theme.MediaStopIcon(), a.disconnectSelected)
	a.setActionButtonsEnabled(false)

	toolbar := container.NewHBox(addBtn, a.editBtn, a.removeBtn, layout.NewSpacer(), a.connectBtn, a.disconnectBtn)
	bottom := container.NewVBox(widget.NewSeparator(), a.statusLabel, toolbar)

	a.window.SetContent(container.NewBorder(nil, bottom, nil, nil, a.list))
}

func (a *App) setActionButtonsEnabled(enabled bool) {
	set := func(b *widget.Button) {
		if enabled {
			b.Enable()
		} else {
			b.Disable()
		}
	}
	set(a.editBtn)
	set(a.removeBtn)
	set(a.connectBtn)
	set(a.disconnectBtn)
}

func (a *App) selectedProfile() (profile.Profile, bool) {
	if a.selected < 0 || a.selected >= len(a.store.Profiles) {
		return profile.Profile{}, false
	}
	return a.store.Profiles[a.selected], true
}

func (a *App) editSelected() {
	if p, ok := a.selectedProfile(); ok {
		a.showProfileForm(&p)
	}
}

func (a *App) connectSelected() {
	p, ok := a.selectedProfile()
	if !ok {
		return
	}
	a.statusLabel.SetText(fmt.Sprintf("%s: connecting…", p.Name))
	go func() {
		if _, err := a.client.Connect(p.ConnName()); err != nil {
			fyne.Do(func() { dialog.ShowError(err, a.window) })
		}
	}()
}

func (a *App) disconnectSelected() {
	p, ok := a.selectedProfile()
	if !ok {
		return
	}
	go func() {
		if _, err := a.client.Disconnect(p.ConnName()); err != nil {
			fyne.Do(func() { dialog.ShowError(err, a.window) })
		}
	}()
}

func (a *App) removeSelected() {
	p, ok := a.selectedProfile()
	if !ok {
		return
	}
	idx := a.selected
	dialog.ShowConfirm("Remove profile", fmt.Sprintf("Remove %q?", p.Name), func(confirmed bool) {
		if !confirmed {
			return
		}
		go func() {
			if _, err := a.client.RemoveProfile(p.ConnName()); err != nil {
				fyne.Do(func() { dialog.ShowError(err, a.window) })
				return
			}
			_ = DeleteCredentials(p.ConnName())
			fyne.Do(func() {
				a.store.Profiles = append(a.store.Profiles[:idx], a.store.Profiles[idx+1:]...)
				if err := profile.SaveStore(a.store); err != nil {
					dialog.ShowError(err, a.window)
				}
				a.selected = -1
				a.list.UnselectAll()
				a.list.Refresh()
				a.setActionButtonsEnabled(false)
			})
		}()
	}, a.window)
}

// saveProfile applies the profile (writing swanctl conf.d — a privileged,
// polkit-gated call) and persists credentials/profile list only after that
// succeeds, so profiles.toml never references a profile the daemon doesn't
// actually know about.
func (a *App) saveProfile(p profile.Profile, creds profile.Credentials, isNew bool) {
	go func() {
		if err := SaveCredentials(p.ConnName(), creds); err != nil {
			fyne.Do(func() { dialog.ShowError(fmt.Errorf("saving credentials: %w", err), a.window) })
			return
		}
		if _, err := a.client.ApplyProfile(p, creds); err != nil {
			fyne.Do(func() { dialog.ShowError(fmt.Errorf("applying profile: %w", err), a.window) })
			return
		}
		fyne.Do(func() {
			if isNew {
				a.store.Profiles = append(a.store.Profiles, p)
			} else {
				for i := range a.store.Profiles {
					if a.store.Profiles[i].ID == p.ID {
						a.store.Profiles[i] = p
						break
					}
				}
			}
			if err := profile.SaveStore(a.store); err != nil {
				dialog.ShowError(fmt.Errorf("saving profile list: %w", err), a.window)
			}
			a.list.Refresh()
		})
	}()
}
