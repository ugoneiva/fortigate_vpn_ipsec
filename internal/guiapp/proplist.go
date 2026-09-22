package guiapp

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"fortivpn-client/internal/profile"
)

func (a *App) newProfileList() *widget.List {
	list := widget.NewList(
		func() int { return len(a.store.Profiles) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			p := a.store.Profiles[id]
			obj.(*widget.Label).SetText(fmt.Sprintf("%s — %s (%s)", p.Name, p.Gateway, ikeLabel(p)))
		},
	)
	list.OnSelected = func(id widget.ListItemID) {
		a.selected = id
		a.setActionButtonsEnabled(true)
	}
	list.OnUnselected = func(id widget.ListItemID) {
		a.selected = -1
		a.setActionButtonsEnabled(false)
	}
	return list
}

func ikeLabel(p profile.Profile) string {
	if p.IkeVersion == profile.IkeV1 {
		if p.Ikev1Mode == profile.Ikev1Aggressive {
			return "IKEv1 Aggressive"
		}
		return "IKEv1 Main"
	}
	return "IKEv2"
}
