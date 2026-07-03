package main

import (
	"bytes"
	"context"
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
	"time"

	"database/sql"
	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"github.com/gorilla/websocket"
	"github.com/yusufpapurcu/wmi"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
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
var backendAddr = "localhost:8080"
var agentVersion = "1.2.0"

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

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	if envID := os.Getenv("AGENT_ID"); envID != "" {
		agentID = envID
	}
	if envBackend := os.Getenv("BACKEND_URL"); envBackend != "" {
		backendAddr = envBackend
	}
	if envVersion := os.Getenv("AGENT_VERSION"); envVersion != "" {
		agentVersion = envVersion
	}

	hostname, _ := os.Hostname()
	log.Printf("Starting agent v%s on %s (%s) [ID: %s, Backend: %s]", AGENT_VERSION, hostname, runtime.GOOS, agentID, backendAddr)

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

			if stdin != nil {
				_, _ = stdin.Write([]byte(msg.Payload))
			}

		case "backup_command":
			log.Printf("Received backup command: %s", msg.Payload)
			go executeBackupSidecar(conn, msg.Payload)

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
	JobID       string   `json:"job_id"`
	SourcePaths []string `json:"source_paths"`
	RepoURL     string   `json:"repo_url"`
	Password    string   `json:"password"`
	TimeoutMin  int      `json:"timeout_min"`
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
		cmd.Env = append(cmd.Environ(), "KOPIA_PASSWORD="+job.Password)

		var errBuf bytes.Buffer
		cmd.Stderr = &errBuf
		if err := cmd.Run(); err != nil {
			sendWSMsg(conn, "backup_status", fmt.Sprintf("ERROR: Failed to connect to repository: %v | %s", err, errBuf.String()))
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
	cmd.Env = append(cmd.Environ(), "KOPIA_PASSWORD="+job.Password)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()

	if ctx.Err() == context.DeadlineExceeded {
		sendWSMsg(conn, "backup_status", fmt.Sprintf("ERROR: Backup timed out after %d minutes", job.TimeoutMin))
		return
	}

	if err != nil {
		sendWSMsg(conn, "backup_status", fmt.Sprintf("ERROR: Backup failed: %v | %s", err, errBuf.String()))
		return
	}

	// Parse Kopia JSON output for snapshot info
	snapshotInfo := parseKopiaOutput(outBuf.Bytes())
	if snapshotInfo != "" {
		sendWSMsg(conn, "backup_status", fmt.Sprintf("SUCCESS: %s", snapshotInfo))
	} else {
		sendWSMsg(conn, "backup_status", "SUCCESS: Backup completed. Snapshot created.")
	}
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

func parseKopiaOutput(data []byte) string {
	var result struct {
		SnapshotID string `json:"snapshotId"`
		Hash       string `json:"hash"`
		RootID     string `json:"rootId"`
		Size       struct {
			TotalBytes int64 `json:"totalBytes"`
		} `json:"size"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return ""
	}

	if result.Hash != "" {
		return fmt.Sprintf("Snapshot %s created (hash: %s)", result.SnapshotID, result.Hash[:min(12, len(result.Hash))])
	}
	return ""
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
