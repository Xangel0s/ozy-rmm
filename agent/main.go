package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"database/sql"
	"net/http"
	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"github.com/gorilla/websocket"
	"github.com/yusufpapurcu/wmi"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	_ "modernc.org/sqlite"
)

var kbRegex = regexp.MustCompile(`KB(\d+)`)

// ─── Message Types ────────────────────────────────────────────────────────────

type Message struct {
	AgentID string `json:"agentId"`
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

type TelemetryData struct {
	OS            string          `json:"os"`
	Hostname      string          `json:"hostname"`
	CPUModel      string          `json:"cpuModel"`
	CPULoad       float64         `json:"cpuLoad"`
	TotalRAM      uint64          `json:"totalRam"`
	FreeRAM       uint64          `json:"freeRam"`
	DiskTotal     uint64          `json:"diskTotal"`
	DiskFree      uint64          `json:"diskFree"`
	Disks         []DiskPartition `json:"disks"`
	Vendor        string          `json:"vendor"`
	Model         string          `json:"model"`
	SerialNumber  string          `json:"serialNumber"`
	Uptime        string          `json:"uptime"`
	KernelVersion string          `json:"kernelVersion"`
	AgentVersion  string          `json:"agentVersion"`
	LocalIP       string          `json:"localIP"`
	MACAddress    string          `json:"macAddress"`
	Gateway       string          `json:"gateway"`
	NumCPU        int             `json:"numCPU"`
	GPUName       string          `json:"gpuName"`
	GPUDriver     string          `json:"gpuDriver"`
}

type DiskPartition struct {
	DeviceID   string `json:"deviceID"`
	Size       uint64 `json:"size"`
	FreeSpace  uint64 `json:"freeSpace"`
	Label      string `json:"label"`
	Filesystem string `json:"filesystem"`
}

// ─── WMI Query Structs ────────────────────────────────────────────────────────

type Win32_OperatingSystem struct {
	Caption                string
	TotalVisibleMemorySize uint64
	FreePhysicalMemory     uint64
	LastBootUpTime         string
	Version                string
}

type Win32_Processor struct {
	Name           string
	LoadPercentage uint16
	NumberOfCores  uint32
	NumberOfLogicalProcessors uint32
}

type Win32_LogicalDisk struct {
	DeviceID   string
	Size       uint64
	FreeSpace  uint64
	FileSystem string
}

type Win32_BaseBoard struct {
	Manufacturer string
	Product      string
	SerialNumber string
}

type Win32_ComputerSystem struct {
	Manufacturer string
	Model        string
}

type Win32_NetworkAdapterConfiguration struct {
	MACAddress         string
	DefaultIPGateway   []string
	IPAddress          []string
	IPEnabled          bool
}

type Win32_VideoController struct {
	Name           string
	DriverVersion  string
}

type SoftwareItem struct {
	Name                 string `json:"name"`
	Publisher            string `json:"publisher"`
	Version              string `json:"version"`
	InstallDate          string `json:"installDate"`
	EstimatedSizeKB      int64  `json:"estimatedSizeKB"`
	QuietUninstallString string `json:"quietUninstallString"`
}

// ─── Configuration ────────────────────────────────────────────────────────────

var agentID = "windows-client-dev"
var backendAddr = "127.0.0.1:8080"
var agentVersion = "1.2.0"
var agentJWT = ""

const AGENT_VERSION = "1.2.0"

// ─── Windows Service ──────────────────────────────────────────────────────────

type agentService struct{}

func (m *agentService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

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

// ─── Local SQLite Queue ──────────────────────────────────────────────────────

var localDB *sql.DB

// ─── Remote Screen State ──────────────────────────────────────────────────────

var (
	screenCancel  context.CancelFunc
	screenMu      sync.Mutex
)

// ─── Local SQLite Queue ──────────────────────────────────────────────────────

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
	);
	CREATE TABLE IF NOT EXISTS agent_credentials (
		id         INTEGER PRIMARY KEY CHECK (id = 1),
		agent_id   TEXT NOT NULL,
		tenant_id  TEXT NOT NULL,
		jwt_token  TEXT NOT NULL,
		created_at TEXT NOT NULL
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

type enrollResponse struct {
	AgentID  string `json:"agentId"`
	TenantID string `json:"tenantId"`
	Token    string `json:"token"`
}

func loadCredentials() (agentID, tenantID, jwt string, ok bool) {
	var a, t, j string
	err := localDB.QueryRow(`SELECT agent_id, tenant_id, jwt_token FROM agent_credentials WHERE id = 1`).Scan(&a, &t, &j)
	if err != nil {
		return "", "", "", false
	}
	return a, t, j, true
}

func saveCredentials(agentID, tenantID, jwt string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := localDB.Exec(`
		INSERT INTO agent_credentials (id, agent_id, tenant_id, jwt_token, created_at)
		VALUES (1, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			agent_id = EXCLUDED.agent_id,
			tenant_id = EXCLUDED.tenant_id,
			jwt_token = EXCLUDED.jwt_token,
			created_at = EXCLUDED.created_at
	`, agentID, tenantID, jwt, now)
	return err
}

func enrollWithToken(enrollToken string) error {
	info := collectTelemetry()
	body, err := json.Marshal(map[string]interface{}{
		"token": enrollToken,
		"info":  info,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal enroll payload: %w", err)
	}

	enrollURL := "http://" + backendAddr + "/api/enroll"
	req, err := http.NewRequest(http.MethodPost, enrollURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to build enroll request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("enroll request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("enroll returned %d: %s", resp.StatusCode, string(respBody))
	}

	var er enrollResponse
	if err := json.Unmarshal(respBody, &er); err != nil {
		return fmt.Errorf("failed to parse enroll response: %w", err)
	}

	if err := saveCredentials(er.AgentID, er.TenantID, er.Token); err != nil {
		return fmt.Errorf("failed to persist credentials: %w", err)
	}

	log.Printf("Enrolled successfully. agent_id=%s tenant_id=%s", er.AgentID, er.TenantID)
	return nil
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

	for _, id := range flushedIDs {
		_, _ = localDB.Exec(`DELETE FROM pending_telemetry WHERE id = ?`, id)
	}
	log.Println("Offline telemetry flush completed and local cache cleared.")
}

// ─── Network Utilities ────────────────────────────────────────────────────────

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}
	return ""
}

func getMACAddress() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp != 0 && iface.HardwareAddr != nil {
			// Skip loopback and virtual interfaces
			if !strings.Contains(iface.Name, "Loopback") && len(iface.HardwareAddr) > 0 {
				return iface.HardwareAddr.String()
			}
		}
	}
	return ""
}

