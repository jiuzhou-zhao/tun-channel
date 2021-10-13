package udpchannel

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/jiuzhou-zhao/udp-channel/internal/proto"
	"github.com/jiuzhou-zhao/udp-channel/pkg"
	"github.com/sgostarter/i/logger"
)

type KeyParser interface {
	ParseData(d []byte) (key string, dd []byte, err error)
}

type ChannelClientDataInfo struct {
	Addr   net.UDPAddr
	Key    string
	VpnIPs []string
	LanIPs []string

	lanIPsMap map[string]interface{}
}

func (di *ChannelClientDataInfo) Build() {
	di.lanIPsMap = make(map[string]interface{})
	for _, ip := range di.LanIPs {
		di.lanIPsMap[ip] = make([]int, 0)
	}
}

type ChannelServer struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	wg sync.WaitGroup

	logger        logger.Wrapper
	keyParser     KeyParser
	vpnVip        string
	vpnVipAddress *net.UDPAddr

	udpSrv *pkg.UDPServer

	livePool      *pkg.LivePool
	pendingKeyMap map[string]interface{}            // ip+port ->
	clientMap     map[string]*ChannelClientDataInfo // ip+port -> client
	keyAddressMap map[string]net.UDPAddr            // key -> ip+port
	pingChannel   chan net.UDPAddr
	writeChannel  chan *pkg.UDPPackage
	removeChannel chan net.UDPAddr
}

func NewChannelServer(ctx context.Context, addr string, log logger.Wrapper, keyParser KeyParser,
	crypt pkg.EnDecrypt, vpnVip string) (*ChannelServer, error) {
	if log == nil {
		log = logger.NewWrapper(&logger.NopLogger{}).WithFields(logger.FieldString("role", "channelClient"))
	}

	chnServer := &ChannelServer{
		logger:        log,
		keyParser:     keyParser,
		vpnVip:        vpnVip,
		livePool:      pkg.NewLivePool(context.Background(), 20*time.Second, 60*time.Second),
		pendingKeyMap: make(map[string]interface{}),
		clientMap:     make(map[string]*ChannelClientDataInfo),
		keyAddressMap: make(map[string]net.UDPAddr),
		pingChannel:   make(chan net.UDPAddr, 10),
		writeChannel:  make(chan *pkg.UDPPackage, 10),
		removeChannel: make(chan net.UDPAddr, 10),
	}
	chnServer.ctx, chnServer.ctxCancel = context.WithCancel(ctx)

	udpSrv, err := pkg.NewUDPServer(chnServer.ctx, addr, 0, log, crypt)
	if err != nil {
		log.Errorf("new udp server failed: %v", err)

		return nil, err
	}

	chnServer.udpSrv = udpSrv

	chnServer.wg.Add(1)

	go chnServer.reader()

	chnServer.wg.Add(1)

	go chnServer.writer()

	return chnServer, nil
}

func (srv *ChannelServer) cleanupClient(key string) {
	if key == srv.vpnVip {
		srv.vpnVipAddress = nil
	}

	addr, ok := srv.keyAddressMap[key]
	// nolint: nestif
	if ok {
		delete(srv.keyAddressMap, key)

		if cli, ok := srv.clientMap[addr.String()]; ok {
			for _, cidr := range cli.LanIPs {
				ip, _, err := net.ParseCIDR(cidr)
				if err != nil {
					srv.logger.Fatal(err)
				}

				delete(srv.keyAddressMap, ip.To4().String())
			}

			delete(srv.clientMap, addr.String())

			for otherAddr, info := range srv.clientMap {
				if otherAddr == addr.String() {
					continue
				}

				if len(cli.LanIPs) == 0 {
					continue
				}

				srv.logger.Debugf("setupClient unset lanIPs to %s [%s]", info.Addr.String(), info.Key)
				addrTo := info.Addr
				srv.writeChannel <- &pkg.UDPPackage{
					Package: proto.BuildForwardControlData(cli.LanIPs, nil),
					Addr:    &addrTo,
				}
			}
		}
	}
}

func (srv *ChannelServer) setupClient(key string, addr net.UDPAddr, vpnIPs, lanIPs []string) {
	cli := &ChannelClientDataInfo{
		Addr:   addr,
		Key:    key,
		VpnIPs: vpnIPs,
		LanIPs: lanIPs,
	}
	cli.Build()
	srv.clientMap[addr.String()] = cli

	srv.keyAddressMap[key] = addr

	for _, cidr := range lanIPs {
		ip, _, err := net.ParseCIDR(cidr)
		if err != nil {
			srv.logger.Fatal(err)
		}

		srv.keyAddressMap[ip.To4().String()] = addr
	}

	if key == srv.vpnVip {
		srv.vpnVipAddress = &addr
	}

	for otherAddr, info := range srv.clientMap {
		if otherAddr == addr.String() {
			continue
		}

		if len(lanIPs) == 0 {
			continue
		}

		srv.logger.Debugf("setupClient set lanIPs to %s [%s]", info.Addr.String(), info.Key)
		addrTo := info.Addr
		srv.writeChannel <- &pkg.UDPPackage{
			Package: proto.BuildForwardControlData(nil, lanIPs),
			Addr:    &addrTo,
		}
	}
}

