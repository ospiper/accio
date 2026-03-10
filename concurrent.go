package accio

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"sync"
	"time"
)

var (
	maxConnections = 8

	maxReadBufferSize = 1 * 1024 * 1024 // 1MB

	maxRetry = 8
)

type Meta struct {
	Size int64
}

type Chunk struct {
	StartByte int64
	EndByte   int64
	Data      []byte
	Error     error
	Retry     int // indicating this chunk is the n-th retry
}

// CanGetByChunk returns (can, size, error)
// will perform a head request
func CanGetByChunk(ctx context.Context, req *Request) (bool, int64, error) {
	headRequest := req.Method(http.MethodHead).Timeout(time.Second * 30)
	headResp, err := headRequest.Do(ctx)
	if err != nil {
		return false, 0, err
	}
	if headResp.ContentLength < 0 {
		return false, -1, nil
	}
	return headResp.Header.Get("Accept-Ranges") == "bytes", headResp.ContentLength, nil
}

func GetConcurrent(ctx context.Context, req *Request, connections int) (*Meta, <-chan *Chunk, error) {
	can, length, err := CanGetByChunk(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	meta := &Meta{Size: length}
	if !can || length <= 0 {
		fmt.Println("getting as a whole unit")
		out := GetWhole(ctx, req, -1)
		return meta, out, nil
	}
	fmt.Println("getting as a pool")
	out := GetPool(ctx, req, length, connections)
	return meta, out, nil
}

func GetWhole(ctx context.Context, req *Request, size int64) <-chan *Chunk {
	out := make(chan *Chunk, 1)
	go func() {
		defer func() {
			close(out)
			if e := recover(); e != nil {
				fmt.Println(e)
				debug.PrintStack()
			}
		}()
		resp, err := req.Do(ctx)
		if err != nil {
			out <- &Chunk{
				StartByte: 0,
				EndByte:   size,
				Error:     err,
			}
		} else {
			out <- &Chunk{
				StartByte: 0,
				EndByte:   size,
				Data:      resp.BodyBytes,
			}
		}
	}()
	return out
}

func GetPool(ctx context.Context, req *Request, size int64, connections int) <-chan *Chunk {
	if connections <= 0 {
		connections = 1
	}
	if connections > maxConnections {
		connections = maxConnections
	}
	var chunkSize int64 = -1
	if size < 0 {
		connections = 1
	} else {
		chunkSize = size / int64(connections)
	}
	fmt.Println("Using connections", connections)
	fmt.Println("Chunk size", chunkSize)
	noRetry := size <= 0
	tasks := make(chan *Chunk, connections*4)
	results := make(chan *Chunk, connections*4)
	out := make(chan *Chunk, connections*8)
	poolCtx, cancel := context.WithCancel(ctx)
	// runWorker workers
	wg := &sync.WaitGroup{}
	wg.Add(connections)

	// normal: tasks -> results -> out
	// retry: results(error) -> tasks -> results -> out
	// tasks closes immediately only when noRetry is true
	//

	go func() {
		defer func() {
			fmt.Println("[pool reporter] exit")
			cancel()
			close(out)
			if e := recover(); e != nil {
				fmt.Println(e)
				debug.PrintStack()
			}
		}()
		fmt.Println("[pool reporter] start")
		fmt.Println("[chunk emitter] start")
		if size < 0 {
			fmt.Printf("[chunk emitter] start: %d, end: %d\n", 0, -1)
			if !sendChunk(poolCtx, tasks, &Chunk{
				StartByte: 0,
				EndByte:   -1,
			}) {
				return
			}
		} else {
			for start := int64(0); start < size; start += chunkSize + 1 {
				select {
				case <-poolCtx.Done():
					fmt.Println("chunk emitter canceled")
					return
				default:
					end := start + chunkSize
					if end >= size {
						end = size - 1
					}
					fmt.Printf("[chunk emitter] start: %d, end: %d\n", start, end)
					if !sendChunk(poolCtx, tasks, &Chunk{
						StartByte: start,
						EndByte:   end,
					}) {
						return
					}
				}
			}
		}
		fmt.Println("[chunk emitter] all task sent")
		if noRetry {
			close(tasks)
		}
		received := int64(0)
		for {
			select {
			case <-poolCtx.Done():
				fmt.Println("pool reporter canceled")
				return
			case c, ok := <-results:
				if !ok {
					return
				}
				if c.Error == nil {
					received += c.EndByte - c.StartByte + 1
					if !sendChunk(poolCtx, out, c) {
						return
					}
					if size > 0 && received >= size {
						return // tasks and results should be empty now
					}
					continue
				}
				if noRetry {
					sendChunk(poolCtx, out, c)
					return
				}
				if c.Retry >= maxRetry {
					fmt.Println("[pool reporter] max retry exceeded")
					sendChunk(poolCtx, out, c)
					return // no more retries, tasks might not be empty but can be discarded, out must be stopped
				}
				fmt.Println("[pool reporter] retry", c.Retry)
				// emit a retry
				if !sendChunk(poolCtx, tasks, &Chunk{
					StartByte: c.StartByte,
					EndByte:   c.EndByte,
					Retry:     c.Retry + 1,
				}) {
					return
				}
			}
		}
	}()
	// run tasks
	go func() {
		defer func() {
			fmt.Println("[pool watcher] exit")
			close(results)
			if e := recover(); e != nil {
				fmt.Println(e)
				debug.PrintStack()
			}
		}()
		fmt.Println("[pool watcher] start")
		wg.Wait()
	}()

	// run workers
	for i := 0; i < connections; i++ {
		go runWorker(poolCtx, i, req, wg, tasks, results)
	}

	return out
}

func runWorker(ctx context.Context, id int, req *Request, wg *sync.WaitGroup, in <-chan *Chunk, out chan<- *Chunk) {
	defer func() {
		wg.Done()
		fmt.Printf("[worker %d] exit\n", id)
		if e := recover(); e != nil {
			fmt.Println(e)
			debug.PrintStack()
		}
	}()
	fmt.Printf("[worker %d] start\n", id)
	for {
		select {
		case <-ctx.Done():
			fmt.Println("runWorker canceled")
			return
		case t, ok := <-in:
			if !ok {
				return
			}
			fmt.Printf("[worker %d] retrive %d - %d\n", id, t.StartByte, t.EndByte)
			getByRange(ctx, req, t.StartByte, t.EndByte, t.Retry, out)
		}
	}
}

func getByRange(ctx context.Context, req *Request, startByte, endByte int64, retry int, outChan chan<- *Chunk) {
	if startByte < 0 {
		panic("startByte cannot be negative")
	}
	req = req.Clone()
	if endByte >= 0 {
		req = req.Range(startByte, endByte)
	}
	resp, cancel, err := req.WithoutTimeout().DoRaw(ctx)
	defer cancel()
	if err != nil {
		sendChunk(ctx, outChan, &Chunk{
			StartByte: startByte,
			EndByte:   endByte,
			Error:     err,
			Retry:     retry,
		})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		sendChunk(ctx, outChan, &Chunk{
			StartByte: startByte,
			EndByte:   endByte,
			Error:     fmt.Errorf("status code %d", resp.StatusCode),
			Retry:     retry,
		})
		return
	}
	if resp.ContentLength != endByte-startByte+1 {
		fmt.Printf("Warning: header length not match, req: %d, header: %d\n", endByte-startByte+1, resp.ContentLength)
	}
	buf := make([]byte, maxReadBufferSize)
	cursor := startByte
	var readSize int
	for err != io.EOF {
		select {
		case <-ctx.Done():
			fmt.Println("getByRange canceled")
			sendChunk(ctx, outChan, &Chunk{
				StartByte: cursor,
				EndByte:   endByte,
				Error:     ctx.Err(),
				Retry:     retry,
			})
			return
		default:
		}
		readSize, err = resp.Body.Read(buf)
		if err != nil && err != io.EOF {
			sendChunk(ctx, outChan, &Chunk{
				StartByte: cursor,
				EndByte:   endByte,
				Error:     err,
				Retry:     retry,
			})
			break
		}
		//fmt.Printf("read offset %d size %d\n", cursor, readSize)
		currentEnd := cursor + int64(readSize) - 1
		ret := make([]byte, readSize)
		copy(ret, buf[:readSize])
		if !sendChunk(ctx, outChan, &Chunk{
			StartByte: cursor,
			EndByte:   currentEnd,
			Data:      ret,
			Retry:     retry,
		}) {
			return
		}
		cursor = currentEnd + 1
	}
}

func sendChunk(ctx context.Context, out chan<- *Chunk, c *Chunk) bool {
	select {
	case <-ctx.Done():
		return false
	case out <- c:
		return true
	}
}

func getMD5(bs []byte) string {
	h := md5.New()
	h.Write(bs)
	return hex.EncodeToString(h.Sum(nil))
}
