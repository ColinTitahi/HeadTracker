//go:build !tinygo

// Preview renders the head tracker's OLED layouts on your PC, without a board.
//
//	go run ./preview              # writes preview/out/sheet.png + one PNG per screen
//	go run ./preview -scale 6     # bigger pixels
//	go run ./preview -mono        # 128x64 panels in a single colour (no yellow band)
//
// It uses the exact same drawing code as the firmware (src/display), drawn
// into an in-memory framebuffer instead of the SSD1306.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"github.com/ysoldak/HeadTracker/src/display"
	"tinygo.org/x/tinyfont"
	"tinygo.org/x/tinyfont/proggy"
)

// --- fake OLED --------------------------------------------------------------

const maxH = 64

// Framebuffer implements drivers.Displayer, like the SSD1306 driver does.
type Framebuffer struct {
	h  int16
	px [display.W * maxH]bool
}

func (f *Framebuffer) Size() (int16, int16) { return display.W, f.h }
func (f *Framebuffer) Display() error       { return nil }
func (f *Framebuffer) SetPixel(x, y int16, c color.RGBA) {
	if x < 0 || y < 0 || x >= display.W || y >= f.h {
		return // the real driver ignores out-of-range pixels too
	}
	f.px[int(y)*display.W+int(x)] = c.R|c.G|c.B != 0
}

var (
	bezel  = color.RGBA{0x14, 0x16, 0x1a, 255}
	off    = color.RGBA{0x0b, 0x0d, 0x10, 255}
	white  = color.RGBA{0xe6, 0xf2, 0xff, 255}
	yellow = color.RGBA{0xff, 0xd2, 0x1f, 255}
	blue   = color.RGBA{0x3c, 0xb4, 0xff, 255}
	paper  = color.RGBA{0x24, 0x27, 0x2e, 255}
	ink    = color.RGBA{0xc8, 0xcd, 0xd6, 255}

	mono bool
)

// Two-colour 128x64 panels: top 16 rows yellow, the rest blue, with a thin
// physical gap between the two areas.
const yellowRows = 16

func (f *Framebuffer) twoColour() bool { return f.h == 64 && !mono }

// panelSize is the drawn size of a panel (bezel included) at scale s.
func panelSize(h int16, s int) (int, int) {
	extra := 0
	if h == 64 && !mono {
		extra = s // the gap between yellow and blue
	}
	return display.W*s + 4*s, int(h)*s + 4*s + extra
}

// draw paints the framebuffer into img at (ox,oy), each OLED pixel as an
// s x s square with a 1 px gap so it reads like a real panel.
func (f *Framebuffer) draw(img *image.RGBA, ox, oy, s int) {
	pad := s * 2
	w, h := panelSize(f.h, s)
	fill(img, ox, oy, w, h, bezel)
	gap := 0
	if s >= 4 {
		gap = 1
	}
	for y := 0; y < int(f.h); y++ {
		on, dy := white, 0
		if f.twoColour() {
			on = blue
			if y < yellowRows {
				on = yellow
			} else {
				dy = s
			}
		}
		for x := 0; x < display.W; x++ {
			c := off
			if f.px[y*display.W+x] {
				c = on
			}
			fill(img, ox+pad+x*s, oy+pad+y*s+dy, s-gap, s-gap, c)
		}
	}
}

func fill(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			img.SetRGBA(i, j, c)
		}
	}
}

// label writes text onto the sheet using a tinyfont font, scaled up.
type label struct {
	img        *image.RGBA
	ox, oy, sc int
}

func (l label) Size() (int16, int16) { return 32767, 32767 }
func (l label) Display() error       { return nil }
func (l label) SetPixel(x, y int16, c color.RGBA) {
	fill(l.img, l.ox+int(x)*l.sc, l.oy+int(y)*l.sc, l.sc, l.sc, c)
}

func text(img *image.RGBA, x, y, sc int, s string) {
	tinyfont.WriteLine(label{img, x, y, sc}, &proggy.TinySZ8pt7b, 0, 8, s, ink)
}

// --- scenarios --------------------------------------------------------------

func ch(deg float64) uint16 { return uint16(1500 + deg*512/180) }

const addr = "7b:f5:1e:35:de:94"