func (srv *ChannelServer) processPingMessage(log logger.Wrapper, udpPackage *pkg.UDPPackage, d []byte) {
	log.Debugf("receive ping message from %v", udpPackage.Addr.String())
	srv.writeChannel <- &pkg.UDPPackage{
		Package: proto.BuildPongMethodData(d),
		Addr:    udpPackage.Addr,
	}
}

func (srv *ChannelServer) processPongMessage(log logger.Wrapper, udpPackage *pkg.UDPPackage, _ []byte) {
	log.Debugf("receive pong message from %v", udpPackage.Addr.String())
	srv.livePool.OnPongResponse(srv.UDPConnectionLive(*udpPackage.Addr))
}

func (srv *ChannelServer) processKeyResponseMessage(log logger.Wrapper, udpPackage *pkg.UDPPackage, d []byte) {
	key, vpnIPs, lanIPs, err := proto.ParseKeyResponsePayloadData(d)
	if err != nil {
		log.WithFields(logger.FieldError("error", err)).Error("parse payload failed")

		return
	}

	log.Debugf("receive key response message [%v] from %v", key, udpPackage.Addr.String())
	log.Info("--- vpnIPs", vpnIPs)
	log.Info("--- lanIPs", lanIPs)

	srv.cleanupClient(key)

	delete(srv.pendingKeyMap, udpPackage.Addr.String())

	srv.setupClient(key, *udpPackage.Addr, vpnIPs, lanIPs)
}

func (srv *ChannelServer) processDataMessage(log logger.Wrapper, d []byte) {
	key, data, err := srv.keyParser.ParseData(d)
	if err != nil {
		log.Errorf("key parser parse key failed: %v", err)

		return
	}

	if addr, ok := srv.keyAddressMap[key]; ok {
		srv.writeChannel <- &pkg.UDPPackage{
			Package: proto.BuildData(data),
			Addr:    &addr,
		}
	} else if srv.vpnVipAddress != nil {
		srv.writeChannel <- &pkg.UDPPackage{
			Package: proto.BuildData(data),
			Addr:    srv.vpnVipAddress,
		}
	} else {
		log.Errorf("no key %v for data", key)
	}
}

func (srv *ChannelServer) processUDPServerInput(log logger.Wrapper, udpPackage *pkg.UDPPackage) {
	m, d, e := proto.Decode(udpPackage.Package)
	if e != nil {
		log.Errorf("decode data failed: %v", e)

		return
	}

	log.Debugf("receive udp package [len:%v] from %v", len(udpPackage.Package), udpPackage.Addr.String())

	_, isPending := srv.pendingKeyMap[udpPackage.Addr.String()]
	_, isChannel := srv.clientMap[udpPackage.Addr.String()]

	if !isPending && !isChannel {
		srv.livePool.Add(srv.UDPConnectionLive(*udpPackage.Addr))
		srv.pendingKeyMap[udpPackage.Addr.String()] = true

		log.Debugf("%v put live pool", udpPackage.Addr.String())
	}

	switch m {
	case proto.MethodPing:
		srv.processPingMessage(log, udpPackage, d)
	case proto.MethodPong:
		srv.processPongMessage(log, udpPackage, d)
	case proto.MethodKeyRequest:
		log.Debugf("receive key request message from %v", udpPackage.Addr.String())
	case proto.MethodKeyResponse:
		srv.processKeyResponseMessage(log, udpPackage, d)
	case proto.MethodData:
		srv.processDataMessage(log, d)
	}
}

func (srv *ChannelServer) reader() {
	log := srv.logger.WithFields(logger.FieldString("module", "reader"))

	log.Infof("enter channel server reader")

	defer func() {
		srv.wg.Done()
		log.Infof("leave channel server reader")
	}()

	var quit bool
	for !quit {
		select {
		case <-srv.ctx.Done():
			quit = true
		case udpPackage := <-srv.udpSrv.ChRead:
			srv.processUDPServerInput(log, udpPackage)
		case addr := <-srv.pingChannel:
			if _, ok := srv.pendingKeyMap[addr.String()]; ok {
				log.Debugf("ping: try request key %v", addr.String())
				srv.writeChannel <- &pkg.UDPPackage{
					Package: proto.BuildKeyRequestData(),
					Addr:    &addr,
				}
			} else {
				log.Debugf("ping: try ping %v", addr.String())
				srv.writeChannel <- &pkg.UDPPackage{
					Package: proto.BuildPingMethodData(nil),
					Addr:    &addr,
				}
			}
		case addr := <-srv.removeChannel:
			log.Debugf("remove on %v", addr.String())

			if cli, ok := srv.clientMap[addr.String()]; ok {
				srv.cleanupClient(cli.Key)
			}
		}
	}
}

func (srv *ChannelServer) writer() {
	log := srv.logger.WithFields(logger.FieldString("module", "writer"))

	log.Infof("enter channel server writer")

	defer func() {
		srv.wg.Done()
		log.Infof("leave channel server writer")
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
