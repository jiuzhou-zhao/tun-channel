package pkg

import (
	"context"
	"github.com/sgostarter/i/logger"
	"net"
	"sync"
)

type UDPServer struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	wg        sync.WaitGroup
	logger    logger.Wrapper
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

func NewUDPServer(ctx context.Context, addr string, frameSize uint16, log logger.Wrapper,
	crypt EnDecrypt) (*UDPServer, error) {
	if frameSize == 0 {
		frameSize = 65507
	}

	if log == nil {
		log = logger.NewWrapper(&logger.NopLogger{}).WithFields(logger.FieldString("role", "udpServer"))
	}

	if crypt == nil {
		crypt = &NonEnDecrypt{}
	}

	srv := &UDPServer{
		logger:    log,
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
	log := svr.logger.WithFields(logger.FieldString("module", "reader"))

	log.Info("udp server reader start")

	defer func() {
		svr.wg.Done()
		svr.ctxCancel()
		log.Info("udp server reader finish")
		close(svr.ChRead)
		svr.ChRead = nil
	}()

	var (
		e    error
		n    int
		loop = true
	)
	for loop {
		select {
		case <-svr.ctx.Done():
			loop = false
			break
		default:
			break
		}
		if !loop {
			break
		}
		pack := &UDPPackage{}
		pack.Package = make([]byte, svr.frameSize)
		n, pack.Addr, e = svr.conn.ReadFromUDP(pack.Package)
		if e != nil {
			log.Errorf("read from udp failed: %v", e)
			continue
		}
		d, e := svr.crypt.Decrypt(pack.Package[:n])
		if e != nil {
			log.Warnf("decrypt data failed: %v, %v", e, n)
			continue
		}
		pack.Package = d
		svr.ChRead <- pack
	}
}

func (svr *UDPServer) writer() {
	log := svr.logger.WithFields(logger.FieldString("module", "writer"))

	log.Info("udp server writer start")

	defer func() {
		svr.wg.Done()
		_ = svr.conn.Close()
		log.Info("udp server writer finish")
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
				log.Errorf("udp server write failed: %v", e)
				continue
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
