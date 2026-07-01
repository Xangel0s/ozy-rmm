package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	_ "modernc.org/sqlite"
)

// ─── WebSocket Upgrader ──────────────────────────────────────────────────────

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// ─── Message Structs ─────────────────────────────────────────────────────────

// Message is the envelope every WebSocket participant uses.
type Message struct {
	AgentID string `json:"agentId"`
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

// TelemetryPayload mirrors the JSON the Go agent sends on the "telemetry" type.
type TelemetryPayload struct {
	OS        string  `json:"os"`
	Hostname  string  `json:"hostname"`
	CPUModel  string  `json:"cpuModel"`
	CPULoad   float64 `json:"cpuLoad"`
	TotalRAM  uint64  `json:"totalRam"`
	FreeRAM   uint64  `json:"freeRam"`
	DiskTotal uint64  `json:"diskTotal"`
	DiskFree  uint64  `json:"diskFree"`
}

// AgentInfo is the shape returned by /api/agents.
type AgentInfo struct {
	ID        string  `json:"id"`
	Hostname  string  `json:"hostname"`
	OS        string  `json:"os"`
	CPUModel  string  `json:"cpuModel"`
	CPULoad   float64 `json:"cpuLoad"`
	TotalRAM  uint64  `json:"totalRam"`
	FreeRAM   uint64  `json:"freeRam"`
	DiskTotal uint64  `json:"diskTotal"`
	DiskFree  uint64  `json:"diskFree"`
	Status    string  `json:"status"`
	LastSeen  string  `json:"lastSeen"`
}

// BackupJob is the shape returned by /api/backups.
type BackupJob struct {
	ID         int64  `json:"id"`
	AgentID    string `json:"agentId"`
	Name       string `json:"name"`
	Location   string `json:"location"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	SizeBytes  int64  `json:"sizeBytes"`
	Cron       string `json:"cron"`
	ExecutedAt string `json:"executedAt"`
	CreatedAt  string `json:"createdAt"`
}

type AlertRow struct {
	ID       int64  `json:"id"`
	AgentID  string `json:"agentId"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Time     string `json:"time"`
}

// ─── In-memory agent connections ─────────────────────────────────────────────

type AgentConnection struct {
	ID   string
	Conn *websocket.Conn
	Mu   sync.Mutex
}

var (
	agentsMu sync.Mutex
	agents   = make(map[string]*AgentConnection)
)

// ─── In-memory frontend sessions ─────────────────────────────────────────────

var (
	frontendsMu sync.Mutex
	frontends   = make(map[string]*websocket.Conn)
)

// ─── Database ─────────────────────────────────────────────────────────────────

var db *sql.DB

// initDB opens (or creates) the SQLite database and runs the schema migration.
func initDB() {
	var err error
	db, err = sql.Open("sqlite", "./rmm.db")
	if err != nil {
		log.Fatalf("Cannot open database: %v", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS agents (
		id         TEXT PRIMARY KEY,
		hostname   TEXT,
		os         TEXT,
		cpu_model  TEXT,
		cpu_load   REAL DEFAULT 0,
		total_ram  INTEGER DEFAULT 0,
		free_ram   INTEGER DEFAULT 0,
		disk_total INTEGER DEFAULT 0,
		disk_free  INTEGER DEFAULT 0,
		status     TEXT DEFAULT 'offline',
		last_seen  TEXT
	);

	CREATE TABLE IF NOT EXISTS telemetry (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		agent_id   TEXT,
		cpu_load   REAL,
		total_ram  INTEGER,
		free_ram   INTEGER,
		disk_total INTEGER,
		disk_free  INTEGER,
		recorded_at TEXT,
		FOREIGN KEY(agent_id) REFERENCES agents(id)
	);

	CREATE TABLE IF NOT EXISTS alerts (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		agent_id   TEXT,
		severity   TEXT,
		message    TEXT,
		created_at TEXT
	);

	CREATE TABLE IF NOT EXISTS backup_jobs (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		agent_id   TEXT,
		name       TEXT,
		location   TEXT,
		type       TEXT DEFAULT 'full',
		status     TEXT DEFAULT 'pending',
		size_bytes INTEGER DEFAULT 0,
		cron       TEXT DEFAULT '0 2 * * *',
		executed_at TEXT,
		created_at TEXT
	);
	`

	if _, err := db.Exec(schema); err != nil {
		log.Fatalf("Schema migration failed: %v", err)
	}

	log.Println("Database initialised at ./rmm.db")
}

// upsertAgent creates or updates the agent row and marks it online.
func upsertAgent(id string) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO agents (id, status, last_seen)
		VALUES (?, 'online', ?)
		ON CONFLICT(id) DO UPDATE SET
			status    = 'online',
			last_seen = excluded.last_seen
	`, id, now)
	if err != nil {
		log.Printf("upsertAgent error: %v", err)
	}
	// Ensure each agent has at least a default backup policy seeded
	seedBackupJob(id)
}


// markAgentOffline sets status=offline for an agent.
func markAgentOffline(id string) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`
		UPDATE agents SET status='offline', last_seen=? WHERE id=?
	`, now, id)
	if err != nil {
		log.Printf("markAgentOffline error: %v", err)
	}
}

