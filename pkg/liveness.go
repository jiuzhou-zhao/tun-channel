package pkg

import (
	"context"
	"time"
)

type LiveItem interface {
	Key() string
	DoPingRequest()
	OnRemoved()
}

type LivePool struct {
	ctx                 context.Context
	ctxCancel           context.CancelFunc
	checkInterval       time.Duration
	checkFailedInterval time.Duration

	itemsMap map[string]*liveItemInfo

	addChan   chan LiveItem
	checkChan chan interface{}
	pongChan  chan LiveItem
}

type liveItemInfo struct {
	item     LiveItem
	lastTime time.Time
}

func NewLivePool(ctx context.Context, checkInterval time.Duration, checkFailedInterval time.Duration) *LivePool {
	pool := &LivePool{
		checkInterval:       checkInterval,
		checkFailedInterval: checkFailedInterval,
		itemsMap:            make(map[string]*liveItemInfo),
		addChan:             make(chan LiveItem, 10),
		checkChan:           make(chan interface{}, 10),
		pongChan:            make(chan LiveItem, 10),
	}
	pool.ctx, pool.ctxCancel = context.WithCancel(ctx)

	go pool.checkRoutine()
	go pool.mainRoutine()

	return pool
}

func (pool *LivePool) checkRoutine() {
	loop := true

	for loop {
		select {
		case <-pool.ctx.Done():
			loop = false
		case <-time.After(pool.checkInterval):
			pool.checkChan <- 1
		}
	}
}

func (pool *LivePool) mainRoutine() {
	loop := true

	for loop {
		select {
		case <-pool.ctx.Done():
			loop = false
		case item := <-pool.addChan:
			if ii, ok := pool.itemsMap[item.Key()]; ok {
				ii.lastTime = time.Now()
			} else {
				pool.itemsMap[item.Key()] = &liveItemInfo{
					item:     item,
					lastTime: time.Now(),
				}
				go func() {
					item.DoPingRequest()
				}()
			}
		case <-pool.checkChan:
			for _, ii := range pool.itemsMap {
				if time.Since(ii.lastTime) >= pool.checkFailedInterval {
					delete(pool.itemsMap, ii.item.Key())

					go func(item LiveItem) {
						item.OnRemoved()
					}(ii.item)

					continue
				}

				go func(item LiveItem) {
					item.DoPingRequest()
				}(ii.item)
			}
		case item := <-pool.pongChan:
			if ii, ok := pool.itemsMap[item.Key()]; ok {
				ii.lastTime = time.Now()
			}
		}
	}
}

func (pool *LivePool) Add(item LiveItem) {
	pool.addChan <- item
}

func (pool *LivePool) OnPongResponse(item LiveItem) {
	pool.pongChan <- item
}
