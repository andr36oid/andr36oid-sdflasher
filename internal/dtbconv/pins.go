package dtbconv

import (
	"fmt"
	"sort"
	"strings"
)

type pinUse struct {
	who string
	g   GPIO
}

func enabledPath(n *Node) bool {
	for p := n; p != nil; p = p.parent {
		if !p.Enabled() {
			return false
		}
	}
	return true
}

func isGPIOProp(name string) bool {
	if name == "gpio-controller" || name == "#gpio-cells" || name == "gpio-ranges" || name == "gpio-line-names" {
		return false
	}
	return strings.HasSuffix(name, "gpios") || strings.HasSuffix(name, "gpio") || name == "WIFI,host_wake_irq"
}

// GPIOUses lists every GPIO pin an enabled node uses.
func GPIOUses(t *Tree) []pinUse {
	var out []pinUse
	pc := t.Node("/pinctrl")
	t.Walk(func(n *Node) {
		if pc != nil && n != pc && strings.HasPrefix(n.Path(), "/pinctrl/") || !enabledPath(n) {
			return
		}
		for _, p := range n.Props {
			if !isGPIOProp(p.Name) {
				continue
			}
			gs, e := t.GPIOs(n, p.Name)
			if e != nil {
				continue
			}
			for _, g := range gs {
				out = append(out, pinUse{n.Path() + " " + p.Name, g})
			}
		}
		// the power chip interrupt is a GPIO0 pin
		if n.Compatible("rockchip,rk817") {
			if irq, ok := n.Cells("interrupts"); ok && len(irq) == 2 {
				if par, ok := n.U32("interrupt-parent"); ok {
					if b, ok := gpioBank(t.Phandle(par)); ok {
						out = append(out, pinUse{n.Path() + " interrupts", GPIO{b, irq[0], 0}})
					}
				}
			}
		}
	})
	return out
}

// peripheralPins maps pins that an enabled device muxes to a non-GPIO
// function (UART, SD, I2C, ...) to that device.
func peripheralPins(t *Tree) map[uint32]string {
	out := map[uint32]string{}
	pc := t.Node("/pinctrl")
	t.Walk(func(n *Node) {
		if pc != nil && strings.HasPrefix(n.Path(), "/pinctrl") || !enabledPath(n) {
			return
		}
		names := n.Strings("pinctrl-names")
		idx := 0
		if len(names) > 0 {
			idx = -1
			for i, s := range names {
				if s == "default" {
					idx = i
				}
			}
			if idx < 0 {
				return
			}
		}
		groups, ok := n.Cells(fmt.Sprintf("pinctrl-%d", idx))
		if !ok {
			return
		}
		for _, ph := range groups {
			g := t.Phandle(ph)
			if g == nil {
				continue
			}
			pins, e := t.Pins(g)
			if e != nil {
				continue
			}
			for _, p := range pins {
				if p.Func != 0 {
					out[p.Bank<<8|p.Pin] = n.Path()
				}
			}
		}
	})
	return out
}

// checkPins refuses a tree where two things drive the same pin, or where a
// GPIO user sits on a pin a peripheral has taken (the O2-46 dead-buttons case).
func checkPins(t *Tree) error {
	uses := GPIOUses(t)
	periph := peripheralPins(t)
	seen := map[uint32]string{}
	var problems []string
	for _, u := range uses {
		k := u.g.key()
		if who, ok := seen[k]; ok {
			problems = append(problems, fmt.Sprintf("%s is used by %s and by %s", u.g.Name(), who, u.who))
			continue
		}
		seen[k] = u.who
		if who, ok := periph[k]; ok && who != strings.SplitN(u.who, " ", 2)[0] {
			problems = append(problems, fmt.Sprintf("%s (%s) is taken by %s", u.g.Name(), u.who, who))
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return refuse("The converted DTB would use a pin twice, so it was not made: %s.", strings.Join(problems, "; "))
	}
	return nil
}
