package flash

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/andr36oid/andr36oid-sdflasher/internal/firmware"
	"github.com/andr36oid/andr36oid-sdflasher/internal/layout"
	"io"
	"os"
	"strings"
)

type Range struct {
	Name   string `json:"name"`
	Offset int64  `json:"offset"`
	Length int64  `json:"length"`
}

func (r Range) End() int64 { return r.Offset + r.Length }

type Write struct {
	Range
	SourceOffset int64  `json:"source_offset"`
	Boot         bool   `json:"boot"`
	Data         []byte `json:"data,omitempty"`
}
type Plan struct {
	Mode           string       `json:"mode"`
	Size           int64        `json:"size"`
	Writes         []Write      `json:"writes"`
	Protected      []Range      `json:"protected"`
	Table          layout.Table `json:"table"`
	OldFingerprint string       `json:"old_fingerprint"`
	Profile        string       `json:"profile"`
	NoROMs         bool         `json:"no_roms"`
}
type Card struct {
	Table       layout.Table `json:"table"`
	Installed   bool         `json:"installed"`
	Treble      bool         `json:"treble"`
	Profile     string       `json:"profile"`
	NoROMs      bool         `json:"no_roms"`
	Fingerprint string       `json:"fingerprint"`
	Reason      string       `json:"reason,omitempty"`
}

