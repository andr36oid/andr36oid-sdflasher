package firmware

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andr36oid/andr36oid-sdflasher/internal/layout"
)

func systemImage(t *testing.T, sectors uint64) *Image {
	t.Helper()
	return &Image{Table: layout.Table{Parts: []layout.Partition{{Number: 2, Name: "system", Start: 2048, End: 2048 + sectors - 1}}}}
}

// fsBlock is a 4 KiB block with the ext4 magic where a superblock has it.
func fsBlock() []byte {
	b := bytes.Repeat([]byte{0x11}, 4096)
	b[1080], b[1081] = 0x53, 0xef
	return b
}

func sparse(chunks ...[]byte) []byte {
	var out bytes.Buffer
	blocks := uint32(0)
	for _, c := range chunks {
		blocks += binary.LittleEndian.Uint32(c[4:8])
	}
	for _, v := range []any{uint32(sparseMagic), uint16(1), uint16(0), uint16(28), uint16(12), uint32(4096), blocks, uint32(len(chunks)), uint32(0)} {
		binary.Write(&out, binary.LittleEndian, v)
	}
	for _, c := range chunks {
		out.Write(c)
	}
	return out.Bytes()
}

func chunk(kind uint16, blocks uint32, body []byte) []byte {
	var b bytes.Buffer
	for _, v := range []any{kind, uint16(0), blocks, uint32(12 + len(body))} {
		binary.Write(&b, binary.LittleEndian, v)
	}
	b.Write(body)
	return b.Bytes()
}

func TestPrepareSystem(t *testing.T) {
	ctx := context.Background()
	im := systemImage(t, 64) // 32 KiB: 8 blocks
	dir := t.TempDir()
	write := func(b []byte) string {
		f, _ := os.CreateTemp(dir, "src-*")
		f.Write(b)
		f.Close()
		return f.Name()
	}
	fill := []byte{1, 2, 3, 4}
	want := make([]byte, 64*512)
	copy(want, fsBlock())
	copy(want[4096:], bytes.Repeat(fill, 2048))
	// blocks 2-3 skipped, block 4 zero fill, blocks 5-7 beyond the image
	sp := sparse(chunk(0xcac1, 1, fsBlock()), chunk(0xcac2, 2, fill), chunk(0xcac3, 2, nil), chunk(0xcac4, 0, []byte{9, 9, 9, 9}), chunk(0xcac2, 1, make([]byte, 4)))
	for name, src := range map[string][]byte{"sparse": sp, "raw": want[:4096*3]} {
		exp := want
		if name == "raw" {
			exp = make([]byte, len(want))
			copy(exp, want[:4096*3])
		}
		out, e := PrepareSystem(ctx, im, write(src), dir)
		if e != nil {
			t.Fatal(name, e)
		}
		got, _ := os.ReadFile(out)
		if !bytes.Equal(got, exp) {
			t.Fatalf("%s: wrong content", name)
		}
	}
	bad := map[string]struct {
		src  []byte
		text string
	}{
		"too big raw":    {append(fsBlock(), make([]byte, 64*512)...), "too large"},
		"too big sparse": {sparse(chunk(0xcac1, 1, fsBlock()), chunk(0xcac3, 8, nil)), "too large"},
		"xz":             {append([]byte{0xfd, '7', 'z', 'X', 'Z', 0}, make([]byte, 100)...), "xz"},
		"no filesystem":  {make([]byte, 8192), "filesystem"},
		"broken sparse":  {sparse(chunk(0xcac1, 2, fsBlock())), "damaged"},
		"unknown chunk":  {sparse(chunk(0xcaff, 1, nil)), "damaged"},
	}
	for name, c := range bad {
		if _, e := PrepareSystem(ctx, im, write(c.src), dir); e == nil || !strings.Contains(e.Error(), c.text) {
			t.Errorf("%s: %v", name, e)
		}
	}
	left, _ := filepath.Glob(filepath.Join(dir, "system-*"))
	if len(left) != 2 {
		t.Errorf("failed preparations left files behind: %v", left)
	}
}
