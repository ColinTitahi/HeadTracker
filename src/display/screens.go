package display

// Full-screen layouts for normal (tracking) mode.
//
// Each layout redraws the whole frame from a Status snapshot.
// They are plain Go (no TinyGo/hardware imports), so the same code runs on
// the board and in the PC preview tool (go run ./preview).

import (
	"math"
	"strconv"

	"tinygo.org/x/drivers"
	"tinygo.org/x/tinydraw"
	"tinygo.org/x/tinyfont"
	"tinygo.org/x/tinyfont/freesans"
	"tinygo.org/x/tinyfont/proggy"
)

const (
	W = 128
	H = 32 // height used by the 128x32 layout (C); see PanelHeight for the panel in use
)

// Status is everything a layout may show.
type Status struct {
	PPM       bool      // wired PPM output instead of Bluetooth
	Connected bool      // Bluetooth link to the radio is up
	Address   string    // Bluetooth address, "7b:f5:1e:35:de:94"
	Name      string    // device name set over BLE (default "Hello")
	Channels  [3]uint16 // pan, tilt, roll as sent to the radio, 988..2012 us
	BatteryV  float64   // battery voltage; <= 0 means no reading (e.g. Nano 33 BLE)
	Blink     bool      // blink phase, toggled by the caller every ~500 ms
}

// Layout draws a complete frame.
type Layout func(dev drivers.Displayer, s *Status)

// Layouts available for selection, with the panel height each is drawn for.
var Layouts = []struct {
	Name   string
	Height int16
	Render Layout
}{
	{"HUD-128x64", 64, LayoutHUD},
	{"C-attitude-128x32", 32, LayoutAttitude},
}

// LowBatteryV is the voltage below which layouts flag a low battery.
const LowBatteryV = 3.50

// ---------------------------------------------------------------------------
// C (128x32 panels): Attitude -- big state text, battery, and a 32x32 "where am I looking"
// box: dot = pan/tilt, line through it = roll.

func LayoutAttitude(dev drivers.Displayer, s *Status) {
	clearFrame(dev)

	// Headline
	head := s.Name
	if head == "" {
		head = "LINKED"
	}
	switch {
	case s.PPM:
		head = "PPM"
	case !s.Connected:
		head = "PAIRING"
	}
	big := &freesans.Bold9pt7b
	if _, w := tinyfont.LineWidth(big, head); w > 92 {
		tinyfont.WriteLine(dev, &proggy.TinySZ8pt7b, 0, 10, head, WHITE)
	} else if s.PPM || s.Connected || !s.Blink {
		tinyfont.WriteLine(dev, big, 0, 13, head, WHITE)
	}

	// Second line: what to pair with, or link state
	sub := "linked to radio"
	if s.PPM {
		sub = "wired trainer out"
	} else if !s.Connected {
		sub = s.Address
	}
	tinyfont.WriteLine(dev, &tinyfont.TomThumb, 0, 21, sub, WHITE)

	// Battery
	batteryGauge(dev, 0, 25, 22, 7, s, true)

	// Attitude box
	bx, by := int16(W-32), int16(0)
	tinydraw.Rectangle(dev, bx, by, 32, 32, WHITE)
	for k := int16(2); k < 31; k += 3 { // dotted crosshair
		dev.SetPixel(bx+16, by+k, WHITE)
		dev.SetPixel(bx+k, by+16, WHITE)
	}
	pan := float64(channelToDegrees(s.Channels[0]))
	tilt := float64(channelToDegrees(s.Channels[1]))
	roll := float64(channelToDegrees(s.Channels[2]))
	px := bx + 16 + int16(clampF(pan/90*13, -13, 13))
	py := by + 16 + int16(clampF(-tilt/90*13, -13, 13))
	r := roll * math.Pi / 180
	dx, dy := int16(math.Round(7*math.Cos(r))), int16(math.Round(7*math.Sin(r)))
	tinydraw.Line(dev, px-dx, py-dy, px+dx, py+dy, WHITE)
	tinydraw.FilledRectangle(dev, px-1, py-1, 3, 3, WHITE)
}

// ---------------------------------------------------------------------------
// Battery

// BatteryPercent converts a 1S LiPo resting voltage to an approximate charge.
func BatteryPercent(v float64) int {
	table := [...]struct{ v, p float64 }{
		{3.30, 0}, {3.60, 5}, {3.69, 10}, {3.73, 20}, {3.77, 30},
		{3.80, 40}, {3.84, 50}, {3.87, 60}, {3.95, 70}, {4.02, 80},
		{4.08, 90}, {4.20, 100},
	}
	if v <= table[0].v {
		return 0
	}
	for i := 1; i < len(table); i++ {
		if v <= table[i].v {
			a, b := table[i-1], table[i]
			return int(a.p + (v-a.v)/(b.v-a.v)*(b.p-a.p) + 0.5)
		}
	}
	return 100
}

