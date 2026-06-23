package gui

import (
	"image/color"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type brandPalette struct {
	background    color.NRGBA
	panel         color.NRGBA
	elevatedPanel color.NRGBA
	border        color.NRGBA
	borderBright  color.NRGBA
	accentGreen   color.NRGBA
	mutedGreen    color.NRGBA
	primaryText   color.NRGBA
	secondaryText color.NRGBA
	disabledText  color.NRGBA
	error         color.NRGBA
	warning       color.NRGBA
	hover         color.NRGBA
	selection     color.NRGBA
	shadow        color.NRGBA
}

// Brand palettes for the terminal-native theme. The package-level color vars
// keep the dark palette available to existing tests and helpers; runtime UI code
// should resolve through currentBrandPalette so light-mode surfaces are coherent.
var (
	brandDarkPalette = brandPalette{
		background:    color.NRGBA{R: 0x07, G: 0x0B, B: 0x08, A: 0xFF},
		panel:         color.NRGBA{R: 0x0D, G: 0x13, B: 0x0F, A: 0xFF},
		elevatedPanel: color.NRGBA{R: 0x10, G: 0x18, B: 0x12, A: 0xFF},
		border:        color.NRGBA{R: 0x1D, G: 0x3A, B: 0x22, A: 0xFF},
		borderBright:  color.NRGBA{R: 0x24, G: 0x54, B: 0x2A, A: 0xFF},
		accentGreen:   color.NRGBA{R: 0x66, G: 0xE0, B: 0x5D, A: 0xFF},
		mutedGreen:    color.NRGBA{R: 0x7B, G: 0xAF, B: 0x72, A: 0xFF},
		primaryText:   color.NRGBA{R: 0xE8, G: 0xE8, B: 0xDF, A: 0xFF},
		secondaryText: color.NRGBA{R: 0x9B, G: 0xA3, B: 0x9A, A: 0xFF},
		disabledText:  color.NRGBA{R: 0x5E, G: 0x66, B: 0x5E, A: 0xFF},
		error:         color.NRGBA{R: 0xE0, G: 0x6D, B: 0x5D, A: 0xFF},
		warning:       color.NRGBA{R: 0x7B, G: 0xAF, B: 0x72, A: 0xFF},
		// Hover is a translucent tint, not an opaque fill: the button renderer
		// alpha-blends it over the button color, so an opaque hover would replace a
		// colored (importance) button's background and hide its on-color text.
		hover:     color.NRGBA{R: 0x18, G: 0x28, B: 0x1B, A: 0x40},
		selection: color.NRGBA{R: 0x24, G: 0x54, B: 0x2A, A: 0x99},
		shadow:    color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x66},
	}

	brandLightPalette = brandPalette{
		background:    color.NRGBA{R: 0xF7, G: 0xFA, B: 0xF2, A: 0xFF},
		panel:         color.NRGBA{R: 0xEE, G: 0xF5, B: 0xE8, A: 0xFF},
		elevatedPanel: color.NRGBA{R: 0xE2, G: 0xEF, B: 0xD8, A: 0xFF},
		border:        color.NRGBA{R: 0xB4, G: 0xD2, B: 0xAD, A: 0xFF},
		borderBright:  color.NRGBA{R: 0x4F, G: 0x9E, B: 0x45, A: 0xFF},
		accentGreen:   color.NRGBA{R: 0x24, G: 0x8A, B: 0x2B, A: 0xFF},
		mutedGreen:    color.NRGBA{R: 0x5D, G: 0x86, B: 0x55, A: 0xFF},
		primaryText:   color.NRGBA{R: 0x16, G: 0x20, B: 0x17, A: 0xFF},
		secondaryText: color.NRGBA{R: 0x59, G: 0x66, B: 0x57, A: 0xFF},
		disabledText:  color.NRGBA{R: 0x9B, G: 0xA8, B: 0x98, A: 0xFF},
		error:         color.NRGBA{R: 0xB5, G: 0x48, B: 0x3D, A: 0xFF},
		warning:       color.NRGBA{R: 0x8A, G: 0x6A, B: 0x22, A: 0xFF},
		hover:         color.NRGBA{R: 0xDC, G: 0xEB, B: 0xD3, A: 0x40},
		selection:     color.NRGBA{R: 0xB9, G: 0xE4, B: 0xB2, A: 0x99},
		shadow:        color.NRGBA{R: 0x22, G: 0x36, B: 0x22, A: 0x22},
	}

	brandBackground    = brandDarkPalette.background
	brandPanel         = brandDarkPalette.panel
	brandElevatedPanel = brandDarkPalette.elevatedPanel
	brandBorder        = brandDarkPalette.border
	brandBorderBright  = brandDarkPalette.borderBright
	brandAccentGreen   = brandDarkPalette.accentGreen
	brandMutedGreen    = brandDarkPalette.mutedGreen
	brandPrimaryText   = brandDarkPalette.primaryText
	brandSecondaryText = brandDarkPalette.secondaryText
	brandDisabledText  = brandDarkPalette.disabledText
	brandError         = brandDarkPalette.error
)

