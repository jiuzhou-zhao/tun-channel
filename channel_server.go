package udpchannel

import (
	"context"
	"sync"
	"time"

	"github.com/jiuzhou-zhao/data-channel/inter"
	"github.com/jiuzhou-zhao/udp-channel/internal/proto"
	"github.com/jiuzhou-zhao/udp-channel/pkg"
	"github.com/sgostarter/i/logger"
)

type KeyParser interface {
	ParseData(d []byte) (key string, dd []byte, err error)
	ParseKeyFromIPOrCIDR(s string) (key string, err error)
	CompareKeyWithCIDR(key string, cidr string) bool
}

type ChannelClientDataInfo struct {
	Addr   string
	Key    string
	VpnIPs []string
	LanIPs []string

	CreateTime     time.Time
	LastAccessTime time.Time
	TransBytes     uint64
}

type ClientInfos struct {
	VIP     string
	Address string
	VpnIPs  []string
	LanIPs  []string

	CreateTime     time.Time
	LastAccessTime time.Time
	TransBytes     uint64
}

type GetClientsInfosRequest struct {
	ci []*ClientInfos
	wg *sync.WaitGroup
}

type ChannelServer struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	wg sync.WaitGroup

	logger        logger.Wrapper
	keyParser     KeyParser
	vpnCIDR       string
	vpnVipAddress string

	server inter.Server

	livePool               *pkg.LivePool
	pendingKeyMap          map[string]interface{}            // data_channel_client_addr ->
	clientMap              map[string]*ChannelClientDataInfo // data_channel_client_addr -> client
	keyAddressMap          map[string]string                 // key -> data_channel_client_addr
	pingChannel            chan string
	writeChannel           chan *inter.ServerData
	removeChannel          chan string
	getClientsInfosChannel chan *GetClientsInfosRequest
}

func NewChannelServer(ctx context.Context, log logger.Wrapper, keyParser KeyParser,
	server inter.Server, vpnIP string) (*ChannelServer, error) {
	if log == nil {
		log = logger.NewWrapper(logger.NewCommLogger(&logger.FmtRecorder{})).WithFields(logger.FieldString("role", "channelClient"))
	}

	if vpnIP != "" {
		vpnCIDR, err := ToCIDR(vpnIP)
		if err != nil {
			return nil, err
		}

		vpnIP = vpnCIDR
	}

	chnServer := &ChannelServer{
		logger:                 log,
		keyParser:              keyParser,
		vpnCIDR:                vpnIP,
		server:                 server,
		livePool:               pkg.NewLivePool(context.Background(), 20*time.Second, 60*time.Second),
		pendingKeyMap:          make(map[string]interface{}),
		clientMap:              make(map[string]*ChannelClientDataInfo),
		keyAddressMap:          make(map[string]string),
		pingChannel:            make(chan string, 10),
		writeChannel:           make(chan *inter.ServerData, 10),
		removeChannel:          make(chan string, 10),
		getClientsInfosChannel: make(chan *GetClientsInfosRequest, 10),
	}
	chnServer.ctx, chnServer.ctxCancel = context.WithCancel(ctx)

	server.SetOb(chnServer)

	chnServer.wg.Add(1)

	go chnServer.reader()

	chnServer.wg.Add(1)

	go chnServer.writer()

	return chnServer, nil
}

func (srv *ChannelServer) OnConnect(addr string) {
	srv.logger.Info("OnConnect %s", addr)

	srv.writeChannel <- &inter.ServerData{
		Data: proto.BuildKeyRequestData(),
		Addr: addr,
	}
}

func (srv *ChannelServer) OnClose(addr string) {
	srv.logger.Info("OnClose %s", addr)
}

func (srv *ChannelServer) OnException(addr string, err error) {
	srv.logger.Errorf("OnException %s: %s", addr, err)
}

func (srv *ChannelServer) GetClientInfos() []*ClientInfos {
	req := &GetClientsInfosRequest{
		wg: &sync.WaitGroup{},
	}
	req.wg.Add(1)
	srv.getClientsInfosChannel <- req

	req.wg.Wait()

	return req.ci
}

