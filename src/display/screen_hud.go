package display

// HUD layout for 128x64 OLEDs, designed around the common two-colour panels:
// the top 16 rows glow yellow and the remaining 48 rows glow blue. The colour
// is fixed in the glass by row, so we use position to carry meaning:
//
//	rows  0-15  YELLOW  pan tape: where you are looking left/right
//	                    (while pairing: the Bluetooth address to look for)
//	rows 17-46  BLUE    motion box: dot = pan/tilt, bar through it = roll,
//	                    tilt and roll readouts in the corners
//	rows 48-63  BLUE    status: PPM / Bluetooth, battery gauge and percentage
//
// The pan marker in the yellow band sits directly above the dot in the box,
// so the two read as one graph.

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
	hudYellowRows = 16 // height of the yellow band on two-colour panels

	hudBoxY   = 17 // motion box top (first blue row is 16; leave one blank)
	hudBoxH   = 30 // rows 17..46
	hudStripY = 48 // status strip, rows 48..63

	hudPanRange  = 90.0 // degrees at the left/right edge of the tape and box
	hudTiltRange = 60.0 // degrees at the top/bottom of the box
	hudHalfW     = 60   // pixels from centre to +-hudPanRange
	hudCX        = 64   // shared centre column of tape and box
)

func LayoutHUD(dev drivers.Displayer, s *Status) {
	clearFrame(dev)

	pan := float64(channelToDegrees(s.Channels[0]))
	tilt := float64(channelToDegrees(s.Channels[1]))
	roll := float64(channelToDegrees(s.Channels[2]))
	px := hudCX + int16(clampF(pan/hudPanRange*hudHalfW, -hudHalfW, hudHalfW))

	// --- Yellow band ----------------------------------------------------------
	if !s.PPM && !s.Connected {
		hudPairingBand(dev, s)
	} else {
		hudPanTape(dev, px, int(pan))
	}

	// --- Blue motion box --------------------------------------------------------
	hudMotionBox(dev, px, tilt, roll)

	// --- Blue status strip ------------------------------------------------------
	hudStatusStrip(dev, s)
}

// Pan tape: tick scale along the bottom of the yellow band, a marker that
// slides with your head, and the pan angle riding next to the marker.
func hudPanTape(dev drivers.Displayer, px int16, pan int) {
	// scale: ticks every 30 degrees, taller at centre and ends
	for d := -90; d <= 90; d += 30 {
		x := hudCX + int16(float64(d)/hudPanRange*hudHalfW)
		top := int16(13)
		if d == 0 || d == -90 || d == 90 {
			top = 11
		}
		tinydraw.Line(dev, x, top, x, 15, WHITE)
	}
	tinydraw.Line(dev, hudCX-hudHalfW, 15, hudCX+hudHalfW, 15, WHITE)

	// marker: downward triangle pointing at the tape
	tinydraw.FilledTriangle(dev, px-4, 2, px+4, 2, px, 9, WHITE)

	// pan readout beside the marker, on whichever side has room
	txt := signed(pan)
	_, w := tinyfont.LineWidth(&proggy.TinySZ8pt7b, txt)
	tw := int16(w) + 4 // text + degree sign
	tx := px + 7
	if tx+tw > W-1 {
		tx = px - 7 - tw
	}
	tinyfont.WriteLine(dev, &proggy.TinySZ8pt7b, tx, 9, txt, WHITE)
	degreeSign(dev, tx+int16(w)+1, 1)
}

// While pairing, the yellow band shows what to pick in the radio's list.
func hudPairingBand(dev drivers.Displayer, s *Status) {
	if !s.Blink {
		btIcon(dev, 0, 4)
	}
	tinyfont.WriteLine(dev, &proggy.TinySZ8pt7b, 8, 11, s.Address, WHITE)
}

