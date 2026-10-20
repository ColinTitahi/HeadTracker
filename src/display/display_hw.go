//go:build tinygo

package display

import (
	"machine"

	"tinygo.org/x/drivers/ssd1306"
)

const oledAddress = ssd1306.Address_128_32 // 0x3C, used by both 128x32 and most 128x64 modules

// Configure sets up the real SSD1306 OLED over I2C.
// Only compiled for the board (TinyGo); the PC preview uses NewWithDevice instead.
func (d *Display) Configure() {
	machine.I2C0.Configure(machine.I2CConfig{
		Frequency: 400 * machine.KHz,
		SDA:       machine.SDA0_PIN,
		SCL:       machine.SCL0_PIN,
	})
	dev := &oled{Device: ssd1306.NewI2C(machine.I2C0)}
	dev.Configure(ssd1306.Config{
		Address: oledAddress,
		Width:   128,
		Height:  PanelHeight,
	})
	dev.ClearDisplay()
	d.device = dev
}

// oled adds partial (page range) updates to the ssd1306 driver,
// which otherwise always sends the whole frame.
type oled struct {
	*ssd1306.Device
	scratch [1 + 128*4]byte // data-mode byte + up to 4 pages, static so no allocations
}

// FlushPages sends pages first..last (each page = 8 rows) to the panel.
func (o *oled) FlushPages(first, last uint8) error {
	o.Command(ssd1306.COLUMNADDR)
	o.Command(0)
	o.Command(127)
	o.Command(ssd1306.PAGEADDR)
	o.Command(first)
	o.Command(last)

	buf := o.GetBuffer()
	from, to := int(first)*128, int(last+1)*128
	if to > len(buf) {
		to = len(buf)
	}
	if to-from > len(o.scratch)-1 {
		to = from + len(o.scratch) - 1
	}
	o.scratch[0] = 0x40 // data mode
	n := copy(o.scratch[1:], buf[from:to])
	return machine.I2C0.Tx(oledAddress, o.scratch[:1+n], nil)
}
