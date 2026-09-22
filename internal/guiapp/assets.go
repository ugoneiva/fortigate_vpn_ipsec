package guiapp

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

//go:embed assets/icon.svg
var iconSVGData []byte

var appIcon = fyne.NewStaticResource("fortivpn-client.svg", iconSVGData)
