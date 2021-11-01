package udpchannel

import (
	"context"
	"sync"
	"time"

	"github.com/jiuzhou-zhao/data-channel/inter"
	"github.com/jiuzhou-zhao/udp-channel/internal/proto"
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

	key    string
	vpnIPs []string
	lanIPs []string
	logger logger.Wrapper

	client inter.Client

	incomingMsgChan chan *IncomingMsg

	lastTouchTime time.Time
}

type ChannelClientData struct {
	Key               string // VIP CIDR
	VpnIPs            []string
	LanIPs            []string
	Log               logger.Wrapper
	ClientDataChannel inter.Client
}

func NewChannelClient(ctx context.Context, d *ChannelClientData) (*ChannelClient, error) {
	if d.Log == nil {
		d.Log = logger.NewWrapper(&logger.NopLogger{}).WithFields(logger.FieldString("role", "channelClient"))
	}

	vpnCidrs := make([]string, 0, len(d.VpnIPs))

	for _, ip := range d.VpnIPs {
		cidr, err := ToCIDR(ip)
		if err != nil {
			return nil, err
		}

		vpnCidrs = append(vpnCidrs, cidr)
	}

	lanCidrs := make([]string, 0, len(d.LanIPs))

	for _, ip := range d.LanIPs {
		cidr, err := ToCIDR(ip)
		if err != nil {
			return nil, err
		}

		lanCidrs = append(lanCidrs, cidr)
	}

	chnClient := &ChannelClient{
		key:             d.Key,
		vpnIPs:          vpnCidrs,
		lanIPs:          lanCidrs,
		logger:          d.Log,
		client:          d.ClientDataChannel,
		incomingMsgChan: make(chan *IncomingMsg, 10),
		lastTouchTime:   time.Now(),
	}

	chnClient.ctx, chnClient.ctxCancel = context.WithCancel(ctx)

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
		case d := <-cli.client.ReadCh():
			m, d, e := proto.Decode(d)
			if e != nil {
				log.Errorf("decode data failed: %v", e)

				continue
			}

			log.Debugf("receive udp package [len:%v] %v", len(d), m)

			switch m {
			case proto.MethodPing:
				log.Debug("receive ping message")
				cli.client.WriteCh() <- proto.BuildPongMethodData(d)
			case proto.MethodPong:
				log.Debug("receive pong message")

				cli.lastTouchTime = time.Now()
			case proto.MethodKeyRequest:
				log.Debug("receive key request message")
				cli.client.WriteCh() <- proto.BuildKeyResponseData(cli.key, cli.vpnIPs, cli.lanIPs)
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
			cli.client.WriteCh() <- proto.BuildPingMethodData(nil)

			if time.Since(cli.lastTouchTime) > time.Minute {
				log.Errorf("server miss: %v", time.Since(cli.lastTouchTime))
			}
		}
	}
}

func (cli *ChannelClient) WritePackage(d []byte) {
	cli.client.WriteCh() <- proto.BuildData(d)
}

func (cli *ChannelClient) ReadIncomingMsgChan() <-chan *IncomingMsg {
	return cli.incomingMsgChan
}

func (cli *ChannelClient) Wait() {
	cli.client.Wait()
	cli.wg.Wait()
}

func (cli *ChannelClient) StopAndWait() {
	cli.ctxCancel()
	cli.client.CloseAndWait()
	cli.wg.Wait()
}
