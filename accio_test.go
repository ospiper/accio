package accio

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ospiper/accio/progress"
)

func TestBasic(t *testing.T) {
	ctx := context.TODO()
	//runChunk(t, ctx, 1)
	runChunk(t, ctx, 16)
}

func runChunk(t *testing.T, ctx context.Context, connections int) {
	req := New().Get("http://localhost:19527/test-file").Timeout(time.Second * 1000)
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
			fmt.Println(v.Error)
			continue
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
}