// saveTelemetry persists telemetry data and updates the agents summary row.
func saveTelemetry(agentID string, t TelemetryPayload) {
	now := time.Now().UTC().Format(time.RFC3339)

	// Update the latest snapshot on the agents table
	_, err := db.Exec(`
		UPDATE agents SET
			hostname   = ?,
			os         = ?,
			cpu_model  = ?,
			cpu_load   = ?,
			total_ram  = ?,
			free_ram   = ?,
			disk_total = ?,
			disk_free  = ?,
			status     = 'online',
			last_seen  = ?
		WHERE id = ?
	`, t.Hostname, t.OS, t.CPUModel, t.CPULoad,
		t.TotalRAM, t.FreeRAM, t.DiskTotal, t.DiskFree, now, agentID)
	if err != nil {
		log.Printf("saveTelemetry update agents error: %v", err)
	}

	// Append a history row
	_, err = db.Exec(`
		INSERT INTO telemetry (agent_id, cpu_load, total_ram, free_ram, disk_total, disk_free, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, agentID, t.CPULoad, t.TotalRAM, t.FreeRAM, t.DiskTotal, t.DiskFree, now)
	if err != nil {
		log.Printf("saveTelemetry insert telemetry error: %v", err)
	}

	// Auto-generate an alert if CPU is above 90 %
	if t.CPULoad >= 90 {
		saveAlert(agentID, "warning",
			fmt.Sprintf("CPU usage critical: %.0f%% on %s", t.CPULoad, t.Hostname))
	}
}

// saveAlert writes an alert row to the database.
func saveAlert(agentID, severity, message string) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO alerts (agent_id, severity, message, created_at) VALUES (?, ?, ?, ?)
	`, agentID, severity, message, now)
	if err != nil {
		log.Printf("saveAlert error: %v", err)
	}
}

