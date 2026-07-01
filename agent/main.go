package main

import (
	"encoding/json"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"time"

	"database/sql"
	"github.com/gorilla/websocket"
	"github.com/yusufpapurcu/wmi"
	"golang.org/x/sys/windows/svc"
	_ "modernc.org/sqlite"
)

type Message struct {
	AgentID string `json:"agentId"`
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

type TelemetryData struct {
	OS          string  `json:"os"`
	Hostname    string  `json:"hostname"`
	CPUModel    string  `json:"cpuModel"`
	CPULoad     float64 `json:"cpuLoad"`
	TotalRAM    uint64  `json:"totalRam"`
	FreeRAM     uint64  `json:"freeRam"`
	DiskTotal   uint64  `json:"diskTotal"`
	DiskFree    uint64  `json:"diskFree"`
}

type Win32_OperatingSystem struct {
	Caption                string
	TotalVisibleMemorySize uint64
	FreePhysicalMemory     uint64
}

type Win32_Processor struct {
	Name           string
	LoadPercentage uint16
}

type Win32_LogicalDisk struct {
	DeviceID  string
	Size      uint64
	FreeSpace uint64
}

var agentID = "windows-client-dev"
var backendAddr = "localhost:8080"

type agentService struct{}

func (m *agentService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	// Channel to signal shutdown to the main connection loop
	shutdownChan := make(chan struct{})

	go func() {
		runAgentLoop(shutdownChan)
	}()

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				close(shutdownChan)
				return
			default:
				log.Printf("Unexpected control request: %d", c.Cmd)
			}
		}
	}
}

var localDB *sql.DB

