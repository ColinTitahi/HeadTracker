package display

import (
	"image/color"

	"tinygo.org/x/drivers"
	"tinygo.org/x/tinydraw"
	"tinygo.org/x/tinyfont"
	"tinygo.org/x/tinyfont/proggy"
)

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

// Show draws a full-screen layout and sends it to the panel.
func (d *Display) Show(l Layout, s *Status) {
	l(d.device, s)
	d.device.Display()
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