func (srv *ChannelServer) cleanupClient(key string) {
	if srv.vpnCIDR != "" && srv.keyParser.CompareKeyWithCIDR(key, srv.vpnCIDR) {
		srv.vpnVipAddress = ""
	}

	addr, ok := srv.keyAddressMap[key]
	if !ok {
		return
	}

	delete(srv.keyAddressMap, key)

	cli, ok := srv.clientMap[addr]
	if !ok {
		return
	}

	for _, cidr := range cli.LanIPs {
		k, err := srv.keyParser.ParseKeyFromIPOrCIDR(cidr)
		if err != nil {
			srv.logger.Fatal(err)
		}

		delete(srv.keyAddressMap, k)
	}

	delete(srv.clientMap, addr)

	if len(cli.LanIPs) > 0 {
		for _, info := range srv.clientMap {
			srv.logger.Debugf("setupClient unset lanIPs to %s [%s]", info.Addr, info.Key)
			srv.writeChannel <- &inter.ServerData{
				Data: proto.BuildForwardControlData(cli.LanIPs, nil),
				Addr: info.Addr,
			}
		}
	}
}

func (srv *ChannelServer) setupClient(key string, addr string, vpnIPs, lanIPs []string) {
	srv.clientMap[addr] = &ChannelClientDataInfo{
		Addr:           addr,
		Key:            key,
		VpnIPs:         vpnIPs,
		LanIPs:         lanIPs,
		CreateTime:     time.Now(),
		LastAccessTime: time.Now(),
	}

	srv.keyAddressMap[key] = addr

	for _, cidr := range lanIPs {
		k, err := srv.keyParser.ParseKeyFromIPOrCIDR(cidr)
		if err != nil {
			srv.logger.Fatal(err)
		}

		srv.keyAddressMap[k] = addr
	}

	if srv.vpnCIDR != "" {
		if srv.keyParser.CompareKeyWithCIDR(key, srv.vpnCIDR) {
			srv.vpnVipAddress = addr
		}
	}

	if len(lanIPs) == 0 {
		return
	}

	for otherAddr, info := range srv.clientMap {
		if otherAddr == addr {
			continue
		}

		srv.logger.Debugf("setupClient set lanIPs to %s [%s]", info.Addr, info.Key)
		srv.writeChannel <- &inter.ServerData{
			Data: proto.BuildForwardControlData(nil, lanIPs),
			Addr: info.Addr,
		}
	}
}

func (srv *ChannelServer) processPingMessage(log logger.Wrapper, addr string, d []byte) {
	log.Debugf("receive ping message from %v", addr)
	srv.writeChannel <- &inter.ServerData{
		Data: proto.BuildPongMethodData(d),
		Addr: addr,
	}
}

func (srv *ChannelServer) processPongMessage(log logger.Wrapper, addr string, _ []byte) {
	log.Debugf("receive pong message from %v", addr)
	srv.livePool.OnPongResponse(srv.UDPConnectionLive(addr))
}

func (srv *ChannelServer) processKeyResponseMessage(log logger.Wrapper, addr string, d []byte) {
	key, vpnIPs, lanIPs, err := proto.ParseKeyResponsePayloadData(d)
	if err != nil {
		log.WithFields(logger.FieldError("error", err)).Error("parse payload failed")

		return
	}

	log.Debugf("receive key response message [%v] from %v", key, addr)
	log.Info("--- vpnIPs", vpnIPs)
	log.Info("--- lanIPs", lanIPs)

	srv.cleanupClient(key)

	delete(srv.pendingKeyMap, addr)

	srv.setupClient(key, addr, vpnIPs, lanIPs)
}

