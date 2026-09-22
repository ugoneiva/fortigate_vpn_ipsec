package guiapp

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"fortivpn-client/internal/profile"
)

// newProfileList builds the profile list. Each row carries the state dot, the
// name in bold and, below it, the gateway with the negotiation mode: the same
// information the previous version showed, but as a visual hierarchy instead
// of one run-on line of text.
func (a *App) newProfileList() *widget.List {
	list := widget.NewList(
		func() int { return len(a.store.Profiles) },
		func() fyne.CanvasObject {
			// canvas.Text rather than widget.Label: the Label carries the
			// theme's inner padding, which made every row twice as tall.
			name := canvas.NewText("", theme.Color(theme.ColorNameForeground))
			name.TextStyle = fyne.TextStyle{Bold: true}
			name.TextSize = 14
			detail := canvas.NewText("", theme.Color(theme.ColorNamePlaceHolder))
			detail.TextSize = 12
			return container.NewBorder(nil, nil,
				container.NewGridWrap(fyne.NewSize(16, 16), newStatusDot()), nil,
				container.New(layout.NewCustomPaddedVBoxLayout(2), name, detail),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			p := a.store.Profiles[id]
			row := obj.(*fyne.Container)
			dot := row.Objects[1].(*fyne.Container).Objects[0].(*statusDot)
			texts := row.Objects[0].(*fyne.Container)
			setText(texts.Objects[0].(*canvas.Text), p.Name)
			setText(texts.Objects[1].(*canvas.Text), fmt.Sprintf("%s · %s", p.Gateway, ikeLabel(p)))
			dot.setState(a.states[p.ConnName()])
		},
	)
	list.OnSelected = func(id widget.ListItemID) {
		a.selected = id
		a.setActionButtonsEnabled(true)
		go a.pollSelected()
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
