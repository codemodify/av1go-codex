package decoder

import "sync"

// stickyScratchCache keeps a small set of reusable large buffers alive across
// GC cycles. This is more effective than sync.Pool for decoder-sized scratch.
type stickyScratchCache[T any] struct {
	mu        sync.Mutex
	maxStored int
	bufs      []*scratchBuffer[T]
}

func (c *stickyScratchCache[T]) take(n int) *scratchBuffer[T] {
	if n < 0 {
		n = 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	best := -1
	for i, scratch := range c.bufs {
		if cap(scratch.buf) < n {
			continue
		}
		if best < 0 || cap(scratch.buf) < cap(c.bufs[best].buf) {
			best = i
		}
	}
	if best >= 0 {
		scratch := c.bufs[best]
		last := len(c.bufs) - 1
		c.bufs[best] = c.bufs[last]
		c.bufs = c.bufs[:last]
		scratch.buf = scratch.buf[:n]
		return scratch
	}
	return &scratchBuffer[T]{buf: make([]T, n)}
}

func (c *stickyScratchCache[T]) put(scratch *scratchBuffer[T]) {
	if scratch == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	maxStored := c.maxStored
	if maxStored <= 0 {
		maxStored = 4
	}
	if len(c.bufs) < maxStored {
		c.bufs = append(c.bufs, scratch)
		return
	}
	smallest := 0
	for i := 1; i < len(c.bufs); i++ {
		if cap(c.bufs[i].buf) < cap(c.bufs[smallest].buf) {
			smallest = i
		}
	}
	if cap(scratch.buf) > cap(c.bufs[smallest].buf) {
		c.bufs[smallest] = scratch
	}
}

func (c *stickyScratchCache[T]) putZero(scratch *scratchBuffer[T]) {
	if scratch == nil {
		return
	}
	clear(scratch.buf)
	c.put(scratch)
}
