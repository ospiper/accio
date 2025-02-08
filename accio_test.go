package accio

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestBasic(t *testing.T) {
	ctx := context.TODO()
	runChunk(t, ctx, 1)
	runChunk(t, ctx, 4)
}

func runChunk(t *testing.T, ctx context.Context, connections int) {
	req := New().Get("https://link.testfile.org/PDF10MB").Timeout(time.Second * 1000)
	t0 := time.Now()
	meta, out, err := GetConcurrent(ctx, req, connections)
	if err != nil {
		t.Fatal(err)
	}
	length := meta.Size
	fmt.Println("length:", length)
	buf := make([]byte, length)
	received := 0
	for v := range out {
		if v.Error != nil {
			fmt.Println(v.Error)
			continue
		}
		if v.EndByte-v.StartByte+1 != int64(len(v.Data)) {
			fmt.Printf("[main] Warning: size not match, expected: %d, size: %d bytes\n", v.EndByte-v.StartByte+1, len(v.Data))
		}
		received += len(v.Data)
		copy(buf[v.StartByte:], v.Data)
	}
	fmt.Println("[main] received:", received)
	fmt.Println("[main] md5:", getMD5(buf))
	fmt.Println("[main] time elapsed:", time.Since(t0).Milliseconds())
}