// ─── Windows Event Log Writer ──────────────────────────────────────────────────

type winEventLogWriter struct {
	el *eventlog.Log
}

func (w *winEventLogWriter) Write(p []byte) (int, error) {
	if w.el == nil {
		return len(p), nil
	}
	msg := strings.TrimRight(string(p), "\r\n")
	w.el.Info(1, msg)
	return len(p), nil
}

func initEventLog() {
	if runtime.GOOS != "windows" {
		return
	}
	el, err := eventlog.Open("OzyShieldAgent")
	if err != nil {
		log.Printf("Warning: could not open Windows Event Log: %v", err)
		return
	}
	log.SetOutput(io.MultiWriter(os.Stdout, &winEventLogWriter{el: el}))
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	if envBackend := os.Getenv("BACKEND_URL"); envBackend != "" {
		backendAddr = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(envBackend, "https://"), "http://"))
	}
	if envVersion := os.Getenv("AGENT_VERSION"); envVersion != "" {
		agentVersion = envVersion
	}

	hostname, _ := os.Hostname()
	log.Printf("Starting agent v%s on %s (%s) [Backend: %s]", AGENT_VERSION, hostname, runtime.GOOS, backendAddr)

	initLocalDB()
	initEventLog()

	if id, _, jwt, ok := loadCredentials(); ok {
		agentID = id
		agentJWT = jwt
		log.Printf("Loaded credentials from local DB. agent_id=%s", agentID)
	} else {
		enrollToken := os.Getenv("ENROLL_TOKEN")
		if enrollToken == "" {
			log.Fatal("ENROLL_TOKEN environment variable is required for first run")
		}
		if envID := os.Getenv("AGENT_ID"); envID != "" {
			agentID = envID
		}
		log.Printf("No local credentials found. Enrolling with backend...")
		if err := enrollWithToken(enrollToken); err != nil {
			log.Fatalf("Enrollment failed: %v", err)
		}
		id, _, jwt, _ := loadCredentials()
		agentID = id
		agentJWT = jwt
	}

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

		u := url.URL{Scheme: "ws", Host: backendAddr, Path: "/agent/connect", RawQuery: "token=" + agentJWT}
		log.Printf("Connecting to %s/agent/connect (agent_id=%s)", "ws://"+backendAddr, agentID)

		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			log.Printf("Dial failed: %v. Retrying in 5 seconds...", err)

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
		flushOfflineTelemetry(conn)

		connDone := make(chan struct{})
		go func() {
			handleConnection(conn)
			close(connDone)
		}()

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		telemetrySent := false

	loop:
		for {
			select {
			case <-shutdownChan:
				log.Println("Shutdown signal received while connected. Closing connection...")
				conn.Close()
				<-connDone
				return
			case <-connDone:
				conn.Close()
				break loop
			case <-ticker.C:
				data := collectTelemetry()
				payloadBytes, err := json.Marshal(data)
				if err != nil {
					log.Printf("Failed to marshal telemetry: %v", err)
					continue
				}
				msg := Message{AgentID: agentID, Type: "telemetry", Payload: string(payloadBytes)}
				msgBytes, err := json.Marshal(msg)
				if err != nil {
					log.Printf("Failed to wrap telemetry message: %v", err)
					continue
				}
				if err := conn.WriteMessage(websocket.TextMessage, msgBytes); err != nil {
					log.Printf("Failed to send telemetry: %v", err)
				} else if !telemetrySent {
					log.Println("Initial telemetry sent.")
					telemetrySent = true
				}
			}
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

	var cmd *exec.Cmd
	var stdin io.WriteCloser
	var termJob windows.Handle

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
				shell := "powershell.exe"
				if runtime.GOOS != "windows" {
					shell = "sh"
				}

				// Use the same Job Object + CREATE_SUSPENDED pattern as scripts.
				// This ensures any child process the shell creates (cmd, ping,
				// etc.) is killed when the session ends — same protection, same
				// AppX exception documented in scripts.
				cmd = exec.Command(shell)
				termJob = setupJobObject(cmd)
				var err error
				stdin, err = cmd.StdinPipe()
				if err != nil {
					log.Printf("terminal: stdin pipe failed: %v", err)
					cmd = nil
					continue
				}
				stdout, err := cmd.StdoutPipe()
				if err != nil {
					log.Printf("terminal: stdout pipe failed: %v", err)
					cmd = nil
					continue
				}
				stderr, err := cmd.StderrPipe()
				if err != nil {
					log.Printf("terminal: stderr pipe failed: %v", err)
					cmd = nil
					continue
				}

				if err := cmd.Start(); err != nil {
					log.Printf("terminal: start failed: %v", err)
					cmd = nil
					continue
				}
				if termJob != 0 {
					attachJob(termJob, cmd.Process.Pid)
				}

				go pipeReader(stdout, conn)
				go pipeReader(stderr, conn)

				// Cleanup: wait for shell to exit, then release Job Object.
				// terminal_closed kills the shell first, which makes
				// cmd.Wait() return, which lets this goroutine clean up.
				go func() {
					cmd.Wait()
					log.Printf("terminal: cmd.Wait() returned, termJob=%d", termJob)
					if termJob != 0 {
						windows.TerminateJobObject(termJob, 1)
						windows.CloseHandle(termJob)
						termJob = 0
					}
					cmd = nil
					stdin = nil
				}()
			}

			if stdin != nil {
				_, _ = stdin.Write([]byte(msg.Payload))
			}

		case "terminal_closed":
			log.Printf("terminal: received terminal_closed from backend")
			if termJob != 0 {
				log.Printf("terminal: TerminateJobObject(%d)", termJob)
				err := windows.TerminateJobObject(termJob, 1)
				log.Printf("terminal: TerminateJobObject result: %v", err)
			} else {
				log.Printf("terminal: termJob is 0, nothing to kill")
			}
			if cmd != nil {
				cmd.Process.Kill()
			}
			if stdin != nil {
				stdin.Close()
			}

		case "backup_command":
			log.Printf("Received backup command: %s", msg.Payload)
			go executeBackupSidecar(conn, msg.Payload)

		case "list_snapshots":
			log.Printf("Received list_snapshots command: %s", msg.Payload)
			go listSnapshotsSidecar(conn, msg.Payload)

		case "restore_command":
			log.Printf("Received restore command: %s", msg.Payload)
			go executeRestoreSidecar(conn, msg.Payload)

		case "scan_software":
			log.Printf("Received software scan request")
			go func() {
				software := scanInstalledSoftware()
				payloadBytes, _ := json.Marshal(software)
				sendWSMsg(conn, "software_list", string(payloadBytes))
			}()

		case "scan_patches":
			log.Printf("Received patches scan request")
			go func() {
				patches := scanInstalledPatches()
				payloadBytes, _ := json.Marshal(patches)
				sendWSMsg(conn, "patch_list", string(payloadBytes))
			}()

		case "check_command":
			log.Printf("Received check command: %s", msg.Payload)
			go executeCheck(conn, msg.Payload)

		case "software_uninstall_command":
			log.Printf("Received software uninstall command: %s", msg.Payload)
			go executeSoftwareUninstall(conn, msg.Payload)

		case "script_command":
			log.Printf("Received script command: %s", msg.Payload)
			go executeScript(conn, msg.Payload)

		case "screen_start":
			log.Printf("screen: starting capture")

			screenMu.Lock()
			if screenCancel != nil {
				screenCancel()
			}
			var ctx context.Context
			ctx, screenCancel = context.WithCancel(context.Background())
			screenMu.Unlock()

			var cfg struct {
				Quality int `json:"quality"`
				FPS     int `json:"fps"`
			}
			json.Unmarshal([]byte(msg.Payload), &cfg)
			if cfg.Quality <= 0 {
				cfg.Quality = 50
			}
			if cfg.FPS <= 0 {
				cfg.FPS = 5
			}

			go func() {
				frameInterval := time.Second / time.Duration(cfg.FPS)
				ticker := time.NewTicker(frameInterval)
				defer ticker.Stop()

				// Send first frame immediately
				if data, err := captureScreenJPEG(cfg.Quality); err == nil {
					encoded := base64.StdEncoding.EncodeToString(data)
					sendWSMsg(conn, "screen_frame", encoded)
				}

				for {
					select {
					case <-ctx.Done():
						log.Printf("screen: capture stopped")
						return
					case <-ticker.C:
						data, err := captureScreenJPEG(cfg.Quality)
						if err != nil {
							log.Printf("screen: capture error: %v", err)
							continue
						}
						encoded := base64.StdEncoding.EncodeToString(data)
						sendWSMsg(conn, "screen_frame", encoded)
					}
				}
			}()

		case "screen_stop":
			log.Printf("screen: stopping capture")
			screenMu.Lock()
			if screenCancel != nil {
				screenCancel()
				screenCancel = nil
			}
			screenMu.Unlock()

		case "screen_input":
			handleScreenInput(msg.Payload)
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

