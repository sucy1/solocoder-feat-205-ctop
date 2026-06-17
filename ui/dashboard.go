package ui

import (
	"time"

	"github.com/bcicen/ctop/container"
	"github.com/bcicen/ctop/logging"
	ui "github.com/gizak/termui"
)

var logUI = logging.Init()

func ShowLogView(c *container.Container) {
	ui.Clear()
	ui.DefaultEvtStream.ResetHandlers()
	defer ui.DefaultEvtStream.ResetHandlers()

	lv := NewLogView(c)
	defer lv.Stop()

	ui.Render(lv)

	refreshTicker := time.NewTicker(1 * time.Second)
	defer refreshTicker.Stop()

	go func() {
		for range refreshTicker.C {
			lv.Resize()
		}
	}()

	ui.Handle("/sys/kbd/q", func(ui.Event) {
		ui.StopLoop()
	})
	ui.Handle("/sys/kbd/<escape>", func(e ui.Event) {
		if lv.InSearchMode() {
			lv.HandleSearchKey(e)
		} else {
			ui.StopLoop()
		}
	})
	ui.Handle("/sys/kbd/C-c", func(ui.Event) {
		ui.StopLoop()
	})
	ui.Handle("/sys/kbd/", func(e ui.Event) {
		if lv.InSearchMode() {
			lv.HandleSearchKey(e)
		}
	})
	ui.Handle("/sys/kbd//", func(e ui.Event) {
		if !lv.InSearchMode() {
			lv.StartSearch()
		} else {
			lv.HandleSearchKey(e)
		}
	})
	ui.Handle("/sys/kbd/n", func(e ui.Event) {
		if !lv.InSearchMode() {
			lv.NextMatch()
		} else {
			lv.HandleSearchKey(e)
		}
	})

	ui.Handle("/sys/wnd/resize", func(e ui.Event) {
		lv.Resize()
	})

	ui.Loop()
}
