package gui

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	appservice "github.com/laszukdawid/terminal-agent/internal/app"
	"github.com/laszukdawid/terminal-agent/internal/sessionlog"
)

const (
	historyLimit              = 50
	historyDetailDialogWidth  = 700
	historyDetailDialogHeight = 430
	historyDetailDialogMargin = 48
	historyDetailFooterHeight = 88
	historyCornerSize         = 24
	historyCardGap            = 6
)

var historyBackdropColor = color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xB8}

func (g *App) loadHistory() {
	runs, err := sessionlog.Recent(appservice.SessionDir(), historyLimit)
	if err != nil {
		g.popup.setHistory(nil, "History unavailable: "+err.Error())
		return
	}
	g.popup.setHistory(runs, "")
}

func (p *popupWindow) setHistory(runs []sessionlog.Summary, errorText string) {
	if p.historyBody == nil {
		return
	}
	p.historyBody.Objects = nil
	if errorText != "" {
		label := widget.NewLabel(errorText)
		label.Wrapping = fyne.TextWrapWord
		p.historyBody.Add(label)
	} else if len(runs) == 0 {
		p.historyBody.Add(historyEmptyState())
	} else {
		for _, run := range runs {
			card := newHistoryCard(run, func() {
				p.showHistoryDetail(run)
			})
			p.historyBody.Add(container.New(layout.NewCustomPaddedLayout(0, historyCardGap, 0, 0), card))
		}
	}
	p.historyBody.Refresh()
}

func historyEmptyState() fyne.CanvasObject {
	label := canvas.NewText("No Ask or Task executions recorded yet.", brandSecondaryText)
	label.TextSize = theme.TextSize()
	return container.NewPadded(label)
}

func newHistoryCard(run sessionlog.Summary, onTap func()) fyne.CanvasObject {
	return newTappableHistoryCard(historyCardContent(run), onTap)
}

func historyCardContent(run sessionlog.Summary) fyne.CanvasObject {
	title := canvas.NewText(historyTitle(run), brandPrimaryText)
	title.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
	title.TextSize = theme.TextSize() - 1

	meta := canvas.NewText(historyMeta(run), brandSecondaryText)
	meta.TextSize = theme.TextSize() - 2

	fallbackPrompt := run.Command
	if fallbackPrompt == "" {
		fallbackPrompt = "No request recorded."
	}
	promptText, promptTruncated := historyPreview(run.Request, fallbackPrompt)
	prompt := widget.NewLabel(promptText)
	prompt.Wrapping = fyne.TextWrapWord

	responseText := run.Response
	if run.Error != "" {
		responseText = "Error: " + run.Error
	}
	responseText, responseTruncated := historyPreview(responseText, "No response recorded.")
	response := widget.NewLabel(responseText)
	response.Wrapping = fyne.TextWrapWord

	body := container.NewVBox(title, meta, brandSeparator(), prompt, response)
	card := borderedBox(body, brandBorder)
	if promptTruncated || responseTruncated {
		return withHistoryTruncationCorner(card)
	}
	return card
}

func (p *popupWindow) showHistoryDetail(run sessionlog.Summary) {
	p.dismissHistoryDetail()
	title := historyTitle(run)
	meta := historyMeta(run)
	promptText := historyFullText(run.Request, run.Command, "No request recorded.")
	responseText := historyFullResponse(run)

	content := container.NewVBox(
		widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true, Monospace: true}),
		widget.NewLabel(meta),
		brandSeparator(),
		historyDetailSection("Prompt", promptText),
		historyDetailSection("Response", responseText),
	)
	size := historyDetailPopupSize(p.window.Canvas().Size())
	scroll := container.NewVScroll(content)
	scroll.SetMinSize(fyne.NewSize(size.Width, max(120, size.Height-historyDetailFooterHeight)))
	closeButton := widget.NewButton("Close", nil)
	detail := borderedBox(container.NewBorder(nil, closeButton, nil, nil, scroll), brandBorder)
	canvasSize := p.window.Canvas().Size()
	overlay := newHistoryDetailOverlay(detail, size, nil)
	pop := widget.NewPopUp(overlay, p.window.Canvas())
	overlay.onDismiss = func() {
		p.dismissHistoryDetail()
	}
	closeButton.OnTapped = func() {
		p.dismissHistoryDetail()
	}
	pop.Resize(canvasSize)
	p.historyDetail = pop
	p.window.Canvas().Unfocus()
	pop.ShowAtPosition(fyne.NewPos(0, 0))
}

