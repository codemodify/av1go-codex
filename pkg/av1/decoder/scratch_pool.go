package decoder

import "sync"

type scratchBuffer[T any] struct {
	buf []T
}

func takeScratch[T any](pool *sync.Pool, n int) *scratchBuffer[T] {
	if n <= 0 {
		if v := pool.Get(); v != nil {
			scratch := v.(*scratchBuffer[T])
			scratch.buf = scratch.buf[:0]
			return scratch
		}
		return &scratchBuffer[T]{}
	}
	if v := pool.Get(); v != nil {
		scratch := v.(*scratchBuffer[T])
		if cap(scratch.buf) >= n {
			scratch.buf = scratch.buf[:n]
			return scratch
		}
		scratch.buf = make([]T, n)
		return scratch
	}
	return &scratchBuffer[T]{buf: make([]T, n)}
}

func putScratch[T any](pool *sync.Pool, scratch *scratchBuffer[T]) {
	if scratch == nil {
		return
	}
	pool.Put(scratch)
}

func putZeroScratch[T any](pool *sync.Pool, scratch *scratchBuffer[T]) {
	if scratch == nil {
		return
	}
	clear(scratch.buf)
	pool.Put(scratch)
}
