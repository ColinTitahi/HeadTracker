//go:build tinygo

package display

import (
	"machine"

	"tinygo.org/x/drivers/ssd1306"
)

// Configure sets up the real SSD1306 128x32 OLED over I2C.
// Only compiled for the board (TinyGo); the PC preview uses NewWithDevice instead.
func (d *Display) Configure() {
	machine.I2C0.Configure(machine.I2CConfig{
		Frequency: 400 * machine.KHz,
		SDA:       machine.SDA0_PIN,
		SCL:       machine.SCL0_PIN,
	})
	dev := ssd1306.NewI2C(machine.I2C0)
	dev.Configure(ssd1306.Config{
		Address: ssd1306.Address_128_32,
		Width:   128,
		Height:  32,
	})
	dev.ClearDisplay()
	d.device = dev
}
