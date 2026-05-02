package platform

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
)

type Handler func(command string) error

type IPCServer struct {
	listener net.Listener
	handler  Handler
	once     sync.Once
}

func Listen(socketPath string, handler Handler) (*IPCServer, error) {
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}

	server := &IPCServer{
		listener: listener,
		handler:  handler,
	}
	go server.serve()
	return server, nil
}

func (s *IPCServer) Close() error {
	var err error
	s.once.Do(func() {
		err = s.listener.Close()
	})
	return err
}

func (s *IPCServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		go s.handleConn(conn)
	}
}

func (s *IPCServer) handleConn(conn net.Conn) {
	defer conn.Close()

	command, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		_, _ = fmt.Fprintln(conn, "ERR read command")
		return
	}

	command = strings.TrimSpace(command)
	if command == "" {
		_, _ = fmt.Fprintln(conn, "ERR empty command")
		return
	}

	if err := s.handler(command); err != nil {
		_, _ = fmt.Fprintf(conn, "ERR %s\n", err.Error())
		return
	}

	_, _ = fmt.Fprintln(conn, "OK")
}

func Send(ctx context.Context, socketPath, command string) error {
	if strings.TrimSpace(command) == "" {
		return errors.New("command cannot be empty")
	}

	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := fmt.Fprintf(conn, "%s\n", command); err != nil {
		return err
	}

	response, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return err
	}

	response = strings.TrimSpace(response)
	if strings.HasPrefix(response, "ERR ") {
		return errors.New(strings.TrimPrefix(response, "ERR "))
	}
	if response != "OK" {
		return fmt.Errorf("unexpected IPC response: %s", response)
	}

	return nil
}
