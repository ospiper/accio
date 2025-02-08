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
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if connections > maxConnections {
		connections = maxConnections
	}
	chunkSize := size / int64(connections)
	fmt.Println("Using connections", connections)
	fmt.Println("Chunk size", chunkSize)
	in := make(chan *Chunk, connections*4)
	inter := make(chan *Chunk, connections*2)
	out := make(chan *Chunk, connections*8)
	// runWorker workers
	wg := &sync.WaitGroup{}
	wg.Add(connections)

	// normal: in -> inter -> out
	// retry: in -> inter -> retry -> in -> inter -> out
	// in closes at all task emit for the first time
	// retry closes at no inter is received
	//

	go func() {
		defer func() {
			fmt.Println("[pool reporter] exit")
			close(in)
			close(out)
			cancel()
			if e := recover(); e != nil {
				fmt.Println(e)
				debug.PrintStack()
			}
		}()
		fmt.Println("[pool reporter] start")
		received := int64(0)
		for {
			select {
			case <-ctx.Done():
				return
			case c, ok := <-inter:
				if !ok {
					return
				}
				if c.Error == nil {
					received += c.EndByte - c.StartByte + 1
					out <- c
					if received >= size {
						return // in, inter and out should all be empty now
					}
					continue
				}
				if c.Retry >= maxRetry {
					fmt.Println("[pool reporter] max retry exceeded")
					out <- c
					return // no more retries, in might not be empty but can be discarded, inter might not be empty but can be discarded, out must be stopped
				} else {
					fmt.Println("[pool reporter] retry")
					// emit a retry
					in <- &Chunk{
						StartByte: c.StartByte,
						EndByte:   c.EndByte,
						Retry:     c.Retry + 1,
					}
				}
			}
		}
	}()
	go func() {
		defer func() {
			fmt.Println("[chunk emitter] exit")
			if e := recover(); e != nil {
				fmt.Println(e)
				debug.PrintStack()
			}
		}()
		fmt.Println("[chunk emitter] start")
		for start := int64(0); start < size; start += chunkSize + 1 {
			select {
			case <-ctx.Done():
				return
			default:
				end := start + chunkSize
				if end >= size {
					end = size - 1
				}
				fmt.Printf("[chunk emitter] start: %d, end: %d\n", start, end)
				in <- &Chunk{
					StartByte: start,
					EndByte:   end,
				}
			}
		}
		fmt.Println("[chunk emitter] all task sent")
	}()
	// run tasks
	go func() {
		defer func() {
			fmt.Println("[pool watcher] exit")
			close(inter)
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
		go runWorker(ctx, i, req, wg, in, inter)
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
			return
		case t, ok := <-in:
			if !ok {
				return
			}
			fmt.Printf("[worker %d] retrive %d - %d\n", id, t.StartByte, t.EndByte)
			getByRange(ctx, req, t.StartByte, t.EndByte, out)
		}
	}
}

func getByRange(ctx context.Context, req *Request, startByte, endByte int64, outChan chan<- *Chunk) {
	if startByte < 0 {
		panic("startByte cannot be negative")
	}
	req = req.Range(startByte, endByte)
	resp, err := req.WithoutTimeout().DoRaw(ctx)
	if err != nil {
		outChan <- &Chunk{
			StartByte: startByte,
			EndByte:   endByte,
			Error:     err,
		}
		return
	}
	defer resp.Body.Close()
	if resp.ContentLength != endByte-startByte+1 {
		fmt.Printf("Warning: header length not match, req: %d, header: %d\n", endByte-startByte+1, resp.ContentLength)
	}
	buf := make([]byte, maxReadBufferSize)
	var cursor int64
	var readSize int
	for err != io.EOF {
		select {
		case <-ctx.Done():
			outChan <- &Chunk{
				StartByte: cursor,
				EndByte:   endByte,
				Error:     ctx.Err(),
			}
			return
		default:
		}
		readSize, err = resp.Body.Read(buf)
		if err != nil && err != io.EOF {
			outChan <- &Chunk{
				StartByte: cursor,
				EndByte:   endByte,
				Error:     err,
			}
			break
		}
		currentEnd := cursor + int64(readSize) - 1
		ret := make([]byte, readSize)
		copy(ret, buf[:readSize])
		outChan <- &Chunk{
			StartByte: cursor,
			EndByte:   currentEnd,
			Data:      ret,
		}
		cursor = currentEnd + 1
	}
}

func getMD5(bs []byte) string {
	h := md5.New()
	h.Write(bs)
	return hex.EncodeToString(h.Sum(nil))
}