// ─── Telemetry Collection ─────────────────────────────────────────────────────

func collectTelemetry() TelemetryData {
	hostname, _ := os.Hostname()
	data := TelemetryData{
		Hostname:     hostname,
		AgentVersion: AGENT_VERSION,
		NumCPU:       runtime.NumCPU(),
		LocalIP:      getLocalIP(),
		MACAddress:   getMACAddress(),
	}

	if runtime.GOOS == "windows" {
		collectWindowsTelemetry(&data)
	} else {
		collectUnixTelemetry(&data)
	}

	return data
}

func collectWindowsTelemetry(data *TelemetryData) {
	// OS info
	var osInfo []Win32_OperatingSystem
	if err := wmi.Query("SELECT Caption, TotalVisibleMemorySize, FreePhysicalMemory, LastBootUpTime, Version FROM Win32_OperatingSystem", &osInfo); err == nil && len(osInfo) > 0 {
		data.OS = osInfo[0].Caption
		data.TotalRAM = osInfo[0].TotalVisibleMemorySize * 1024
		data.FreeRAM = osInfo[0].FreePhysicalMemory * 1024
		data.KernelVersion = osInfo[0].Version
		data.Uptime = parseWMITime(osInfo[0].LastBootUpTime)
	} else if err != nil {
		log.Printf("WMI OS Query failed: %v", err)
	}

	// CPU info
	var cpuInfo []Win32_Processor
	if err := wmi.Query("SELECT Name, LoadPercentage, NumberOfCores, NumberOfLogicalProcessors FROM Win32_Processor", &cpuInfo); err == nil && len(cpuInfo) > 0 {
		data.CPUModel = cpuInfo[0].Name
		data.CPULoad = float64(cpuInfo[0].LoadPercentage)
	} else if err != nil {
		log.Printf("WMI CPU Query failed: %v", err)
	}

	// Disk info - all logical disks with individual partitions
	var diskInfo []Win32_LogicalDisk
	if err := wmi.Query("SELECT DeviceID, Size, FreeSpace, FileSystem FROM Win32_LogicalDisk", &diskInfo); err == nil {
		for _, d := range diskInfo {
			data.DiskTotal += d.Size
			data.DiskFree += d.FreeSpace
			data.Disks = append(data.Disks, DiskPartition{
				DeviceID:   d.DeviceID,
				Size:       d.Size,
				FreeSpace:  d.FreeSpace,
				Label:      d.DeviceID,
				Filesystem: d.FileSystem,
			})
		}
	} else if err != nil {
		log.Printf("WMI Disk Query failed: %v", err)
	}

	// Hardware info - BaseBoard
	var boardInfo []Win32_BaseBoard
	if err := wmi.Query("SELECT Manufacturer, Product, SerialNumber FROM Win32_BaseBoard", &boardInfo); err == nil && len(boardInfo) > 0 {
		data.Vendor = boardInfo[0].Manufacturer
		data.Model = boardInfo[0].Product
		data.SerialNumber = boardInfo[0].SerialNumber
	}

	// If no board info, try ComputerSystem
	if data.Model == "" {
		var csInfo []Win32_ComputerSystem
		if err := wmi.Query("SELECT Manufacturer, Model FROM Win32_ComputerSystem", &csInfo); err == nil && len(csInfo) > 0 {
			data.Vendor = csInfo[0].Manufacturer
			data.Model = csInfo[0].Model
		}
	}

	// Network info
	var netInfo []Win32_NetworkAdapterConfiguration
	if err := wmi.Query("SELECT MACAddress, DefaultIPGateway, IPAddress, IPEnabled FROM Win32_NetworkAdapterConfiguration WHERE IPEnabled = TRUE", &netInfo); err == nil {
		for _, n := range netInfo {
			if n.MACAddress != "" && data.MACAddress == "" {
				data.MACAddress = n.MACAddress
			}
			if len(n.DefaultIPGateway) > 0 && data.Gateway == "" {
				data.Gateway = n.DefaultIPGateway[0]
			}
		}
	}

	// GPU info
	var gpuInfo []Win32_VideoController
	if err := wmi.Query("SELECT Name, DriverVersion FROM Win32_VideoController", &gpuInfo); err == nil && len(gpuInfo) > 0 {
		data.GPUName = gpuInfo[0].Name
		data.GPUDriver = gpuInfo[0].DriverVersion
	}
}

