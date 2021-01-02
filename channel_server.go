package udp_channel

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/jiuzhou-zhao/udp-channel/internal/proto"
	"github.com/jiuzhou-zhao/udp-channel/pkg"
)

type KeyParser interface {
	ParseData(d []byte) (key string, dd []byte, err error)
}

type ChannelServer struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	wg sync.WaitGroup

	logger        pkg.Logger
	keyParser     KeyParser
	vpnVip        string
	vpnVipAddress *net.UDPAddr

	udpSrv *pkg.UDPServer

	livePool      *pkg.LivePool
	pendingKeyMap map[string]interface{} // ip+port ->
	addressKeyMap map[string]string      // ip+port -> key
	keyAddressMap map[string]net.UDPAddr // key -> ip+port
	pingChannel   chan net.UDPAddr
	writeChannel  chan *pkg.UDPPackage
	removeChannel chan net.UDPAddr
}

func NewChannelServer(ctx context.Context, addr string, logger pkg.Logger, keyParser KeyParser,
	crypt pkg.EnDecrypt, vpnVip string) (*ChannelServer, error) {
	if logger == nil {
		logger = &pkg.ConsoleLogger{}
	}
	chnServer := &ChannelServer{
		logger:        logger,
		keyParser:     keyParser,
		vpnVip:        vpnVip,
		livePool:      pkg.NewLivePool(context.Background(), 20*time.Second, 60*time.Second),
		pendingKeyMap: make(map[string]interface{}),
		addressKeyMap: make(map[string]string),
		keyAddressMap: make(map[string]net.UDPAddr),
		pingChannel:   make(chan net.UDPAddr, 10),
		writeChannel:  make(chan *pkg.UDPPackage, 10),
		removeChannel: make(chan net.UDPAddr, 10),
	}
	chnServer.ctx, chnServer.ctxCancel = context.WithCancel(ctx)

	udpSrv, err := pkg.NewUDPServer(chnServer.ctx, addr, 0, logger, crypt)
	if err != nil {
		logger.Errorf("new udp server failed: %v", err)
		return nil, err
	}
	chnServer.udpSrv = udpSrv

	chnServer.wg.Add(1)
	go chnServer.reader()
	chnServer.wg.Add(1)
	go chnServer.writer()

	return chnServer, nil
}