func historyDetailSection(title, text string) fyne.CanvasObject {
	label := widget.NewRichTextFromMarkdown(decorateDollarMarkers(unwrapMarkdownFence(text)))
	label.Wrapping = fyne.TextWrapWord
	label.Segments = colorizeDollarMarkers(label.Segments)
	return container.NewVBox(
		widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		label,
	)
}

func historyTitle(run sessionlog.Summary) string {
	kind := strings.ToUpper(run.Kind)
	if kind == "" {
		kind = "RUN"
	}
	when := run.CreatedAt.Format("2006-01-02 15:04")
	if run.CreatedAt.IsZero() {
		when = "unknown time"
	}
	return kind + "  " + when
}

func historyMeta(run sessionlog.Summary) string {
	parts := make([]string, 0, 3)
	if run.Provider != "" && run.Model != "" {
		parts = append(parts, run.Provider+" / "+run.Model)
	} else if run.Provider != "" {
		parts = append(parts, run.Provider)
	} else if run.Model != "" {
		parts = append(parts, run.Model)
	}
	if run.Cwd != "" {
		parts = append(parts, displayCwd(run.Cwd))
	}
	if !run.CompletedAt.IsZero() && !run.CreatedAt.IsZero() {
		parts = append(parts, formatElapsed(run.CompletedAt.Sub(run.CreatedAt)))
	}
	return strings.Join(parts, metaSeparator)
}

func historyPreview(value, fallback string) (string, bool) {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return fallback, false
	}
	const maxPreviewRunes = 280
	runes := []rune(value)
	if len(runes) <= maxPreviewRunes {
		return value, false
	}
	return string(runes[:maxPreviewRunes]) + "...", true
}

func centerPopupPosition(canvasSize, popupSize fyne.Size) fyne.Position {
	x := (canvasSize.Width - popupSize.Width) / 2
	y := (canvasSize.Height - popupSize.Height) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return fyne.NewPos(x, y)
}

func historyDetailPopupSize(canvasSize fyne.Size) fyne.Size {
	width := min(historyDetailDialogWidth, canvasSize.Width-historyDetailDialogMargin)
	height := min(historyDetailDialogHeight, canvasSize.Height-historyDetailDialogMargin)
	return fyne.NewSize(max(260, width), max(220, height))
}

func withHistoryTruncationCorner(content fyne.CanvasObject) fyne.CanvasObject {
	corner := container.NewGridWrap(fyne.NewSize(historyCornerSize, historyCornerSize), historyTruncationCorner())
	cornerLayer := container.NewBorder(nil, container.NewHBox(layout.NewSpacer(), corner), nil, nil, nil)
	return container.NewStack(content, cornerLayer)
}

func historyTruncationCorner() fyne.CanvasObject {
	return canvas.NewRasterWithPixels(func(x, y, w, h int) color.Color {
		if x+y >= w-1 {
			return brandAccentGreen
		}
		return color.NRGBA{A: 0}
	})
}

func historyFullText(value, fallback, empty string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	fallback = strings.TrimSpace(fallback)
	if fallback != "" {
		return fallback
	}
	return empty
}

func historyFullResponse(run sessionlog.Summary) string {
	if strings.TrimSpace(run.Error) != "" {
		return "Error: " + strings.TrimSpace(run.Error)
	}
	return historyFullText(run.Response, "", "No response recorded.")
}

type tappableHistoryCard struct {
	widget.BaseWidget
	content fyne.CanvasObject
	bg      *canvas.Rectangle
	onTap   func()
}

