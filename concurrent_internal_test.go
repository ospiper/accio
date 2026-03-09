package accio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func parseRangeHeader(t *testing.T, h string, size int) (int, int) {
	t.Helper()
	if h == "" {
		return 0, size - 1
	}
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

func newRangeServer(t *testing.T, data []byte, failRange string, slow bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.WriteHeader(http.StatusOK)
			return
		}

		rangeHeader := r.Header.Get("Range")
		if failRange != "" && rangeHeader == failRange {
			http.Error(w, "forced range failure", http.StatusInternalServerError)
			return
		}

		start, end := parseRangeHeader(t, rangeHeader, len(data))
		if slow {
			time.Sleep(20 * time.Millisecond)
		}

		part := data[start : end+1]
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", strconv.Itoa(len(part)))
		if rangeHeader != "" {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		_, _ = w.Write(part)
	}))
}

func TestGetPoolCancelNoDeadlock(t *testing.T) {
	data := make([]byte, 128*1024)
	s := newRangeServer(t, data, "", true)
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := GetPool(ctx, New().Get(s.URL), int64(len(data)), 4)

	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			t.Fatal("timeout waiting for output channel to close after cancellation")
		case c, ok := <-out:
			if !ok {
				return
			}
			if c.Error == nil {
				cancel()
			}
		}
	}
}

func TestGetPoolRetryMaxRetryExit(t *testing.T) {
	data := []byte("01234567")
	s := newRangeServer(t, data, "bytes=0-4", false)
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out := GetPool(ctx, New().Get(s.URL), int64(len(data)), 2)

	var gotErr bool
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for GetPool to close: %v", ctx.Err())
		case c, ok := <-out:
			if !ok {
				if !gotErr {
					t.Fatal("expected at least one terminal error chunk")
				}
				return
			}
			if c.Error != nil {
				gotErr = true
			}
		}
	}
}

func TestGetByRangeReuseConcurrent(t *testing.T) {
	data := []byte("abcdefghijklmnopqrstuvwxyz012345")
	s := newRangeServer(t, data, "", false)
	defer s.Close()

	baseReq := New().Reuse().Get(s.URL)
	out := make(chan *Chunk, 128)

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := int64(i % len(data))
			getByRange(context.Background(), baseReq, start, start, 0, out)
		}()
	}
	wg.Wait()
	close(out)

	count := 0
	for c := range out {
		if c.Error != nil {
			t.Fatalf("unexpected error chunk: %v", c.Error)
		}
		count++
	}
	if count == 0 {
		t.Fatal("expected at least one chunk")
	}
}

func TestGetByRangeStatusErrorNoData(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer s.Close()

	out := make(chan *Chunk, 4)
	getByRange(context.Background(), New().Get(s.URL), 0, 3, 0, out)
	close(out)

	errCount := 0
	dataCount := 0
	for c := range out {
		if c.Error != nil {
			errCount++
		}
		if len(c.Data) > 0 {
			dataCount++
		}
	}
	if errCount != 1 {
		t.Fatalf("expected exactly 1 error chunk, got %d", errCount)
	}
	if dataCount != 0 {
		t.Fatalf("expected no data chunks when status is error, got %d", dataCount)
	}
}
