package tunchannel

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/jiuzhou-zhao/data-channel/dataprocessor"
	"github.com/jiuzhou-zhao/data-channel/inter"
	"github.com/jiuzhou-zhao/data-channel/tcp"
	"github.com/jiuzhou-zhao/data-channel/wrapper"
	"github.com/sgostarter/i/l"
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
	rLog := l.NewCommLogger(&l.FmtRecorder{})
	log := l.NewWrapper(rLog).WithFields(l.FieldString("role", "udpServer"))

	serverAddr := "127.0.0.1:9877"

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sdps := []inter.ServerDataProcessor{
		dataprocessor.NewServerEncryptDataProcess([]byte{0x01}),
	}
	cdps := []inter.ClientDataProcessor{
		dataprocessor.NewClientEncryptDataProcess([]byte{0x01}),
	}

	/*
		s, err := udp.NewServer(ctx, serverAddr, nil, log)
		assert.Nil(t, err)

		c, err := udp.NewClient(ctx, serverAddr, nil, log)
		assert.Nil(t, err)

	*/

	//*
	sdps = append([]inter.ServerDataProcessor{dataprocessor.NewServerTCPBag()}, sdps...)
	cdps = append([]inter.ClientDataProcessor{dataprocessor.NewClientTCPBag()}, cdps...)

	s, err := tcp.NewServer(ctx, serverAddr, nil, log)
	assert.Nil(t, err)

	c, err := tcp.NewClient(ctx, serverAddr, nil, log)
	assert.Nil(t, err)
	//*/

	sw := wrapper.NewServer(s, log, sdps...)
	sc := wrapper.NewClient(c, log, cdps...)
	testChannelEx(ctx, t, sw, sc, log)
}

func testChannelEx(ctx context.Context, t *testing.T, s inter.Server, c inter.Client, log l.Wrapper) {
	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer func() {
			wg.Done()
			log.Info("ut exit server routine")
		}()

		svr, err := NewChannelServer(ctx, log, &TestLocalKeyParser{}, s, "")
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
			Key:               "129.1.1.10",
			ClientDataChannel: c,
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