func newTappableHistoryCard(content fyne.CanvasObject, onTap func()) *tappableHistoryCard {
	card := &tappableHistoryCard{
		content: content,
		bg:      canvas.NewRectangle(color.Transparent),
		onTap:   onTap,
	}
	card.bg.CornerRadius = 4
	card.ExtendBaseWidget(card)
	return card
}

func (c *tappableHistoryCard) Tapped(*fyne.PointEvent) {
	if c.onTap != nil {
		c.onTap()
	}
}

func (c *tappableHistoryCard) MouseIn(*desktop.MouseEvent) {
	c.bg.FillColor = brandElevatedPanel
	c.bg.Refresh()
}

func (c *tappableHistoryCard) MouseMoved(*desktop.MouseEvent) {}

func (c *tappableHistoryCard) MouseOut() {
	c.bg.FillColor = color.Transparent
	c.bg.Refresh()
}

func (c *tappableHistoryCard) Cursor() desktop.Cursor {
	if c.onTap != nil {
		return desktop.PointerCursor
	}
	return desktop.DefaultCursor
}

func (c *tappableHistoryCard) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewStack(c.bg, c.content))
}

func (p *popupWindow) dismissHistoryDetail() bool {
	if p.historyDetail == nil || !p.historyDetail.Visible() {
		return false
	}
	p.historyDetail.Hide()
	return true
}

type historyDetailOverlay struct {
	widget.BaseWidget
	detail    fyne.CanvasObject
	panelSize fyne.Size
	panelPos  fyne.Position
	bg        *canvas.Rectangle
	onDismiss func()
}

func newHistoryDetailOverlay(detail fyne.CanvasObject, panelSize fyne.Size, onDismiss func()) *historyDetailOverlay {
	overlay := &historyDetailOverlay{
		detail:    detail,
		panelSize: panelSize,
		bg:        canvas.NewRectangle(historyBackdropColor),
		onDismiss: onDismiss,
	}
	overlay.ExtendBaseWidget(overlay)
	return overlay
}

func (o *historyDetailOverlay) Tapped(event *fyne.PointEvent) {
	if event == nil || !pointInside(event.Position, o.panelPos, o.panelSize) {
		if o.onDismiss != nil {
			o.onDismiss()
		}
	}
}

func (o *historyDetailOverlay) CreateRenderer() fyne.WidgetRenderer {
	return &historyDetailOverlayRenderer{overlay: o, objects: []fyne.CanvasObject{o.bg, o.detail}}
}

type historyDetailOverlayRenderer struct {
	overlay *historyDetailOverlay
	objects []fyne.CanvasObject
}

func (r *historyDetailOverlayRenderer) Layout(size fyne.Size) {
	r.overlay.bg.Resize(size)
	panelSize := r.overlay.panelSize.Min(size.Subtract(fyne.NewSize(historyDetailDialogMargin, historyDetailDialogMargin)))
	panelSize = panelSize.Max(fyne.NewSize(260, 220))
	r.overlay.panelSize = panelSize
	r.overlay.panelPos = centerPopupPosition(size, panelSize)
	r.overlay.detail.Move(r.overlay.panelPos)
	r.overlay.detail.Resize(panelSize)
}

func (r *historyDetailOverlayRenderer) MinSize() fyne.Size { return fyne.NewSize(1, 1) }

func (r *historyDetailOverlayRenderer) Objects() []fyne.CanvasObject { return r.objects }

func (r *historyDetailOverlayRenderer) Refresh() {
	r.overlay.bg.FillColor = historyBackdropColor
	r.Layout(r.overlay.Size())
	r.overlay.bg.Refresh()
	r.overlay.detail.Refresh()
}

func (r *historyDetailOverlayRenderer) Destroy() {}

func pointInside(point, pos fyne.Position, size fyne.Size) bool {
	return point.X >= pos.X && point.Y >= pos.Y && point.X <= pos.X+size.Width && point.Y <= pos.Y+size.Height
}
