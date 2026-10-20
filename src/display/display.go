package display

import (
	"image/color"

	"tinygo.org/x/drivers"
	"tinygo.org/x/tinydraw"
	"tinygo.org/x/tinyfont"
	"tinygo.org/x/tinyfont/proggy"
)

// PanelHeight is the OLED in use: 64 for the 0.96" 128x64 modules (including
// the yellow/blue two-colour ones), 32 for the 128x32 modules the upstream
// project was designed for.
const PanelHeight = 64

var BLACK = color.RGBA{0, 0, 0, 255}
var WHITE = color.RGBA{255, 255, 255, 255}

type Text struct {
	row     byte
	text    string
	altText string
	blink   func() bool
}

type Bar struct {
	value int16
	bidir bool
}

type Display struct {
	device drivers.Displayer // the SSD1306 on the board, or an in-memory framebuffer in the preview tool

	blinkCount int
	blinkColor color.RGBA

	showBars bool

	pendingHalf bool // bottom half of the last Show() frame still to be sent

	Texts []*Text
	Bars  [3]Bar
}

func New() *Display {
	return &Display{
		Texts: []*Text{},
		Bars:  [3]Bar{},
	}
}

// NewWithDevice creates a display that draws onto any drivers.Displayer.
// Used by the PC preview tool; the firmware uses New() + Configure().
func NewWithDevice(dev drivers.Displayer) *Display {
	d := New()
	d.device = dev
	return d
}

// Device returns the underlying drawing target, so full-screen layouts
// (see screens.go) can draw directly onto it.
func (d *Display) Device() drivers.Displayer {
	return d.device
}

func (d *Display) RemoveTextRow(row byte) { // TODO use reslice for efficiency
	result := []*Text{}
	for _, t := range d.Texts {
		if t.row == row {
			d.print(t.row, t.text, BLACK)
			d.print(t.row, t.altText, BLACK)
			continue
		}
		result = append(result, t)
	}
	d.Texts = result
}

func (d *Display) RemoveText(text *Text) { // TODO use reslice for efficiency
	result := []*Text{}
	for _, t := range d.Texts {
		if text == nil || t == text {
			d.print(t.row, t.text, BLACK)
			d.print(t.row, t.altText, BLACK)
			continue
		}
		result = append(result, t)
	}
	d.Texts = result
}

func (d *Display) AddText(row byte, text string) *Text {
	t := &Text{row, text, text, nil}
	d.Texts = append(d.Texts, t)
	d.print(t.row, t.text, WHITE)
	return t
}

func (d *Display) SetTextBlinkFunc(text *Text, other string, blink func() bool) *Text {
	for _, t := range d.Texts {
		if t == text {
			t.altText = other
			t.blink = blink
		}
	}
	return text
}

func (d *Display) SetBar(row byte, value int16, bidir bool) {
	d.showBars = true
	d.Bars[row].value = value
	d.Bars[row].bidir = bidir
}

// pageFlusher is implemented by the board's OLED wrapper (display_hw.go):
// it sends only a range of 8-pixel-high pages instead of the whole buffer.
type pageFlusher interface {
	FlushPages(first, last uint8) error
}

// Show draws a full-screen layout and sends it to the panel.
//
// A full 128x64 frame takes ~25ms over 400kHz I2C, longer than the main
// loop's 20ms tick. So on a 64-row panel Show sends the top half now and
// FlushPending sends the bottom half on the next tick (~12ms each). The
// frame buffer isn't touched in between, so there is no tearing.
func (d *Display) Show(l Layout, s *Status) {
	l(d.device, s)
	if pf, ok := d.device.(pageFlusher); ok && PanelHeight == 64 {
		pf.FlushPages(0, 3)
		d.pendingHalf = true
		return
	}
	d.device.Display()
}

// FlushPending sends the bottom half of the last frame, if any. Call it once
// per main-loop tick, before anything that might call Show.
func (d *Display) FlushPending() {
	if !d.pendingHalf {
		return
	}
	d.pendingHalf = false
	if pf, ok := d.device.(pageFlusher); ok {
		pf.FlushPages(4, 7)
	}
}

func (d *Display) Update() {
	d.bars()
	d.blink()
	d.device.Display()
}

func (d *Display) bars() {
	if !d.showBars {
		return
	}
	for row, b := range d.Bars {
		y := int16(row * 5)
		tinydraw.FilledRectangle(d.device, 13, y, 115, 3, BLACK)
		if b.value == 0 {
			continue
		}
		x := int16(64)
		w := int16(b.value)
		if b.bidir {
			x = 64 - w
			w = w * 2
		}
		if b.value < 0 {
			w = -w
			x -= w // move starting point to the left
		}
		tinydraw.FilledRectangle(d.device, x, y, w, 3, WHITE)
	}
}

func (d *Display) blink() {
	if d.blinkCount > 0 {
		d.blinkCount--
		return
	}
	for _, t := range d.Texts {
		if t.blink != nil && t.blink() {
			if d.blinkColor == WHITE {
				d.print(t.row, t.altText, BLACK) // hide alt text
				d.print(t.row, t.text, WHITE)    // show main text
			} else {
				d.print(t.row, t.text, BLACK)    // hide main text
				d.print(t.row, t.altText, WHITE) // show alt text
			}
		}
	}
	d.blinkColor = toggleColor(d.blinkColor)
	d.blinkCount = 5 // blink every 5th iteration (~500mS)
}

func toggleColor(c color.RGBA) color.RGBA {
	if c == WHITE {
		return BLACK
	}
	return WHITE
}

func (d *Display) print(row byte, text string, c color.RGBA) {
	tinyfont.WriteLineRotated(d.device, &proggy.TinySZ8pt7b, 14, 12+int16(row)*16, text, c, tinyfont.NO_ROTATION)
}
