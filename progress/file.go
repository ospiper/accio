package progress

import (
	"fmt"
	"io"
	"sync"
	"time"
)

type Writer struct {
	Size     int64
	Progress *Bucket
	w        io.WriterAt
	tick     *time.Ticker
}

func (w *Writer) Close() {
	w.tick.Stop()
}

func (w *Writer) WriteAt(p []byte, off int64) (n int, err error) {
	n, err = w.w.WriteAt(p, off)
	w.Progress.Report(time.Now().UnixMilli(), int64(n))
	return
}

func NewWriter(w io.WriterAt, size int64) *Writer {
	ret := &Writer{
		Size: size,
		// 0.1s granule, reports millisecond, collects 1s
		Progress: New(10, func(in int64) int64 {
			return in / 100
		}),
		w:    w,
		tick: time.NewTicker(time.Millisecond * 100),
	}
	go func() {
		for range ret.tick.C {
			ret.Progress.Report(time.Now().UnixMilli(), 0)
		}
	}()

	return ret
}

type BufferWriterAt struct {
	mu    sync.Mutex
	under []byte
}

func NewBufferWriterAt(size int64) *BufferWriterAt {
	return &BufferWriterAt{
		under: make([]byte, size),
	}
}

func (b *BufferWriterAt) WriteAt(p []byte, off int64) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if off+int64(len(p)) > int64(len(b.under)) { // expand if not sufficient size
		return 0, fmt.Errorf("buffer overflow")
	}
	if n := copy(b.under[off:], p); n != len(p) {
		fmt.Println("warning: copy size not match", len(p), "!=", n)
	}
	return len(p), nil
}

func (b *BufferWriterAt) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.under
}