// brandTheme is the terminal-native theme. It renders every text style in
// monospace, and maps the active brand palette onto Fyne's semantic color names
// so standard widgets pick up the look without per-widget styling.
type brandTheme struct{}

type forcedVariantTheme struct {
	fyne.Theme
	variant fyne.ThemeVariant
}

type promptEntryTheme struct{ fyne.Theme }

var (
	_ fyne.Theme = brandTheme{}
	_ fyne.Theme = forcedVariantTheme{}
)

func newBrandTheme() fyne.Theme {
	switch os.Getenv("FYNE_THEME") {
	case "light":
		return forcedVariantTheme{Theme: brandTheme{}, variant: theme.VariantLight}
	case "dark":
		return forcedVariantTheme{Theme: brandTheme{}, variant: theme.VariantDark}
	default:
		return brandTheme{}
	}
}

func (t forcedVariantTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	return t.Theme.Color(name, t.variant)
}

func (t promptEntryTheme) Size(name fyne.ThemeSizeName) float32 {
	if name == theme.SizeNameInputBorder {
		return promptNativeCursorWidth
	}
	return t.Theme.Size(name)
}

func brandPaletteForVariant(variant fyne.ThemeVariant) brandPalette {
	if variant == theme.VariantLight {
		return brandLightPalette
	}
	return brandDarkPalette
}

func currentBrandPalette() brandPalette {
	switch os.Getenv("FYNE_THEME") {
	case "light":
		return brandLightPalette
	case "dark":
		return brandDarkPalette
	}
	if fyne.CurrentApp() == nil {
		return brandDarkPalette
	}
	return brandPaletteForVariant(fyne.CurrentApp().Settings().ThemeVariant())
}

func (brandTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	palette := brandPaletteForVariant(variant)
	switch name {
	case theme.ColorNameBackground:
		return palette.background
	case theme.ColorNameForeground:
		return palette.primaryText
	case theme.ColorNameForegroundOnPrimary:
		return palette.background
	case theme.ColorNamePrimary:
		return palette.accentGreen
	case theme.ColorNameHyperlink:
		return palette.accentGreen
	case theme.ColorNameInputBackground:
		return palette.panel
	case theme.ColorNameInputBorder:
		return palette.borderBright
	case theme.ColorNameButton:
		return palette.elevatedPanel
	case theme.ColorNameDisabledButton:
		return palette.panel
	case theme.ColorNameDisabled:
		return palette.disabledText
	case theme.ColorNamePlaceHolder:
		return palette.secondaryText
	case theme.ColorNameHover:
		return palette.hover
	case theme.ColorNameFocus:
		return palette.borderBright
	case theme.ColorNameSelection:
		return palette.selection
	case theme.ColorNameSuccess:
		return palette.accentGreen
	case theme.ColorNameError:
		return palette.error
	case theme.ColorNameWarning:
		return palette.warning
	case theme.ColorNameMenuBackground:
		return palette.background
	case theme.ColorNameOverlayBackground:
		return palette.elevatedPanel
	case theme.ColorNameScrollBar:
		return palette.border
	case theme.ColorNameSeparator:
		return palette.border
	case theme.ColorNameShadow:
		return palette.shadow
	}
	return theme.DefaultTheme().Color(name, variant)
}

// Font returns the monospace face for every style. Typography is central to
// the brand: the whole UI should read as a crafted terminal surface, so bold
// and italic still resolve to monospace variants rather than a proportional
// fallback.
func (brandTheme) Font(style fyne.TextStyle) fyne.Resource {
	style.Monospace = true
	return theme.DefaultTheme().Font(style)
}

func (brandTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (brandTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameInputRadius:
		return 4
	case theme.SizeNameSelectionRadius:
		return 4
	case theme.SizeNameInputBorder:
		// Entries draw no border line of their own; the brand frames inputs with
		// their surrounding panel (e.g. the green prompt panel) so the text field
		// blends seamlessly into it instead of reading as a nested box.
		return 0
	}
	return theme.DefaultTheme().Size(name)
}
