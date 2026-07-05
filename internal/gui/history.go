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
	// historyPreviewMaxRunes/MaxLines bound how much of a request or response a
	// card preview shows; the full text stays available in the detail popup.
	historyPreviewMaxRunes = 280
	historyPreviewMaxLines = 8
)

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
	palette := currentBrandPalette()
	label := newElidedText("No Ask or Task executions recorded yet.", palette.secondaryText)
	label.SetTextSize(theme.TextSize())
	return container.NewPadded(label)
}

func newHistoryCard(run sessionlog.Summary, onTap func()) fyne.CanvasObject {
	return newTappableHistoryCard(historyCardContent(run), onTap)
}

func historyCardContent(run sessionlog.Summary) fyne.CanvasObject {
	palette := currentBrandPalette()
	title := newElidedText(historyTitle(run), palette.primaryText)
	title.SetTextStyle(fyne.TextStyle{Bold: true, Monospace: true})
	title.SetTextSize(theme.TextSize() - 1)

	meta := newElidedText(historyMeta(run), palette.secondaryText)
	meta.SetTextSize(theme.TextSize() - 2)

	fallbackPrompt := run.Command
	if fallbackPrompt == "" {
		fallbackPrompt = "No request recorded."
	}
	promptText, promptTruncated := historyPreview(run.Request, fallbackPrompt)
	prompt := historyQueryLine(promptText)

	responseText := run.Response
	if run.Error != "" {
		responseText = "Error: " + run.Error
	}
	if strings.TrimSpace(responseText) == "" {
		responseText = "No response recorded."
	}
	response := widget.NewRichText()
	response.Wrapping = fyne.TextWrapWord
	responseTruncated := renderResponseMarkdownPreview(response, responseText, historyPreviewMaxRunes, historyPreviewMaxLines)

	body := container.NewVBox(title, meta, brandSeparator(), prompt, response)
	card := borderedBox(body, palette.border)
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
		historyDetailQuerySection("Prompt", promptText),
		historyDetailSection("Response", responseText),
	)
	size := historyDetailPopupSize(p.window.Canvas().Size())
	scroll := container.NewVScroll(content)
	scroll.SetMinSize(fyne.NewSize(size.Width, max(120, size.Height-historyDetailFooterHeight)))
	closeButton := widget.NewButton("Close", func() { p.dismissHistoryDetail() })
	detail := borderedBox(container.NewBorder(nil, closeButton, nil, nil, scroll), currentBrandPalette().border)
	// Use a modal popup so its backdrop dims and tracks the whole canvas, including
	// on window resize (see presentRoutineDetail for the non-modal sizing pitfall).
	pop := widget.NewModalPopUp(detail, p.window.Canvas())
	p.historyDetail = pop
	p.window.Canvas().Unfocus()
	pop.Show()
	pop.Resize(size)
}

// historyQueryLine renders the user's query behind the terminal input marker,
// mirroring the main prompt field, so a card visibly separates what was asked
// (">", bold) from what was answered (rendered markdown with "$" markers).
func historyQueryLine(text string) *widget.RichText {
	return markedTextLine(inputPrompt, text, fyne.TextStyle{Bold: true})
}

// historyDetailQuerySection is the detail-popup counterpart of historyQueryLine:
// the full query under its heading, marked as input rather than rendered as a
// response.
func historyDetailQuerySection(title, text string) fyne.CanvasObject {
	return container.NewVBox(
		widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		historyQueryLine(text),
	)
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
	runes := []rune(value)
	if len(runes) <= historyPreviewMaxRunes {
		return value, false
	}
	keep := completeWord(runes, historyPreviewMaxRunes)
	if keep == len(runes) {
		return value, false
	}
	return strings.TrimRight(string(runes[:keep]), " ") + previewEllipsis, true
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
			return currentBrandPalette().accentGreen
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
	c.bg.FillColor = currentBrandPalette().elevatedPanel
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