func collectUnixTelemetry(data *TelemetryData) {
	data.OS = fmt.Sprintf("%s %s", runtime.GOOS, runtime.GOARCH)
	data.CPUModel = "Unix CPU"
	data.KernelVersion = runtime.Version()

	// Try to get hostname for OS
	if out, err := exec.Command("uname", "-sr").Output(); err == nil {
		data.KernelVersion = strings.TrimSpace(string(out))
	}

	// Get CPU load from /proc/loadavg
	if loadAvg, err := os.ReadFile("/proc/loadavg"); err == nil {
		var load1 float64
		fmt.Sscanf(string(loadAvg), "%f", &load1)
		data.CPULoad = load1 * 100 / float64(runtime.NumCPU())
	}

	// Get memory from /proc/meminfo
	if memInfo, err := os.ReadFile("/proc/meminfo"); err == nil {
		lines := strings.Split(string(memInfo), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "MemTotal:") {
				fmt.Sscanf(line, "MemTotal: %d kB", &data.TotalRAM)
				data.TotalRAM *= 1024
			} else if strings.HasPrefix(line, "MemAvailable:") {
				fmt.Sscanf(line, "MemAvailable: %d kB", &data.FreeRAM)
				data.FreeRAM *= 1024
			}
		}
	}

	// Get disk usage
	if out, err := exec.Command("df", "-B1", "/").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) > 1 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 4 {
				fmt.Sscanf(fields[1], "%d", &data.DiskTotal)
				fmt.Sscanf(fields[3], "%d", &data.DiskFree)
			}
		}
	}
}

func parseWMITime(wmiTime string) string {
	// WMI datetime format: yyyymmddHHMMSS.ffffff+UUU
	if len(wmiTime) < 14 {
		return "unknown"
	}
	year := wmiTime[:4]
	month := wmiTime[4:6]
	day := wmiTime[6:8]
	hour := wmiTime[8:10]
	min := wmiTime[10:12]
	sec := wmiTime[12:14]

	t, err := time.Parse("20060102150405", wmiTime[:14])
	if err != nil {
		return fmt.Sprintf("%s-%s-%s %s:%s:%s", year, month, day, hour, min, sec)
	}

	uptime := time.Since(t)
	days := int(uptime.Hours() / 24)
	hours := int(uptime.Hours()) % 24
	mins := int(uptime.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	return fmt.Sprintf("%dh %dm", hours, mins)
}

func scanInstalledSoftware() []SoftwareItem {
	var items []SoftwareItem

	paths := []string{
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`,
		`SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`,
	}

	for _, path := range paths {
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.READ)
		if err != nil {
			continue
		}
		defer key.Close()

		names, err := key.ReadSubKeyNames(0)
		if err != nil {
			continue
		}

		for _, name := range names {
			subkey, err := registry.OpenKey(key, name, registry.READ)
			if err != nil {
				continue
			}

			displayName, _, err := subkey.GetStringValue("DisplayName")
			if err != nil || displayName == "" {
				subkey.Close()
				continue
			}

			if val, _, err := subkey.GetIntegerValue("SystemComponent"); err == nil && val == 1 {
				subkey.Close()
				continue
			}

			publisher, _, _ := subkey.GetStringValue("Publisher")
			version, _, _ := subkey.GetStringValue("DisplayVersion")
			installDate, _, _ := subkey.GetStringValue("InstallDate")
			quietUninstall, _, _ := subkey.GetStringValue("QuietUninstallString")

			var estimatedSize int64
			if val, _, err := subkey.GetIntegerValue("EstimatedSize"); err == nil {
				estimatedSize = int64(val)
			}

			items = append(items, SoftwareItem{
				Name:                 displayName,
				Publisher:            publisher,
				Version:              version,
				InstallDate:          installDate,
				EstimatedSizeKB:      estimatedSize,
				QuietUninstallString: quietUninstall,
			})

			subkey.Close()
		}
	}

	return items
}

type PatchItem struct {
	KbID        string `json:"kbId"`
	Name        string `json:"name"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	Installed   bool   `json:"installed"`
	InstalledAt string `json:"installedAt"`
}

func scanInstalledPatches() []PatchItem {
	var items []PatchItem

	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		log.Printf("COM CoInitializeEx failed: %v", err)
		return items
	}
	defer ole.CoUninitialize()

	unknown, err := oleutil.CreateObject("Microsoft.Update.Session")
	if err != nil {
		log.Printf("CreateObject Microsoft.Update.Session failed: %v", err)
		return items
	}
	defer unknown.Release()

	session, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		log.Printf("QueryInterface failed: %v", err)
		return items
	}
	defer session.Release()

	searcherDisp, err := oleutil.CallMethod(session, "CreateUpdateSearcher")
	if err != nil || searcherDisp == nil {
		log.Printf("CreateUpdateSearcher failed: %v", err)
		return items
	}
	searcher := searcherDisp.ToIDispatch()
	defer searcher.Release()

	historyDisp, err := oleutil.CallMethod(searcher, "QueryHistory", 0, 50)
	if err != nil || historyDisp == nil {
		log.Printf("QueryHistory failed: %v", err)
		return items
	}
	history := historyDisp.ToIDispatch()
	if history == nil {
		return items
	}
	defer history.Release()

	countProp, err := history.GetProperty("Count")
	if err != nil || countProp == nil {
		return items
	}
	count := int(countProp.Val)

	for i := 0; i < count; i++ {
		itemVal, err := oleutil.GetProperty(history, "Item", i)
		if err != nil || itemVal == nil {
			continue
		}
		item := itemVal.ToIDispatch()
		if item == nil {
			continue
		}

		titleProp, _ := item.GetProperty("Title")
		dateProp, _ := item.GetProperty("Date")
		resultProp, _ := item.GetProperty("ResultCode")

		title := ""
		if titleProp != nil && titleProp.VT == ole.VT_BSTR {
			title = titleProp.ToString()
		}

		// KBArticleIDs is not always available via IDispatch.
		// Fallback: extract KB number from title via regex.
		kbs := ""
		if kbProp, err := item.GetProperty("KBArticleIDs"); err == nil && kbProp != nil && kbProp.Value() != nil {
			kbs = fmt.Sprintf("%v", kbProp.Value())
		}
		if kbs == "" {
			if matches := kbRegex.FindStringSubmatch(title); len(matches) > 1 {
				kbs = matches[1]
			}
		}

		// Skip entries without KB ID — these are driver/firmware/Store updates,
		// not traditional Windows patches. They lack KB identifiers and would
		// appear as orphaned entries in patch management.
		if kbs == "" {
			log.Printf("patches: skipped entry without KB: %s", title)
			item.Release()
			continue
		}

		dateStr := ""
		if dateProp != nil && dateProp.VT == ole.VT_DATE {
			dateStr = dateProp.ToString()
		}

		resultCode := int64(0)
		if resultProp != nil {
			resultCode = resultProp.Val
		}

		installed := resultCode == 2

		items = append(items, PatchItem{
			KbID:        kbs,
			Name:        title,
			Installed:   installed,
			InstalledAt: dateStr,
		})

		item.Release()
	}

	return items
}

