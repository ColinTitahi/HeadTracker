//go:build !tinygo

// Preview renders the head tracker's OLED layouts on your PC, without a board.
//
//	go run ./preview              # writes preview/out/sheet.png + one PNG per screen
//	go run ./preview -scale 6     # bigger pixels
//
// It uses the exact same drawing code as the firmware (src/display), drawn
// into an in-memory 128x32 framebuffer instead of the SSD1306.
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

// Framebuffer implements drivers.Displayer, like the SSD1306 driver does.
type Framebuffer struct {
	px [display.W * display.H]bool
}

func (f *Framebuffer) Size() (int16, int16) { return display.W, display.H }
func (f *Framebuffer) Display() error       { return nil }
func (f *Framebuffer) SetPixel(x, y int16, c color.RGBA) {
	if x < 0 || y < 0 || x >= display.W || y >= display.H {
		return // the real driver ignores out-of-range pixels too
	}
	f.px[int(y)*display.W+int(x)] = c.R|c.G|c.B != 0
}

var (
	bezel = color.RGBA{0x14, 0x16, 0x1a, 255}
	off   = color.RGBA{0x0b, 0x0d, 0x10, 255}
	on    = color.RGBA{0xe6, 0xf2, 0xff, 255}
	paper = color.RGBA{0x24, 0x27, 0x2e, 255}
	ink   = color.RGBA{0xc8, 0xcd, 0xd6, 255}
)

// draw paints the framebuffer into img at (ox,oy), each OLED pixel as an
// s x s square with a 1 px gap so it reads like a real panel.
func (f *Framebuffer) draw(img *image.RGBA, ox, oy, s int) {
	pad := s * 2
	fill(img, ox, oy, display.W*s+2*pad, display.H*s+2*pad, bezel)
	for y := 0; y < display.H; y++ {
		for x := 0; x < display.W; x++ {
			c := off
			if f.px[y*display.W+x] {
				c = on
			}
			gap := 0
			if s >= 4 {
				gap = 1
			}
			fill(img, ox+pad+x*s, oy+pad+y*s, s-gap, s-gap, c)
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

var scenarios = []struct {
	name string
	s    display.Status
}{
	{"pairing", display.Status{Address: "7b:f5:1e:35:de:94", Name: "Hello",
		Channels: [3]uint16{1500, 1500, 1500}, BatteryV: 4.05, Blink: false}},
	{"connected", display.Status{Connected: true, Address: "7b:f5:1e:35:de:94", Name: "Hello",
		Channels: [3]uint16{ch(35), ch(-12), ch(8)}, BatteryV: 3.86}},
	{"looking-hard-left", display.Status{Connected: true, Address: "7b:f5:1e:35:de:94", Name: "Hello",
		Channels: [3]uint16{ch(-80), ch(20), ch(-25)}, BatteryV: 3.74}},
	{"low-battery", display.Status{Connected: true, Address: "7b:f5:1e:35:de:94", Name: "Hello",
		Channels: [3]uint16{ch(3), ch(-2), ch(0)}, BatteryV: 3.45}},
	{"ppm-on-usb", display.Status{PPM: true, Address: "7b:f5:1e:35:de:94", Name: "Hello",
		Channels: [3]uint16{ch(-20), ch(5), ch(0)}, BatteryV: 0}},
}

// --- main -------------------------------------------------------------------

func main() {
	scale := flag.Int("scale", 4, "screen pixels per OLED pixel")
	outDir := flag.String("out", filepath.Join("preview", "out"), "output folder")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fail(err)
	}

	s := *scale
	panelW, panelH := display.W*s+4*s, display.H*s+4*s
	leftCol, header, gap := 46*s, 16*s/2+8, 3*s
	cols, rows := len(display.Layouts), len(scenarios)
	sheet := image.NewRGBA(image.Rect(0, 0, leftCol+cols*(panelW+gap)+gap, header+rows*(panelH+gap)+gap))
	fill(sheet, 0, 0, sheet.Bounds().Dx(), sheet.Bounds().Dy(), paper)

	for c, l := range display.Layouts {
		text(sheet, leftCol+c*(panelW+gap)+gap, 6, s/2, l.Name)
	}
	for r, sc := range scenarios {
		y := header + r*(panelH+gap) + gap
		text(sheet, 6, y+panelH/2-8, s/2, sc.name)
		for c, l := range display.Layouts {
			fb := &Framebuffer{}
			st := sc.s
			l.Render(fb, &st)
			fb.draw(sheet, leftCol+c*(panelW+gap)+gap, y, s)

			one := image.NewRGBA(image.Rect(0, 0, panelW, panelH))
			fb.draw(one, 0, 0, s)
			save(filepath.Join(*outDir, fmt.Sprintf("%s_%s.png", l.Name, sc.name)), one)
		}
	}
	save(filepath.Join(*outDir, "sheet.png"), sheet)

	// Animated GIF per layout: head sweeping around, blinking, battery draining.
	for _, l := range display.Layouts {
		anim := &gif.GIF{}
		pal := color.Palette{bezel, off, on}
		for i := 0; i < 60; i++ {
			t := float64(i) / 60 * 2 * math.Pi
			st := display.Status{Connected: i >= 15, Address: "7b:f5:1e:35:de:94", Name: "Hello",
				Channels: [3]uint16{ch(60 * math.Sin(t)), ch(25 * math.Sin(2*t)), ch(15 * math.Cos(t))},
				BatteryV: 4.1 - float64(i)*0.012, Blink: (i/3)%2 == 0}
			fb := &Framebuffer{}
			l.Render(fb, &st)
			rgba := image.NewRGBA(image.Rect(0, 0, panelW, panelH))
			fb.draw(rgba, 0, 0, s)
			p := image.NewPaletted(rgba.Bounds(), pal)
			for y := 0; y < panelH; y++ {
				for x := 0; x < panelW; x++ {
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
