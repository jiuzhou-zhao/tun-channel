package pkg

import (
	"context"
	"net"
	"sync"
)

type UDPServer struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	wg        sync.WaitGroup
	logger    Logger
	crypt     EnDecrypt
	conn      *net.UDPConn
	frameSize uint16
	ChRead    chan *UDPPackage
	ChWrite   chan *UDPPackage
}

type UDPPackage struct {
	Package []byte
	Addr    *net.UDPAddr
}

func NewUDPServer(ctx context.Context, addr string, frameSize uint16, logger Logger,
	crypt EnDecrypt) (*UDPServer, error) {
	if frameSize == 0 {
		frameSize = 65507
	}

	if logger == nil {
		logger = &DummyLogger{}
	}

	if crypt == nil {
		crypt = &NonEnDecrypt{}
	}

	srv := &UDPServer{
		logger:    logger,
		crypt:     crypt,
		frameSize: frameSize,
		ChRead:    make(chan *UDPPackage, 10),
		ChWrite:   make(chan *UDPPackage, 10),
	}
	srv.ctx, srv.ctxCancel = context.WithCancel(ctx)

	lAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	srv.conn, err = net.ListenUDP("udp", lAddr)
	if err != nil {
		return nil, err
	}

	srv.wg.Add(1)
	go srv.reader()
	srv.wg.Add(1)
	go srv.writer()

	return srv, nil
}

func (svr *UDPServer) reader() {
	svr.logger.Info("udp server reader start")
	defer func() {
		svr.wg.Done()
		svr.ctxCancel()
		svr.logger.Info("udp server reader finish")
	}()

	var (
		e error
		n int
	)
	for {
		pack := &UDPPackage{}
		pack.Package = make([]byte, svr.frameSize)
		n, pack.Addr, e = svr.conn.ReadFromUDP(pack.Package)
		if e != nil {
			svr.logger.Errorf("read from udp failed: %v", e)
			break
		}
		d, e := svr.crypt.Decrypt(pack.Package[:n])
		if e != nil {
			svr.logger.Warnf("decrypt data failed: %v, %v", e, n)
			continue
		}
		pack.Package = d
		svr.ChRead <- pack
	}
}

func (svr *UDPServer) writer() {
	svr.logger.Info("udp server writer start")
	defer func() {
		svr.wg.Done()
		_ = svr.conn.Close()
		svr.logger.Info("udp server writer finish")
	}()

	var (
		v *UDPPackage
		e error
	)

	var quit bool
	for !quit {
		select {
		case <-svr.ctx.Done():
			quit = true
		case v = <-svr.ChWrite:
			_, e = svr.conn.WriteToUDP(svr.crypt.Encrypt(v.Package), v.Addr)
			if e != nil {
				svr.logger.Errorf("udp server write failed: %v", e)
				break
			}
		}
	}
}

func (svr *UDPServer) StopAndWait() {
	if svr.conn != nil {
		svr.ctxCancel()
		_ = svr.conn.Close()
	}
	svr.wg.Wait()
}

func (svr *UDPServer) Wait() {
	svr.wg.Wait()
}