type CheckConfig struct {
	CheckID  int64                  `json:"checkId"`
	Type     string                 `json:"type"`
	Config   map[string]interface{} `json:"config"`
}

func executeCheck(conn *websocket.Conn, payload string) {
	var config CheckConfig
	if err := json.Unmarshal([]byte(payload), &config); err != nil {
		log.Printf("Failed to parse check config: %v", err)
		return
	}

	timeoutSec := 30
	if t, ok := config.Config["timeout"].(float64); ok && t > 0 {
		timeoutSec = int(t)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	var status, output string

	switch config.Type {
	case "disk_space":
		threshold := 90
		if t, ok := config.Config["threshold"].(float64); ok {
			threshold = int(t)
		}
		cmd := exec.CommandContext(ctx, "wmic", "logicaldisk", "where", "Size>0", "get", "DeviceID,FreeSpace,Size", "/format:list")
		out, err := cmd.CombinedOutput()
		if err != nil {
			status = "error"
			output = fmt.Sprintf("Command failed: %v", err)
		} else {
			output = string(out)
			if strings.Contains(output, "Error") {
				status = "error"
			} else {
				status = "pass"
				_ = threshold
			}
		}

	case "cpu_load":
		threshold := 80
		if t, ok := config.Config["threshold"].(float64); ok {
			threshold = int(t)
		}
		cmd := exec.CommandContext(ctx, "wmic", "cpu", "get", "LoadPercentage", "/format:list")
		out, err := cmd.CombinedOutput()
		if err != nil {
			status = "error"
			output = fmt.Sprintf("Command failed: %v", err)
		} else {
			output = string(out)
			var load float64
			fmt.Sscanf(output, "LoadPercentage=%f", &load)
			if int(load) > threshold {
				status = "fail"
				output = fmt.Sprintf("CPU load %.0f%% exceeds threshold %d%%", load, threshold)
			} else {
				status = "pass"
				output = fmt.Sprintf("CPU load %.0f%% within threshold %d%%", load, threshold)
			}
		}

	case "ping":
		host := "8.8.8.8"
		if h, ok := config.Config["host"].(string); ok && h != "" {
			host = h
		}
		cmd := exec.CommandContext(ctx, "ping", "-n", "3", host)
		out, err := cmd.CombinedOutput()
		if err != nil {
			status = "fail"
			output = fmt.Sprintf("Ping to %s failed: %s", host, string(out))
		} else {
			status = "pass"
			output = fmt.Sprintf("Ping to %s successful", host)
		}

	default:
		status = "error"
		output = fmt.Sprintf("Unknown check type: %s", config.Type)
	}

	result := map[string]interface{}{
		"checkId": config.CheckID,
		"status":  status,
		"output":  output,
	}
	resultBytes, _ := json.Marshal(result)
	sendWSMsg(conn, "check_result", string(resultBytes))
}

// ─── Backup Sidecar (Kopia) ──────────────────────────────────────────────────

type BackupJobConfig struct {
	JobID       int64    `json:"jobId"`
	SourcePaths []string `json:"sourcePaths"`
	JobType     string   `json:"jobType"`
	RepoURL     string   `json:"repoUrl"`
	Password    string   `json:"password"`
	TimeoutMin  int      `json:"timeoutMin"`
}

func executeBackupSidecar(conn *websocket.Conn, payload string) {
	var job BackupJobConfig
	if err := json.Unmarshal([]byte(payload), &job); err != nil {
		sendWSMsg(conn, "backup_status", fmt.Sprintf("ERROR: Invalid backup payload: %v", err))
		return
	}

	if job.TimeoutMin <= 0 {
		job.TimeoutMin = 60
	}

	sendWSMsg(conn, "backup_status", "Initializing backup engine...")

	// Find kopia binary
	kopiaPath := findKopiaBinary()
	if kopiaPath == "" {
		sendWSMsg(conn, "backup_status", "ERROR: Kopia binary not found. Install Kopia or place kopia.exe in agent directory.")
		return
	}

	sendWSMsg(conn, "backup_status", fmt.Sprintf("Using backup engine: %s", kopiaPath))

	// Connect to repository
	if job.RepoURL != "" {
		sendWSMsg(conn, "backup_status", fmt.Sprintf("Connecting to repository: %s", job.RepoURL))
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		connectArgs := []string{
			"repository", "connect",
			job.RepoURL,
			"--json",
		}
		cmd := exec.CommandContext(ctx, kopiaPath, connectArgs...)
		if job.Password != "" {
			cmd.Env = append(cmd.Environ(), "KOPIA_PASSWORD="+job.Password)
		}

		var errBuf bytes.Buffer
		cmd.Stderr = &errBuf
		if err := cmd.Run(); err != nil {
			sendWSMsg(conn, "backup_status", fmt.Sprintf("ERROR: Failed to connect to repository: %v | %s", err, errBuf.String()))
			resultPayload, _ := json.Marshal(map[string]interface{}{
				"jobId":  job.JobID,
				"status": "failed",
				"error":  errBuf.String(),
			})
			sendWSMsg(conn, "backup_result", string(resultPayload))
			return
		}
		sendWSMsg(conn, "backup_status", "Repository connected successfully.")
	}

	// Execute snapshot
	sendWSMsg(conn, "backup_status", "Starting snapshot...")
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(job.TimeoutMin)*time.Minute)
	defer cancel()

	args := []string{"snapshot", "create", "--json"}
	args = append(args, job.SourcePaths...)

	cmd := exec.CommandContext(ctx, kopiaPath, args...)
	if job.Password != "" {
		cmd.Env = append(cmd.Environ(), "KOPIA_PASSWORD="+job.Password)
	}

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()

	if ctx.Err() == context.DeadlineExceeded {
		sendWSMsg(conn, "backup_status", fmt.Sprintf("ERROR: Backup timed out after %d minutes", job.TimeoutMin))
		resultPayload, _ := json.Marshal(map[string]interface{}{
			"jobId":  job.JobID,
			"status": "failed",
			"error":  "backup timed out",
		})
		sendWSMsg(conn, "backup_result", string(resultPayload))
		return
	}

	if err != nil {
		sendWSMsg(conn, "backup_status", fmt.Sprintf("ERROR: Backup failed: %v | %s", err, errBuf.String()))
		resultPayload, _ := json.Marshal(map[string]interface{}{
			"jobId":  job.JobID,
			"status": "failed",
			"error":  errBuf.String(),
		})
		sendWSMsg(conn, "backup_result", string(resultPayload))
		return
	}

	// Parse Kopia JSON output for snapshot info
	snapshotID, sizeBytes := parseKopiaResult(outBuf.Bytes())
	snapshotInfo := parseKopiaOutput(outBuf.Bytes())
	if snapshotInfo != "" {
		sendWSMsg(conn, "backup_status", fmt.Sprintf("SUCCESS: %s", snapshotInfo))
	} else {
		sendWSMsg(conn, "backup_status", "SUCCESS: Backup completed. Snapshot created.")
	}

	resultPayload, _ := json.Marshal(map[string]interface{}{
		"jobId":      job.JobID,
		"status":     "completed",
		"sizeBytes":  sizeBytes,
		"snapshotId": snapshotID,
		"error":      "",
	})
	sendWSMsg(conn, "backup_result", string(resultPayload))
}

