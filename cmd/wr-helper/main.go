// Command wr-helper is the local native SSH/RDP tunnel client described in
// docs/RELAY.md: it binds a loopback-only local port, redeems a single-use
// tunnel ticket against wr-core's relay, and pipes bytes between a native
// ssh/mstsc client and the agent's local SSH/RDP port. It never binds
// 0.0.0.0 and never forwards to an arbitrary host/port — the target is
// fixed server-side by the ticket, not by anything wr-helper is told.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/coder/websocket"
)

func main() {
	serverURL := flag.String("server", "", "wr-core server base URL, e.g. https://remote.example.de")
	ticket := flag.String("ticket", "", "one-time tunnel ticket from the dashboard")
	port := flag.Int("port", 0, "local loopback port to bind (0 = pick an ephemeral port)")
	flag.Parse()

	if *serverURL == "" || *ticket == "" {
		fmt.Fprintln(os.Stderr, "usage: wr-helper --server https://remote.example.de --ticket wr_tunnel_XXXX [--port 41022]")
		os.Exit(1)
	}

	if err := run(*serverURL, *ticket, *port); err != nil {
		fmt.Fprintln(os.Stderr, "wr-helper:", err)
		os.Exit(1)
	}
}

func run(serverURL, ticket string, port int) error {
	// Loopback-only bind, per docs/SECURITY.md §12 and docs/RELAY.md §3:
	// "Helper bindet nur Loopback. Kein 0.0.0.0."
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("bind local port: %w", err)
	}
	defer ln.Close()

	localAddr := ln.Addr().(*net.TCPAddr)
	fmt.Printf("wr-helper listening on 127.0.0.1:%d\n", localAddr.Port)
	fmt.Printf("Connect your SSH/RDP client to 127.0.0.1:%d now. This ticket is single-use;\n", localAddr.Port)
	fmt.Println("wr-helper exits after that one connection ends.")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	type acceptResult struct {
		conn net.Conn
		err  error
	}
	acceptCh := make(chan acceptResult, 1)
	go func() {
		conn, err := ln.Accept()
		acceptCh <- acceptResult{conn, err}
	}()

	select {
	case <-ctx.Done():
		return nil
	case res := <-acceptCh:
		if res.err != nil {
			return fmt.Errorf("accept local connection: %w", res.err)
		}
		defer res.conn.Close()
		return pipeTunnel(ctx, serverURL, ticket, res.conn)
	}
}

func pipeTunnel(ctx context.Context, serverURL, ticket string, local net.Conn) error {
	wsURL := strings.Replace(strings.TrimRight(serverURL, "/"), "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL += "/api/v1/tunnels/stream?ticket=" + ticket

	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("connect to relay: %w", err)
	}
	defer conn.CloseNow()

	fmt.Println("tunnel established")

	errCh := make(chan error, 2)

	// local -> relay
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := local.Read(buf)
			if n > 0 {
				if werr := conn.Write(ctx, websocket.MessageBinary, buf[:n]); werr != nil {
					errCh <- werr
					return
				}
			}
			if err != nil {
				errCh <- err
				return
			}
		}
	}()

	// relay -> local
	go func() {
		for {
			msgType, data, err := conn.Read(ctx)
			if err != nil {
				errCh <- err
				return
			}
			if msgType != websocket.MessageBinary {
				continue
			}
			if _, err := local.Write(data); err != nil {
				errCh <- err
				return
			}
		}
	}()

	select {
	case <-ctx.Done():
	case <-errCh:
	}
	_ = conn.Close(websocket.StatusNormalClosure, "done")
	fmt.Println("tunnel closed")
	return nil
}
