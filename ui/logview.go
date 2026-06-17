package ui

import (
	"strings"
	"sync"

	"github.com/bcicen/ctop/connector/collector"
	"github.com/bcicen/ctop/container"
	"github.com/bcicen/ctop/logging"
	"github.com/bcicen/ctop/models"
	"github.com/bcicen/ctop/widgets"
	ui "github.com/gizak/termui"
	"github.com/mattn/go-runewidth"
)

var log = logging.Init()

type LogView struct {
	ui.Block
	container      *container.Container
	logCollector   collector.LogCollector
	logStream      chan models.Log
	lines          []string
	matches        []int
	searchQuery    string
	currentMatch   int
	searchMode     bool
	input          *widgets.Input
	renderChan     chan bool
	stopChan       chan bool
	mu             sync.Mutex
	padding        [2]int
	streamClosed   bool
}

func NewLogView(c *container.Container) *LogView {
	lv := &LogView{
		Block:      *ui.NewBlock(),
		container:  c,
		lines:      []string{},
		matches:    []int{},
		renderChan: make(chan bool, 64),
		stopChan:   make(chan bool),
		padding:    [2]int{2, 1},
	}

	lv.BorderFg = ui.ThemeAttr("menu.border.fg")
	lv.BorderLabelFg = ui.ThemeAttr("menu.label.fg")
	lv.BorderLabel = "Logs [" + c.GetMeta("name") + "]"
	lv.Height = ui.TermHeight()
	lv.Width = ui.TermWidth()

	lv.logCollector = c.Logs()
	lv.logStream = lv.logCollector.Stream()

	lv.input = widgets.NewInput()
	lv.input.BorderLabel = "Search"
	lv.input.MaxLen = 80
	lv.input.Data = ""

	go lv.readLogs()
	go lv.renderLoop()

	return lv
}

func (lv *LogView) readLogs() {
	for {
		select {
		case logLine, ok := <-lv.logStream:
			if !ok {
				lv.mu.Lock()
				lv.streamClosed = true
				lv.mu.Unlock()
				lv.renderChan <- true
				return
			}
			if logLine.EOF {
				lv.mu.Lock()
				lv.streamClosed = true
				lv.mu.Unlock()
				lv.renderChan <- true
				continue
			}
			lv.mu.Lock()
			lv.lines = append(lv.lines, logLine.Message)
			if len(lv.lines) > 5000 {
				lv.lines = lv.lines[len(lv.lines)-5000:]
			}
			if lv.searchQuery != "" {
				lv.updateMatches()
			}
			lv.mu.Unlock()
			lv.renderChan <- true
		case <-lv.stopChan:
			return
		}
	}
}

func (lv *LogView) updateMatches() {
	oldLen := len(lv.matches)
	lv.matches = []int{}
	for i, line := range lv.lines {
		if strings.Contains(strings.ToLower(line), strings.ToLower(lv.searchQuery)) {
			lv.matches = append(lv.matches, i)
		}
	}
	newLen := len(lv.matches)
	if newLen == 0 {
		lv.currentMatch = 0
	} else if oldLen == 0 {
		lv.currentMatch = 0
	} else if lv.currentMatch >= newLen {
		lv.currentMatch = lv.currentMatch % newLen
	}
}

func (lv *LogView) renderLoop() {
	for {
		select {
		case <-lv.renderChan:
			ui.Render(lv)
		case <-lv.stopChan:
			return
		}
	}
}

func (lv *LogView) Resize() {
	ui.Clear()
	lv.Height = ui.TermHeight()
	lv.Width = ui.TermWidth()
	ui.Render(lv)
}

func (lv *LogView) Stop() {
	close(lv.stopChan)
	lv.logCollector.Stop()
}

func (lv *LogView) StartSearch() {
	lv.searchMode = true
	lv.input.Data = lv.searchQuery
	lv.input.SetY(ui.TermHeight() - lv.input.Height)
	ui.Render(lv)
	ui.Render(lv.input)
}

func (lv *LogView) HandleSearchKey(e ui.Event) {
	ch := strings.Replace(e.Path, "/sys/kbd/", "", -1)
	if ch == "<escape>" {
		lv.searchMode = false
		lv.searchQuery = ""
		lv.matches = []int{}
		lv.currentMatch = 0
		ui.Render(lv)
		return
	}
	if ch == "<enter>" {
		lv.searchMode = false
		lv.searchQuery = lv.input.Data
		lv.mu.Lock()
		lv.currentMatch = 0
		lv.updateMatches()
		lv.mu.Unlock()
		ui.Render(lv)
		return
	}
	lv.input.KeyPress(e)
	ui.Render(lv.input)
}