// Motion box: full-width frame, dotted crosshair, tilt/roll readouts,
// dot at (pan, tilt) with a roll bar through it.
func hudMotionBox(dev drivers.Displayer, px int16, tilt, roll float64) {
	x0, x1 := int16(hudCX-hudHalfW-3), int16(hudCX+hudHalfW+3)
	tinydraw.Rectangle(dev, x0, hudBoxY, x1-x0+1, hudBoxH, WHITE)

	cy := int16(hudBoxY + hudBoxH/2)
	halfH := int16(hudBoxH/2 - 3)
	for y := int16(hudBoxY + 2); y < hudBoxY+hudBoxH-1; y += 3 {
		dev.SetPixel(hudCX, y, WHITE)
	}
	for x := x0 + 2; x < x1; x += 3 {
		dev.SetPixel(x, cy, WHITE)
	}

	py := cy - int16(clampF(tilt/hudTiltRange*float64(halfH), -float64(halfH), float64(halfH)))

	// tilt/roll readouts in the top corners, hidden while the dot is over them
	nearTop := py < hudBoxY+13
	t := "T" + signed(int(tilt))
	_, tw := tinyfont.LineWidth(&tinyfont.TomThumb, t)
	if !(nearTop && px < x0+3+int16(tw)+6) {
		tinyfont.WriteLine(dev, &tinyfont.TomThumb, x0+3, hudBoxY+7, t, WHITE)
	}
	r := "R" + signed(int(roll))
	_, rw := tinyfont.LineWidth(&tinyfont.TomThumb, r)
	if !(nearTop && px > x1-2-int16(rw)-6) {
		tinyfont.WriteLine(dev, &tinyfont.TomThumb, x1-2-int16(rw), hudBoxY+7, r, WHITE)
	}

	// roll bar, 2 px thick
	a := roll * math.Pi / 180
	dx, dy := int16(math.Round(11*math.Cos(a))), int16(math.Round(11*math.Sin(a)))
	tinydraw.Line(dev, px-dx, py-dy, px+dx, py+dy, WHITE)
	tinydraw.Line(dev, px-dx, py-dy+1, px+dx, py+dy+1, WHITE)

	// dot, with a black ring so it stands out from the crosshair and bar
	tinydraw.FilledCircle(dev, px, py, 4, BLACK)
	tinydraw.FilledCircle(dev, px, py, 3, WHITE)
}

// Status strip: link mode on the left, big battery gauge + percentage on the right.
func hudStatusStrip(dev drivers.Displayer, s *Status) {
	big := &freesans.Bold9pt7b
	baseline := int16(62)

	// left: mode
	switch {
	case s.PPM:
		tinyfont.WriteLine(dev, big, 0, baseline, "PPM", WHITE)
	case s.Connected:
		btIconLarge(dev, 2, hudStripY+1)
	default: // pairing: blink
		if !s.Blink {
			btIconLarge(dev, 2, hudStripY+1)
		}
	}

	// right: battery
	if s.BatteryV <= 0 {
		txt := "USB"
		_, w := tinyfont.LineWidth(big, txt)
		tinyfont.WriteLine(dev, big, W-int16(w), baseline, txt, WHITE)
		return
	}
	pct := BatteryPercent(s.BatteryV)
	low := s.BatteryV < LowBatteryV
	txt := strconv.Itoa(pct) + "%"
	_, w := tinyfont.LineWidth(big, txt)
	tinyfont.WriteLine(dev, big, W-int16(w), baseline, txt, WHITE)

	if low && s.Blink {
		return // flash the gauge when low; the percentage stays readable
	}
	const bw, bh = 30, 14
	bx := W - int16(w) - 5 - bw - 3
	by := int16(hudStripY + 1)
	tinydraw.Rectangle(dev, bx, by, bw, bh, WHITE)
	tinydraw.Rectangle(dev, bx+1, by+1, bw-2, bh-2, WHITE)     // 2 px wall
	tinydraw.FilledRectangle(dev, bx+bw, by+4, 3, bh-8, WHITE) // nub

	// four segments, lit by charge (a part-lit segment shows as a partial fill)
	inner := int16(bw - 6)
	segW := (inner - 3) / 4
	lit := float64(pct) / 25
	for i := int16(0); i < 4; i++ {
		f := clampF(lit-float64(i), 0, 1)
		if f <= 0 {
			break
		}
		sw := int16(math.Round(float64(segW) * f))
		if sw < 1 {
			sw = 1
		}
		tinydraw.FilledRectangle(dev, bx+3+i*(segW+1), by+3, sw, bh-6, WHITE)
	}
}

// btIconLarge draws a 9x14 Bluetooth rune with its top-left at (x,y).
func btIconLarge(dev drivers.Displayer, x, y int16) {
	rows := [14]string{
		"...##....",
		"...###...",
		"...#.##..",
		"##.#..##.",
		".###.##..",
		"..####...",
		"...##....",
		"...##....",
		"..####...",
		".###.##..",
		"##.#..##.",
		"...#.##..",
		"...###...",
		"...##....",
	}
	for j, r := range rows {
		for i, c := range r {
			if c == '#' {
				dev.SetPixel(x+int16(i), y+int16(j), WHITE)
			}
		}
	}
}
