package bytespool

import "sync"

func createAllocFunc(size int32) func() interface{} {
	return func() interface{} {
		return make([]byte, size)
	}
}

// The following parameters controls the size of buffer pools.
// There are numPools pools. Starting from 4k size, the size of each pool is sizeMulti of the previous one.
// Package buf is guaranteed to not use buffers larger than the largest pool.
// Other packets may use larger buffers.
const (
	numPools  = 5
	sizeMulti = 2
)

var (
	pool     [numPools]sync.Pool
	poolSize [numPools]int32
)

func init() {
	size := int32(4096)
	for i := 0; i < numPools; i++ {
		pool[i] = sync.Pool{
			New: createAllocFunc(size),
		}
		poolSize[i] = size
		size *= sizeMulti
	}
}

// GetPool returns a sync.Pool that generates bytes array with at least the given size.
// It may return nil if no such pool exists.
//
// v2ray:api:stable
func GetPool(size int32) *sync.Pool {
	for idx, ps := range poolSize {
		if size <= ps {
			return &pool[idx]
		}
	}
	return nil
}

// Alloc returns a byte slice with at least the given size. Minimum size of returned slice is 4096.
//
// v2ray:api:stable
func Alloc(size int32) []byte {
	pool := GetPool(size)
	if pool != nil {
		return pool.Get().([]byte)
	}
	return make([]byte, size)
}

// Free puts a byte slice into the internal pool.
//
// v2ray:api:stable
func Free(b []byte) {
	size := int32(cap(b))
	b = b[0:cap(b)]
	for i := numPools - 1; i >= 0; i-- {
		if size >= poolSize[i] {
			pool[i].Put(b) // nolint: staticcheck
			return
		}
	}
}
