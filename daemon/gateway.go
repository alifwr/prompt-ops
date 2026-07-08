package main

import (
	"encoding/json"
	"log"
	"runtime"
	"sync"
	"time"

	"aether-daemon/mcp"
	"aether-daemon/pty"

	"github.com/gorilla/websocket"
)

// GatewayMessage represents a message sent to/from the Control Panel Gateway
type GatewayMessage struct {
	Action    string                 `json:"action"`
	Name      string                 `json:"name,omitempty"`
	Cols      uint16                 `json:"cols,omitempty"`
	Rows      uint16                 `json:"rows,omitempty"`
	Data      string                 `json:"data,omitempty"`
	RpcID     string                 `json:"rpc_id,omitempty"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// GatewayClient manages the WebSocket connection to the Control Panel Gateway
type GatewayClient struct {
	gatewayURL string

	wsConn  *websocket.Conn
	wsMutex sync.Mutex

	ptySession *pty.PtySession
	ptyMutex   sync.Mutex
}

// NewGatewayClient creates a new GatewayClient instance
func NewGatewayClient(gatewayURL string) *GatewayClient {
	return &GatewayClient{
		gatewayURL: gatewayURL,
	}
}

// Start runs the client's connection retry loop with exponential backoff
func (gc *GatewayClient) Start() {
	backoff := 1 * time.Second
	maxBackoff := 60 * time.Second

	for {
		log.Printf("Connecting to Control Panel Gateway at %s...", gc.gatewayURL)
		conn, _, err := websocket.DefaultDialer.Dial(gc.gatewayURL, nil)
		if err != nil {
			log.Printf("Gateway connection failed: %v. Retrying in %v...", err, backoff)
			time.Sleep(backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		log.Println("Successfully connected to Control Panel Gateway.")

		gc.wsMutex.Lock()
		gc.wsConn = conn
		gc.wsMutex.Unlock()

		connectedAt := time.Now()

		// Run the communication loop
		gc.handleConnection(conn)

		log.Println("Connection closed.")

		// If we stayed connected for more than 10 seconds, reset backoff
		if time.Since(connectedAt) > 10*time.Second {
			backoff = 1 * time.Second
		}

		log.Printf("Retrying connection in %v...", backoff)
		time.Sleep(backoff)

		// Double backoff for next reconnection attempt
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (gc *GatewayClient) handleConnection(conn *websocket.Conn) {
	// Clean up PTY session and connection on disconnect
	defer func() {
		gc.ptyMutex.Lock()
		if gc.ptySession != nil {
			log.Println("Closing PTY session on WebSocket disconnect...")
			gc.ptySession.Close()
			gc.ptySession = nil
		}
		gc.ptyMutex.Unlock()

		conn.Close()

		gc.wsMutex.Lock()
		if gc.wsConn == conn {
			gc.wsConn = nil
		}
		gc.wsMutex.Unlock()
	}()

	// Send handshake payload
	handshake := GatewayMessage{
		Action: "handshake",
		Name:   "local-dev-vps",
	}
	handshakeBytes, err := json.Marshal(handshake)
	if err != nil {
		log.Printf("Failed to marshal handshake: %v", err)
		return
	}

	gc.wsMutex.Lock()
	err = conn.WriteMessage(websocket.TextMessage, handshakeBytes)
	gc.wsMutex.Unlock()
	if err != nil {
		log.Printf("Failed to send handshake: %v", err)
		return
	}
	log.Println("Handshake sent successfully.")

	// Listen for incoming messages
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading WebSocket message: %v", err)
			break
		}

		var msg GatewayMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("Failed to parse WebSocket message JSON: %v. Raw message: %s", err, string(message))
			continue
		}

		gc.routeMessage(msg)
	}
}

func (gc *GatewayClient) routeMessage(msg GatewayMessage) {
	switch msg.Action {
	case "spawn_pty":
		gc.spawnPty(msg.Rows, msg.Cols)
	case "pty_input":
		gc.writePtyInput(msg.Data)
	case "pty_resize":
		gc.resizePty(msg.Rows, msg.Cols)
	case "close_pty":
		gc.closePty()
	case "tools/call":
		go gc.handleToolCall(msg)
	default:
		log.Printf("Unknown action received from WebSocket: %s", msg.Action)
	}
}

// handleToolCall processes an MCP tools/call request from the Gateway
func (gc *GatewayClient) handleToolCall(msg GatewayMessage) {
	log.Printf("Executing MCP tool: %s (rpc_id: %s)", msg.Name, msg.RpcID)

	// Marshal arguments back to JSON for MCP handler
	argsJSON, err := json.Marshal(msg.Arguments)
	if err != nil {
		log.Printf("Failed to marshal tool arguments: %v", err)
		gc.sendToolResponse(msg.RpcID, nil, err)
		return
	}

	// Execute through MCP handler
	result, err := mcp.HandleToolCall(msg.Name, argsJSON)
	gc.sendToolResponse(msg.RpcID, result, err)
}

// sendToolResponse sends the result of a tool execution back over WebSocket
func (gc *GatewayClient) sendToolResponse(rpcID string, result interface{}, execErr error) {
	response := map[string]interface{}{
		"action": "tools/response",
		"rpc_id": rpcID,
	}
	if execErr != nil {
		response["result"] = map[string]string{"error": execErr.Error()}
	} else {
		response["result"] = result
	}

	respBytes, err := json.Marshal(response)
	if err != nil {
		log.Printf("Failed to marshal tool response: %v", err)
		return
	}

	gc.wsMutex.Lock()
	defer gc.wsMutex.Unlock()
	if gc.wsConn != nil {
		if err := gc.wsConn.WriteMessage(websocket.TextMessage, respBytes); err != nil {
			log.Printf("Failed to send tool response: %v", err)
		}
	}
}

func (gc *GatewayClient) spawnPty(rows, cols uint16) {
	gc.ptyMutex.Lock()
	defer gc.ptyMutex.Unlock()

	if gc.ptySession != nil {
		log.Println("A PTY session is already active. Closing it before spawning a new one...")
		gc.ptySession.Close()
		gc.ptySession = nil
	}

	var shellCmd string
	if runtime.GOOS == "windows" {
		shellCmd = "cmd.exe"
	} else {
		shellCmd = "/bin/sh"
	}

	log.Printf("Spawning PTY session with shell: %s, size: %dx%d...", shellCmd, cols, rows)
	ps, err := pty.StartPty(shellCmd, nil, rows, cols)
	if err != nil {
		log.Printf("Failed to start PTY session: %v", err)
		return
	}

	gc.ptySession = ps

	// Start goroutine to read from PTY stdout and send it back to WebSocket
	go func(session *pty.PtySession) {
		buf := make([]byte, 4096)
		for {
			n, err := session.Read(buf)
			if n > 0 {
				outputMsg := GatewayMessage{
					Action: "pty_output",
					Data:   string(buf[:n]),
				}
				outputBytes, err := json.Marshal(outputMsg)
				if err != nil {
					log.Printf("Failed to marshal PTY output: %v", err)
					continue
				}

				gc.wsMutex.Lock()
				conn := gc.wsConn
				var writeErr error
				if conn != nil {
					writeErr = conn.WriteMessage(websocket.TextMessage, outputBytes)
				}
				gc.wsMutex.Unlock()

				if conn == nil {
					log.Println("WebSocket is nil, terminating PTY reader goroutine.")
					return
				}
				if writeErr != nil {
					log.Printf("WebSocket write error, terminating PTY reader goroutine: %v", writeErr)
					return
				}
			}
			if err != nil {
				log.Printf("PTY read error (EOF/closed): %v", err)
				return
			}
		}
	}(ps)
}

func (gc *GatewayClient) writePtyInput(data string) {
	gc.ptyMutex.Lock()
	ps := gc.ptySession
	gc.ptyMutex.Unlock()

	if ps == nil {
		log.Println("Warning: Received pty_input but no active PTY session exists.")
		return
	}

	_, err := ps.Write([]byte(data))
	if err != nil {
		log.Printf("Failed to write to PTY stdin: %v", err)
	}
}

func (gc *GatewayClient) resizePty(rows, cols uint16) {
	gc.ptyMutex.Lock()
	ps := gc.ptySession
	gc.ptyMutex.Unlock()

	if ps == nil {
		log.Println("Warning: Received pty_resize but no active PTY session exists.")
		return
	}

	log.Printf("Resizing active PTY to %dx%d...", cols, rows)
	err := ps.Resize(rows, cols)
	if err != nil {
		log.Printf("Failed to resize PTY: %v", err)
	}
}

func (gc *GatewayClient) closePty() {
	gc.ptyMutex.Lock()
	defer gc.ptyMutex.Unlock()

	if gc.ptySession == nil {
		log.Println("Warning: Received close_pty but no active PTY session exists.")
		return
	}

	log.Println("Closing active PTY session and killing process...")
	gc.ptySession.Close()
	gc.ptySession = nil
}
