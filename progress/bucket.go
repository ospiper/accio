package progress

import "sync"

type HashFn = func(in int64) int64

var AsIs HashFn = nil

type Bucket struct {
	bs         []int64
	hashFn     func(in int64) int64
	lastBucket int
	mu         sync.Mutex
}

func New(size int, hashFn HashFn) *Bucket {
	return &Bucket{
		bs:     make([]int64, size),
		hashFn: hashFn,
	}
}

func (b *Bucket) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.bs)
}

func (b *Bucket) Report(in int64, inc int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var h int64
	if b.hashFn != nil {
		h = b.hashFn(in)
	} else {
		h = in
	}
	bucketNo := int(h % int64(len(b.bs)))
	if bucketNo != b.lastBucket {
		b.bs[bucketNo] = inc
		b.lastBucket = bucketNo
	} else {
		b.bs[bucketNo] += inc
	}
}

func (b *Bucket) Collect() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	var ret int64
	for _, i := range b.bs {
		ret += i
	}
	return ret
}