func (lv *LogView) NextMatch() {
	lv.mu.Lock()
	if len(lv.matches) == 0 {
		lv.mu.Unlock()
		return
	}
	lv.currentMatch = (lv.currentMatch + 1) % len(lv.matches)
	lv.mu.Unlock()
	ui.Render(lv)
}

func (lv *LogView) IsStopped() bool {
	lv.mu.Lock()
	defer lv.mu.Unlock()
	return lv.isStoppedLocked()
}

func (lv *LogView) isStoppedLocked() bool {
	if lv.streamClosed {
		return true
	}
	state := lv.container.GetMeta("state")
	return state != "running"
}

func (lv *LogView) Buffer() ui.Buffer {
	buf := lv.Block.Buffer()

	maxWidth := lv.Width - (lv.padding[0] * 2)
	maxHeight := lv.Height - (lv.padding[1] * 2)

	currentY := lv.Block.Y + lv.padding[1]
	startX := lv.Block.X + lv.padding[0]

	lv.mu.Lock()
	defer lv.mu.Unlock()

	if lv.isStoppedLocked() {
		msg := "container stopped"
		x := startX
		for _, ch := range msg {
			cell := ui.Cell{Ch: ch, Fg: ui.ColorRed, Bg: ui.ColorDefault}
			buf.Set(x, currentY, cell)
			x += runewidth.RuneWidth(ch)
		}
		currentY += 2
		maxHeight -= 2
	}

	totalLines := len(lv.lines)
	var startIdx int
	var highlightLine = -1

	if len(lv.matches) > 0 && lv.currentMatch < len(lv.matches) {
		highlightLine = lv.matches[lv.currentMatch]
		startIdx = highlightLine - maxHeight/2
		if startIdx < 0 {
			startIdx = 0
		}
	} else {
		startIdx = totalLines - maxHeight
		if startIdx < 0 {
			startIdx = 0
		}
	}

	endIdx := startIdx + maxHeight
	if endIdx > totalLines {
		endIdx = totalLines
	}

	displayLines := lv.lines[startIdx:endIdx]

	for displayIdx, line := range displayLines {
		if currentY >= lv.Block.Y+lv.Height-lv.padding[1] {
			break
		}

		actualIdx := startIdx + displayIdx
		isHighlighted := (lv.searchQuery != "" && actualIdx == highlightLine)
		lowQuery := strings.ToLower(lv.searchQuery)

		wrapped := splitLine(line, maxWidth)
		for wi, wrappedLine := range wrapped {
			if currentY >= lv.Block.Y+lv.Height-lv.padding[1] {
				break
			}

			currentX := startX
			runes := []rune(wrappedLine)

			if isHighlighted && wi == 0 {
				lowLine := strings.ToLower(wrappedLine)
				queryRunes := []rune(lowQuery)
				lineRunes := []rune(lowLine)
				matchStart := -1

				for i := 0; i <= len(lineRunes)-len(queryRunes); i++ {
					if string(lineRunes[i:i+len(queryRunes)]) == string(queryRunes) {
						matchStart = i
						break
					}
				}

				for ci, ch := range runes {
					fg := lv.TextFgColor()
					bg := lv.TextBgColor()
					if matchStart >= 0 && ci >= matchStart && ci < matchStart+len(queryRunes) {
						fg = ui.ColorBlack
						bg = ui.ColorYellow
					}
					cell := ui.Cell{Ch: ch, Fg: fg, Bg: bg}
					buf.Set(currentX, currentY, cell)
					currentX += runewidth.RuneWidth(ch)
				}
			} else {
				for _, ch := range runes {
					fg := lv.TextFgColor()
					bg := lv.TextBgColor()
					cell := ui.Cell{Ch: ch, Fg: fg, Bg: bg}
					buf.Set(currentX, currentY, cell)
					currentX += runewidth.RuneWidth(ch)
				}
			}
			currentY++
		}
	}

	return buf
}

func (lv *LogView) TextFgColor() ui.Attribute {
	return ui.ThemeAttr("menu.text.fg")
}

func (lv *LogView) TextBgColor() ui.Attribute {
	return ui.ThemeAttr("menu.text.bg")
}

func (lv *LogView) InSearchMode() bool {
	return lv.searchMode
}

func splitLine(line string, lineSize int) []string {
	if line == "" {
		return []string{""}
	}

	var lines []string
	runes := []rune(line)
	for len(runes) > 0 {
		if len(runes) <= lineSize {
			lines = append(lines, string(runes))
			return lines
		}
		lines = append(lines, string(runes[:lineSize]))
		runes = runes[lineSize:]
	}
	return lines
}
