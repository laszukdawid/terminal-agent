package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// showTestDialog presents the dev-only Test menu. Each entry runs its action
// and closes the dialog. The dialog is only reachable when the GUI is started
// in dev mode.
func (p *popupWindow) showTestDialog(tests []devTest) {
	win := p.window
	var dlg dialog.Dialog
	buttons := make([]fyne.CanvasObject, 0, len(tests))
	for _, t := range tests {
		run := t.run
		btn := widget.NewButton(t.name, func() {
			dlg.Hide()
			if run != nil {
				run()
			}
		})
		btn.Alignment = widget.ButtonAlignLeading
		buttons = append(buttons, btn)
	}
	content := container.NewVBox(buttons...)
	dlg = dialog.NewCustom("Test", "Close", content, win)
	dlg.Resize(fyne.NewSize(360, 0))
	dlg.Show()
}
