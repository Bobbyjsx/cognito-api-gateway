package proxy

import "sync"

// BufferPool implements httputil.BufferPool using sync.Pool for high-throughput zero-allocation proxying.
type BufferPool struct {
	pool sync.Pool
}

// NewBufferPool creates a 32KB buffer pool.
func NewBufferPool() *BufferPool {
	return &BufferPool{
		pool: sync.Pool{
			New: func() any {
				b := make([]byte, 32*1024) // 32KB chunks
				return &b
			},
		},
	}
}

// Get fetches a slice from the pool.
func (p *BufferPool) Get() []byte {
	return *p.pool.Get().(*[]byte)
}

// Put returns the slice to the pool.
func (p *BufferPool) Put(b []byte) {
	p.pool.Put(&b)
}
