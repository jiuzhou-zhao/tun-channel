package udp_channel

import (
	"context"
	"github.com/jiuzhou-zhao/udp-channel/internal/proto"
	"github.com/jiuzhou-zhao/udp-channel/pkg"
	"sync"
	"time"
)

type ChannelClient struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	wg sync.WaitGroup

	vip    string
	logger pkg.Logger

	udpCli *pkg.UDPClient

	readPackageChan chan []byte

	lastTouchTime time.Time
}

func NewChannelClient(ctx context.Context, svrAddress string, vip string, logger pkg.Logger, crypt pkg.EnDecrypt) (*ChannelClient, error) {
	if logger == nil {
		logger = &pkg.ConsoleLogger{}
	}

	chnClient := &ChannelClient{
		vip:             vip,
		logger:          logger,
		readPackageChan: make(chan []byte, 10),
		lastTouchTime:   time.Now(),
	}

	chnClient.ctx, chnClient.ctxCancel = context.WithCancel(ctx)

	udpCli, err := pkg.NewUDPClient(chnClient.ctx, svrAddress, 0, logger, crypt)
	if err != nil {
		logger.Errorf("new udp client failed: %v", err)
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
	cli.logger.Infof("enter channel client reader")
	defer func() {
		cli.wg.Done()
		cli.logger.Infof("leave channel client reader")
	}()

	var quit bool
	for !quit {
		select {
		case <-cli.ctx.Done():
			quit = true
		case d := <-cli.udpCli.ChRead:
			m, d, e := proto.Decode(d)
			if e != nil {
				cli.logger.Errorf("decode data failed: %v", e)
				continue
			}
			cli.logger.Debugf("receive udp package [len:%v] %v", len(d), m)
			switch m {
			case proto.MethodPing:
				cli.logger.Debug("receive ping message")
				cli.udpCli.ChWrite <- proto.BuildPongMethodData(d)
			case proto.MethodPong:
				cli.logger.Debug("receive pong message")
				cli.lastTouchTime = time.Now()
			case proto.MethodKeyRequest:
				cli.logger.Debug("receive key request message")
				cli.udpCli.ChWrite <- proto.BuildKeyResponseData(cli.vip)
			case proto.MethodKeyResponse:
			case proto.MethodData:
				cli.readPackageChan <- d
			}
		}
	}
}

func (cli *ChannelClient) checker() {
	cli.logger.Infof("enter channel client checker")
	defer func() {
		cli.wg.Done()
		cli.logger.Infof("leave channel client checker")
	}()

	var quit bool
	for !quit {
		select {
		case <-cli.ctx.Done():
			quit = true
		case <-time.After(30 * time.Second):
			cli.udpCli.ChWrite <- proto.BuildPingMethodData(nil)
			if time.Since(cli.lastTouchTime) > time.Minute {
				cli.logger.Errorf("server miss: %v", time.Since(cli.lastTouchTime))
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
