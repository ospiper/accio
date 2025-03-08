package progress

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func prepareContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func TestSimulatedBucket(t *testing.T) {
	bucket := New(10, AsIs)
	collected := make([]int64, 0)
	for i := int64(1); i <= 20; i++ {
		bucket.Report(i, i)
		collected = append(collected, bucket.Collect())
	}
	assert.Equal(t, []int64{1, 3, 6, 10, 15, 21, 28, 36, 45, 55, 65, 75, 85, 95, 105, 115, 125, 135, 145, 155}, collected)
}

func consumeChan[T any](ctx context.Context, ch <-chan T, h func(ctx context.Context, t T)) {
	for {
		select {
		case <-ctx.Done():
			return
		case v, ok := <-ch:
			if !ok {
				return
			}
			h(ctx, v)
		}
	}
}

func TestTicketBucket(t *testing.T) {
	ctx, cancel := prepareContext()
	bucket := New(50, func(in int64) int64 { // input is milliseconds
		return in / 10
	})
	reportTicker := time.NewTicker(time.Millisecond * 10)
	collectTicker := time.NewTicker(time.Millisecond * 500)
	defer func() {
		reportTicker.Stop()
		collectTicker.Stop()
		cancel()
	}()
	go func() {
		defer func() {
			fmt.Println("reporter exit")
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-reportTicker.C:
				if !ok {
					return
				}
				bucket.Report(time.Now().UnixMilli(), rand.Int64N(500))
			}
		}
	}()
	go func() {
		defer func() {
			fmt.Println("collector exit")
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-collectTicker.C:
				if !ok {
					return
				}
				fmt.Println(bucket.Collect())
			}
		}
	}()
	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Second * 30):
		return
	}
}
