package daemon

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fabioroma/remarkable-cli/pkg/model"
	"github.com/fabioroma/remarkable-cli/pkg/transport"
)

// socket path for IPC
func SocketPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "remarkable-cli", "daemon.sock")
}

// PidPath returns the path to the daemon PID file
func PidPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "remarkable-cli", "daemon.pid")
}

// Request is a command sent to the daemon
type Request struct {
	Command string   `json:"command"` // "ls", "read", "write", "ping", "stop"
	Args    []string `json:"args,omitempty"`
}

// Response from the daemon
type Response struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// Daemon holds a persistent transport connection + in-memory doc cache
type Daemon struct {
	transport transport.Transport
	listener  net.Listener
	mu        sync.Mutex
	stopCh    chan struct{}

	// in-memory cache — populated on connect, refreshed on mutations
	docs       []model.Document
	docsLoaded time.Time
}

// Start launches the daemon with a pre-connected transport
func Start(t transport.Transport) error {
	sock := SocketPath()

	// clean up stale socket
	os.Remove(sock)
	os.MkdirAll(filepath.Dir(sock), 0700)

	listener, err := net.Listen("unix", sock)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w", sock, err)
	}

	// write PID file
	os.WriteFile(PidPath(), []byte(fmt.Sprintf("%d", os.Getpid())), 0600)

	d := &Daemon{
		transport: t,
		listener:  listener,
		stopCh:    make(chan struct{}),
	}

	// pre-load document list into memory
	docs, err := t.ListDocuments()
	if err == nil {
		d.docs = docs
		d.docsLoaded = time.Now()
	}

	fmt.Printf("daemon started (pid %d, transport: %s, %d docs cached)\n", os.Getpid(), t.Name(), len(d.docs))

	// background refresh every 30s
	go d.refreshLoop()

	// accept connections
	go d.acceptLoop()

	// wait for stop signal
	<-d.stopCh

	listener.Close()
	os.Remove(sock)
	os.Remove(PidPath())
	t.Close()
	return nil
}

func (d *Daemon) refreshLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			docs, err := d.transport.ListDocuments()
			if err != nil {
				continue
			}
			d.mu.Lock()
			d.docs = docs
			d.docsLoaded = time.Now()
			d.mu.Unlock()
		}
	}
}

func (d *Daemon) acceptLoop() {
	for {
		conn, err := d.listener.Accept()
		if err != nil {
			select {
			case <-d.stopCh:
				return
			default:
				continue
			}
		}
		go d.handleConn(conn)
	}
}

func (d *Daemon) handleConn(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	// read request
	var req Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		writeResponse(conn, Response{Error: "invalid request"})
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	switch req.Command {

	case "ping":
		writeResponse(conn, Response{OK: true, Data: raw(`{"status":"alive","transport":"` + d.transport.Name() + `"}`)})

	case "ls":
		// return in-memory cached docs — instant
		data, _ := json.Marshal(d.docs)
		writeResponse(conn, Response{OK: true, Data: data})

	case "refresh":
		// force refresh the cache
		docs, err := d.transport.ListDocuments()
		if err != nil {
			writeResponse(conn, Response{Error: err.Error()})
			return
		}
		d.docs = docs
		d.docsLoaded = time.Now()
		data, _ := json.Marshal(map[string]any{"documents": len(docs), "refreshed": d.docsLoaded})
		writeResponse(conn, Response{OK: true, Data: data})

	case "stop":
		writeResponse(conn, Response{OK: true, Data: raw(`{"status":"stopped"}`)})
		close(d.stopCh)

	default:
		writeResponse(conn, Response{Error: fmt.Sprintf("unknown command: %s", req.Command)})
	}
}

func writeResponse(conn net.Conn, resp Response) {
	json.NewEncoder(conn).Encode(resp)
}

func raw(s string) json.RawMessage {
	return json.RawMessage(s)
}

// IsRunning checks if a daemon is alive
func IsRunning() bool {
	conn, err := net.DialTimeout("unix", SocketPath(), 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// SendCommand sends a request to the running daemon and returns the response
func SendCommand(req Request) (*Response, error) {
	conn, err := net.DialTimeout("unix", SocketPath(), 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("daemon not running (use 'remarkable connect' first)")
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(30 * time.Second))

	// send request
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, err
	}

	// read response
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// Stop tells the daemon to shut down
func Stop() error {
	resp, err := SendCommand(Request{Command: "stop"})
	if err != nil {
		// daemon already dead, clean up
		os.Remove(SocketPath())
		os.Remove(PidPath())
		return nil
	}
	if !resp.OK {
		return fmt.Errorf("stop failed: %s", resp.Error)
	}
	return nil
}
