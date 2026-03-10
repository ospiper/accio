package accio

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ospiper/accio/progress"
)

func TestBasic(t *testing.T) {
	data := make([]byte, 128*1024)
	for i := range data {
		data[i] = byte(i % 251)
	}

	s := newRangeServerBasic(t, data)
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	runChunk(t, ctx, 16, s.URL, data)
}

func runChunk(t *testing.T, ctx context.Context, connections int, url string, expect []byte) {
	req := New().Get(url).Timeout(5 * time.Second)
	t0 := time.Now()
	meta, out, err := GetConcurrent(ctx, req, connections)
	if err != nil {
		t.Fatal(err)
	}
	length := meta.Size
	fmt.Println("length:", length)
	received := 0
	_buf := progress.NewBufferWriterAt(meta.Size)
	writer := progress.NewWriter(_buf, meta.Size)
	for v := range out {
		if v.Error != nil {
			t.Fatalf("unexpected error chunk: %v", v.Error)
		}
		if v.EndByte-v.StartByte+1 != int64(len(v.Data)) {
			fmt.Printf("[main] Warning: size not match, expected: %d, size: %d bytes\n", v.EndByte-v.StartByte+1, len(v.Data))
		}
		received += len(v.Data)
		//fmt.Printf("[main] write offset %d size %d\n", v.StartByte, v.EndByte-v.StartByte)
		//fmt.Println(writer.Progress.Collect(), "bytes/s")
		writer.WriteAt(v.Data, v.StartByte)
	}
	writer.Close()
	fmt.Println("[main] md5:", getMD5(_buf.Bytes()))
	fmt.Println("[main] received:", received)
	fmt.Println("[main] time elapsed:", time.Since(t0).Milliseconds())

	if int64(len(expect)) != meta.Size {
		t.Fatalf("meta size mismatch: got %d want %d", meta.Size, len(expect))
	}
	if received != len(expect) {
		t.Fatalf("received size mismatch: got %d want %d", received, len(expect))
	}
	if !bytes.Equal(_buf.Bytes(), expect) {
		t.Fatal("downloaded data mismatch")
	}
}

func newRangeServerBasic(t *testing.T, data []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.WriteHeader(http.StatusOK)
			return
		}

		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}

		start, end := parseRangeHeaderBasic(t, rangeHeader, len(data))
		part := data[start : end+1]
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", strconv.Itoa(len(part)))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(part)
	}))
}

func parseRangeHeaderBasic(t *testing.T, h string, size int) (int, int) {
	t.Helper()
	if !strings.HasPrefix(h, "bytes=") {
		t.Fatalf("invalid range header: %q", h)
	}
	parts := strings.Split(strings.TrimPrefix(h, "bytes="), "-")
	if len(parts) != 2 {
		t.Fatalf("invalid range header: %q", h)
	}
	start, err := strconv.Atoi(parts[0])
	if err != nil {
		t.Fatalf("invalid range start: %v", err)
	}
	end, err := strconv.Atoi(parts[1])
	if err != nil {
		t.Fatalf("invalid range end: %v", err)
	}
	if start < 0 || end < start || end >= size {
		t.Fatalf("range out of bounds: %d-%d, size=%d", start, end, size)
	}
	return start, end
}
