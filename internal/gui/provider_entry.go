package gui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	xwidget "fyne.io/x/fyne/widget"

	"github.com/laszukdawid/terminal-agent/internal/connector"
)

// newProviderEntry builds an editable autocomplete field for choosing a
// provider, pre-filled with initial. The user may type any value; suggestions
// are filtered from the connector registry by case-insensitive substring, and
// the full list is shown when the field is focused while empty. In each
// suggestion the portion matching what the user typed is emphasized (bold,
// accent color).
//
// It uses *CompletionEntry directly rather than embedding/subclassing it:
// CompletionEntry's popup methods resolve their canvas via the receiver, so a
// subclass breaks the lookup under the real driver (the embedded value is not
// the object registered in the canvas cache, so CanvasForObject returns nil).
// The OnFocusGained hook (added upstream) lets us react to focus without
// subclassing.
//
// onChange (may be nil) is invoked after each text change so callers can update
// dependent UI (e.g. hint labels).
func newProviderEntry(initial string, onChange func(string)) *xwidget.CompletionEntry {
	e := xwidget.NewCompletionEntry(connector.SupportedProviders())

	// shown mirrors the options currently displayed and query is what the user
	// typed; the custom item renderer reads them to emphasize the matched part.
	var (
		shown []string
		query string
	)
	display := func(opts []string, q string) {
		shown = opts
		query = q
		// Always update the widget's options first so a no-match query clears
		// the (hidden) suggestion list rather than leaving stale items that
		// keyboard navigation could still act on.
		e.SetOptions(opts)
		if len(opts) == 0 {
			e.HideCompletion()
			return
		}
		e.ShowCompletion()
	}

	// Render each suggestion as RichText so the matched substring can be
	// emphasized. The template carries a single space so the list measures a
	// full line height for its rows.
	e.CustomCreate = func() fyne.CanvasObject {
		rt := widget.NewRichText(&widget.TextSegment{Style: widget.RichTextStyleInline, Text: " "})
		rt.Wrapping = fyne.TextWrapOff
		return rt
	}
	e.CustomUpdate = func(id widget.ListItemID, o fyne.CanvasObject) {
		rt, ok := o.(*widget.RichText)
		if !ok {
			return
		}
		text := ""
		if id >= 0 && id < len(shown) {
			text = shown[id]
		}
		rt.Segments = matchSegments(text, query)
		rt.Refresh()
	}

	e.SetText(initial)
	e.OnChanged = func(s string) {
		display(filterProviders(connector.SupportedProviders(), s), s)
		if onChange != nil {
			onChange(s)
		}
	}
	e.OnFocusGained = func() {
		if e.Text == "" {
			display(connector.SupportedProviders(), "")
		}
	}
	return e
}

// matchSegments splits text into RichText segments, emphasizing the first
// case-insensitive occurrence of query in bold and the theme's accent color.
// With an empty query (or no match) the whole text is returned as a single
// plain inline segment.
func matchSegments(text, query string) []widget.RichTextSegment {
	plain := widget.RichTextStyleInline

	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return []widget.RichTextSegment{&widget.TextSegment{Style: plain, Text: text}}
	}
	idx := strings.Index(strings.ToLower(text), q)
	if idx < 0 {
		return []widget.RichTextSegment{&widget.TextSegment{Style: plain, Text: text}}
	}

	matched := plain
	matched.TextStyle = fyne.TextStyle{Bold: true}
	matched.ColorName = theme.ColorNamePrimary

	end := idx + len(q)
	segs := make([]widget.RichTextSegment, 0, 3)
	if idx > 0 {
		segs = append(segs, &widget.TextSegment{Style: plain, Text: text[:idx]})
	}
	segs = append(segs, &widget.TextSegment{Style: matched, Text: text[idx:end]})
	if end < len(text) {
		segs = append(segs, &widget.TextSegment{Style: plain, Text: text[end:]})
	}
	return segs
}
