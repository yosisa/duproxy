package main

import (
	"io"
	"sync"
)

type MultiReader struct {
	src  io.Reader
	pos  map[*pseudoReader]*position
	bufs [][]byte
	err  error
	sync.RWMutex
}

func NewMultiReader(src io.Reader) *MultiReader {
	return &MultiReader{
		src: src,
		pos: make(map[*pseudoReader]*position),
	}
}

func (r *MultiReader) Reader() io.ReadCloser {
	pr := &pseudoReader{r, make(chan struct{}, 1)}
	r.Lock()
	r.pos[pr] = new(position)
	r.Unlock()
	return pr
}

func (r *MultiReader) Read(p []byte) (n int, err error) {
	n, err = r.src.Read(p)
	if n > 0 {
		b := make([]byte, n)
		copy(b, p[:n])
		r.bufs = append(r.bufs, b)
		r.gc()
	}
	if err != nil {
		r.err = err
	}
	r.broadcast()
	return
}

func (r *MultiReader) gc() {
	minIndex := -1
	for _, pos := range r.pos {
		if minIndex == -1 || pos.index < minIndex {
			minIndex = pos.index
		}
	}
	for i := 0; i < minIndex; i++ {
		r.bufs[i] = nil
	}
}

func (r *MultiReader) broadcast() {
	r.RLock()
	defer r.RUnlock()
	for pr := range r.pos {
		select {
		case pr.c <- struct{}{}:
		default:
		}
	}
}

func (r *MultiReader) getpos(pr *pseudoReader) *position {
	r.RLock()
	defer r.RUnlock()
	return r.pos[pr]
}

type pseudoReader struct {
	parent *MultiReader
	c      chan struct{}
}

func (r *pseudoReader) Read(p []byte) (n int, err error) {
	pos := r.parent.getpos(r)
	for pos.index >= len(r.parent.bufs) && r.parent.err == nil {
		<-r.c
	}
	if pos.index >= len(r.parent.bufs) {
		return 0, r.parent.err
	}

	b := r.parent.bufs[pos.index]
	n = copy(p, b[pos.n:])
	if n <= len(p) {
		pos.index++
		pos.n = 0
	} else {
		pos.n += n
	}
	return
}

func (r *pseudoReader) Close() error {
	r.parent.Lock()
	defer r.parent.Unlock()
	delete(r.parent.pos, r)
	return nil
}

type position struct {
	index int
	n     int
}