// batteryGauge draws a battery icon at (x,y) of height h, followed by text.
// textW > 0 reserves width for "72%" or "3.82V" depending on space; when
// withText is false only the icon is drawn (iconW = textW in that case).
func batteryGauge(dev drivers.Displayer, x, y, textW, h int16, s *Status, withText bool) {
	if s.BatteryV <= 0 {
		if withText {
			tinyfont.WriteLine(dev, &tinyfont.TomThumb, x, y+h-1, "USB", WHITE)
		}
		return
	}
	pct := BatteryPercent(s.BatteryV)
	low := s.BatteryV < LowBatteryV
	if low && s.Blink {
		return // flash the whole gauge when low
	}
	iw := int16(13)
	if !withText && textW > 0 {
		iw = textW - 2
	}
	tinydraw.Rectangle(dev, x, y, iw, h, WHITE)
	tinydraw.FilledRectangle(dev, x+iw, y+h/2-1, 2, 3-h%2, WHITE) // nub
	fill := int16(pct) * (iw - 4) / 100
	if fill < 1 && pct > 0 {
		fill = 1
	}
	if fill > 0 {
		tinydraw.FilledRectangle(dev, x+2, y+2, fill, h-4, WHITE)
	}
	if withText {
		tinyfont.WriteLine(dev, &tinyfont.TomThumb, x+iw+4, y+h-1, batteryText(s), WHITE)
	}
}

func batteryText(s *Status) string {
	if s.BatteryV <= 0 {
		return "USB"
	}
	txt := strconv.Itoa(BatteryPercent(s.BatteryV)) + "%"
	if s.BatteryV < LowBatteryV {
		txt = "LOW " + txt
	}
	return txt
}

// batteryGaugeWidth is the width of a 13 px icon + text, for right-aligning.
func batteryGaugeWidth(s *Status) int16 {
	_, w := tinyfont.LineWidth(&tinyfont.TomThumb, batteryText(s))
	if s.BatteryV <= 0 {
		return int16(w)
	}
	return 13 + 4 + int16(w)
}

// ---------------------------------------------------------------------------
// Drawing helpers

// clearFrame blanks the frame buffer (no I2C traffic; Display() does the transfer).
func clearFrame(dev drivers.Displayer) {
	if c, ok := dev.(interface{ ClearBuffer() }); ok {
		c.ClearBuffer() // fast path on the SSD1306
		return
	}
	w, h := dev.Size()
	for y := int16(0); y < h; y++ {
		for x := int16(0); x < w; x++ {
			dev.SetPixel(x, y, BLACK)
		}
	}
}

// bar draws a horizontal bar of height h starting at x0, extending v pixels
// (negative = to the left).
func bar(dev drivers.Displayer, x0, y, v, h int16) {
	if v == 0 {
		return
	}
	if v < 0 {
		tinydraw.FilledRectangle(dev, x0+v, y, -v, h, WHITE)
	} else {
		tinydraw.FilledRectangle(dev, x0+1, y, v, h, WHITE)
	}
}

func centered(dev drivers.Displayer, f tinyfont.Fonter, cx, y int16, s string) {
	_, w := tinyfont.LineWidth(f, s)
	tinyfont.WriteLine(dev, f, cx-int16(w)/2, y, s, WHITE)
}

func rightEdge(f tinyfont.Fonter, s string) int16 {
	_, w := tinyfont.LineWidth(f, s)
	return int16(w) / 2
}

func degreeSign(dev drivers.Displayer, x, y int16) {
	dev.SetPixel(x+1, y, WHITE)
	dev.SetPixel(x, y+1, WHITE)
	dev.SetPixel(x+2, y+1, WHITE)
	dev.SetPixel(x+1, y+2, WHITE)
}

// btIcon draws a 5x7 Bluetooth rune with its top-left at (x,y).
func btIcon(dev drivers.Displayer, x, y int16) {
	rows := [7]string{
		"..#..",
		"..##.",
		"#.#.#",
		".###.",
		"#.#.#",
		"..##.",
		"..#..",
	}
	for j, r := range rows {
		for i, c := range r {
			if c == '#' {
				dev.SetPixel(x+int16(i), y+int16(j), WHITE)
			}
		}
	}
}

func channelToDegrees(ch uint16) int {
	// firmware: ch = 1500 + angle * 512/pi  ->  deg = (ch-1500) * 180/512
	return int(math.Round(float64(int(ch)-1500) * 180 / 512))
}

func signed(v int) string {
	if v > 0 {
		return "+" + strconv.Itoa(v)
	}
	return strconv.Itoa(v)
}

func shortAddr(a string) string {
	if len(a) >= 8 {
		return a[len(a)-8:] // last 3 bytes are enough to tell boards apart
	}
	return a
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampF(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}
