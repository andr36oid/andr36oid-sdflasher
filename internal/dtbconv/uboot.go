package dtbconv

import "fmt"

// MakeUBoot makes the U-Boot panel DTB (rg351mp/rg351v/rg351p-kernel.dtb)
// for a kernel DTB. U-Boot uses it only to light the panel for the boot logo
// and the charge screen. It takes our R36S U-Boot DTB and copies the panel
// from the kernel DTB: delays, size, DSI flags, init and exit sequences, the
// native timing, the reset GPIO, the enable GPIO (or the GPIO of a switched
// LCD rail) and the LDO4 and LDO8 voltages. Same as the kernel repo's
// experimental-dtbs/mk-uboot-dtb.py.
func MakeUBoot(base, kernel []byte) ([]byte, error) {
	k, e := Parse(kernel)
	if e != nil {
		return nil, e
	}
	b, e := Parse(base)
	if e != nil {
		return nil, e
	}
	kp, bp := k.Node(pathPanel), b.Node(pathPanel)
	if kp == nil || bp == nil {
		return nil, fmt.Errorf("panel node missing")
	}
	for _, p := range []string{"prepare-delay-ms", "reset-delay-ms", "init-delay-ms", "enable-delay-ms", "disable-delay-ms", "unprepare-delay-ms", "width-mm", "height-mm", "dsi,flags", "dsi,lanes", "dsi,format", "panel-init-sequence", "panel-exit-sequence"} {
		if v, ok := kp.Get(p); ok {
			bp.Set(p, v)
		}
	}
	rst, ok, e := k.GPIO(kp, "reset-gpios")
	if e != nil {
		return nil, e
	}
	if ok {
		if e := b.SetGPIOs(bp, "reset-gpios", rst); e != nil {
			return nil, e
		}
	} else {
		bp.Delete("reset-gpios")
	}
	en, ok, e := k.GPIO(kp, "enable-gpios")
	if e != nil {
		return nil, e
	}
	if !ok {
		// LCD power switched by a GPIO: U-Boot drives it as enable
		if ph, has := kp.U32("power-supply"); has {
			if sup := k.Phandle(ph); sup != nil && sup.Enabled() {
				en, ok, e = k.GPIO(sup, "gpio")
				if e != nil {
					return nil, e
				}
			}
		}
	}
	if ok {
		if e := b.SetGPIOs(bp, "enable-gpios", en); e != nil {
			return nil, e
		}
	} else {
		bp.Delete("enable-gpios")
	}
	for _, reg := range []string{"LDO_REG4", "LDO_REG8"} {
		kr, br := k.Node(pathRegs+"/"+reg), b.Node(pathRegs+"/"+reg)
		if kr == nil || br == nil {
			continue
		}
		for _, p := range []string{"regulator-min-microvolt", "regulator-max-microvolt"} {
			if v, ok := kr.Get(p); ok {
				br.Set(p, v)
			}
		}
	}
	c := &conv{}
	kt, e := c.nativeTiming(k)
	if e != nil {
		return nil, e
	}
	bt, e := c.nativeTiming(b)
	if e != nil {
		return nil, e
	}
	for _, p := range timingProps {
		if v, ok := kt.Get(p); ok {
			bt.Set(p, v)
		}
	}
	return b.Bytes(), nil
}