// seedBackupJob ensures the agent has at least one backup job entry in the DB.
// This represents the default system backup policy applied to every agent.
func seedBackupJob(agentID string) {
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM backup_jobs WHERE agent_id = ?`, agentID).Scan(&count)
	if count > 0 {
		return // already seeded
	}
	now := time.Now().UTC().Format(time.RFC3339)
	jobs := []struct {
		name, location, typ, cron string
	}{
		{"Nightly-System", "/backups/system", "full", "0 2 * * *"},
		{"Hourly-Delta", "/backups/delta", "incremental", "0 * * * *"},
	}
	for _, j := range jobs {
		_, err := db.Exec(`
			INSERT INTO backup_jobs (agent_id, name, location, type, status, cron, created_at)
			VALUES (?, ?, ?, ?, 'completed', ?, ?)
		`, agentID, j.name, j.location, j.typ, j.cron, now)
		if err != nil {
			log.Printf("seedBackupJob error: %v", err)
		}
	}
}

// handleListBackups returns all backup jobs from the DB.
func handleListBackups(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT id, agent_id, COALESCE(name,''), COALESCE(location,''), type, status,
		       size_bytes, cron, COALESCE(executed_at,''), COALESCE(created_at,'')
		FROM backup_jobs
		ORDER BY id DESC
	`)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	list := []BackupJob{}
	for rows.Next() {
		var b BackupJob
		if err := rows.Scan(&b.ID, &b.AgentID, &b.Name, &b.Location, &b.Type, &b.Status,
			&b.SizeBytes, &b.Cron, &b.ExecutedAt, &b.CreatedAt); err != nil {
			continue
		}
		list = append(list, b)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

// handleRunBackup marks a specific backup job as 'running'.
func handleRunBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		AgentID string `json:"agentId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AgentID == "" {
		http.Error(w, "missing agentId", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO backup_jobs (agent_id, name, location, type, status, cron, executed_at, created_at)
		VALUES (?, 'Manual-Backup', '/backups/manual', 'full', 'running', '@manual', ?, ?)
	`, req.AgentID, now, now)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	saveAlert(req.AgentID, "info", "Manual backup job started")
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"queued"}`))
}

// ─── HTTP Handlers ────────────────────────────────────────────────────────────

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

// handleAgentConnection accepts a WebSocket connection from a Go agent.
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

	agentConn := &AgentConnection{ID: agentID, Conn: conn}
	agentsMu.Lock()
	agents[agentID] = agentConn
	agentsMu.Unlock()

	upsertAgent(agentID)
	log.Printf("Agent connected: %s", agentID)

	defer func() {
		agentsMu.Lock()
		delete(agents, agentID)
		agentsMu.Unlock()
		markAgentOffline(agentID)
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
			var t TelemetryPayload
			if err := json.Unmarshal([]byte(msg.Payload), &t); err != nil {
				log.Printf("Failed to parse telemetry payload: %v", err)
				continue
			}
			saveTelemetry(agentID, t)
			broadcastToFrontend(agentID, msgBytes)

		case "terminal_output":
			broadcastToFrontend(agentID, msgBytes)

		case "backup_status":
			log.Printf("Backup progress from agent %s: %s", agentID, msg.Payload)
			saveAlert(agentID, "info", "Backup: "+msg.Payload)
			broadcastToFrontend(agentID, msgBytes)
		}
	}
}

// handleListAgents returns the full agents inventory from the DB as JSON.
func handleListAgents(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT id, COALESCE(hostname,''), COALESCE(os,''), COALESCE(cpu_model,''),
		       cpu_load, total_ram, free_ram, disk_total, disk_free,
		       status, COALESCE(last_seen,'')
		FROM agents
		ORDER BY status DESC, last_seen DESC
	`)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	list := []AgentInfo{}
	for rows.Next() {
		var a AgentInfo
		if err := rows.Scan(&a.ID, &a.Hostname, &a.OS, &a.CPUModel,
			&a.CPULoad, &a.TotalRAM, &a.FreeRAM, &a.DiskTotal, &a.DiskFree,
			&a.Status, &a.LastSeen); err != nil {
			continue
		}
		list = append(list, a)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

// handleListAlerts returns the 50 most recent alerts from the DB.
func handleListAlerts(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT id, agent_id, severity, message, created_at
		FROM alerts
		ORDER BY id DESC
		LIMIT 50
	`)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	list := []AlertRow{}
	for rows.Next() {
		var a AlertRow
		if err := rows.Scan(&a.ID, &a.AgentID, &a.Severity, &a.Message, &a.Time); err != nil {
			continue
		}
		list = append(list, a)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

// handleAgentTelemetry returns the last 100 telemetry rows for a given agent.
func handleAgentTelemetry(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("id")
	if agentID == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	rows, err := db.Query(`
		SELECT cpu_load, total_ram, free_ram, disk_total, disk_free, recorded_at
		FROM telemetry
		WHERE agent_id = ?
		ORDER BY id DESC
		LIMIT 100
	`, agentID)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Row struct {
		CPULoad    float64 `json:"cpuLoad"`
		TotalRAM   uint64  `json:"totalRam"`
		FreeRAM    uint64  `json:"freeRam"`
		DiskTotal  uint64  `json:"diskTotal"`
		DiskFree   uint64  `json:"diskFree"`
		RecordedAt string  `json:"recordedAt"`
	}

	list := []Row{}
	for rows.Next() {
		var row Row
		if err := rows.Scan(&row.CPULoad, &row.TotalRAM, &row.FreeRAM,
			&row.DiskTotal, &row.DiskFree, &row.RecordedAt); err != nil {
			continue
		}
		list = append(list, row)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

// handleTerminalWebSocket proxies stdin/stdout between the frontend and an agent.
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

	log.Printf("Frontend terminal session connected: %s", sessionID)

	defer func() {
		frontendsMu.Lock()
		delete(frontends, sessionID)
		frontendsMu.Unlock()
		log.Printf("Frontend terminal session disconnected: %s", sessionID)
	}()

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var cmd struct {
			AgentID string `json:"agentId"`
			Type    string `json:"type"`
			Payload string `json:"payload"`
		}

		if err := json.Unmarshal(msgBytes, &cmd); err != nil {
			log.Printf("Error parsing frontend message: %v", err)
			continue
		}

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

// broadcastToFrontend sends a raw WebSocket message to all connected dashboard sessions.
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

// ─── Entry Point ──────────────────────────────────────────────────────────────

func main() {
	initDB()

	http.HandleFunc("/agent/connect", handleAgentConnection)
	http.HandleFunc("/api/agents", corsMiddleware(handleListAgents))
	http.HandleFunc("/api/agents/telemetry", corsMiddleware(handleAgentTelemetry))
	http.HandleFunc("/api/alerts", corsMiddleware(handleListAlerts))
	http.HandleFunc("/api/backups", corsMiddleware(handleListBackups))
	http.HandleFunc("/api/backups/run", corsMiddleware(handleRunBackup))
	http.HandleFunc("/terminal/ws", handleTerminalWebSocket)

	port := ":8080"
	log.Printf("Backend starting on http://localhost%s\n", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Failed to start backend: %v", err)
	}
}