// ─── Snapshot Listing ─────────────────────────────────────────────────────────

type SnapshotListReq struct {
	RepoURL    string `json:"repoUrl"`
	Password   string `json:"password"`
	TimeoutMin int    `json:"timeoutMin"`
}

func listSnapshotsSidecar(conn *websocket.Conn, payload string) {
	var req SnapshotListReq
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		sendWSMsg(conn, "snapshot_list", fmt.Sprintf(`{"error":"invalid payload: %v"}`, err))
		return
	}
	if req.TimeoutMin <= 0 {
		req.TimeoutMin = 2
	}

	sendWSMsg(conn, "restore_status", "Finding backup engine...")
	kopiaPath := findKopiaBinary()
	if kopiaPath == "" {
		sendWSMsg(conn, "snapshot_list", `{"error":"Kopia binary not found"}`)
		return
	}

	if req.RepoURL != "" {
		sendWSMsg(conn, "restore_status", "Connecting to repository...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		connectArgs := []string{"repository", "connect", req.RepoURL, "--json"}
		cmd := exec.CommandContext(ctx, kopiaPath, connectArgs...)
		if req.Password != "" {
			cmd.Env = append(cmd.Environ(), "KOPIA_PASSWORD="+req.Password)
		}
		var errBuf bytes.Buffer
		cmd.Stderr = &errBuf
		if err := cmd.Run(); err != nil {
			sendWSMsg(conn, "snapshot_list", fmt.Sprintf(`{"error":"repo connect failed: %s"}`, errBuf.String()))
			return
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(req.TimeoutMin)*time.Minute)
	defer cancel()

	args := []string{"snapshot", "list", "--json"}
	cmd := exec.CommandContext(ctx, kopiaPath, args...)
	if req.Password != "" {
		cmd.Env = append(cmd.Environ(), "KOPIA_PASSWORD="+req.Password)
	}

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		sendWSMsg(conn, "snapshot_list", fmt.Sprintf(`{"error":"%s"}`, errBuf.String()))
		return
	}

	sendWSMsg(conn, "snapshot_list", string(outBuf.Bytes()))
}

// ─── Restore ──────────────────────────────────────────────────────────────────

type RestoreReq struct {
	SnapshotID  string `json:"snapshotId"`
	Destination string `json:"destination"`
	RepoURL     string `json:"repoUrl"`
	Password    string `json:"password"`
	TimeoutMin  int    `json:"timeoutMin"`
}

