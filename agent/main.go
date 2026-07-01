package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yusufpapurcu/wmi"
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

func main() {
	hostname, _ := os.Hostname()
	log.Printf("Starting agent on %s (%s)", hostname, runtime.GOOS)

	// Keep trying to connect to the backend WebSocket server
	for {
		u := url.URL{Scheme: "ws", Host: backendAddr, Path: "/agent/connect", RawQuery: "id=" + agentID}
		log.Printf("Connecting to %s", u.String())

		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			log.Printf("Dial failed: %v. Retrying in 5 seconds...", err)
			time.Sleep(5 * time.Second)
			continue
		}

		log.Println("Connected to Backend successfully.")
		handleConnection(conn)
		conn.Close()
		log.Println("Disconnected from Backend. Retrying in 5 seconds...")
		time.Sleep(5 * time.Second)
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
				_ = conn.WriteMessage(websocket.TextMessage, msgBytes)
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

func runBackupSidecar(conn *websocket.Conn, _ string) {
	// A dummy backup execution engine mapping to Phase 4 sidecar CLI
	// (Simulates invoking kopia/restic CLI)
	msg := Message{
		AgentID: agentID,
		Type:    "backup_status",
		Payload: "Initializing backup snapshot (using sidecar)...",
	}
	msgBytes, _ := json.Marshal(msg)
	_ = conn.WriteMessage(websocket.TextMessage, msgBytes)

	// Simulate steps
	for pct := 10; pct <= 100; pct += 30 {
		time.Sleep(1 * time.Second)
		statusMsg := Message{
			AgentID: agentID,
			Type:    "backup_status",
			Payload: fmt.Sprintf("Processing block storage backup: %d%%", pct),
		}
		statusBytes, _ := json.Marshal(statusMsg)
		_ = conn.WriteMessage(websocket.TextMessage, statusBytes)
	}

	completeMsg := Message{
		AgentID: agentID,
		Type:    "backup_status",
		Payload: "Backup complete. Snapshot hash created: rmm_restic_73a21bc9",
	}
	completeBytes, _ := json.Marshal(completeMsg)
	_ = conn.WriteMessage(websocket.TextMessage, completeBytes)
}
