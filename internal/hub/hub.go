package hub

import (
	"sync"
	"sync/atomic"
)

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

type subscriber struct {
	ch     chan Event
	closed atomic.Bool
}

type Hub struct {
	subs    sync.Map
	count   atomic.Int64
	queueSz int
}

func New(queueSz int) *Hub {
	if queueSz <= 0 {
		queueSz = 32
	}
	return &Hub{queueSz: queueSz}
}

func (h *Hub) Subscribe(key string) (<-chan Event, func()) {
	s := &subscriber{ch: make(chan Event, h.queueSz)}
	prev, _ := h.subs.LoadOrStore(key, &sync.Map{})
	m := prev.(*sync.Map)
	id := h.count.Add(1)
	m.Store(id, s)
	cancel := func() {
		if s.closed.CompareAndSwap(false, true) {
			close(s.ch)
		}
		m.Delete(id)
	}
	return s.ch, cancel
}

func (h *Hub) Publish(key string, e Event) {
	v, ok := h.subs.Load(key)
	if !ok {
		return
	}
	m := v.(*sync.Map)
	m.Range(func(_ any, v any) bool {
		s := v.(*subscriber)
		if s.closed.Load() {
			return true
		}
		select {
		case s.ch <- e:
		default:
		}
		return true
	})
}