type RestoreProgress struct {
	SnapshotID  string `json:"snapshotId"`
	Destination string `json:"destination"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	Error       string `json:"error,omitempty"`
}

func executeRestoreSidecar(conn *websocket.Conn, payload string) {
	var req RestoreReq
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		sendWSMsg(conn, "restore_result", fmt.Sprintf(`{"error":"invalid payload: %v"}`, err))
		return
	}
	if req.SnapshotID == "" || req.Destination == "" {
		sendWSMsg(conn, "restore_result", `{"error":"snapshotId and destination required"}`)
		return
	}
	if req.TimeoutMin <= 0 {
		req.TimeoutMin = 30
	}

	progress := func(status, msg string) {
		p := RestoreProgress{SnapshotID: req.SnapshotID, Destination: req.Destination, Status: status, Message: msg}
		b, _ := json.Marshal(p)
		sendWSMsg(conn, "restore_status", string(b))
	}

	progress("running", "Finding backup engine...")
	kopiaPath := findKopiaBinary()
	if kopiaPath == "" {
		sendWSMsg(conn, "restore_result", fmt.Sprintf(`{"snapshotId":"%s","status":"failed","error":"Kopia binary not found"}`, req.SnapshotID))
		return
	}

	if req.RepoURL != "" {
		progress("running", "Connecting to repository...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		connectArgs := []string{"repository", "connect", req.RepoURL, "--json"}
		cmd := exec.CommandContext(ctx, kopiaPath, connectArgs...)
		if req.Password != "" {
			cmd.Env = append(cmd.Environ(), "KOPIA_PASSWORD="+req.Password)
		}
		var errBuf bytes.Buffer
		cmd.Stderr = &errBuf
		if err := cmd.Run(); err != nil {
			sendWSMsg(conn, "restore_result", fmt.Sprintf(`{"snapshotId":"%s","status":"failed","error":"repo connect failed: %s"}`, req.SnapshotID, errBuf.String()))
			return
		}
	}

	progress("running", fmt.Sprintf("Restoring snapshot %s to %s...", req.SnapshotID, req.Destination))
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(req.TimeoutMin)*time.Minute)
	defer cancel()

	args := []string{"restore", req.SnapshotID, req.Destination, "--json"}
	cmd := exec.CommandContext(ctx, kopiaPath, args...)
	if req.Password != "" {
		cmd.Env = append(cmd.Environ(), "KOPIA_PASSWORD="+req.Password)
	}

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		sendWSMsg(conn, "restore_result", fmt.Sprintf(`{"snapshotId":"%s","status":"failed","error":"restore timed out after %d minutes"}`, req.SnapshotID, req.TimeoutMin))
		return
	}
	if err != nil {
		sendWSMsg(conn, "restore_result", fmt.Sprintf(`{"snapshotId":"%s","status":"failed","error":"%s"}`, req.SnapshotID, errBuf.String()))
		return
	}

	progress("completed", fmt.Sprintf("Restore completed to %s", req.Destination))
	sendWSMsg(conn, "restore_result", fmt.Sprintf(`{"snapshotId":"%s","status":"completed","destination":"%s"}`, req.SnapshotID, req.Destination))
}

func findKopiaBinary() string {
	// Check in order: same directory as agent, ProgramData, PATH
	candidates := []string{
		"kopia.exe",
		"C:\\ProgramData\\OzyShield\\kopia.exe",
		"kopia",
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Try to find in PATH
	if path, err := exec.LookPath("kopia"); err == nil {
		return path
	}

	return ""
}

func parseKopiaResult(data []byte) (snapshotID string, sizeBytes int64) {
	var result struct {
		ID        string `json:"id"`
		RootEntry struct {
			Summary struct {
				TotalFileSize int64 `json:"size"`
			} `json:"summ"`
		} `json:"rootEntry"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", 0
	}
	return result.ID, result.RootEntry.Summary.TotalFileSize
}