func (srv *ChannelServer) processDataMessage(log logger.Wrapper, d []byte) {
	key, data, err := srv.keyParser.ParseData(d)
	if err != nil {
		log.Errorf("key parser parse key failed: %v", err)

		return
	}

	if addr, ok := srv.keyAddressMap[key]; ok {
		srv.writeChannel <- &inter.ServerData{
			Data: proto.BuildData(data),
			Addr: addr,
		}
	} else if srv.vpnVipAddress != "" {
		srv.writeChannel <- &inter.ServerData{
			Data: proto.BuildData(data),
			Addr: srv.vpnVipAddress,
		}
	} else {
		log.Errorf("no key %v for data", key)
	}
}

// nolint: cyclop
func (srv *ChannelServer) processServerInput(log logger.Wrapper, data *inter.ServerData) {
	m, d, e := proto.Decode(data.Data)
	if e != nil {
		log.Errorf("decode data failed: %v", e)

		return
	}

	log.Debugf("receive udp package [len:%v] from %v", len(data.Data), data.Addr)

	_, isPending := srv.pendingKeyMap[data.Addr]
	_, isChannel := srv.clientMap[data.Addr]

	if !isPending && !isChannel {
		srv.livePool.Add(srv.UDPConnectionLive(data.Addr))
		srv.pendingKeyMap[data.Addr] = true

		log.Debugf("%v put live pool", data.Addr)
	}

	switch m {
	case proto.MethodPing:
		srv.processPingMessage(log, data.Addr, d)
	case proto.MethodPong:
		srv.processPongMessage(log, data.Addr, d)

		if ci, ok := srv.clientMap[data.Addr]; ok {
			ci.LastAccessTime = time.Now()
		}
	case proto.MethodKeyRequest:
		log.Debugf("receive key request message from %v", data.Addr)
	case proto.MethodKeyResponse:
		srv.processKeyResponseMessage(log, data.Addr, d)
	case proto.MethodData:
		srv.processDataMessage(log, d)

		if ci, ok := srv.clientMap[data.Addr]; ok {
			ci.TransBytes += uint64(len(d))
		}
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

			continue
		case d := <-srv.server.ReadCh():
			srv.processServerInput(log, d)
		case addr := <-srv.pingChannel:
			if _, ok := srv.pendingKeyMap[addr]; ok {
				log.Debugf("ping: try request key %v", addr)
				srv.writeChannel <- &inter.ServerData{
					Data: proto.BuildKeyRequestData(),
					Addr: addr,
				}
			} else {
				log.Debugf("ping: try ping %v", addr)
				srv.writeChannel <- &inter.ServerData{
					Data: proto.BuildPingMethodData(nil),
					Addr: addr,
				}
			}
		case addr := <-srv.removeChannel:
			log.Debugf("remove on %v", addr)

			if cli, ok := srv.clientMap[addr]; ok {
				srv.cleanupClient(cli.Key)
			}
		case gci := <-srv.getClientsInfosChannel:
			for _, info := range srv.clientMap {
				gci.ci = append(gci.ci, &ClientInfos{
					VIP:            info.Key,
					Address:        info.Addr,
					VpnIPs:         info.VpnIPs,
					LanIPs:         info.LanIPs,
					CreateTime:     info.CreateTime,
					LastAccessTime: info.LastAccessTime,
					TransBytes:     info.TransBytes,
				})
			}

			gci.wg.Done()
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
			srv.server.WriteCh() <- d
		}
	}
}

func (srv *ChannelServer) UDPConnectionLive(addr string) *UDPConnectionLive {
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

func (srv *ChannelServer) Wait() {
	srv.wg.Wait()
	srv.server.CloseAndWait()
}

func (srv *ChannelServer) StopAndWait() {
	srv.ctxCancel()
	srv.server.CloseAndWait()
	srv.wg.Wait()
}

type UDPConnectionLive struct {
	addr            string
	fnDoPingRequest func()
	fnOnRemoved     func()
}

func (live *UDPConnectionLive) Key() string {
	return live.addr
}
func (live *UDPConnectionLive) DoPingRequest() {
	live.fnDoPingRequest()
}

func (live *UDPConnectionLive) OnRemoved() {
	live.fnOnRemoved()
}
