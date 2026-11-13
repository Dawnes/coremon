package agent

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"coremon/internal/proto"
)

type outputBuffer struct {
	mu    sync.Mutex
	bufs  map[string]*strings.Builder
	order []string
	cl    *client
	kick  chan struct{}
}

func newOutputBuffer(cl *client) *outputBuffer {
	return &outputBuffer{bufs: map[string]*strings.Builder{}, cl: cl, kick: make(chan struct{}, 1)}
}

func (o *outputBuffer) add(cmdID, line string) {
	o.mu.Lock()
	b, ok := o.bufs[cmdID]
	if !ok {
		b = &strings.Builder{}
		o.bufs[cmdID] = b
		o.order = append(o.order, cmdID)
	}
	b.WriteString(line)
	if !strings.HasSuffix(line, "\n") {
		b.WriteByte('\n')
	}
	big := b.Len() > 32*1024
	o.mu.Unlock()
	if big {
		select {
		case o.kick <- struct{}{}:
		default:
		}
	}
}

func (o *outputBuffer) take() []proto.OutputItem {
	o.mu.Lock()
	defer o.mu.Unlock()
	var items []proto.OutputItem
	for _, id := range o.order {
		if b := o.bufs[id]; b.Len() > 0 {
			items = append(items, proto.OutputItem{CommandID: id, Data: b.String()})
		}
	}
	o.bufs = map[string]*strings.Builder{}
	o.order = nil
	return items
}

func (o *outputBuffer) flush(ctx context.Context) {
	items := o.take()
	if len(items) == 0 {
		return
	}
	for attempt := 0; attempt < 3; attempt++ {
		err := o.cl.postJSON(ctx, "/api/agent/output", proto.OutputBatch{Items: items}, nil, 30*time.Second)
		if err == nil {
			return
		}
		log.Printf("output upload failed: %v", err)
		time.Sleep(time.Duration(attempt+1) * time.Second)
	}
}

func (o *outputBuffer) loop(ctx context.Context) {
	t := time.NewTicker(700 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-o.kick:
		}
		o.flush(ctx)
	}
}
