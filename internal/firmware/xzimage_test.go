package firmware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulikunitz/xz"
)

// xzPayload: compressible but not trivial, so blocks hold real LZMA2 data
func xzPayload(size int) []byte {
	r := rand.New(rand.NewSource(1))
	b := make([]byte, size)
	for i := 0; i < size; i += 512 {
		if r.Intn(3) == 0 {
			r.Read(b[i:min(i+512, size)])
		} else {
			copy(b[i:min(i+512, size)], bytes.Repeat([]byte("andr36oid "), 52))
		}
	}
	return b
}

func xzStream(t *testing.T, payload []byte, blockSize int64) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := xz.WriterConfig{BlockSize: blockSize}.NewWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func xzFile(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "release.img.xz")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func checkStagedXZ(t *testing.T, source string, payload []byte) {
	t.Helper()
	expected := sha256.Sum256(payload)
	digest := sha256.New()
	var done int64
	path, err := stage(context.Background(), source, t.TempDir(), func(n, total int64) {
		if n < done || n > total {
			t.Errorf("unexpected progress %d/%d after %d", n, total, done)
		}
		done = n
	}, digest)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, payload) {
		t.Fatalf("staged data differs: %v", err)
	}
	if !bytes.Equal(digest.Sum(nil), expected[:]) || done != int64(len(payload)) {
		t.Fatalf("digest or progress does not cover the entire image (progress %d of %d)", done, len(payload))
	}
}

func TestStageXZBlocksInParallel(t *testing.T) {
	payload := xzPayload(3<<20 + 12345)
	data := xzStream(t, payload, 256<<10)
	l, err := xzLayoutOf(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(l.blocks) < 10 || l.size != int64(len(payload)) {
		t.Fatalf("expected many blocks covering the payload, got %d blocks, %d bytes", len(l.blocks), l.size)
	}
	source := xzFile(t, data)
	for _, workers := range []int{1, 3, 8} {
		t.Run("workers", func(t *testing.T) {
			saved := xzWorkers
			xzWorkers = workers
			defer func() { xzWorkers = saved }()
			checkStagedXZ(t, source, payload)
		})
	}
}

func TestStageXZConcatenatedStreamsInOrder(t *testing.T) {
	first, second := xzPayload(700<<10), xzPayload(300<<10)
	data := append(xzStream(t, first, 128<<10), xzStream(t, second, 128<<10)...)
	if _, err := xzLayoutOf(bytes.NewReader(data), int64(len(data))); err != errNotSingleStream {
		t.Fatalf("two streams taken as one: %v", err)
	}
	checkStagedXZ(t, xzFile(t, data), append(first, second...))
}

func TestStageXZFromXZTool(t *testing.T) {
	tool, err := exec.LookPath("xz")
	if err != nil {
		t.Skip("no xz tool")
	}
	payload := xzPayload(6 << 20)
	// -T2 with a small block size gives the multi-block layout of the release images
	cmd := exec.Command(tool, "-6", "-T2", "--block-size=1MiB", "-c")
	cmd.Stdin = bytes.NewReader(payload)
	data, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	l, err := xzLayoutOf(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(l.blocks) < 6 {
		t.Fatalf("xz -T2 output not read as blocks: %v", err)
	}
	checkStagedXZ(t, xzFile(t, data), payload)
}

func TestLoadRejectsDamagedXZ(t *testing.T) {
	payload := xzPayload(2 << 20)
	good := xzStream(t, payload, 256<<10)
	l, err := xzLayoutOf(bytes.NewReader(good), int64(len(good)))
	if err != nil {
		t.Fatal(err)
	}
	mid := l.blocks[len(l.blocks)/2]
	cases := map[string]func([]byte) []byte{
		"block data":  func(b []byte) []byte { b[mid.offset+mid.unpadded/2] ^= 0x40; return b },
		"block check": func(b []byte) []byte { b[mid.offset+mid.unpadded-1] ^= 1; return b },
		"index":       func(b []byte) []byte { b[len(b)-20] ^= 1; return b },
		"truncated":   func(b []byte) []byte { return b[:len(b)/2] },
	}
	for name, damage := range cases {
		t.Run(name, func(t *testing.T) {
			source := xzFile(t, damage(append([]byte(nil), good...)))
			dir := t.TempDir()
			_, err := Load(context.Background(), source, dir, nil)
			if err == nil || !strings.Contains(err.Error(), "damaged archive") {
				t.Fatalf("expected damaged archive, got %v", err)
			}
			if files, err := os.ReadDir(dir); err != nil || len(files) != 0 {
				t.Fatalf("failed extraction left files: %v %v", files, err)
			}
		})
	}
}

func TestStageXZCancellationRemovesStagingFile(t *testing.T) {
	data := xzStream(t, xzPayload(2<<20), 128<<10)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dir := t.TempDir()
	if _, err := stage(ctx, xzFile(t, data), dir, nil, sha256.New()); err == nil {
		t.Fatal("cancelled unpacking succeeded")
	}
	if files, err := os.ReadDir(dir); err != nil || len(files) != 0 {
		t.Fatalf("cancelled unpacking left files: %v %v", files, err)
	}
}
