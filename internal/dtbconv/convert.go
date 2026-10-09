package dtbconv

import (
	"errors"
	"fmt"
	"strings"
)

// Family is the board family. Each family has its own base DTB.
type Family string

const (
	Genuine Family = "r36s" // genuine R36S wiring (buttons on GPIO1, LCD on LDO8)
	EE      Family = "ee"   // "EE clones": G80, K36, HG36, R36 Ultra/Max, ... (EmuELEC on the stock card)
	Soy     Family = "soy"  // "Soy Sauce" Y3506 boards
)

func (f Family) Label() string {
	switch f {
	case Genuine:
		return "R36S (genuine wiring)"
	case EE:
		return "EE clone (G80 / K36 family)"
	case Soy:
		return "Soy Sauce clone"
	}
	return string(f)
}

// Bases are our own DTBs the conversion starts from, one per family, plus the
// R36S U-Boot panel DTB.
type Bases struct {
	Genuine, EE, Soy, UBoot []byte
}

func (b Bases) forFamily(f Family) []byte {
	switch f {
	case Genuine:
		return b.Genuine
	case EE:
		return b.EE
	case Soy:
		return b.Soy
	}
	return nil
}

// Result is a finished conversion.
type Result struct {
	Kernel        []byte // rk3326-r36s-android.dtb
	UBoot         []byte // rg351mp/rg351v/rg351p-kernel.dtb (boot logo panel); nil if there is no U-Boot base
	Family        Family
	Model         string // model string of the input
	Width, Height uint32
	Square        bool
	Notes         []string // what was copied or decided, plain language
	Warnings      []string // things a person should know before trusting the result
}

// Error is a conversion refusal with a plain-language reason.
type Error struct{ Reason string }

func (e *Error) Error() string { return e.Reason }

func refuse(format string, args ...any) error { return &Error{fmt.Sprintf(format, args...)} }

// ErrAlreadyAndroid means the input already is an andr36oid (or 351droid-style) DTB.
var ErrAlreadyAndroid = &Error{"This file already is an Android DTB for this kind of console. Use it as a custom DTB instead of converting it."}

// IsRefusal reports whether e is a plain-language refusal (as opposed to a bug).
func IsRefusal(e error) bool {
	var r *Error
	return errors.As(e, &r)
}

const (
	pathDSI   = "/dsi@ff450000"
	pathPanel = "/dsi@ff450000/panel@0"
	pathRK817 = "/i2c@ff180000/pmic@20"
	pathRegs  = "/i2c@ff180000/pmic@20/regulators"
	pathSDIO  = "/dwmmc@ff380000"
	pathSDMMC = "/dwmmc@ff370000"
)

type conv struct {
	in, out *Tree
	res     *Result
	inPad   *Node
	padKind string // "arkos" or "emuelec"
	buttons map[uint32]GPIO
}

func (c *conv) note(format string, args ...any) {
	c.res.Notes = append(c.res.Notes, fmt.Sprintf(format, args...))
}

func (c *conv) warn(format string, args ...any) {
	c.res.Warnings = append(c.res.Warnings, fmt.Sprintf(format, args...))
}

// Convert turns an ArkOS/dArkOS or stock EmuELEC DTB into an andr36oid DTB.
// It refuses (returns an *Error) when the input can't be matched safely.
func Convert(input []byte, bases Bases) (*Result, error) {
	in, e := Parse(input)
	if e != nil {
		return nil, refuse("This is not a valid DTB file (%v).", e)
	}
	compat := in.Root.Strings("compatible")
	ok := false
	for _, s := range compat {
		if s == "rockchip,rk3326" || s == "rockchip,px30" {
			ok = true
		}
	}
	if !ok {
		return nil, refuse("This DTB is not for an RK3326 console (compatible: %s).", strings.Join(compat, ", "))
	}
	c := &conv{in: in, res: &Result{Model: in.Root.String("model")}}
	if e := c.findJoypad(); e != nil {
		return nil, e
	}
	fam, e := c.detectFamily()
	if e != nil {
		return nil, e
	}
	c.res.Family = fam
	baseBytes := bases.forFamily(fam)
	if baseBytes == nil {
		return nil, fmt.Errorf("no base DTB for family %s", fam)
	}
	if c.out, e = Parse(baseBytes); e != nil {
		return nil, fmt.Errorf("base DTB for %s: %w", fam, e)
	}
	c.note("Board family: %s. Started from our own %s DTB.", fam.Label(), fam)
	steps := []func() error{
		c.panel,
		c.backlight,
		c.joypad,
		c.volumeKeys,
		c.battery,
		c.charger,
		c.leds,
		c.rumble,
		c.regulators,
		c.pmicIRQ,
		c.secondSlot,
		c.usbHost,
		c.audio,
		c.model,
	}
	for _, step := range steps {
		if e := step(); e != nil {
			return nil, e
		}
	}
	if e := checkPins(c.out); e != nil {
		return nil, e
	}
	c.res.Kernel = c.out.Bytes()
	if bases.UBoot != nil {
		ub, e := MakeUBoot(bases.UBoot, c.res.Kernel)
		if e != nil {
			return nil, fmt.Errorf("boot logo DTB: %w", e)
		}
		c.res.UBoot = ub
	}
	return c.res, nil
}

// Skipped lists what the converter never copies from the input, and why.
// It is the same for every input; the CLI prints it and the report shows it.
var Skipped = []string{
	"Driver nodes of the 4.4 kernel that our 4.19 kernel does not have or names differently: rg351-keys, vpu_combo / vpu_service, bus-soc, charge-animation, the play_joystick / micro,gamepad gamepad nodes, sdio-pwrseq, seekwave / wireless-bluetooth, multicodecs sound card, sensors and touch on I2C1/I2C2. Our base already has the 4.19 equivalents.",
	"Joypad key codes: ArkOS and EmuELEC send other codes. We keep ours (the Android key layout depends on them) and only move the GPIO pins.",
	"CPU, GPU and memory speed tables, DDR settings, DCDC1/DCDC2 limits: ours carry the boost speeds and the safe limits we tested.",
	"Backlight brightness steps and default level, and the backlight supply (on EE boards LDO7 feeds the second card slot, not the backlight).",
	"Debug console settings (fiq-debugger speed, IRQ mode). EE bases already use UART5 like the stock firmware.",
	"eMMC (dwmmc@ff390000): andr36oid runs from the SD card.",
	"Higher charger current or voltage limits than the family base: the base values are kept. Lower ones are copied.",
	"Headphone detect polarity: the 4.4 sound driver reads it the other way round; our base value works with our driver.",
	"LDO1-3, LDO5-7, DCDC3/4 and BOOST settings: taken from the family base. Only LDO4, LDO8 (when it feeds the screen) and a lower LDO9 are copied.",
	"Panel compatible string: our panel driver binds through simple-panel-dsi either way.",
	"ramoops / reserved memory layout: ours matches our kernel's pstore setup.",
}
