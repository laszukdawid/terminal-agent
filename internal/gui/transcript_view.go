package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type transcriptBlockView struct {
	kind     transcriptBlockKind
	text     string
	chunks   int
	root     fyne.CanvasObject
	richText *widget.RichText
	textGrid *widget.TextGrid
}

func (p *popupWindow) setTranscript(blocks []transcriptBlock) {
	p.errorBody.Hide()
	p.outputBody.Show()
	p.outputField.Hide()
	p.transcriptBody.Show()
	p.lastRendered = ""
	if !p.reuseTranscriptViews(blocks) {
		p.rebuildTranscriptViews(blocks)
		return
	}
	for i := range blocks {
		if p.transcriptViews[i].sameContent(blocks[i]) {
			continue
		}
		p.refreshTranscriptView(i, blocks[i])
	}
}

func (p *popupWindow) reuseTranscriptViews(blocks []transcriptBlock) bool {
	if len(p.transcriptViews) != len(blocks) {
		return false
	}
	for i := range blocks {
		if p.transcriptViews[i].kind != blocks[i].Kind {
			return false
		}
	}
	return true
}

func (p *popupWindow) rebuildTranscriptViews(blocks []transcriptBlock) {
	p.transcriptViews = make([]transcriptBlockView, len(blocks))
	p.transcriptBody.Objects = p.transcriptBody.Objects[:0]
	for i, block := range blocks {
		view := newTranscriptBlockView(block, i > 0)
		p.transcriptViews[i] = view
		p.transcriptBody.Add(view.root)
	}
	p.transcriptBody.Refresh()
}

func newTranscriptBlockView(block transcriptBlock, showFinalSeparator bool) transcriptBlockView {
	view := transcriptBlockView{kind: block.Kind, text: block.Text, chunks: len(block.Chunks)}
	switch block.Kind {
	case transcriptBlockToolOutput:
		grid := widget.NewTextGrid()
		grid.Scroll = fyne.ScrollNone
		appendTextGridText(grid, block.content())
		view.textGrid = grid
		view.root = borderedBox(grid, currentBrandPalette().border)
	case transcriptBlockFinal:
		rt := widget.NewRichText()
		rt.Wrapping = fyne.TextWrapWord
		view.richText = rt
		if showFinalSeparator {
			view.root = container.NewVBox(brandSeparator(), rt)
		} else {
			view.root = rt
		}
		setTranscriptRichText(rt, block.Text)
	default:
		rt := widget.NewRichText()
		rt.Wrapping = fyne.TextWrapWord
		view.richText = rt
		view.root = rt
		setTranscriptRichText(rt, block.Text)
	}
	return view
}

func (v transcriptBlockView) sameContent(block transcriptBlock) bool {
	if v.kind != block.Kind {
		return false
	}
	if block.Kind == transcriptBlockToolOutput {
		return v.chunks == len(block.Chunks) && v.text == block.Text
	}
	return v.text == block.Text
}

func (p *popupWindow) refreshTranscriptView(index int, block transcriptBlock) {
	view := &p.transcriptViews[index]
	if view.textGrid != nil {
		if view.text == block.Text && len(block.Chunks) >= view.chunks {
			for _, chunk := range block.Chunks[view.chunks:] {
				appendTextGridTextNoRefresh(view.textGrid, chunk)
			}
			view.textGrid.Refresh()
		} else {
			view.textGrid.Rows = nil
			appendTextGridText(view.textGrid, block.content())
		}
		view.text = block.Text
		view.chunks = len(block.Chunks)
		return
	}
	if view.richText != nil {
		view.text = block.Text
		setTranscriptRichText(view.richText, view.text)
	}
}

func appendTextGridText(grid *widget.TextGrid, text string) {
	appendTextGridTextNoRefresh(grid, text)
	grid.Refresh()
}

func appendTextGridTextNoRefresh(grid *widget.TextGrid, text string) {
	if text == "" {
		return
	}
	if len(grid.Rows) == 0 {
		grid.Rows = []widget.TextGridRow{{}}
	}
	for _, r := range text {
		if r == '\n' {
			grid.Rows = append(grid.Rows, widget.TextGridRow{})
			continue
		}
		last := len(grid.Rows) - 1
		grid.Rows[last].Cells = append(grid.Rows[last].Cells, widget.TextGridCell{Rune: r})
	}
}

func setTranscriptRichText(rt *widget.RichText, content string) {
	rt.ParseMarkdown(decorateDollarMarkers(unwrapMarkdownFence(content)))
	rt.Segments = colorizeDollarMarkers(rt.Segments)
	rt.Refresh()
}
