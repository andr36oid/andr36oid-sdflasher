package dtbconv

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var bases = EmbeddedBases()

// toArkOS turns one of our DTBs into what an ArkOS DTB of the same board
// looks like to the converter: ArkOS key codes and another model name.
func toArkOS(t *testing.T, b []byte) *Tree {
	t.Helper()
	tr, e := Parse(b)
	if e != nil {
		t.Fatal(e)
	}
	rev := map[uint32]uint32{}
	for ark, our := range arkosKeys {
		if ark >= 0x220 || ark <= 0x139 { // prefer the BTN_TRIGGER_HAPPY / BTN_* set
			if old, ok := rev[our]; !ok || ark > old {
				rev[our] = ark
			}
		}
	}
	pad := findCompatible(tr, "odroidgo3-joypad")
	if pad == nil {
		t.Fatal("base has no joypad")
	}
	for _, ch := range pad.Children {
		if code, ok := ch.U32("linux,code"); ok {
			ark, ok := rev[code]
			if !ok {
				t.Fatalf("no ArkOS code for %d", code)
			}
			ch.SetU32("linux,code", ark)
		}
	}
	tr.Root.SetString("model", "Test Board")
	return tr
}

func diffTrees(a, b *Tree) []string {
	var out []string
	a.Walk(func(n *Node) {
		m := b.Node(n.Path())
		if m == nil {
			out = append(out, n.Path()+" (missing)")
			return
		}
		for _, p := range n.Props {
			v, ok := m.Get(p.Name)
			if !ok || !bytes.Equal(v, p.Value) {
				out = append(out, n.Path()+":"+p.Name)
			}
		}
		for _, p := range m.Props {
			if !n.Has(p.Name) {
				out = append(out, n.Path()+":"+p.Name+" (added)")
			}
		}
	})
	b.Walk(func(n *Node) {
		if a.Node(n.Path()) == nil {
			out = append(out, n.Path()+" (added)")
		}
	})
	return out
}

func TestRoundTrip(t *testing.T) {
	for name, b := range map[string][]byte{"r36s": bases.Genuine, "ee": bases.EE, "soy": bases.Soy, "uboot": bases.UBoot} {
		tr, e := Parse(b)
		if e != nil {
			t.Fatalf("%s: %v", name, e)
		}
		out := tr.Bytes()
		tr2, e := Parse(out)
		if e != nil {
			t.Fatalf("%s: reparse: %v", name, e)
		}
		if d := diffTrees(tr, tr2); len(d) > 0 {
			t.Errorf("%s: round trip changed %v", name, d)
		}
		if !bytes.Equal(out, tr2.Bytes()) {
			t.Errorf("%s: second write differs", name)
		}
	}
}

func TestBasesHaveNoPinClash(t *testing.T) {
	for name, b := range map[string][]byte{"r36s": bases.Genuine, "ee": bases.EE, "soy": bases.Soy} {
		tr, _ := Parse(b)
		if e := checkPins(tr); e != nil {
			t.Errorf("%s: %v", name, e)
		}
	}
}

// Converting an ArkOS-style copy of a base must give the base back: same
// family, same pins, same panel. Only the model name changes.
func TestConvertBaseBack(t *testing.T) {
	for _, tc := range []struct {
		fam  Family
		base []byte
	}{{Genuine, bases.Genuine}, {EE, bases.EE}, {Soy, bases.Soy}} {
		in := toArkOS(t, tc.base)
		r, e := Convert(in.Bytes(), bases)
		if e != nil {
			t.Fatalf("%s: %v", tc.fam, e)
		}
		if r.Family != tc.fam {
			t.Errorf("family %s, want %s", r.Family, tc.fam)
		}
		got, e := Parse(r.Kernel)
		if e != nil {
			t.Fatal(e)
		}
		want, _ := Parse(tc.base)
		var d []string
		for _, s := range diffTrees(want, got) {
			// the alternate timings go away on purpose: one mode only
			if s != "/:model" && !strings.HasPrefix(s, pathPanel+"/display-timings/timing") {
				d = append(d, s)
			}
		}
		if len(d) > 0 {
			t.Errorf("%s: converted base differs from base: %v", tc.fam, d)
		}
		if m := got.Root.String("model"); m != "Test Board (converted for andr36oid)" {
			t.Errorf("model %q", m)
		}
		if r.UBoot == nil {
			t.Errorf("%s: no U-Boot DTB", tc.fam)
		}
		if !strings.Contains(r.Report(), "Never copied") {
			t.Errorf("report lacks the skipped list")
		}
	}
}