func Snapshot(r io.ReaderAt, size int64) (string, error) {
	h := sha256.New()
	for _, off := range []int64{0, size - 34*512} {
		b := make([]byte, 34*512)
		if _, e := r.ReadAt(b, off); e != nil {
			return "", e
		}
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func InspectCard(f *os.File, size int64) (Card, error) {
	c := Card{}
	var e error
	c.Fingerprint, e = Snapshot(f, size)
	if e != nil {
		return c, e
	}
	t, e := layout.Read(f, size)
	if e != nil {
		c.Reason = e.Error()
		return c, nil
	}
	c.Table = t
	boot, e := firmware.OpenBoot(f, t, true)
	if e != nil {
		c.Reason = "BOOT filesystem was not recognized"
		return c, nil
	}
	script, e := firmware.ReadFile(boot, "/boot.ini", 128<<10)
	if e != nil || !bytes.Contains(script, []byte("odroidgoa-uboot-config")) {
		c.Reason = "This is not a recognized andr36oid installation"
		return c, nil
	}
	root, e := boot.ReadDir(".")
	if e != nil {
		return c, e
	}
	var active []byte
	for _, n := range root {
		if strings.HasSuffix(n.Name(), "-android.dtb") {
			b, e := firmware.ReadFile(boot, "/"+n.Name(), 4<<20)
			if e == nil {
				active = b
				break
			}
		}
	}
	if len(active) == 0 {
		c.Reason = "The existing Android device tree was not found"
		return c, nil
	}
	if !t.GPT {
		for i := range t.Parts {
			switch t.Parts[i].Number {
			case 1:
				t.Parts[i].Name = "BOOT"
			case 2:
				t.Parts[i].Name = "system"
			case 3:
				t.Parts[i].Name = "userdata"
			default:
				c.Reason = "Unrecognized legacy MBR layout"
				return c, nil
			}
		}
	}
	data, ok := t.Find("userdata")
	if !ok {
		c.Reason = "This card has not finished its first boot; use a fresh installation"
		return c, nil
	}
	magic := make([]byte, 2048)
	if _, e = f.ReadAt(magic, data.Offset()); e != nil {
		return c, e
	}
	if !((magic[1080] == 0x53 && magic[1081] == 0xef) || bytes.Equal(magic[1024:1028], []byte{0x10, 0x20, 0xf5, 0xf2})) {
		c.Reason = "The userdata filesystem was not recognized"
		return c, nil
	}
	profiles, _ := firmware.Profiles(boot)
	sum := sha256.Sum256(active)
	for _, p := range profiles {
		if p.Hash == hex.EncodeToString(sum[:]) {
			c.Profile = p.ID
			break
		}
	}

	if c.Profile == "" {
		if saved, e := firmware.ReadFile(boot, "/andr36oid-profile.txt", 4096); e == nil {
			id := strings.TrimSpace(string(saved))
			for _, p := range profiles {
				if p.ID == id {
					c.Profile = id
					break
				}
			}
		}
	}
	_, roms := t.Find("EASYROMS")
	c.NoROMs = !roms
	c.Table = t
	c.Installed = true
	_, c.Treble = t.Find("vendor")
	return c, nil
}
func Build(im *firmware.Image, card Card, size int64, mode, profile string, noROMs bool) (*Plan, error) {
	if mode != "install" && mode != "update" {
		return nil, errors.New("invalid operation")
	}
	if size < im.Size {
		return nil, errors.New("the card is smaller than the release image")
	}
	found := false
	for _, p := range im.Profiles {
		if p.ID == profile {
			found = true
		}
	}
	if !found {
		return nil, errors.New("choose a device profile shipped with this image")
	}
	p := &Plan{Mode: mode, Size: size, Profile: profile, NoROMs: noROMs, OldFingerprint: card.Fingerprint}
	p.Table = im.Table
	p.Table.Parts = append([]layout.Partition{}, im.Table.Parts...)
	p.Table.Size = size
	p.Table.ID = layout.GUID()
	if mode == "install" {
		if !noROMs && size < 20<<30 {
			return nil, errors.New("the separate games layout needs a card of at least 20 GiB; choose all storage for Android")
		}
		p.Writes = append(p.Writes, Write{Range: Range{"Release image", 34 * 512, im.Size - 67*512}, SourceOffset: 34 * 512})
	} else {
		if !card.Installed {
			return nil, fmt.Errorf("cannot update this card: %s", card.Reason)
		}
		p.NoROMs = card.NoROMs
		if card.Table.GPT {
			p.Table.ID = card.Table.ID
		}
		for _, q := range card.Table.Parts {
			switch q.Name {
			case "userdata", "EASYROMS":
				n := 6
				if q.Name == "EASYROMS" {
					n = 7
				}
				q.Number = n
				p.Table.Parts = append(p.Table.Parts, q)
				p.Protected = append(p.Protected, Range{q.Name, q.Offset(), q.Size()})
			case "BOOT", "system", "vendor", "metadata", "misc", "cache", "idbloader", "uboot", "trust":
			default:
				return nil, fmt.Errorf("unknown partition %q; refusing to overwrite this layout", q.Name)
			}
		}
		for i, q := range p.Table.Parts {
			if q.Name == "userdata" || q.Name == "EASYROMS" {
				continue
			}
			old, exists := card.Table.Find(q.Name)
			if exists {
				p.Table.Parts[i].ID = old.ID
			}
			if exists && (q.Name == "metadata" || q.Name == "misc") {
				if q.Start != old.Start || q.End != old.End {
					return nil, fmt.Errorf("%s would need to move", q.Name)
				}
				p.Protected = append(p.Protected, Range{q.Name, q.Offset(), q.Size()})
				continue
			}
			p.Writes = append(p.Writes, Write{Range: Range{q.Name, q.Offset(), q.Size()}, SourceOffset: q.Offset(), Boot: q.Name == "BOOT"})
		}
	}
	if e := p.Table.Validate(true); e != nil {
		return nil, fmt.Errorf("cannot preserve this layout: %w", e)
	}
	head, tail, e := p.Table.Encode()
	if e != nil {
		return nil, e
	}
	// Commit the backup table before the primary table, after all partition payloads.
	p.Writes = append(p.Writes, Write{Range: Range{"Backup partition table", size - int64(len(tail)), int64(len(tail))}, Data: tail}, Write{Range: Range{"Partition table", 0, int64(len(head))}, Data: head})
	if e = p.Validate(); e != nil {
		return nil, e
	}
	return p, nil
}
func (p *Plan) Validate() error {
	if p.Size < 68*512 {
		return errors.New("invalid target size")
	}
	for _, w := range p.Writes {
		if w.Offset < 0 || w.Length <= 0 || w.Offset > p.Size || w.Length > p.Size-w.Offset {
			return errors.New("write outside target")
		}
		if w.Length%512 != 0 || w.Offset%512 != 0 {
			return errors.New("unaligned write")
		}
		for _, keep := range p.Protected {
			if w.Offset < keep.End() && keep.Offset < w.End() {
				return fmt.Errorf("write to %s would damage %s", w.Name, keep.Name)
			}
		}
	}
	return nil
}
