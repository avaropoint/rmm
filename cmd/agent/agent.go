package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/avaropoint/rmm/internal/protocol"
)

// heartbeatInterval is the keep-alive period for the server connection.
const heartbeatInterval = 30 * time.Second

// Agent handles the connection to the server and manages
// screen capture and input injection.
type Agent struct {
	serverURL      string
	name           string
	conn           net.Conn
	reader         *bufio.Reader
	capturing      bool
	captureMu      sync.Mutex
	stopCapture    chan struct{}
	currentDisplay int
}

// run establishes a connection to the server, registers, and enters
// the main message loop. It returns on disconnect.
func (a *Agent) run() error {
	var err error
	a.conn, a.reader, err = dialWebSocket(a.serverURL)
	if err != nil {
		return err
	}
	defer a.conn.Close()

	log.Println("Connected to server")

	if err := a.register(); err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	opcode, data, err := protocol.ReadFrame(a.reader)
	if err != nil {
		return fmt.Errorf("failed to read registration response: %w", err)
	}
	if opcode != protocol.OpText {
		return fmt.Errorf("unexpected response opcode: %d", opcode)
	}

	var resp protocol.Message
	if err := json.Unmarshal(data, &resp); err != nil || resp.Type != "registered" {
		return fmt.Errorf("registration not confirmed")
	}
	log.Println("Registration confirmed")

	// Heartbeat goroutine (stopped on disconnect via done channel).
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				a.sendMessage(protocol.Message{Type: "heartbeat"})
			}
		}
	}()

	// Message loop.
	for {
		opcode, data, err := protocol.ReadFrame(a.reader)
		if err != nil {
			return fmt.Errorf("read error: %w", err)
		}

		switch opcode {
		case protocol.OpClose:
			return nil
		case protocol.OpPing:
			protocol.WriteClientFrame(a.conn, protocol.OpPong, data)
		case protocol.OpText:
			var msg protocol.Message
			if err := json.Unmarshal(data, &msg); err != nil {
				log.Printf("Failed to unmarshal message: %v", err)
				continue
			}

			log.Printf("Agent received message type: %s", msg.Type)

			switch msg.Type {
			case "start_capture":
				a.startCapture()
			case "stop_capture":
				a.stopCaptureLoop()
			case "input":
				log.Printf("Processing input message")
				a.handleInput(msg.Payload)
			case "switch_display":
				a.handleSwitchDisplay(msg.Payload)
			}
		}
	}
}

// sendMessage marshals and sends a protocol message over the WebSocket.
func (a *Agent) sendMessage(msg protocol.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return protocol.WriteClientFrame(a.conn, protocol.OpText, data)
}

// register collects system information and sends it to the server.
func (a *Agent) register() error {
	info := CollectSystemInfo(a.name)
	a.name = info.Name
	a.currentDisplay = 1

	return a.sendMessage(protocol.Message{
		Type:    "register",
		Payload: info.ToJSON(),
	})
}

// dialWebSocket connects to the server using a raw TCP connection
// and performs the WebSocket handshake.
func dialWebSocket(serverURL string) (net.Conn, *bufio.Reader, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return nil, nil, err
	}

	host := u.Host
	path := u.Path
	if path == "" {
		path = "/ws/agent"
	} else {
		path = path + "/ws/agent"
	}

	conn, err := net.Dial("tcp", host)
	if err != nil {
		return nil, nil, err
	}

	key := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))

	request := fmt.Sprintf("GET %s HTTP/1.1\r\n"+
		"Host: %s\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Key: %s\r\n"+
		"Sec-WebSocket-Version: 13\r\n\r\n",
		path, host, key)

	if _, err := conn.Write([]byte(request)); err != nil {
		conn.Close()
		return nil, nil, err
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, nil, err
	}

	if len(statusLine) < 12 || statusLine[9:12] != "101" {
		conn.Close()
		return nil, nil, fmt.Errorf("websocket handshake failed: %s", statusLine)
	}

	// Skip response headers.
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			conn.Close()
			return nil, nil, err
		}
		if line == "\r\n" {
			break
		}
	}

	return conn, reader, nil
}
