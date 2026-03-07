package export

import (
	"log"
	"net"
	"sync"
)

// TCPExport accepts TCP connections and fans out NMEA sentences to all clients.
type TCPExport struct {
	ln      net.Listener
	mu      sync.Mutex
	clients map[net.Conn]struct{}
}

func NewTCP(addr string) (*TCPExport, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	t := &TCPExport{
		ln:      ln,
		clients: make(map[net.Conn]struct{}),
	}
	go t.accept()
	log.Printf("tcp: listening on %s", addr)
	return t, nil
}

func (t *TCPExport) accept() {
	for {
		conn, err := t.ln.Accept()
		if err != nil {
			return
		}
		log.Printf("tcp: client connected %s", conn.RemoteAddr())
		t.mu.Lock()
		t.clients[conn] = struct{}{}
		t.mu.Unlock()
		go t.drain(conn)
	}
}

// drain reads and discards anything the client sends; detects disconnect.
func (t *TCPExport) drain(conn net.Conn) {
	buf := make([]byte, 256)
	for {
		if _, err := conn.Read(buf); err != nil {
			break
		}
	}
	t.mu.Lock()
	delete(t.clients, conn)
	t.mu.Unlock()
	conn.Close()
	log.Printf("tcp: client disconnected %s", conn.RemoteAddr())
}

// Broadcast sends data to all connected clients, dropping slow/dead ones.
func (t *TCPExport) Broadcast(data []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for conn := range t.clients {
		if _, err := conn.Write(data); err != nil {
			conn.Close()
			delete(t.clients, conn)
		}
	}
}

func (t *TCPExport) Close() error {
	t.mu.Lock()
	for conn := range t.clients {
		conn.Close()
	}
	t.mu.Unlock()
	return t.ln.Close()
}
