package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow connections from frontend
	},
}

// Message defines the structure of data sent over the WebSocket
type Message struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

type AgentConnection struct {
	ID   string
	Conn *websocket.Conn
	Mu   sync.Mutex
}

var (
	agentsMu sync.Mutex
	agents   = make(map[string]*AgentConnection)
)

func main() {
	http.HandleFunc("/agent/connect", handleAgentConnection)
	http.HandleFunc("/api/agents", handleListAgents)
	http.HandleFunc("/terminal/ws", handleTerminalWebSocket)

	port := ":8080"
	log.Printf("Backend starting on http://localhost%s\n", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Failed to start backend: %v", err)
	}
}

func handleAgentConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade agent connection: %v", err)
		return
	}
	defer conn.Close()

	agentID := r.URL.Query().Get("id")
	if agentID == "" {
		agentID = "anonymous-agent"
	}

	agentConn := &AgentConnection{
		ID:   agentID,
		Conn: conn,
	}

	agentsMu.Lock()
	agents[agentID] = agentConn
	agentsMu.Unlock()

	log.Printf("Agent connected: %s", agentID)

	defer func() {
		agentsMu.Lock()
		delete(agents, agentID)
		agentsMu.Unlock()
		log.Printf("Agent disconnected: %s", agentID)
	}()

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Agent %s read error: %v", agentID, err)
			break
		}

		var msg Message
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			log.Printf("Failed to parse agent message: %v", err)
			continue
		}

		switch msg.Type {
		case "telemetry":
			log.Printf("Received telemetry from agent %s: %s", agentID, msg.Payload)
		case "terminal_output":
			// Forward terminal output to the frontend listening on /terminal/ws
			broadcastToFrontend(agentID, msgBytes)
		case "backup_status":
			log.Printf("Backup progress from agent %s: %s", agentID, msg.Payload)
			broadcastToFrontend(agentID, msgBytes)
		}
	}
}

func handleListAgents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	agentsMu.Lock()
	list := make([]string, 0, len(agents))
	for id := range agents {
		list = append(list, id)
	}
	agentsMu.Unlock()
	json.NewEncoder(w).Encode(list)
}

// Keep track of active terminal frontend sessions
var (
	frontendsMu sync.Mutex
	frontends   = make(map[string]*websocket.Conn)
)

func handleTerminalWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade frontend connection: %v", err)
		return
	}
	defer conn.Close()

	sessionID := fmt.Sprintf("%p", conn)
	frontendsMu.Lock()
	frontends[sessionID] = conn
	frontendsMu.Unlock()

	log.Printf("Frontend session connected: %s", sessionID)

	defer func() {
		frontendsMu.Lock()
		delete(frontends, sessionID)
		frontendsMu.Unlock()
		log.Printf("Frontend session disconnected: %s", sessionID)
	}()

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break
		}

		// Parse the frontend command, which target specific agent
		var cmd struct {
			AgentID string `json:"agentId"`
			Type    string `json:"type"`
			Payload string `json:"payload"`
		}

		if err := json.Unmarshal(msgBytes, &cmd); err != nil {
			log.Printf("Error parsing frontend message: %v", err)
			continue
		}

		// Forward terminal input or backup command to agent
		agentsMu.Lock()
		agent, ok := agents[cmd.AgentID]
		agentsMu.Unlock()

		if ok {
			agent.Mu.Lock()
			agent.Conn.WriteMessage(websocket.TextMessage, msgBytes)
			agent.Mu.Unlock()
		} else {
			log.Printf("Target agent %s not found", cmd.AgentID)
		}
	}
}

func broadcastToFrontend(agentID string, data []byte) {
	frontendsMu.Lock()
	defer frontendsMu.Unlock()

	log.Printf("Broadcasting payload for agent %s to frontend clients", agentID)
	for id, conn := range frontends {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("Error sending to frontend %s: %v", id, err)
		}
	}
}
