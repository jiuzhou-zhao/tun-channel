package pkg

import (
	"context"
	"net"
	"sync"
)

type UDPClient struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	wg        sync.WaitGroup
	logger    Logger
	crypt     EnDecrypt
	conn      net.Conn
	frameSize uint16
	ChRead    chan []byte
	ChWrite   chan []byte
}

func NewUDPClient(ctx context.Context, addr string, frameSize uint16, logger Logger,
	crypt EnDecrypt) (t *UDPClient, err error) {
	if frameSize == 0 {
		frameSize = 65507
	}

	if logger == nil {
		logger = &DummyLogger{}
	}

	if crypt == nil {
		crypt = &NonEnDecrypt{}
	}

	cli := &UDPClient{
		logger:    logger,
		crypt:     crypt,
		frameSize: frameSize,
		ChRead:    make(chan []byte, 10),
		ChWrite:   make(chan []byte, 10),
	}
	cli.ctx, cli.ctxCancel = context.WithCancel(ctx)

	cli.conn, err = net.Dial("udp", addr)
	if err != nil {
		return
	}

	cli.wg.Add(1)
	go cli.reader()
	cli.wg.Add(1)
	go cli.writer()

	return cli, nil
}

func (cli *UDPClient) reader() {
	cli.logger.Info("udp client reader start")
	defer func() {
		cli.wg.Done()
		cli.ctxCancel()
		cli.logger.Info("udp client reader finish")
	}()

	for {
		buf := make([]byte, cli.frameSize)
		n, e := cli.conn.Read(buf)
		if e != nil {
			cli.logger.Errorf("read failed: %v", e)
			break
		}
		d, e := cli.crypt.Decrypt(buf[:n])
		if e != nil {
			cli.logger.Warnf("decrypt data failed: %v, %v", d, n)
			continue
		}
		cli.ChRead <- d
	}
}

func (cli *UDPClient) writer() {
	cli.logger.Info("udp client writer start")
	defer func() {
		cli.wg.Done()
		_ = cli.conn.Close()
		cli.logger.Info("udp client writer finish")
	}()

	var (
		v []byte
	)

	var quit bool
	for !quit {
		select {
		case <-cli.ctx.Done():
			quit = true
		case v = <-cli.ChWrite:
			v = cli.crypt.Encrypt(v)
			n, e := cli.conn.Write(v)
			if e != nil || n != len(v) {
				cli.logger.Errorf("write failed: %v, %v-%v", e, n, len(v))
				break
			}
		}
	}
}

func (cli *UDPClient) StopAndWait() {
	if cli.conn != nil {
		cli.ctxCancel()
		_ = cli.conn.Close()
	}
	cli.wg.Wait()
}

func (cli *UDPClient) Wait() {
	cli.wg.Wait()
}