func parseKopiaOutput(data []byte) string {
	var result struct {
		ID        string `json:"id"`
		RootEntry struct {
			Summary struct {
				TotalFileSize int64 `json:"size"`
			} `json:"summ"`
		} `json:"rootEntry"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return ""
	}

	if result.ID != "" {
		return fmt.Sprintf("Snapshot %s created (%d bytes)", result.ID, result.RootEntry.Summary.TotalFileSize)
	}
	return ""
}

// ─── Software Uninstall ──────────────────────────────────────────────────────

type UninstallRequest struct {
	SoftwareID      string `json:"softwareId"`
	SoftwareName    string `json:"softwareName"`
	UninstallString string `json:"uninstallString"`
}

func executeSoftwareUninstall(conn *websocket.Conn, payload string) {
	var req UninstallRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		sendWSMsg(conn, "software_uninstall_result", fmt.Sprintf(`{"error":"invalid payload: %v"}`, err))
		return
	}

	log.Printf("Uninstalling software: %s (ID: %s)", req.SoftwareName, req.SoftwareID)
	sendWSMsg(conn, "software_uninstall_result", fmt.Sprintf(`{"status":"started","softwareId":"%s","name":"%s"}`, req.SoftwareID, req.SoftwareName))

	// Execute the uninstall command
	// Use cmd.exe /c to run the uninstall string (handles msiexec, executables, etc.)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "cmd.exe", "/c", req.UninstallString)
	output, err := cmd.CombinedOutput()

	if err != nil {
		log.Printf("Uninstall failed for %s: %v", req.SoftwareName, err)
		sendWSMsg(conn, "software_uninstall_result", fmt.Sprintf(`{"status":"failed","softwareId":"%s","name":"%s","error":"%s","output":"%s"}`,
			req.SoftwareID, req.SoftwareName, err.Error(), string(output)))
		return
	}

	log.Printf("Uninstall completed for %s", req.SoftwareName)
	sendWSMsg(conn, "software_uninstall_result", fmt.Sprintf(`{"status":"completed","softwareId":"%s","name":"%s","output":"%s"}`,
		req.SoftwareID, req.SoftwareName, string(output)))
}

func executeScript(conn *websocket.Conn, payload string) {
	var req struct {
		ScriptID       int64  `json:"scriptId"`
		ExecutionID    int64  `json:"executionId"`
		Command        string `json:"command"`
		Language       string `json:"language"`
		TimeoutSeconds int    `json:"timeoutSeconds"`
		MaxOutputBytes int    `json:"maxOutputBytes"`
	}
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		sendWSMsg(conn, "script_result", fmt.Sprintf(`{"executionId":0,"status":"failed","error":"invalid payload: %v"}`, err))
		return
	}
	log.Printf("Executing script %d (execution %d, language=%s)", req.ScriptID, req.ExecutionID, req.Language)

	timeout := 300
	if req.TimeoutSeconds > 0 {
		timeout = req.TimeoutSeconds
	}
	maxOutput := 65536
	if req.MaxOutputBytes > 0 {
		maxOutput = req.MaxOutputBytes
	}

	shell := "powershell.exe"
	shellArg := "-Command"
	if req.Language == "batch" {
		shell = "cmd.exe"
		shellArg = "/C"
	} else if req.Language == "sh" && runtime.GOOS != "windows" {
		shell = "sh"
		shellArg = "-c"
	}

	// Force Start-Process to use CreateProcess (not ShellExecuteEx)
	// so child processes are created inside the Job Object.
	command := req.Command
	if shell == "powershell.exe" {
		command = "$PSDefaultParameterValues['Start-Process:UseShellExecute']=$false; " + command
	}

	logID := fmt.Sprintf("script_%d_exec_%d", req.ScriptID, req.ExecutionID)

	cmd := exec.Command(shell, shellArg, command)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	job := setupJobObject(cmd)
	if job != 0 {
		log.Printf("script_exec: job created for %s", logID)
	}

	start := time.Now()
	err := cmd.Start()
	if err == nil && job != 0 {
		if attachJob(job, cmd.Process.Pid) {
			log.Printf("script_exec: process %d assigned to job for %s", cmd.Process.Pid, logID)
		}
	}

	var durationMs int
	var waitErr error
	if err == nil {
		var timedOut bool
		durationMs, timedOut, waitErr = waitAndKillOnTimeout(cmd, job, time.Duration(timeout)*time.Second, logID)
		if timedOut {
			sendWSMsg(conn, "script_result", fmt.Sprintf(`{"executionId":%d,"status":"timeout","exitCode":-1,"output":"","outputTruncated":false,"durationMs":%d,"error":"execution timed out after %ds"}`,
				req.ExecutionID, durationMs, timeout))
			log.Printf("script_exec: %s timed out after %dms", logID, durationMs)
			return
		}
	} else {
		durationMs = int(time.Since(start).Milliseconds())
		sendWSMsg(conn, "script_result", fmt.Sprintf(`{"executionId":%d,"status":"failed","exitCode":1,"output":"","outputTruncated":false,"durationMs":%d,"error":"start failed: %v"}`,
			req.ExecutionID, durationMs, err))
		return
	}

	output := stdoutBuf.String()
	if stderrBuf.Len() > 0 {
		if output != "" {
			output += "\n--- STDERR ---\n"
		}
		output += stderrBuf.String()
	}

	outputTruncated := len(output) > maxOutput
	if outputTruncated {
		output = output[:maxOutput] + "\n... [truncated]"
	}

	exitCode := 0
	if waitErr != nil {
		exitCode = 1
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	status := "completed"
	if exitCode != 0 {
		status = "failed"
	}

	resultPayload, _ := json.Marshal(map[string]interface{}{
		"executionId":     req.ExecutionID,
		"status":          status,
		"exitCode":        exitCode,
		"output":          output,
		"outputTruncated": outputTruncated,
		"durationMs":      durationMs,
		"error":           "",
	})
	sendWSMsg(conn, "script_result", string(resultPayload))
	log.Printf("Script %d execution %d completed: status=%s, exitCode=%d, duration=%dms, truncated=%v",
		req.ScriptID, req.ExecutionID, status, exitCode, durationMs, outputTruncated)
}

// setupJobObject creates a Windows Job Object, sets KILL_ON_JOB_CLOSE,
// and configures the command to start suspended (so the process can be
// assigned to the job before its first instruction runs).
// Returns 0 on non-Windows or if creation fails.
func setupJobObject(cmd *exec.Cmd) windows.Handle {
	if runtime.GOOS != "windows" {
		return 0
	}
	job, _ := windows.CreateJobObject(nil, nil)
	if job == 0 {
		return 0
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000004, // CREATE_SUSPENDED
	}
	return job
}

// attachJob assigns pid to the Job Object and resumes its main thread.
// Must be called immediately after cmd.Start().
func attachJob(job windows.Handle, pid int) bool {
	if job == 0 || runtime.GOOS != "windows" {
		return false
	}
	processHandle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_CREATE_PROCESS,
		false, uint32(pid))
	if err != nil {
		log.Printf("job: OpenProcess(%d) failed: %v", pid, err)
		return false
	}
	defer windows.CloseHandle(processHandle)
	if err := windows.AssignProcessToJobObject(job, processHandle); err != nil {
		log.Printf("job: AssignProcessToJobObject(%d) failed: %v", pid, err)
		return false
	}
	resumeMainThread(pid)
	return true
}

// waitAndKillOnTimeout waits for cmd to finish. If timeout elapses first,
// it force-kills the main process and, if the main process doesn't die
// within the drain window, terminates the entire Job Object tree.
func waitAndKillOnTimeout(cmd *exec.Cmd, job windows.Handle, timeout time.Duration, logID string) (durationMs int, timedOut bool, waitErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case waitErr = <-done:
	case <-ctx.Done():
		log.Printf("job: timeout for %s, force-killing", logID)
		cmd.Process.Kill()
		select {
		case waitErr = <-done:
		case <-time.After(3 * time.Second):
			log.Printf("job: drain timeout for %s, force-killing job tree", logID)
		}
		timedOut = true
		if job != 0 {
			windows.TerminateJobObject(job, 1)
			windows.CloseHandle(job)
		}
	}
	return int(time.Since(start).Milliseconds()), timedOut, waitErr
}

func resumeMainThread(pid int) {
	if runtime.GOOS != "windows" {
		return
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		log.Printf("script_exec: CreateToolhelp32Snapshot failed: %v", err)
		return
	}
	defer windows.CloseHandle(snapshot)

	var te windows.ThreadEntry32
	te.Size = uint32(unsafe.Sizeof(te))
	for err = windows.Thread32First(snapshot, &te); err == nil; err = windows.Thread32Next(snapshot, &te) {
		if te.OwnerProcessID == uint32(pid) {
			hThread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, te.ThreadID)
			if err != nil {
				log.Printf("script_exec: OpenThread(%d) failed: %v", te.ThreadID, err)
				return
			}
			if _, err := windows.ResumeThread(hThread); err != nil {
				log.Printf("script_exec: ResumeThread failed: %v", err)
			}
			windows.CloseHandle(hThread)
			return
		}
	}
	log.Printf("script_exec: no thread found for pid %d", pid)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
