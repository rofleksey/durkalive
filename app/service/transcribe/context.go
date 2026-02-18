package transcribe

import (
	"context"
	"sync"
	"time"
)

var _ context.Context = (*Context)(nil)

type Context struct {
	ctx        context.Context
	cancelFunc context.CancelCauseFunc

	phraseChan chan string
	mu         sync.RWMutex
	cancelled  bool
}

func NewContext(ctx context.Context) *Context {
	ctx, cancel := context.WithCancelCause(ctx)
	return &Context{
		ctx:        ctx,
		cancelFunc: cancel,
		phraseChan: make(chan string, 32),
	}
}

func (c *Context) Deadline() (deadline time.Time, ok bool) {
	return c.ctx.Deadline()
}

func (c *Context) Value(key any) any {
	return c.ctx.Value(key)
}

func (c *Context) GetPhraseChannel() <-chan string {
	return c.phraseChan
}

func (c *Context) Cancel(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.cancelled {
		c.cancelled = true
		c.cancelFunc(err)
	}
}

func (c *Context) sendPhrase(phrase string) {
	select {
	case c.phraseChan <- phrase:
	case <-c.ctx.Done():
	}
}

func (c *Context) Done() <-chan struct{} {
	return c.ctx.Done()
}

func (c *Context) Err() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return context.Cause(c.ctx)
}