func TestConvertMovesPinsAndFixesRefresh(t *testing.T) {
	in := toArkOS(t, bases.Genuine)
	pad := findCompatible(in, "odroidgo3-joypad")
	// put L3 on another free GPIO1 pin
	var l3 *Node
	for _, ch := range pad.Children {
		if code, _ := ch.U32("linux,code"); arkosKeys[code] == keyL3 {
			l3 = ch
		}
	}
	if l3 == nil {
		t.Fatal("no L3 button in base")
	}
	old, _, _ := in.GPIO(l3, "gpios")
	moved := GPIO{old.Bank, old.Pin, old.Flags}
	moved.Pin = 31 // GPIO1_D7, unused on the R36S
	if e := in.SetGPIOs(l3, "gpios", moved); e != nil {
		t.Fatal(e)
	}
	// a 640x480 timing at about 50 Hz
	nt, e := (&conv{}).nativeTiming(in)
	if e != nil {
		t.Fatal(e)
	}
	nt.SetU32("vfront-porch", 300)
	r, e := Convert(in.Bytes(), bases)
	if e != nil {
		t.Fatal(e)
	}
	out, _ := Parse(r.Kernel)
	opad := findCompatible(out, "odroidgo3-joypad")
	found := false
	for _, ch := range opad.Children {
		if code, _ := ch.U32("linux,code"); code == keyL3 {
			g, _, _ := out.GPIO(ch, "gpios")
			found = g.key() == moved.key()
		}
	}
	if !found {
		t.Errorf("L3 was not moved to %s", moved.Name())
	}
	ot, _ := (&conv{}).nativeTiming(out)
	vfp, _ := ot.U32("vfront-porch")
	if vfp == 300 {
		t.Errorf("refresh not fixed")
	}
	get := func(p string) float64 { v, _ := ot.U32(p); return float64(v) }
	hz := get("clock-frequency") / ((get("hactive") + get("hfront-porch") + get("hsync-len") + get("hback-porch")) *
		(get("vactive") + get("vfront-porch") + get("vsync-len") + get("vback-porch")))
	if hz < 59.5 || hz > 60.5 {
		t.Errorf("refresh %.2f Hz", hz)
	}
}

func TestConvertKeepsFastStockRefresh(t *testing.T) {
	in := toArkOS(t, bases.Genuine)
	nt, e := (&conv{}).nativeTiming(in)
	if e != nil {
		t.Fatal(e)
	}
	// a 640x480 timing at about 80 Hz, like the RX6H's stock DTB
	nt.SetU32("clock-frequency", 30000000)
	nt.SetU32("hfront-porch", 46)
	nt.SetU32("hsync-len", 2)
	nt.SetU32("hback-porch", 44)
	nt.SetU32("vfront-porch", 16)
	nt.SetU32("vsync-len", 2)
	nt.SetU32("vback-porch", 14)
	r, e := Convert(in.Bytes(), bases)
	if e != nil {
		t.Fatal(e)
	}
	out, _ := Parse(r.Kernel)
	ot, _ := (&conv{}).nativeTiming(out)
	if vfp, _ := ot.U32("vfront-porch"); vfp != 16 {
		t.Errorf("vertical front porch %d, want the stock 16", vfp)
	}
}