var scenarios = []struct {
	name string
	s    display.Status
}{
	{"pairing", display.Status{Address: addr, Name: "Hello",
		Channels: [3]uint16{ch(10), ch(5), ch(0)}, BatteryV: 4.05}},
	{"connected", display.Status{Connected: true, Address: addr, Name: "Hello",
		Channels: [3]uint16{ch(35), ch(-12), ch(8)}, BatteryV: 3.86}},
	{"hard-left-up", display.Status{Connected: true, Address: addr, Name: "Hello",
		Channels: [3]uint16{ch(-80), ch(30), ch(-25)}, BatteryV: 3.74}},
	{"low-battery", display.Status{Connected: true, Address: addr, Name: "Hello",
		Channels: [3]uint16{ch(3), ch(-2), ch(0)}, BatteryV: 3.45}},
	{"ppm-full", display.Status{PPM: true, Address: addr, Name: "Hello",
		Channels: [3]uint16{ch(-20), ch(5), ch(0)}, BatteryV: 4.19}},
	{"ppm-on-usb", display.Status{PPM: true, Address: addr, Name: "Hello",
		Channels: [3]uint16{ch(60), ch(-40), ch(15)}, BatteryV: 0}},
}

// --- main -------------------------------------------------------------------

func main() {
	scale := flag.Int("scale", 4, "screen pixels per OLED pixel")
	outDir := flag.String("out", filepath.Join("preview", "out"), "output folder")
	flag.BoolVar(&mono, "mono", false, "draw 128x64 panels single-colour instead of yellow/blue")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fail(err)
	}

	s := *scale
	cellW, cellH := panelSize(maxH, s)
	leftCol, header, gap := 46*s, 16*s/2+8, 3*s
	cols, rows := len(display.Layouts), len(scenarios)
	sheet := image.NewRGBA(image.Rect(0, 0, leftCol+cols*(cellW+gap)+gap, header+rows*(cellH+gap)+gap))
	fill(sheet, 0, 0, sheet.Bounds().Dx(), sheet.Bounds().Dy(), paper)

	for c, l := range display.Layouts {
		text(sheet, leftCol+c*(cellW+gap)+gap, 6, s/2, l.Name)
	}
	for r, sc := range scenarios {
		y := header + r*(cellH+gap) + gap
		text(sheet, 6, y+cellH/2-8, s/2, sc.name)
		for c, l := range display.Layouts {
			fb := &Framebuffer{h: l.Height}
			st := sc.s
			l.Render(fb, &st)
			fb.draw(sheet, leftCol+c*(cellW+gap)+gap, y, s)

			w, h := panelSize(l.Height, s)
			one := image.NewRGBA(image.Rect(0, 0, w, h))
			fb.draw(one, 0, 0, s)
			save(filepath.Join(*outDir, fmt.Sprintf("%s_%s.png", l.Name, sc.name)), one)
		}
	}
	save(filepath.Join(*outDir, "sheet.png"), sheet)

	// Animated GIF per layout: pairing then connected, head sweeping around,
	// battery draining into the low-battery warning.
	for _, l := range display.Layouts {
		anim := &gif.GIF{}
		pal := color.Palette{bezel, off, white, yellow, blue}
		w, h := panelSize(l.Height, s)
		for i := 0; i < 80; i++ {
			t := float64(i) / 80 * 2 * math.Pi
			st := display.Status{Connected: i >= 15, Address: addr, Name: "Hello",
				Channels: [3]uint16{ch(70 * math.Sin(t)), ch(35 * math.Sin(2*t)), ch(20 * math.Cos(t))},
				BatteryV: 4.15 - float64(i)*0.009, Blink: (i/5)%2 == 1}
			fb := &Framebuffer{h: l.Height}
			l.Render(fb, &st)
			rgba := image.NewRGBA(image.Rect(0, 0, w, h))
			fb.draw(rgba, 0, 0, s)
			p := image.NewPaletted(rgba.Bounds(), pal)
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					p.Set(x, y, rgba.At(x, y))
				}
			}
			anim.Image = append(anim.Image, p)
			anim.Delay = append(anim.Delay, 10) // 100 ms, same as the firmware's refresh
		}
		f, err := os.Create(filepath.Join(*outDir, l.Name+".gif"))
		if err != nil {
			fail(err)
		}
		if err := gif.EncodeAll(f, anim); err != nil {
			fail(err)
		}
		f.Close()
	}

	fmt.Println("Wrote", filepath.Join(*outDir, "sheet.png"), "plus per-screen PNGs and per-layout GIFs")
}

func save(path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "preview:", err)
	os.Exit(1)
}
