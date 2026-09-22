package guiapp

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// The app's own theme. It is anchored on the blue from the application icon
// (#2d6cdf), lightened for dark backgrounds so it keeps contrast. The state
// colours (connected, no tunnel, error) are shared by the status card, the
// list dots and the action button, so the same situation never shows up in
// two different colours.
var (
	accentColor = color.NRGBA{R: 0x3d, G: 0x82, B: 0xff, A: 0xff}
	accentLight = color.NRGBA{R: 0x2d, G: 0x6c, B: 0xdf, A: 0xff}

	connectedColor  = color.NRGBA{R: 0x27, G: 0xc9, B: 0x7a, A: 0xff}
	attentionColor  = color.NRGBA{R: 0xf5, G: 0x9e, B: 0x0b, A: 0xff}
	failureColor    = color.NRGBA{R: 0xef, G: 0x54, B: 0x4a, A: 0xff}
	idleColor       = color.NRGBA{R: 0x6b, G: 0x78, B: 0x89, A: 0xff}
	connectingColor = accentColor
)

type vpnTheme struct{}

var _ fyne.Theme = vpnTheme{}

func (vpnTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if v == theme.VariantLight {
		switch n {
		case theme.ColorNamePrimary, theme.ColorNameHyperlink:
			return accentLight
		case theme.ColorNameSuccess:
			return connectedColor
		case theme.ColorNameWarning:
			return attentionColor
		case theme.ColorNameError:
			return failureColor
		}
		return theme.DefaultTheme().Color(n, v)
	}

	switch n {
	case theme.ColorNameBackground:
		return color.NRGBA{R: 0x0f, G: 0x14, B: 0x1c, A: 0xff}
	// Card and field surfaces: one step above the background, enough to read
	// as a separate surface without drawing a border.
	case theme.ColorNameOverlayBackground, theme.ColorNameMenuBackground:
		return color.NRGBA{R: 0x16, G: 0x1d, B: 0x27, A: 0xff}
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 0x1b, G: 0x23, B: 0x2f, A: 0xff}
	case theme.ColorNameButton:
		return color.NRGBA{R: 0x1f, G: 0x28, B: 0x36, A: 0xff}
	case theme.ColorNameHover:
		return color.NRGBA{R: 0x28, G: 0x33, B: 0x44, A: 0xff}
	case theme.ColorNameSelection:
		return color.NRGBA{R: 0x3d, G: 0x82, B: 0xff, A: 0x30}
	case theme.ColorNameSeparator, theme.ColorNameInputBorder:
		return color.NRGBA{R: 0x27, G: 0x31, B: 0x40, A: 0xff}
	case theme.ColorNameForeground:
		return color.NRGBA{R: 0xe6, G: 0xed, B: 0xf3, A: 0xff}
	case theme.ColorNameForegroundOnPrimary, theme.ColorNameForegroundOnError,
		theme.ColorNameForegroundOnSuccess, theme.ColorNameForegroundOnWarning:
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	case theme.ColorNamePlaceHolder, theme.ColorNameDisabled:
		return color.NRGBA{R: 0x7d, G: 0x8a, B: 0x9b, A: 0xff}
	case theme.ColorNamePrimary, theme.ColorNameHyperlink, theme.ColorNameFocus:
		return accentColor
	case theme.ColorNameSuccess:
		return connectedColor
	case theme.ColorNameWarning:
		return attentionColor
	case theme.ColorNameError:
		return failureColor
	case theme.ColorNameShadow:
		return color.NRGBA{A: 0x66}
	}
	return theme.DefaultTheme().Color(n, v)
}

func (vpnTheme) Font(s fyne.TextStyle) fyne.Resource { return theme.DefaultTheme().Font(s) }

func (vpnTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }

func (vpnTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNamePadding:
		return 6
	case theme.SizeNameInnerPadding:
		return 10
	case theme.SizeNameText:
		return 14
	case theme.SizeNameHeadingText:
		return 22
	case theme.SizeNameSubHeadingText:
		return 16
	case theme.SizeNameCaptionText:
		return 12
	// Generous corners: this is what shifts the impression towards "modern"
	// without needing any new artwork.
	case theme.SizeNameInputRadius, theme.SizeNameSelectionRadius:
		return 8
	case theme.SizeNameScrollBar:
		return 10
	case theme.SizeNameSeparatorThickness:
		return 1
	}
	return theme.DefaultTheme().Size(n)
}
