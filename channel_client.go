package udp_channel

import (
	"context"
	"sync"
	"time"

	"github.com/jiuzhou-zhao/udp-channel/internal/proto"
	"github.com/jiuzhou-zhao/udp-channel/pkg"
	"github.com/sgostarter/i/logger"
)

type ChannelClient struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	wg sync.WaitGroup

	vip    string
	logger logger.Wrapper

	udpCli *pkg.UDPClient

	readPackageChan chan []byte

	lastTouchTime time.Time
}

func NewChannelClient(ctx context.Context, svrAddress string, vip string, log logger.Wrapper, crypt pkg.EnDecrypt) (*ChannelClient, error) {
	if log == nil {
		log = logger.NewWrapper(&logger.NopLogger{}).WithFields(logger.FieldString("role", "channelClient"))
	}

	chnClient := &ChannelClient{
		vip:             vip,
		logger:          log,
		readPackageChan: make(chan []byte, 10),
		lastTouchTime:   time.Now(),
	}

	chnClient.ctx, chnClient.ctxCancel = context.WithCancel(ctx)

	udpCli, err := pkg.NewUDPClient(chnClient.ctx, svrAddress, 0, log, crypt)
	if err != nil {
		log.Errorf("new udp client failed: %v", err)
		return nil, err
	}

	chnClient.udpCli = udpCli

	chnClient.wg.Add(1)
	go chnClient.reader()
	chnClient.wg.Add(1)
	go chnClient.checker()

	return chnClient, nil
}

func (cli *ChannelClient) reader() {
	log := cli.logger.WithFields(logger.FieldString("module", "reader"))

	log.Infof("enter channel client reader")
	defer func() {
		cli.wg.Done()
		log.Infof("leave channel client reader")
	}()

	var quit bool
	for !quit {
		select {
		case <-cli.ctx.Done():
			quit = true
		case d := <-cli.udpCli.ChRead:
			m, d, e := proto.Decode(d)
			if e != nil {
				log.Errorf("decode data failed: %v", e)
				continue
			}
			log.Debugf("receive udp package [len:%v] %v", len(d), m)
			switch m {
			case proto.MethodPing:
				log.Debug("receive ping message")
				cli.udpCli.ChWrite <- proto.BuildPongMethodData(d)
			case proto.MethodPong:
				log.Debug("receive pong message")
				cli.lastTouchTime = time.Now()
			case proto.MethodKeyRequest:
				log.Debug("receive key request message")
				cli.udpCli.ChWrite <- proto.BuildKeyResponseData(cli.vip)
			case proto.MethodKeyResponse:
			case proto.MethodData:
				cli.readPackageChan <- d
			}
		}
	}
}

func (cli *ChannelClient) checker() {
	log := cli.logger.WithFields(logger.FieldString("module", "checker"))

	log.Infof("enter channel client checker")
	defer func() {
		cli.wg.Done()
		log.Infof("leave channel client checker")
	}()

	var quit bool
	for !quit {
		select {
		case <-cli.ctx.Done():
			quit = true
		case <-time.After(30 * time.Second):
			cli.udpCli.ChWrite <- proto.BuildPingMethodData(nil)
			if time.Since(cli.lastTouchTime) > time.Minute {
				log.Errorf("server miss: %v", time.Since(cli.lastTouchTime))
			}
		}
	}
}

func (cli *ChannelClient) WritePackage(d []byte) {
	cli.udpCli.ChWrite <- proto.BuildData(d)
}

func (cli *ChannelClient) ReadPackageChan() <-chan []byte {
	return cli.readPackageChan
}

func (cli *ChannelClient) StopAndWait() {
	cli.ctxCancel()
	cli.udpCli.StopAndWait()
	cli.wg.Wait()
}

func (cli *ChannelClient) Wait() {
	cli.udpCli.Wait()
	cli.wg.Wait()
}
