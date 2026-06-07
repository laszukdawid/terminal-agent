package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Brand palette for the dark terminal-native theme. Values come from
// brand/brand.md ("Dark palette"). They are exported as package-level vars so
// layout code can paint terminal accents (prompt markers, section headings,
// the ASCII mascot, dotted rules) with the exact brand greens rather than
// re-deriving approximations.
var (
	brandBackground    = color.NRGBA{R: 0x07, G: 0x0B, B: 0x08, A: 0xFF} // app canvas, near-black
	brandPanel         = color.NRGBA{R: 0x0D, G: 0x13, B: 0x0F, A: 0xFF} // bordered panels
	brandElevatedPanel = color.NRGBA{R: 0x10, G: 0x18, B: 0x12, A: 0xFF} // active nav, buttons
	brandBorder        = color.NRGBA{R: 0x1D, G: 0x3A, B: 0x22, A: 0xFF} // subtle panel border
	brandBorderBright  = color.NRGBA{R: 0x24, G: 0x54, B: 0x2A, A: 0xFF} // emphasized/green border
	brandAccentGreen   = color.NRGBA{R: 0x66, G: 0xE0, B: 0x5D, A: 0xFF} // primary accent
	brandMutedGreen    = color.NRGBA{R: 0x7B, G: 0xAF, B: 0x72, A: 0xFF} // secondary green text
	brandPrimaryText   = color.NRGBA{R: 0xE8, G: 0xE8, B: 0xDF, A: 0xFF} // off-white body text
	brandSecondaryText = color.NRGBA{R: 0x9B, G: 0xA3, B: 0x9A, A: 0xFF} // muted labels/hints
	brandDisabledText  = color.NRGBA{R: 0x5E, G: 0x66, B: 0x5E, A: 0xFF} // disabled text
	brandError         = color.NRGBA{R: 0xE0, G: 0x6D, B: 0x5D, A: 0xFF} // muted terminal red
)

// brandTheme is the terminal-native dark theme. It forces the dark variant,
// renders every text style in monospace, and maps the brand palette onto
// Fyne's semantic color names so standard widgets pick up the look without
// per-widget styling.
type brandTheme struct{}

var _ fyne.Theme = brandTheme{}

func newBrandTheme() fyne.Theme { return brandTheme{} }

func (brandTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return brandBackground
	case theme.ColorNameForeground:
		return brandPrimaryText
	case theme.ColorNameForegroundOnPrimary:
		return brandBackground
	case theme.ColorNamePrimary:
		return brandAccentGreen
	case theme.ColorNameHyperlink:
		return brandAccentGreen
	case theme.ColorNameInputBackground:
		return brandPanel
	case theme.ColorNameInputBorder:
		return brandBorderBright
	case theme.ColorNameButton:
		return brandElevatedPanel
	case theme.ColorNameDisabledButton:
		return brandPanel
	case theme.ColorNameDisabled:
		return brandDisabledText
	case theme.ColorNamePlaceHolder:
		return brandSecondaryText
	case theme.ColorNameHover:
		return color.NRGBA{R: 0x18, G: 0x28, B: 0x1B, A: 0xFF}
	case theme.ColorNameFocus:
		return brandBorderBright
	case theme.ColorNameSelection:
		return color.NRGBA{R: 0x24, G: 0x54, B: 0x2A, A: 0x99}
	case theme.ColorNameSuccess:
		return brandAccentGreen
	case theme.ColorNameError:
		return brandError
	case theme.ColorNameWarning:
		return brandMutedGreen
	case theme.ColorNameMenuBackground:
		return brandBackground
	case theme.ColorNameOverlayBackground:
		return brandBackground
	case theme.ColorNameScrollBar:
		return brandBorder
	case theme.ColorNameSeparator:
		return brandBorder
	case theme.ColorNameShadow:
		return color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x66}
	}
	return theme.DefaultTheme().Color(name, theme.VariantDark)
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