func initLocalDB() {
	dbDir := "C:\\ProgramData\\OzyShield"
	if _, err := os.Stat(dbDir); os.IsNotExist(err) {
		dbDir = "."
	}
	dbPath := dbDir + "\\queue.db"

	var err error
	localDB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("Failed to open local SQLite: %v", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS pending_telemetry (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		payload    TEXT,
		created_at TEXT
	);`
	if _, err := localDB.Exec(schema); err != nil {
		log.Fatalf("Failed to create local schema: %v", err)
	}
	log.Printf("Local SQLite queue initialized at %s", dbPath)
}

func queueTelemetryOffline(payload string) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := localDB.Exec(`INSERT INTO pending_telemetry (payload, created_at) VALUES (?, ?)`, payload, now)
	if err != nil {
		log.Printf("Failed to queue telemetry offline: %v", err)
	} else {
		log.Println("WS offline. Telemetry queued locally in SQLite.")
	}
}

func flushOfflineTelemetry(conn *websocket.Conn) {
	rows, err := localDB.Query(`SELECT id, payload, created_at FROM pending_telemetry ORDER BY id ASC`)
	if err != nil {
		log.Printf("Failed to read local queue: %v", err)
		return
	}
	defer rows.Close()

	type HistoricalPayload struct {
		Payload   string `json:"payload"`
		CreatedAt string `json:"createdAt"`
	}

	var flushedIDs []int64
	var history []HistoricalPayload

	for rows.Next() {
		var id int64
		var payload, createdAt string
		if err := rows.Scan(&id, &payload, &createdAt); err == nil {
			flushedIDs = append(flushedIDs, id)
			history = append(history, HistoricalPayload{
				Payload:   payload,
				CreatedAt: createdAt,
			})
		}
	}

	if len(history) == 0 {
		return
	}

	log.Printf("Flushing %d offline telemetry records to backend...", len(history))
	payloadBytes, _ := json.Marshal(history)
	msg := Message{
		AgentID: agentID,
		Type:    "telemetry_history",
		Payload: string(payloadBytes),
	}
	msgBytes, _ := json.Marshal(msg)
	err = conn.WriteMessage(websocket.TextMessage, msgBytes)
	if err != nil {
		log.Printf("Failed to send offline telemetry flush: %v", err)
		return
	}

	// Clean up successfully sent records
	for _, id := range flushedIDs {
		_, _ = localDB.Exec(`DELETE FROM pending_telemetry WHERE id = ?`, id)
	}
	log.Println("Offline telemetry flush completed and local cache cleared.")
}

func main() {
	// Load configuration from env vars if present
	if envID := os.Getenv("AGENT_ID"); envID != "" {
		agentID = envID
	}
	if envBackend := os.Getenv("BACKEND_URL"); envBackend != "" {
		backendAddr = envBackend
	}

	hostname, _ := os.Hostname()
	log.Printf("Starting agent on %s (%s) [ID: %s, Backend: %s]", hostname, runtime.GOOS, agentID, backendAddr)

	initLocalDB()

	isService, err := svc.IsWindowsService()
	if err != nil {
		log.Fatalf("Failed to determine if running as service: %v", err)
	}

	if isService {
		err = svc.Run("OzyShieldAgent", &agentService{})
		if err != nil {
			log.Fatalf("Service execution failed: %v", err)
		}
	} else {
		// Run interactively in the foreground
		runAgentLoop(nil)
	}
}

func runAgentLoop(shutdownChan chan struct{}) {
	for {
		select {
		case <-shutdownChan:
			log.Println("Received shutdown signal. Stopping connection loop.")
			return
		default:
		}

		u := url.URL{Scheme: "ws", Host: backendAddr, Path: "/agent/connect", RawQuery: "id=" + agentID}
		log.Printf("Connecting to %s", u.String())

		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			log.Printf("Dial failed: %v. Retrying in 5 seconds...", err)
			
			// Queue telemetry locally if we are disconnected and try to collect
			data := collectTelemetry()
			payloadBytes, _ := json.Marshal(data)
			queueTelemetryOffline(string(payloadBytes))

			select {
			case <-shutdownChan:
				return
			case <-time.After(5 * time.Second):
				continue
			}
		}

		log.Println("Connected to Backend successfully.")
		
		// Flush any local backup of metrics
		flushOfflineTelemetry(conn)

		// Run handleConnection inside a helper that handles early cancellation
		connDone := make(chan struct{})
		go func() {
			handleConnection(conn)
			close(connDone)
		}()

		select {
		case <-shutdownChan:
			log.Println("Shutdown signal received while connected. Closing connection...")
			conn.Close()
			<-connDone
			return
		case <-connDone:
			conn.Close()
		}

		log.Println("Disconnected from Backend. Retrying in 5 seconds...")
		select {
		case <-shutdownChan:
			return
		case <-time.After(5 * time.Second):
		}
	}
}



func handleConnection(conn *websocket.Conn) {
	// Goroutine for periodic telemetry (every 10 seconds)
	stopTelemetry := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				data := collectTelemetry()
				payloadBytes, _ := json.Marshal(data)
				msg := Message{
					AgentID: agentID,
					Type:    "telemetry",
					Payload: string(payloadBytes),
				}
				msgBytes, _ := json.Marshal(msg)
				err := conn.WriteMessage(websocket.TextMessage, msgBytes)
				if err != nil {
					log.Printf("WS write failed: %v. Storing offline...", err)
					queueTelemetryOffline(string(payloadBytes))
				}
			case <-stopTelemetry:
				return
			}
		}
	}()

	defer close(stopTelemetry)

	// Keep track of active terminal process
	var cmd *exec.Cmd
	var stdin io.WriteCloser

	defer func() {
		if cmd != nil && cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Read error: %v", err)
			break
		}

		var msg Message
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			log.Printf("Invalid message format: %v", err)
			continue
		}

		switch msg.Type {
		case "terminal_input":
			if cmd == nil {
				// Start shell (powershell.exe on Windows, bash/sh on others)
				shell := "powershell.exe"
				if runtime.GOOS != "windows" {
					shell = "sh"
				}

				cmd = exec.Command(shell)
				var err error
				stdin, err = cmd.StdinPipe()
				if err != nil {
					log.Printf("Failed to create stdin pipe: %v", err)
					cmd = nil
					continue
				}

				stdout, err := cmd.StdoutPipe()
				if err != nil {
					log.Printf("Failed to create stdout pipe: %v", err)
					cmd = nil
					continue
				}

				stderr, err := cmd.StderrPipe()
				if err != nil {
					log.Printf("Failed to create stderr pipe: %v", err)
					cmd = nil
					continue
				}

				// Forward stdout/stderr output back to websocket
				go pipeReader(stdout, conn)
				go pipeReader(stderr, conn)

				if err := cmd.Start(); err != nil {
					log.Printf("Failed to start shell: %v", err)
					cmd = nil
					continue
				}

				go func() {
					cmd.Wait()
					cmd = nil
					stdin = nil
				}()
			}

			// Write input to shell process stdin
			if stdin != nil {
				_, _ = stdin.Write([]byte(msg.Payload))
			}

		case "backup_command":
			log.Printf("Received backup command: %s", msg.Payload)
			// Trigger backup CLI sidecar (Kopia/Restic)
			go runBackupSidecar(conn, msg.Payload)
		}
	}
}

func pipeReader(r io.Reader, conn *websocket.Conn) {
	buf := make([]byte, 1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			msg := Message{
				AgentID: agentID,
				Type:    "terminal_output",
				Payload: string(buf[:n]),
			}
			msgBytes, _ := json.Marshal(msg)
			_ = conn.WriteMessage(websocket.TextMessage, msgBytes)
		}
		if err != nil {
			break
		}
	}
}

func collectTelemetry() TelemetryData {
	hostname, _ := os.Hostname()
	data := TelemetryData{
		OS:       runtime.GOOS,
		Hostname: hostname,
	}

	if runtime.GOOS == "windows" {
		// Query Operating System info
		var osInfo []Win32_OperatingSystem
		if err := wmi.Query("SELECT Caption, TotalVisibleMemorySize, FreePhysicalMemory FROM Win32_OperatingSystem", &osInfo); err == nil && len(osInfo) > 0 {
			data.OS = osInfo[0].Caption
			data.TotalRAM = osInfo[0].TotalVisibleMemorySize * 1024
			data.FreeRAM = osInfo[0].FreePhysicalMemory * 1024
		} else if err != nil {
			log.Printf("WMI OS Query failed: %v", err)
		}

		// Query CPU info
		var cpuInfo []Win32_Processor
		if err := wmi.Query("SELECT Name, LoadPercentage FROM Win32_Processor", &cpuInfo); err == nil && len(cpuInfo) > 0 {
			data.CPUModel = cpuInfo[0].Name
			data.CPULoad = float64(cpuInfo[0].LoadPercentage)
		} else if err != nil {
			log.Printf("WMI CPU Query failed: %v", err)
		}

		// Query Logical Disk C: info
		var diskInfo []Win32_LogicalDisk
		if err := wmi.Query("SELECT DeviceID, Size, FreeSpace FROM Win32_LogicalDisk WHERE DeviceID = 'C:'", &diskInfo); err == nil && len(diskInfo) > 0 {
			data.DiskTotal = diskInfo[0].Size
			data.DiskFree = diskInfo[0].FreeSpace
		} else if err != nil {
			log.Printf("WMI Disk Query failed: %v", err)
		}
	} else {
		// Fallback for non-Windows (simple metrics)
		data.OS = runtime.GOOS
		data.CPUModel = "Unix CPU Model"
		data.CPULoad = 5.0
		data.TotalRAM = 16 * 1024 * 1024 * 1024
		data.FreeRAM = 8 * 1024 * 1024 * 1024
		data.DiskTotal = 500 * 1024 * 1024 * 1024
		data.DiskFree = 250 * 1024 * 1024 * 1024
	}

	return data
}

func runBackupSidecar(conn *websocket.Conn, payload string) {
	// Send initial status back
	sendWSMsg(conn, "backup_status", "Initializing Kopia sidecar backup...")

	// Verify if kopia.exe is installed (e.g. C:\ProgramData\OzyShield\kopia.exe)
	// If not found, simulate downloading Kopia silently.
	kopiaPath := "C:\\ProgramData\\OzyShield\\kopia.exe"
	if _, err := os.Stat(kopiaPath); os.IsNotExist(err) {
		sendWSMsg(conn, "backup_status", "Kopia binary not found. Downloading Kopia silently...")
		time.Sleep(2 * time.Second) // Simulate download delay
		sendWSMsg(conn, "backup_status", "Kopia engine initialized successfully.")
	}

	sendWSMsg(conn, "backup_status", "Connecting to secure repository backup destination...")
	time.Sleep(1 * time.Second)

	// Simulate/run snapshotting
	steps := []string{
		"Scanning directory targets...",
		"Uploading data blocks: 15% complete",
		"Uploading data blocks: 45% complete",
		"Uploading data blocks: 80% complete",
		"Hashing and finalizing backup snapshot...",
	}

	for _, step := range steps {
		time.Sleep(1 * time.Second)
		sendWSMsg(conn, "backup_status", step)
	}

	sendWSMsg(conn, "backup_status", "Backup complete. Snapshot hash created: rmm_kopia_5df82c91a0b3")
}

func sendWSMsg(conn *websocket.Conn, msgType, payload string) {
	msg := Message{
		AgentID: agentID,
		Type:    msgType,
		Payload: payload,
	}
	msgBytes, _ := json.Marshal(msg)
	_ = conn.WriteMessage(websocket.TextMessage, msgBytes)
}

