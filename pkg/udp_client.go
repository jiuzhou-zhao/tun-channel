package pkg

import (
	"context"
	"net"
	"sync"

	"github.com/sgostarter/i/logger"
)

type UDPClient struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	wg        sync.WaitGroup
	logger    logger.Wrapper
	crypt     EnDecrypt
	conn      net.Conn
	frameSize uint16
	ChRead    chan []byte
	ChWrite   chan []byte
}

func NewUDPClient(ctx context.Context, addr string, frameSize uint16, log logger.Wrapper,
	crypt EnDecrypt) (t *UDPClient, err error) {
	if frameSize == 0 {
		frameSize = 65507
	}

	if log == nil {
		log = logger.NewWrapper(&logger.NopLogger{}).WithFields(logger.FieldString("role", "udpClient"))
	}

	if crypt == nil {
		crypt = &NonEnDecrypt{}
	}

	cli := &UDPClient{
		logger:    log,
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
	log := cli.logger.WithFields(logger.FieldString("module", "reader"))

	log.Info("udp client reader start")
	defer func() {
		cli.wg.Done()
		cli.ctxCancel()
		log.Info("udp client reader finish")
		close(cli.ChRead)
		cli.ChRead = nil
	}()

	loop := true
	for loop {
		select {
		case <-cli.ctx.Done():
			loop = false
			break
		default:
			break
		}
		if !loop {
			break
		}

		buf := make([]byte, cli.frameSize)
		n, e := cli.conn.Read(buf)
		if e != nil {
			log.Errorf("read failed: %v", e)
			continue
		}
		d, e := cli.crypt.Decrypt(buf[:n])
		if e != nil {
			log.Warnf("decrypt data failed: %v, %v", d, n)
			continue
		}
		cli.ChRead <- d
	}
}

func (cli *UDPClient) writer() {
	log := cli.logger.WithFields(logger.FieldString("module", "writer"))

	log.Info("udp client writer start")
	defer func() {
		cli.wg.Done()
		_ = cli.conn.Close()
		log.Info("udp client writer finish")
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
				log.Errorf("write failed: %v, %v-%v", e, n, len(v))
				continue
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
