package udpchannel

import (
	"context"
	"sync"
	"time"

	"github.com/jiuzhou-zhao/udp-channel/internal/proto"
	"github.com/jiuzhou-zhao/udp-channel/pkg"
	"github.com/sgostarter/i/logger"
)

type IncomingMsg struct {
	Data              []byte
	AddedForwardIPs   []string
	RemovedForwardIPs []string
	Error             error
}

type ChannelClient struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	wg sync.WaitGroup

	vip    string
	vpnIPs []string
	lanIPs []string
	logger logger.Wrapper

	udpCli *pkg.UDPClient

	incomingMsgChan chan *IncomingMsg

	lastTouchTime time.Time
}

type ChannelClientData struct {
	ServerAddr string
	VIP        string
	VpnIPs     []string
	LanIPs     []string
	Log        logger.Wrapper
	Crypt      pkg.EnDecrypt
}

func NewChannelClient(ctx context.Context, d *ChannelClientData) (*ChannelClient, error) {
	if d.Log == nil {
		d.Log = logger.NewWrapper(&logger.NopLogger{}).WithFields(logger.FieldString("role", "channelClient"))
	}

	chnClient := &ChannelClient{
		vip:             d.VIP,
		vpnIPs:          d.VpnIPs,
		lanIPs:          d.LanIPs,
		logger:          d.Log,
		incomingMsgChan: make(chan *IncomingMsg, 10),
		lastTouchTime:   time.Now(),
	}

	chnClient.ctx, chnClient.ctxCancel = context.WithCancel(ctx)

	udpCli, err := pkg.NewUDPClient(chnClient.ctx, d.ServerAddr, 0, d.Log, d.Crypt)
	if err != nil {
		d.Log.Errorf("new udp client failed: %v", err)

		return nil, err
	}

	chnClient.udpCli = udpCli

	chnClient.wg.Add(1)

	go chnClient.reader()

	chnClient.wg.Add(1)

	go chnClient.checker()

	return chnClient, nil
}

// nolint: cyclop
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
				cli.udpCli.ChWrite <- proto.BuildKeyResponseData(cli.vip, cli.vpnIPs, cli.lanIPs)
			case proto.MethodKeyResponse:
			case proto.MethodData:
				cli.incomingMsgChan <- &IncomingMsg{
					Data: d,
				}
			case proto.MethodForwardControl:
				oldIPs, newIPs, err := proto.ParseForwardControlPayloadData(d)
				cli.incomingMsgChan <- &IncomingMsg{
					AddedForwardIPs:   newIPs,
					RemovedForwardIPs: oldIPs,
					Error:             err,
				}
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

func (cli *ChannelClient) ReadIncomingMsgChan() <-chan *IncomingMsg {
	return cli.incomingMsgChan
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
