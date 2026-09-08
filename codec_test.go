package treesitter

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

// The codec was chosen on decompression speed alone, which is half the question.
// This measures the other half on the REAL blobs, in process: the bytes that
// ship in the binary, and the decode every cold start pays behind sync.Once.
//
// It asserts nothing about which codec wins. It prints the table and leaves the
// decision to a reader, because the answer depends on what is scarce -- binary
// size, cold-start time, or the generate step's wall clock.
func TestCodecComparisonOnTheRealBlobs(t *testing.T) {
	blobs, _ := filepath.Glob("grammars/*/tables.zst")
	if len(blobs) == 0 {
		t.Skip("no generated blobs; run go generate first")
	}

	dec, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
	if err != nil {
		t.Fatalf("zstd reader: %v", err)
	}
	defer dec.Close()

	zenc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	if err != nil {
		t.Fatalf("zstd writer: %v", err)
	}
	defer zenc.Close()

	t.Logf("%-8s %10s %10s %10s %10s %9s %9s %9s",
		"grammar", "raw", "zstd", "brotli9", "brotli11", "dec zstd", "dec br9", "dec br11")

	for _, path := range blobs {
		blob, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		raw, err := dec.DecodeAll(blob, nil)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}

		zs := zenc.EncodeAll(raw, nil)
		b9 := brotliEncode(t, raw, 9)
		b11 := brotliEncode(t, raw, 11)

		name := filepath.Base(filepath.Dir(path))
		t.Logf("%-8s %10d %10d %10d %10d %8.2fms %8.2fms %8.2fms",
			name, len(raw), len(zs), len(b9), len(b11),
			ms(func() { _, _ = dec.DecodeAll(zs, nil) }),
			ms(func() { brotliDecode(t, b9, len(raw)) }),
			ms(func() { brotliDecode(t, b11, len(raw)) }))
	}
}

func brotliEncode(t *testing.T, raw []byte, quality int) []byte {
	t.Helper()
	var out bytes.Buffer
	w := brotli.NewWriterLevel(&out, quality)
	if _, err := w.Write(raw); err != nil {
		t.Fatalf("brotli write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("brotli close: %v", err)
	}
	return out.Bytes()
}

func brotliDecode(t *testing.T, blob []byte, want int) {
	t.Helper()
	out := make([]byte, 0, want)
	buf := bytes.NewBuffer(out)
	if _, err := buf.ReadFrom(brotli.NewReader(bytes.NewReader(blob))); err != nil {
		t.Fatalf("brotli read: %v", err)
	}
}

// ms times one call, averaged over enough runs that the clock is not the story.
func ms(fn func()) float64 {
	const runs = 5
	start := time.Now()
	for range runs {
		fn()
	}
	return float64(time.Since(start).Microseconds()) / float64(runs) / 1000
}
