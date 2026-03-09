package progress

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBufferWriterAt(t *testing.T) {
	buf := NewBufferWriterAt(20)
	buf.WriteAt([]byte{5, 6, 7, 8, 9}, 4)
	buf.WriteAt([]byte{15, 16, 17}, 14)
	buf.WriteAt([]byte{1, 2, 3, 4}, 0)
	buf.WriteAt([]byte{10, 11, 12, 13, 14}, 9)
	buf.WriteAt([]byte{18, 19, 20}, 17)
	assert.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}, buf.Bytes())
}

type nopWriterAt struct{}

func (nopWriterAt) WriteAt(p []byte, _ int64) (int, error) {
	return len(p), nil
}

func TestWriterCloseStopsReporterGoroutine(t *testing.T) {
	base := runtime.NumGoroutine()
	writers := make([]*Writer, 0, 40)
	for i := 0; i < 40; i++ {
		writers = append(writers, NewWriter(nopWriterAt{}, 1024))
	}
	time.Sleep(150 * time.Millisecond)

	for _, w := range writers {
		w.Close()
		w.Close() // idempotent close
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if runtime.NumGoroutine() <= base+12 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("writer reporter goroutines did not converge, base=%d current=%d", base, runtime.NumGoroutine())
		}
		time.Sleep(20 * time.Millisecond)
	}
}
