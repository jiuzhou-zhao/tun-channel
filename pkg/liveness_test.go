package pkg

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type TestLiveItem struct {
	key        string
	pingReqCnt int
	removed    int
}

func (item *TestLiveItem) Key() string {
	return item.key
}

func (item *TestLiveItem) DoPingRequest() {
	item.pingReqCnt++
}
func (item *TestLiveItem) OnRemoved() {
	item.removed++
}

func TestLiveness(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pool := NewLivePool(ctx, 500*time.Millisecond, 1500*time.Millisecond)
	item := &TestLiveItem{key: "1"}
	pool.Add(item)

	time.Sleep(2 * time.Second)
	assert.Equal(t, 3, item.pingReqCnt)
	assert.Equal(t, 1, item.removed)
}

func TestLiveness2(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := NewLivePool(ctx, 500*time.Millisecond, 1500*time.Millisecond)
	item := &TestLiveItem{key: "1"}
	pool.Add(item)

	for idx := 0; idx < 10; idx++ {
		time.Sleep(500 * time.Millisecond)
		pool.OnPongResponse(&TestLiveItem{key: "1"})
	}

	assert.Equal(t, 0, item.removed)
	t.Log(item.pingReqCnt)

	time.Sleep(2 * time.Second)
	assert.Equal(t, 1, item.removed)
}
