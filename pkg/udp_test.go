package pkg

import (
	"context"
	"github.com/sgostarter/i/logger"
	"github.com/stretchr/testify/assert"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestUDPServer(t *testing.T) {
	svrAddress := "127.0.0.1:9900"

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	log := logger.NewWrapper(logger.NewCommLogger(&logger.FmtRecorder{})).WithFields(logger.FieldString("role", "udpClient"))

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		svr, err := NewUDPServer(ctx, svrAddress, 0, log, nil)
		assert.Nil(t, err)
		assert.NotNil(t, svr)

		go func() {
			time.Sleep(5 * time.Second)
			svr.StopAndWait()
		}()

		datas := <-svr.ChRead
		v, err := strconv.ParseInt(string(datas.Package), 10, 64)
		assert.Nil(t, err)
		svr.ChWrite <- &UDPPackage{
			Package: []byte(strconv.Itoa(int(v) + 1)),
			Addr:    datas.Addr,
		}

		datas = <-svr.ChRead
		v, err = strconv.ParseInt(string(datas.Package), 10, 64)
		assert.Nil(t, err)
		svr.ChWrite <- &UDPPackage{
			Package: []byte(strconv.Itoa(int(v) + 1)),
			Addr:    datas.Addr,
		}

		time.Sleep(4 * time.Second)
		svr.StopAndWait()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		time.Sleep(time.Second)

		cli, err := NewUDPClient(ctx, svrAddress, 0, log, nil)
		assert.Nil(t, err)
		assert.NotNil(t, cli)

		go func() {
			time.Sleep(6 * time.Second)
			cli.StopAndWait()
		}()

		assert.Nil(t, err)
		cli.ChWrite <- []byte(strconv.Itoa(10))
		datas := <-cli.ChRead
		v, err := strconv.Atoi(string(datas))
		assert.Nil(t, err)
		assert.Equal(t, v, 11)
		v++
		cli.ChWrite <- []byte(strconv.Itoa(v))
		datas = <-cli.ChRead
		v, err = strconv.Atoi(string(datas))
		assert.Nil(t, err)
		assert.Equal(t, v, 13)

		cli.StopAndWait()
	}()

	wg.Wait()
}
