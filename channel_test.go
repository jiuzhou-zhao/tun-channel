package udp_channel

import (
	"context"
	"fmt"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/jiuzhou-zhao/udp-channel/pkg"
	"github.com/stretchr/testify/assert"
)

type TestLocalKeyParser struct {
}

func (parser *TestLocalKeyParser) ParseData(d []byte) (key string, dd []byte, err error) {
	return "129.1.1.10", d, nil
}

func TestChannel(t *testing.T) {
	serverAddr := "127.0.0.1:9877"

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer func() {
			wg.Done()
			log.Println("ut exit server routine")
		}()

		svr, err := NewChannelServer(ctx, serverAddr, nil, &TestLocalKeyParser{}, pkg.NewAESEnDecrypt("12"), "")
		assert.Nil(t, err)
		svr.Wait()
	}()

	wg.Add(1)
	go func() {
		defer func() {
			wg.Done()
			log.Println("ut exit client routine")
		}()

		cli, err := NewChannelClient(ctx, serverAddr, "129.1.1.10", nil, pkg.NewAESEnDecrypt("12"))
		assert.Nil(t, err)

		go func() {
			for d := range cli.ReadPackageChan() {
				log.Printf("client receive %v", string(d))
			}

		}()

		for idx := 0; idx < 10; idx++ {
			cli.WritePackage([]byte(fmt.Sprintf("--%v--", idx)))
			time.Sleep(time.Second)
		}

		cli.StopAndWait()
	}()

	wg.Wait()
}