func TestRefusals(t *testing.T) {
	refused := func(name string, b []byte, want string) {
		t.Helper()
		_, e := Convert(b, bases)
		if e == nil || !IsRefusal(e) {
			t.Errorf("%s: want a refusal, got %v", name, e)
			return
		}
		if !strings.Contains(e.Error(), want) {
			t.Errorf("%s: %q does not mention %q", name, e, want)
		}
	}
	refused("garbage", []byte("not a dtb at all"), "not a valid DTB")

	tr := toArkOS(t, bases.Genuine)
	tr.Root.SetString("compatible", "allwinner,sun50i-h700")
	refused("other SoC", tr.Bytes(), "not for an RK3326")

	_, e := Convert(bases.EE, bases)
	if !errors.Is(e, ErrAlreadyAndroid) {
		t.Errorf("already android: %v", e)
	}

	tr = toArkOS(t, bases.Genuine)
	pad := findCompatible(tr, "odroidgo3-joypad")
	pad.SetString("compatible", "adc-joystick")
	refused("mainline", tr.Bytes(), "mainline")

	tr = toArkOS(t, bases.Genuine)
	nt, _ := (&conv{}).nativeTiming(tr)
	nt.SetU32("hactive", 480)
	nt.SetU32("vactive", 640)
	refused("portrait", tr.Bytes(), "480x640")

	tr = toArkOS(t, bases.Genuine)
	pad = findCompatible(tr, "odroidgo3-joypad")
	for _, ch := range pad.Children {
		if code, _ := ch.U32("linux,code"); arkosKeys[code] == keyUp {
			if e := tr.SetGPIOs(ch, "gpios", GPIO{0, 5, 1}); e != nil {
				t.Fatal(e)
			}
		}
	}
	refused("unknown family", tr.Bytes(), "matches no board family")

	// two buttons on one pin
	tr = toArkOS(t, bases.Genuine)
	pad = findCompatible(tr, "odroidgo3-joypad")
	var a GPIO
	for _, ch := range pad.Children {
		if code, _ := ch.U32("linux,code"); arkosKeys[code] == keyA {
			a, _, _ = tr.GPIO(ch, "gpios")
		}
	}
	for _, ch := range pad.Children {
		if code, _ := ch.U32("linux,code"); arkosKeys[code] == keyStart {
			if e := tr.SetGPIOs(ch, "gpios", a); e != nil {
				t.Fatal(e)
			}
		}
	}
	refused("pin used twice", tr.Bytes(), "twice")
}

func TestMakeUBoot(t *testing.T) {
	ub, e := MakeUBoot(bases.UBoot, bases.EE)
	if e != nil {
		t.Fatal(e)
	}
	u, e := Parse(ub)
	if e != nil {
		t.Fatal(e)
	}
	k, _ := Parse(bases.EE)
	ki, _ := k.Node(pathPanel).Get("panel-init-sequence")
	ui, _ := u.Node(pathPanel).Get("panel-init-sequence")
	if !bytes.Equal(ki, ui) {
		t.Errorf("init sequence not copied")
	}
	kr, _, _ := k.GPIO(k.Node(pathPanel), "reset-gpios")
	ur, _, _ := u.GPIO(u.Node(pathPanel), "reset-gpios")
	if kr.key() != ur.key() {
		t.Errorf("reset %s, want %s", ur.Name(), kr.Name())
	}
}

// DTBCONV_CORPUS=dir converts every *.dtb below dir (except U-Boot ones).
// Each must convert or be refused with a reason; nothing may fail otherwise.
func TestCorpus(t *testing.T) {
	dir := os.Getenv("DTBCONV_CORPUS")
	if dir == "" {
		t.Skip("set DTBCONV_CORPUS to a folder of stock DTBs")
	}
	n := 0
	filepath.Walk(dir, func(p string, fi os.FileInfo, e error) error {
		if e != nil || fi.IsDir() || !strings.HasSuffix(p, ".dtb") || strings.HasPrefix(fi.Name(), "rg351") {
			return nil
		}
		b, _ := os.ReadFile(p)
		r, e := Convert(b, bases)
		n++
		switch {
		case e != nil && !IsRefusal(e):
			t.Errorf("%s: %v", p, e)
		case e == nil:
			if _, e := Parse(r.Kernel); e != nil {
				t.Errorf("%s: output does not parse: %v", p, e)
			}
		}
		return nil
	})
	t.Logf("%d DTBs", n)
}