func (srv *ChannelServer) reader() {
	srv.logger.Infof("enter channel server reader")
	defer func() {
		srv.wg.Done()
		srv.logger.Infof("leave channel server reader")
	}()

	var quit bool
	for !quit {
		select {
		case <-srv.ctx.Done():
			quit = true
		case udpPackage := <-srv.udpSrv.ChRead:
			m, d, e := proto.Decode(udpPackage.Package)
			if e != nil {
				srv.logger.Errorf("decode data failed: %v", e)
				continue
			}
			srv.logger.Debugf("receive udp package [len:%v] from %v", len(udpPackage.Package), udpPackage.Addr.String())

			_, isPending := srv.pendingKeyMap[udpPackage.Addr.String()]
			_, isChannel := srv.addressKeyMap[udpPackage.Addr.String()]
			if !isPending && !isChannel {
				srv.livePool.Add(srv.UDPConnectionLive(*udpPackage.Addr))
				srv.pendingKeyMap[udpPackage.Addr.String()] = true
				srv.logger.Debugf("%v put live pool", udpPackage.Addr.String())
			}
			switch m {
			case proto.MethodPing:
				srv.logger.Debugf("receive ping message from %v", udpPackage.Addr.String())
				srv.writeChannel <- &pkg.UDPPackage{
					Package: proto.BuildPongMethodData(d),
					Addr:    udpPackage.Addr,
				}
			case proto.MethodPong:
				srv.logger.Debugf("receive pong message from %v", udpPackage.Addr.String())
				srv.livePool.OnPongResponse(srv.UDPConnectionLive(*udpPackage.Addr))
			case proto.MethodKeyRequest:
				srv.logger.Debugf("receive key request message from %v", udpPackage.Addr.String())
			case proto.MethodKeyResponse:
				key := proto.ParseKeyResponsePayloadData(d)
				srv.logger.Debugf("receive key response message [%v] from %v", key, udpPackage.Addr.String())
				oldAddr, ok := srv.keyAddressMap[key]
				if ok {
					if oldAddr.String() == udpPackage.Addr.String() {
						continue
					}
					delete(srv.keyAddressMap, key)
					delete(srv.addressKeyMap, oldAddr.String())
					if key == srv.vpnVip {
						srv.vpnVipAddress = nil
					}
				}
				delete(srv.pendingKeyMap, udpPackage.Addr.String())
				srv.keyAddressMap[key] = *udpPackage.Addr
				srv.addressKeyMap[udpPackage.Addr.String()] = key
				if key == srv.vpnVip {
					srv.vpnVipAddress = udpPackage.Addr
				}
			case proto.MethodData:
				key, d, err := srv.keyParser.ParseData(d)
				if err != nil {
					srv.logger.Errorf("key parser parse key failed: %v", err)
					continue
				}
				if addr, ok := srv.keyAddressMap[key]; ok {
					srv.writeChannel <- &pkg.UDPPackage{
						Package: proto.BuildData(d),
						Addr:    &addr,
					}
				} else if srv.vpnVipAddress != nil {
					srv.writeChannel <- &pkg.UDPPackage{
						Package: proto.BuildData(d),
						Addr:    srv.vpnVipAddress,
					}
				} else {
					srv.logger.Errorf("no key %v for data", key)
				}
			}
		case addr := <-srv.pingChannel:
			if _, ok := srv.pendingKeyMap[addr.String()]; ok {
				srv.logger.Debugf("ping: try request key %v", addr.String())
				srv.writeChannel <- &pkg.UDPPackage{
					Package: proto.BuildKeyRequestData(),
					Addr:    &addr,
				}
			} else {
				srv.logger.Debugf("ping: try ping %v", addr.String())
				srv.writeChannel <- &pkg.UDPPackage{
					Package: proto.BuildPingMethodData(nil),
					Addr:    &addr,
				}
			}
		case addr := <-srv.removeChannel:
			srv.logger.Debugf("remove on %v", addr.String())
			delete(srv.pendingKeyMap, addr.String())
			key, ok := srv.addressKeyMap[addr.String()]
			if ok {
				delete(srv.addressKeyMap, addr.String())
				delete(srv.keyAddressMap, key)
			}
		}
	}
}

func (srv *ChannelServer) writer() {
	srv.logger.Infof("enter channel server writer")
	defer func() {
		srv.wg.Done()
		srv.logger.Infof("leave channel server writer")
	}()

	var quit bool
	for !quit {
		select {
		case <-srv.ctx.Done():
			quit = true
		case d := <-srv.writeChannel:
			srv.udpSrv.ChWrite <- d
		}
	}
}

func (srv *ChannelServer) UDPConnectionLive(addr net.UDPAddr) *UDPConnectionLive {
	liveItem := &UDPConnectionLive{
		addr: addr,
	}
	liveItem.fnDoPingRequest = func() {
		srv.pingChannel <- liveItem.addr
	}
	liveItem.fnOnRemoved = func() {
		srv.removeChannel <- liveItem.addr
	}
	return liveItem
}

func (srv *ChannelServer) StopAndWait() {
	srv.ctxCancel()
	srv.udpSrv.StopAndWait()
	srv.wg.Wait()
}

func (srv *ChannelServer) Wait() {
	srv.udpSrv.Wait()
	srv.wg.Wait()
}

type UDPConnectionLive struct {
	addr            net.UDPAddr
	fnDoPingRequest func()
	fnOnRemoved     func()
}

func (live *UDPConnectionLive) Key() string {
	return live.addr.String()
}
func (live *UDPConnectionLive) DoPingRequest() {
	live.fnDoPingRequest()
}

func (live *UDPConnectionLive) OnRemoved() {
	live.fnOnRemoved()
}
