package udpchannel

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/jiuzhou-zhao/udp-channel/pkg"
	"github.com/sgostarter/i/logger"
	"github.com/stretchr/testify/assert"
)

type TestLocalKeyParser struct {
}

func (parser *TestLocalKeyParser) ParseData(d []byte) (key string, dd []byte, err error) {
	return "129.1.1.10", d, nil
}

func (parser *TestLocalKeyParser) ParseKeyFromIPOrCIDR(s string) (key string, err error) {
	return "129.1.1.10", nil
}

func (parser *TestLocalKeyParser) CompareKeyWithCIDR(key string, cidr string) bool {
	return true
}

func TestChannel(t *testing.T) {
	serverAddr := "127.0.0.1:9877"

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	rLog := logger.NewCommLogger(&logger.FmtRecorder{})
	log := logger.NewWrapper(rLog).WithFields(logger.FieldString("role", "udpServer"))

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer func() {
			wg.Done()
			log.Info("ut exit server routine")
		}()

		svr, err := NewChannelServer(ctx, serverAddr, log, &TestLocalKeyParser{}, pkg.NewAESEnDecrypt("12"), "")
		assert.Nil(t, err)
		svr.Wait()
	}()

	wg.Add(1)

	go func() {
		defer func() {
			wg.Done()
			log.Info("ut exit client routine")
		}()

		d := &ChannelClientData{
			ServerAddr: serverAddr,
			Key:        "129.1.1.10",
			Crypt:      pkg.NewAESEnDecrypt("12"),
		}
		cli, err := NewChannelClient(ctx, d)
		assert.Nil(t, err)

		go func() {
			for d := range cli.ReadIncomingMsgChan() {
				log.Infof("client receive %v", string(d.Data))
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

func TestIP(t *testing.T) {
	a := net.ParseIP("192.168.31.12").DefaultMask()
	// a := net.IPMask(net.ParseIP("192.168.31.12"))
	t.Log(a.Size())
	t.Log(a.String())
}
