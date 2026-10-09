package dtbconv

import (
	"fmt"
	"strconv"
	"strings"
)

// GPIO is one pin reference: bank 0-3, pin 0-31, flags (bit 0 = active low).
type GPIO struct {
	Bank, Pin, Flags uint32
}

func (g GPIO) String() string {
	s := fmt.Sprintf("GPIO%d_%c%d", g.Bank, 'A'+rune(g.Pin/8), g.Pin%8)
	if g.Flags&1 != 0 {
		s += " (active low)"
	}
	return s
}

// Name is the pin without flags, e.g. GPIO3_C6.
func (g GPIO) Name() string { return fmt.Sprintf("GPIO%d_%c%d", g.Bank, 'A'+rune(g.Pin/8), g.Pin%8) }

func (g GPIO) key() uint32 { return g.Bank<<8 | g.Pin }

// gpioBank returns the bank number if n is a GPIO controller of this SoC.
func gpioBank(n *Node) (uint32, bool) {
	if n == nil || !n.Has("gpio-controller") {
		return 0, false
	}
	name := strings.SplitN(n.Name, "@", 2)[0]
	if !strings.HasPrefix(name, "gpio") {
		return 0, false
	}
	v, e := strconv.Atoi(strings.TrimPrefix(name, "gpio"))
	if e != nil || v < 0 || v > 3 {
		return 0, false
	}
	return uint32(v), true
}

// GPIOs reads a gpio list property (phandle, pin, flags per entry).
// Entries that point at something other than an SoC GPIO bank are an error.
func (t *Tree) GPIOs(n *Node, name string) ([]GPIO, error) {
	c, ok := n.Cells(name)
	if !ok {
		return nil, nil
	}
	if len(c)%3 != 0 {
		return nil, fmt.Errorf("%s/%s: unexpected GPIO format", n.Path(), name)
	}
	var out []GPIO
	for i := 0; i < len(c); i += 3 {
		bank, ok := gpioBank(t.Phandle(c[i]))
		if !ok {
			return nil, fmt.Errorf("%s/%s: points at something that is not a GPIO bank", n.Path(), name)
		}
		if c[i+1] > 31 {
			return nil, fmt.Errorf("%s/%s: pin number %d is out of range", n.Path(), name, c[i+1])
		}
		out = append(out, GPIO{bank, c[i+1], c[i+2]})
	}
	return out, nil
}

// GPIO reads a single-entry gpio property.
func (t *Tree) GPIO(n *Node, name string) (GPIO, bool, error) {
	g, e := t.GPIOs(n, name)
	if e != nil || len(g) == 0 {
		return GPIO{}, false, e
	}
	if len(g) != 1 {
		return GPIO{}, false, fmt.Errorf("%s/%s: expected one GPIO, found %d", n.Path(), name, len(g))
	}
	return g[0], true, nil
}

// bankNode finds the GPIO controller node for a bank.
func (t *Tree) bankNode(bank uint32) *Node {
	var found *Node
	t.Walk(func(n *Node) {
		if b, ok := gpioBank(n); ok && b == bank && found == nil {
			found = n
		}
	})
	return found
}

// SetGPIOs writes a gpio list property that points at this tree's banks.
func (t *Tree) SetGPIOs(n *Node, name string, gs ...GPIO) error {
	var c []uint32
	for _, g := range gs {
		b := t.bankNode(g.Bank)
		if b == nil {
			return fmt.Errorf("GPIO bank %d missing in the base", g.Bank)
		}
		c = append(c, t.PhandleOf(b), g.Pin, g.Flags)
	}
	n.SetCells(name, c...)
	return nil
}

// PinCfg is one entry of a rockchip,pins list.
type PinCfg struct {
	Bank, Pin, Func uint32
	Conf            *Node
}

func (t *Tree) Pins(group *Node) ([]PinCfg, error) {
	c, ok := group.Cells("rockchip,pins")
	if !ok {
		return nil, nil
	}
	if len(c)%4 != 0 {
		return nil, fmt.Errorf("%s: unexpected rockchip,pins format", group.Path())
	}
	var out []PinCfg
	for i := 0; i < len(c); i += 4 {
		out = append(out, PinCfg{c[i], c[i+1], c[i+2], t.Phandle(c[i+3])})
	}
	return out, nil
}

// pinConf finds a pin configuration node of the base by name ("pcfg-pull-up").
func (t *Tree) pinConf(name string) *Node {
	if p := t.Node("/pinctrl"); p != nil {
		if n := p.Child(name); n != nil {
			return n
		}
		// some trees use underscores
		if n := p.Child(strings.ReplaceAll(name, "-", "_")); n != nil {
			return n
		}
	}
	return nil
}

func (t *Tree) SetPins(group *Node, pins []PinCfg) {
	var c []uint32
	for _, p := range pins {
		c = append(c, p.Bank, p.Pin, p.Func, t.PhandleOf(p.Conf))
	}
	group.SetCells("rockchip,pins", c...)
}
