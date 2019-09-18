package azureapi

import (
	"sync"
)

// Async simple async interface
type Async interface {
	Send(f AsyncMessage)
	Wait()
}

type AsyncMessage struct {
	F    func(interface{})
	Data interface{}
}

type async struct {
	funcs chan AsyncMessage
	wg    *sync.WaitGroup
}

// NewAsync instantiates a new Async object
func NewAsync(concurrency int) Async {
	a := &async{}
	a.funcs = make(chan AsyncMessage, concurrency*2)
	a.wg = &sync.WaitGroup{}
	a.wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			for each := range a.funcs {
				each.F(each.Data)
			}
			a.wg.Done()
		}()
	}
	return a
}

func (a *async) Send(f AsyncMessage) {
	a.funcs <- f
}

func (a *async) Wait() {
	close(a.funcs)
	a.wg.Wait()
}
