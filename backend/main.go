package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

// ─── Configuration ────────────────────────────────────────────────────────────

type Config struct {
	JWTSecret        []byte
	CORSOrigins      []string
	DatabaseURL      string
	AdminEmail       string
	AdminPassword    string
	AgentEnrollSecret string
	LogLevel         string
	Port             string
}

func loadConfig() Config {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable is required")
	}

	corsOrigins := strings.Split(os.Getenv("CORS_ORIGINS"), ",")
	if len(corsOrigins) == 0 || corsOrigins[0] == "" {
		corsOrigins = []string{"http://localhost:3000"}
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://apexrmm:apexrmm_secret@localhost:5432/apexrmm?sslmode=disable"
	}

	adminEmail := os.Getenv("ADMIN_EMAIL")
	if adminEmail == "" {
		adminEmail = "admin@apexrmm.com"
	}

	adminPass := os.Getenv("ADMIN_PASSWORD")
	if adminPass == "" {
		adminPass = "password123"
	}

	enrollSecret := os.Getenv("AGENT_ENROLL_SECRET")
	if enrollSecret == "" {
		log.Fatal("AGENT_ENROLL_SECRET environment variable is required")
	}

	return Config{
		JWTSecret:         []byte(jwtSecret),
		CORSOrigins:       corsOrigins,
		DatabaseURL:       dbURL,
		AdminEmail:        adminEmail,
		AdminPassword:     adminPass,
		AgentEnrollSecret: enrollSecret,
		LogLevel:          os.Getenv("LOG_LEVEL"),
		Port:              ":8080",
	}
}

// ─── Globals ──────────────────────────────────────────────────────────────────

var (
	cfg Config
	db  *sql.DB
)

// ─── WebSocket Upgrader ──────────────────────────────────────────────────────

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		for _, allowed := range cfg.CORSOrigins {
			if strings.TrimSpace(allowed) == origin {
				return true
			}
		}
		return false
	},
}

// ─── Message Structs ─────────────────────────────────────────────────────────

type Message struct {
	AgentID string `json:"agentId"`
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

type TelemetryPayload struct {
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

type AgentInfo struct {
	ID            string          `json:"id"`
	TenantID      string          `json:"tenantId"`
	Hostname      string          `json:"hostname"`
	OS            string          `json:"os"`
	CPUModel      string          `json:"cpuModel"`
	CPULoad       float64         `json:"cpuLoad"`
	TotalRAM      uint64          `json:"totalRam"`
	FreeRAM       uint64          `json:"freeRam"`
	DiskTotal     uint64          `json:"diskTotal"`
	DiskFree      uint64          `json:"diskFree"`
	Disks         []DiskPartition `json:"disks"`
	Status        string          `json:"status"`
	LastSeen      string          `json:"lastSeen"`
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

type Claims struct {
	Username string `json:"username"`
	UserID   string `json:"userId"`
	TenantID string `json:"tenantId"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// ─── In-memory agent connections ─────────────────────────────────────────────

type AgentConnection struct {
	ID       string
	TenantID string
	Conn     *websocket.Conn
	Mu       sync.Mutex
}

var (
	agentsMu sync.Mutex
	agents   = make(map[string]*AgentConnection)
)

// ─── In-memory frontend sessions ─────────────────────────────────────────────

type FrontendSession struct {
	Conn     *websocket.Conn
	TenantID string
}

var (
	frontendsMu sync.Mutex
	frontends   = make(map[string]*FrontendSession)
)

// ─── Rate Limiting ────────────────────────────────────────────────────────────

type LoginAttempt struct {
	Count     int
	LockedAt  time.Time
}

var (
	rateLimitMu sync.Mutex
	rateLimit   = make(map[string]*LoginAttempt)
)

func checkRateLimit(ip string) bool {
	rateLimitMu.Lock()
	defer rateLimitMu.Unlock()

	attempt, exists := rateLimit[ip]
	if !exists {
		return true
	}

	if attempt.Count >= 5 && time.Since(attempt.LockedAt) < 15*time.Minute {
		return false
	}

	if time.Since(attempt.LockedAt) >= 15*time.Minute {
		delete(rateLimit, ip)
		return true
	}

	return true
}

func recordLoginAttempt(ip string) {
	rateLimitMu.Lock()
	defer rateLimitMu.Unlock()

	attempt, exists := rateLimit[ip]
	if !exists {
		rateLimit[ip] = &LoginAttempt{Count: 1, LockedAt: time.Now()}
		return
	}

	attempt.Count++
	if attempt.Count >= 5 {
		attempt.LockedAt = time.Now()
	}
}

// ─── Database ─────────────────────────────────────────────────────────────────

func initDB() {
	var err error
	db, err = sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Cannot connect to database: %v", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Cannot ping database: %v", err)
	}

	runMigrations()
	seedAdminUser()

	log.Println("Database initialized successfully")
}

func runMigrations() {
	migration := `
	CREATE TABLE IF NOT EXISTS tenants (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		name VARCHAR(255) NOT NULL,
		slug VARCHAR(100) UNIQUE NOT NULL,
		plan VARCHAR(50) DEFAULT 'standard',
		max_agents INTEGER DEFAULT 100,
		created_at TIMESTAMPTZ DEFAULT NOW(),
		updated_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS users (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
		email VARCHAR(255) UNIQUE NOT NULL,
		username VARCHAR(100) UNIQUE NOT NULL,
		password_hash VARCHAR(255) NOT NULL,
		full_name VARCHAR(255),
		role VARCHAR(50) NOT NULL DEFAULT 'technician',
		is_active BOOLEAN DEFAULT true,
		last_login TIMESTAMPTZ,
		failed_attempts INTEGER DEFAULT 0,
		locked_until TIMESTAMPTZ,
		created_at TIMESTAMPTZ DEFAULT NOW(),
		updated_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS registration_tokens (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
		token_hash VARCHAR(255) NOT NULL,
		label VARCHAR(255),
		created_by UUID REFERENCES users(id),
		expires_at TIMESTAMPTZ NOT NULL,
		used_at TIMESTAMPTZ,
		max_uses INTEGER DEFAULT 1,
		use_count INTEGER DEFAULT 0,
		is_revoked BOOLEAN DEFAULT false,
		created_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS agents (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
		enrollment_token_id UUID REFERENCES registration_tokens(id),
		hostname VARCHAR(255),
		os VARCHAR(255),
		cpu_model VARCHAR(255),
		cpu_load REAL DEFAULT 0,
		total_ram BIGINT DEFAULT 0,
		free_ram BIGINT DEFAULT 0,
		disk_total BIGINT DEFAULT 0,
		disk_free BIGINT DEFAULT 0,
		disks JSONB DEFAULT '[]',
		status VARCHAR(50) DEFAULT 'offline',
		agent_version VARCHAR(50),
		vendor VARCHAR(255),
		model VARCHAR(255),
		serial_number VARCHAR(255),
		uptime VARCHAR(100),
		kernel_version VARCHAR(255),
		local_ip VARCHAR(50),
		mac_address VARCHAR(50),
		gateway VARCHAR(50),
		num_cpu INTEGER DEFAULT 0,
		gpu_name VARCHAR(255),
		gpu_driver VARCHAR(100),
		last_seen TIMESTAMPTZ,
		enrolled_at TIMESTAMPTZ DEFAULT NOW(),
		created_at TIMESTAMPTZ DEFAULT NOW(),
		updated_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS telemetry (
		id BIGSERIAL PRIMARY KEY,
		agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
		cpu_load REAL,
		total_ram BIGINT,
		free_ram BIGINT,
		disk_total BIGINT,
		disk_free BIGINT,
		recorded_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS alerts (
		id BIGSERIAL PRIMARY KEY,
		tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
		agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
		severity VARCHAR(20) NOT NULL,
		message TEXT,
		acknowledged BOOLEAN DEFAULT false,
		acknowledged_by UUID REFERENCES users(id),
		acknowledged_at TIMESTAMPTZ,
		fingerprint VARCHAR(64),
		created_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS backup_jobs (
		id BIGSERIAL PRIMARY KEY,
		tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
		agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
		name VARCHAR(255),
		location VARCHAR(500),
		type VARCHAR(50) DEFAULT 'full',
		status VARCHAR(50) DEFAULT 'pending',
		size_bytes BIGINT DEFAULT 0,
		cron VARCHAR(100) DEFAULT '0 2 * * *',
		executed_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS audit_log (
		id BIGSERIAL PRIMARY KEY,
		tenant_id UUID REFERENCES tenants(id) ON DELETE SET NULL,
		user_id UUID REFERENCES users(id) ON DELETE SET NULL,
		action VARCHAR(100) NOT NULL,
		resource_type VARCHAR(100),
		resource_id VARCHAR(255),
		details JSONB,
		ip_address INET,
		created_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS agent_software (
		id BIGSERIAL PRIMARY KEY,
		tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
		agent_id UUID REFERENCES agents(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		publisher TEXT,
		version TEXT,
		install_date TEXT,
		estimated_size_kb BIGINT,
		quiet_uninstall_string TEXT,
		scanned_at TIMESTAMPTZ DEFAULT NOW(),
		created_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS agent_notes (
		id BIGSERIAL PRIMARY KEY,
		tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
		agent_id UUID REFERENCES agents(id) ON DELETE CASCADE,
		user_id UUID REFERENCES users(id) ON DELETE SET NULL,
		content TEXT NOT NULL,
		created_at TIMESTAMPTZ DEFAULT NOW(),
		updated_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS agent_logs (
		id BIGSERIAL PRIMARY KEY,
		tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
		agent_id UUID REFERENCES agents(id) ON DELETE CASCADE,
		level VARCHAR(20) NOT NULL DEFAULT 'info',
		log_type VARCHAR(50) NOT NULL DEFAULT 'agent',
		message TEXT NOT NULL,
		created_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS agent_patches (
		id BIGSERIAL PRIMARY KEY,
		tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
		agent_id UUID REFERENCES agents(id) ON DELETE CASCADE,
		kb_id VARCHAR(50),
		name TEXT,
		severity VARCHAR(50),
		description TEXT,
		installed BOOLEAN DEFAULT false,
		installed_at TEXT,
		scanned_at TIMESTAMPTZ DEFAULT NOW(),
		created_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS agent_checks (
		id BIGSERIAL PRIMARY KEY,
		tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
		agent_id UUID REFERENCES agents(id) ON DELETE CASCADE,
		check_type VARCHAR(50) NOT NULL,
		description TEXT,
		config JSONB DEFAULT '{}',
		status VARCHAR(20) DEFAULT 'pending',
		last_output TEXT,
		last_run TIMESTAMPTZ,
		enabled BOOLEAN DEFAULT true,
		created_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_agents_tenant ON agents(tenant_id);
	CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
	CREATE INDEX IF NOT EXISTS idx_telemetry_agent ON telemetry(agent_id);
	CREATE INDEX IF NOT EXISTS idx_telemetry_time ON telemetry(recorded_at);
	CREATE INDEX IF NOT EXISTS idx_alerts_tenant ON alerts(tenant_id);
	CREATE INDEX IF NOT EXISTS idx_alerts_agent ON alerts(agent_id);
	CREATE INDEX IF NOT EXISTS idx_alerts_fingerprint ON alerts(fingerprint);
	CREATE INDEX IF NOT EXISTS idx_alerts_unack ON alerts(tenant_id, acknowledged) WHERE NOT acknowledged;
	CREATE INDEX IF NOT EXISTS idx_backup_jobs_tenant ON backup_jobs(tenant_id);
	CREATE INDEX IF NOT EXISTS idx_backup_jobs_agent ON backup_jobs(agent_id);
	CREATE INDEX IF NOT EXISTS idx_users_tenant ON users(tenant_id);
	CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
	CREATE INDEX IF NOT EXISTS idx_reg_tokens_hash ON registration_tokens(token_hash);
	CREATE INDEX IF NOT EXISTS idx_audit_log_tenant ON audit_log(tenant_id);
	CREATE INDEX IF NOT EXISTS idx_audit_log_time ON audit_log(created_at);
	CREATE INDEX IF NOT EXISTS idx_agent_software_agent ON agent_software(agent_id);
	CREATE INDEX IF NOT EXISTS idx_agent_notes_agent ON agent_notes(agent_id);
	CREATE INDEX IF NOT EXISTS idx_agent_logs_composite ON agent_logs(agent_id, level, created_at);
	CREATE INDEX IF NOT EXISTS idx_audit_log_composite ON audit_log(tenant_id, action, created_at);
	CREATE INDEX IF NOT EXISTS idx_agent_patches_agent ON agent_patches(agent_id);
	CREATE INDEX IF NOT EXISTS idx_agent_checks_agent ON agent_checks(agent_id);
	`

	if _, err := db.Exec(migration); err != nil {
		log.Fatalf("Schema migration failed: %v", err)
	}
}

func seedAdminUser() {
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	if count > 0 {
		return
	}

	// Create default tenant
	var tenantID string
	err := db.QueryRow(`
		INSERT INTO tenants (name, slug) VALUES ('Default Organization', 'default')
		RETURNING id
	`).Scan(&tenantID)
	if err != nil {
		log.Fatalf("Failed to seed default tenant: %v", err)
	}

	// Create admin user
	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("Failed to hash admin password: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO users (tenant_id, email, username, password_hash, full_name, role)
		VALUES (?, ?, 'admin', ?, 'Administrator', 'admin')
	`, tenantID, cfg.AdminEmail, string(hash))
	if err != nil {
		log.Fatalf("Failed to seed admin user: %v", err)
	}

	log.Printf("Default tenant and admin user created (%s)", cfg.AdminEmail)
}

// ─── JWT ──────────────────────────────────────────────────────────────────────

func generateToken(userID, username, tenantID, role string) (string, error) {
	claims := Claims{
		Username: username,
		UserID:   userID,
		TenantID: tenantID,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(cfg.JWTSecret)
}

func parseToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return cfg.JWTSecret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}

// ─── Middleware ────────────────────────────────────────────────────────────────

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := false
		for _, o := range cfg.CORSOrigins {
			if strings.TrimSpace(o) == origin {
				allowed = true
				break
			}
		}

		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

type contextKey string

const (
	claimsKey contextKey = "claims"
	tenantKey contextKey = "tenant_id"
	userKey   contextKey = "user_id"
	roleKey   contextKey = "role"
)

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenStr := ""

		authHeader := r.Header.Get("Authorization")
		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
		}

		if tokenStr == "" {
			tokenStr = r.URL.Query().Get("token")
		}

		if tokenStr == "" {
			http.Error(w, `{"error":"unauthorized: missing token"}`, http.StatusUnauthorized)
			return
		}

		claims, err := parseToken(tokenStr)
		if err != nil {
			http.Error(w, `{"error":"unauthorized: invalid token"}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), claimsKey, claims)
		ctx = context.WithValue(ctx, tenantKey, claims.TenantID)
		ctx = context.WithValue(ctx, userKey, claims.UserID)
		ctx = context.WithValue(ctx, roleKey, claims.Role)
		next(w, r.WithContext(ctx))
	}
}

func requireRole(roles ...string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			role := r.Context().Value(roleKey).(string)
			for _, allowed := range roles {
				if role == allowed {
					next(w, r)
					return
				}
			}
			http.Error(w, `{"error":"forbidden: insufficient permissions"}`, http.StatusForbidden)
		}
	}
}

func getClaims(r *http.Request) *Claims {
	return r.Context().Value(claimsKey).(*Claims)
}

func getTenantID(r *http.Request) string {
	return r.Context().Value(tenantKey).(string)
}

// ─── Audit Logging ────────────────────────────────────────────────────────────

func auditLog(tenantID, userID, action, resourceType, resourceID string, details map[string]interface{}) {
	detailsJSON, _ := json.Marshal(details)
	db.Exec(`
		INSERT INTO audit_log (tenant_id, user_id, action, resource_type, resource_id, details)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, tenantID, userID, action, resourceType, resourceID, detailsJSON)
}

// ─── Enrollment Token Hashing ─────────────────────────────────────────────────

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func generateRandomToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ─── Agent Management ─────────────────────────────────────────────────────────

func upsertAgent(tenantID, agentID string) {
	now := time.Now().UTC()
	_, err := db.Exec(`
		INSERT INTO agents (id, tenant_id, status, last_seen)
		VALUES ($1, $2, 'online', $3)
		ON CONFLICT(id) DO UPDATE SET
			status = 'online',
			last_seen = EXCLUDED.last_seen
	`, agentID, tenantID, now)
	if err != nil {
		log.Printf("upsertAgent error: %v", err)
	}
	seedBackupJob(tenantID, agentID)
}

func markAgentOffline(agentID string) {
	now := time.Now().UTC()
	_, err := db.Exec(`UPDATE agents SET status='offline', last_seen=$1 WHERE id=$2`, now, agentID)
	if err != nil {
		log.Printf("markAgentOffline error: %v", err)
	}
}

func saveTelemetry(agentID string, t TelemetryPayload) {
	now := time.Now().UTC()

	disksJSON, _ := json.Marshal(t.Disks)

	db.Exec(`
		UPDATE agents SET
			hostname = $1, os = $2, cpu_model = $3, cpu_load = $4,
			total_ram = $5, free_ram = $6, disk_total = $7, disk_free = $8,
			disks = $9,
			status = 'online', last_seen = $10, updated_at = $10,
			vendor = $11, model = $12, serial_number = $13, uptime = $14,
			kernel_version = $15, agent_version = $16, local_ip = $17,
			mac_address = $18, gateway = $19, num_cpu = $20,
			gpu_name = $21, gpu_driver = $22
		WHERE id = $23
	`, t.Hostname, t.OS, t.CPUModel, t.CPULoad,
		t.TotalRAM, t.FreeRAM, t.DiskTotal, t.DiskFree, disksJSON, now,
		t.Vendor, t.Model, t.SerialNumber, t.Uptime,
		t.KernelVersion, t.AgentVersion, t.LocalIP,
		t.MACAddress, t.Gateway, t.NumCPU,
		t.GPUName, t.GPUDriver, agentID)

	db.Exec(`
		INSERT INTO telemetry (agent_id, cpu_load, total_ram, free_ram, disk_total, disk_free, recorded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, agentID, t.CPULoad, t.TotalRAM, t.FreeRAM, t.DiskTotal, t.DiskFree, now)

	// Alert deduplication: only create alert if no identical fingerprint in last 10 minutes
	if t.CPULoad >= 90 {
		fingerprint := fmt.Sprintf("cpu-high-%s", agentID)
		var recentCount int
		db.QueryRow(`
			SELECT COUNT(*) FROM alerts
			WHERE fingerprint = $1 AND created_at > NOW() - INTERVAL '10 minutes'
		`, fingerprint).Scan(&recentCount)

		if recentCount == 0 {
			// Get tenant_id from agent
			var tenantID string
			db.QueryRow(`SELECT tenant_id FROM agents WHERE id = $1`, agentID).Scan(&tenantID)
			saveAlert(tenantID, agentID, "warning",
				fmt.Sprintf("CPU usage critical: %.0f%% on %s", t.CPULoad, t.Hostname), fingerprint)
		}
	}
}

func saveAlert(tenantID, agentID, severity, message, fingerprint string) {
	now := time.Now().UTC()
	var id int64
	err := db.QueryRow(`
		INSERT INTO alerts (tenant_id, agent_id, severity, message, fingerprint, created_at)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id
	`, tenantID, agentID, severity, message, fingerprint, now).Scan(&id)
	if err != nil {
		log.Printf("saveAlert error: %v", err)
	}
	broadcastEvent(severity, message, agentID)
}

func seedBackupJob(tenantID, agentID string) {
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM backup_jobs WHERE agent_id = $1`, agentID).Scan(&count)
	if count > 0 {
		return
	}
	now := time.Now().UTC()
	jobs := []struct {
		name, location, typ, cron string
	}{
		{"Nightly-System", "/backups/system", "full", "0 2 * * *"},
		{"Hourly-Delta", "/backups/delta", "incremental", "0 * * * *"},
	}
	for _, j := range jobs {
		db.Exec(`
			INSERT INTO backup_jobs (tenant_id, agent_id, name, location, type, status, cron, created_at)
			VALUES ($1, $2, $3, $4, $5, 'completed', $6, $7)
		`, tenantID, agentID, j.name, j.location, j.typ, j.cron, now)
	}
}

// ─── WebSocket Event Hub ──────────────────────────────────────────────────────

type ClientEventConnection struct {
	Conn *websocket.Conn
}

var (
	eventClients   = make(map[*ClientEventConnection]bool)
	eventClientsMu sync.Mutex
)

func registerEventClient(c *ClientEventConnection) {
	eventClientsMu.Lock()
	defer eventClientsMu.Unlock()
	eventClients[c] = true
}

func unregisterEventClient(c *ClientEventConnection) {
	eventClientsMu.Lock()
	defer eventClientsMu.Unlock()
	delete(eventClients, c)
}

func broadcastEvent(alertType, message, agentID string) {
	eventClientsMu.Lock()
	defer eventClientsMu.Unlock()

	payload := map[string]string{
		"type":    alertType,
		"message": message,
		"agentId": agentID,
		"time":    time.Now().Format(time.RFC3339),
	}
	data, _ := json.Marshal(payload)

	for c := range eventClients {
		_ = c.Conn.WriteMessage(websocket.TextMessage, data)
	}
}

func broadcastToFrontend(agentID string, data []byte) {
	agentsMu.Lock()
	agent, ok := agents[agentID]
	agentsMu.Unlock()
	if !ok {
		return
	}

	frontendsMu.Lock()
	defer frontendsMu.Unlock()

	for id, session := range frontends {
		if session.TenantID != agent.TenantID {
			continue
		}
		if err := session.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("Error sending to frontend %s: %v", id, err)
		}
	}
}

// ─── HTTP Handlers ────────────────────────────────────────────────────────────

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"version": "1.0.0",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	ip := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		ip = strings.Split(fwd, ",")[0]
	}

	if !checkRateLimit(ip) {
		http.Error(w, `{"error":"too many attempts. Try again in 15 minutes."}`, http.StatusTooManyRequests)
		return
	}

	var req struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Support both email and username login
	loginField := req.Username
	if loginField == "" {
		loginField = req.Email
	}

	var hash, userID, tenantID, role string
	err := db.QueryRow(`
		SELECT id, tenant_id, password_hash, role FROM users
		WHERE (username = $1 OR email = $1) AND is_active = true
	`, loginField).Scan(&userID, &tenantID, &hash, &role)
	if err != nil {
		recordLoginAttempt(ip)
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		recordLoginAttempt(ip)
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	// Reset rate limit on success
	rateLimitMu.Lock()
	delete(rateLimit, ip)
	rateLimitMu.Unlock()

	// Update last login
	db.Exec(`UPDATE users SET last_login = NOW(), failed_attempts = 0 WHERE id = $1`, userID)

	// Audit log
	auditLog(tenantID, userID, "login", "user", userID, nil)

	token, err := generateToken(userID, loginField, tenantID, role)
	if err != nil {
		http.Error(w, `{"error":"failed to generate token"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":    token,
		"userId":   userID,
		"tenantId": tenantID,
		"role":     role,
	})
}

func handleListAgents(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r)

	rows, err := db.Query(`
		SELECT id, hostname, os, cpu_model, cpu_load, total_ram, free_ram,
		       disk_total, disk_free, COALESCE(disks, '[]'),
		       status, COALESCE(last_seen::text, ''),
		       COALESCE(vendor,''), COALESCE(model,''), COALESCE(serial_number,''),
		       COALESCE(uptime,''), COALESCE(kernel_version,''), COALESCE(agent_version,''),
		       COALESCE(local_ip,''), COALESCE(mac_address,''), COALESCE(gateway,''),
		       COALESCE(num_cpu, 0),
		       COALESCE(gpu_name,''), COALESCE(gpu_driver,'')
		FROM agents
		WHERE tenant_id = $1
		ORDER BY status DESC, last_seen DESC
	`, tenantID)
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	list := []AgentInfo{}
	for rows.Next() {
		var a AgentInfo
		var disksJSON []byte
		if err := rows.Scan(&a.ID, &a.Hostname, &a.OS, &a.CPUModel,
			&a.CPULoad, &a.TotalRAM, &a.FreeRAM, &a.DiskTotal, &a.DiskFree,
			&disksJSON,
			&a.Status, &a.LastSeen,
			&a.Vendor, &a.Model, &a.SerialNumber,
			&a.Uptime, &a.KernelVersion, &a.AgentVersion,
			&a.LocalIP, &a.MACAddress, &a.Gateway, &a.NumCPU,
			&a.GPUName, &a.GPUDriver); err != nil {
			continue
		}
		json.Unmarshal(disksJSON, &a.Disks)
		a.TenantID = tenantID
		list = append(list, a)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func handleListAlerts(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r)
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}

	agentID := r.URL.Query().Get("agent_id")

	var rows *sql.Rows
	var err error

	if agentID != "" {
		rows, err = db.Query(`
			SELECT id, agent_id, severity, message, COALESCE(created_at::text, '')
			FROM alerts
			WHERE tenant_id = $1 AND agent_id = $2
			ORDER BY id DESC
			LIMIT $3
		`, tenantID, agentID, limit)
	} else {
		rows, err = db.Query(`
			SELECT id, agent_id, severity, message, COALESCE(created_at::text, '')
			FROM alerts
			WHERE tenant_id = $1
			ORDER BY id DESC
			LIMIT $2
		`, tenantID, limit)
	}
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
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

func handleGetAlert(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r)
	alertID := r.URL.Query().Get("id")
	if alertID == "" {
		http.Error(w, `{"error":"missing id"}`, http.StatusBadRequest)
		return
	}

	var a AlertRow
	err := db.QueryRow(`
		SELECT id, agent_id, severity, message, COALESCE(created_at::text, '')
		FROM alerts
		WHERE id = $1 AND tenant_id = $2
	`, alertID, tenantID).Scan(&a.ID, &a.AgentID, &a.Severity, &a.Message, &a.Time)
	if err != nil {
		http.Error(w, `{"error":"alert not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(a)
}

func handleGetAgent(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r)
	agentID := r.URL.Query().Get("id")
	if agentID == "" {
		http.Error(w, `{"error":"missing id"}`, http.StatusBadRequest)
		return
	}

	var a AgentInfo
	err := db.QueryRow(`
		SELECT id, hostname, os, cpu_model, cpu_load, total_ram, free_ram,
		       disk_total, disk_free, status, COALESCE(last_seen::text, ''),
		       COALESCE(vendor,''), COALESCE(model,''), COALESCE(serial_number,''),
		       COALESCE(uptime,''), COALESCE(kernel_version,''), COALESCE(agent_version,''),
		       COALESCE(local_ip,''), COALESCE(mac_address,''), COALESCE(gateway,''),
		       COALESCE(num_cpu, 0)
		FROM agents
		WHERE id = $1 AND tenant_id = $2
	`, agentID, tenantID).Scan(&a.ID, &a.Hostname, &a.OS, &a.CPUModel,
		&a.CPULoad, &a.TotalRAM, &a.FreeRAM, &a.DiskTotal, &a.DiskFree,
		&a.Status, &a.LastSeen,
		&a.Vendor, &a.Model, &a.SerialNumber,
		&a.Uptime, &a.KernelVersion, &a.AgentVersion,
		&a.LocalIP, &a.MACAddress, &a.Gateway, &a.NumCPU)
	if err != nil {
		http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
		return
	}
	a.TenantID = tenantID

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(a)
}

func handleAgentTelemetry(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("id")
	if agentID == "" {
		http.Error(w, `{"error":"missing id"}`, http.StatusBadRequest)
		return
	}

	tenantID := getTenantID(r)

	// Verify agent belongs to this tenant
	var exists bool
	db.QueryRow(`SELECT EXISTS(SELECT 1 FROM agents WHERE id=$1 AND tenant_id=$2)`, agentID, tenantID).Scan(&exists)
	if !exists {
		http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
		return
	}

	rows, err := db.Query(`
		SELECT cpu_load, total_ram, free_ram, disk_total, disk_free, recorded_at::text
		FROM telemetry
		WHERE agent_id = $1
		ORDER BY id DESC
		LIMIT 100
	`, agentID)
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
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

func handleListBackups(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r)

	rows, err := db.Query(`
		SELECT id, agent_id, COALESCE(name,''), COALESCE(location,''), type, status,
		       size_bytes, cron, COALESCE(executed_at::text,''), COALESCE(created_at::text,'')
		FROM backup_jobs
		WHERE tenant_id = $1
		ORDER BY id DESC
	`, tenantID)
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
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

func handleRunBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	tenantID := getTenantID(r)
	claims := getClaims(r)

	var req struct {
		AgentID string `json:"agentId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AgentID == "" {
		http.Error(w, `{"error":"missing agentId"}`, http.StatusBadRequest)
		return
	}

	// Verify agent belongs to this tenant
	var exists bool
	db.QueryRow(`SELECT EXISTS(SELECT 1 FROM agents WHERE id=$1 AND tenant_id=$2)`, req.AgentID, tenantID).Scan(&exists)
	if !exists {
		http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
		return
	}

	now := time.Now().UTC()
	_, err := db.Exec(`
		INSERT INTO backup_jobs (tenant_id, agent_id, name, location, type, status, cron, executed_at, created_at)
		VALUES ($1, $2, 'Manual-Backup', '/backups/manual', 'full', 'running', '@manual', $3, $3)
	`, tenantID, req.AgentID, now)
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	auditLog(tenantID, claims.UserID, "backup.run", "agent", req.AgentID, nil)
	saveAlert(tenantID, req.AgentID, "info", "Manual backup job started", "")

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"queued"}`))
}

func handleAcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	tenantID := getTenantID(r)
	claims := getClaims(r)

	var req struct {
		AlertID int64 `json:"alertId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	result, err := db.Exec(`
		UPDATE alerts SET acknowledged = true, acknowledged_by = $1, acknowledged_at = NOW()
		WHERE id = $2 AND tenant_id = $3
	`, claims.UserID, req.AlertID, tenantID)
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		http.Error(w, `{"error":"alert not found"}`, http.StatusNotFound)
		return
	}

	auditLog(tenantID, claims.UserID, "alert.acknowledge", "alert", fmt.Sprintf("%d", req.AlertID), nil)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"acknowledged"}`))
}

// ─── Enrollment Handlers ──────────────────────────────────────────────────────

func handleCreateRegistrationToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	tenantID := getTenantID(r)
	claims := getClaims(r)

	if claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden: only admins can create tokens"}`, http.StatusForbidden)
		return
	}

	var req struct {
		Label    string `json:"label"`
		MaxUses  int    `json:"maxUses"`
		ExpiryH  int    `json:"expiryHours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Label = "Agent Token"
		req.MaxUses = 1
		req.ExpiryH = 24
	}

	if req.MaxUses <= 0 {
		req.MaxUses = 1
	}
	if req.ExpiryH <= 0 {
		req.ExpiryH = 24
	}

	rawToken, err := generateRandomToken(24)
	if err != nil {
		http.Error(w, `{"error":"failed to generate token"}`, http.StatusInternalServerError)
		return
	}

	tokenHash := hashToken(rawToken)
	expiresAt := time.Now().Add(time.Duration(req.ExpiryH) * time.Hour)

	var tokenID string
	err = db.QueryRow(`
		INSERT INTO registration_tokens (tenant_id, token_hash, label, created_by, expires_at, max_uses)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id
	`, tenantID, tokenHash, req.Label, claims.UserID, expiresAt, req.MaxUses).Scan(&tokenID)
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	auditLog(tenantID, claims.UserID, "token.create", "registration_token", tokenID, nil)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":     rawToken,
		"tokenId":   tokenID,
		"expiresAt": expiresAt.Format(time.RFC3339),
	})
}

func handleEnrollAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Token  string `json:"token"`
		Info   TelemetryPayload `json:"info"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		http.Error(w, `{"error":"missing enrollment token"}`, http.StatusBadRequest)
		return
	}

	tokenHash := hashToken(req.Token)

	var tokenID, tenantID string
	var expiresAt time.Time
	var maxUses, useCount int
	var isRevoked bool

	err := db.QueryRow(`
		SELECT id, tenant_id, expires_at, max_uses, use_count, is_revoked
		FROM registration_tokens
		WHERE token_hash = $1
	`, tokenHash).Scan(&tokenID, &tenantID, &expiresAt, &maxUses, &useCount, &isRevoked)

	if err != nil {
		http.Error(w, `{"error":"invalid enrollment token"}`, http.StatusUnauthorized)
		return
	}

	if isRevoked {
		http.Error(w, `{"error":"token has been revoked"}`, http.StatusForbidden)
		return
	}

	if time.Now().After(expiresAt) {
		http.Error(w, `{"error":"token has expired"}`, http.StatusForbidden)
		return
	}

	if useCount >= maxUses {
		http.Error(w, `{"error":"token has reached maximum uses"}`, http.StatusForbidden)
		return
	}

	// Generate a unique agent ID
	agentID, err := generateRandomToken(16)
	if err != nil {
		http.Error(w, `{"error":"failed to generate agent ID"}`, http.StatusInternalServerError)
		return
	}

	// Register agent
	now := time.Now().UTC()
	_, err = db.Exec(`
		INSERT INTO agents (id, tenant_id, enrollment_token_id, hostname, os, cpu_model, status, last_seen, enrolled_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'online', $7, $7)
	`, agentID, tenantID, tokenID, req.Info.Hostname, req.Info.OS, req.Info.CPUModel, now)
	if err != nil {
		http.Error(w, `{"error":"failed to register agent"}`, http.StatusInternalServerError)
		return
	}

	// Increment token use count
	db.Exec(`UPDATE registration_tokens SET use_count = use_count + 1, used_at = NOW() WHERE id = $1`, tokenID)

	auditLog(tenantID, "", "agent.enroll", "agent", agentID, map[string]interface{}{
		"hostname": req.Info.Hostname,
	})

	// Generate a long-lived agent JWT
	agentJWT, err := generateToken(agentID, "agent", tenantID, "agent")
	if err != nil {
		http.Error(w, `{"error":"failed to generate agent token"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"agentId": agentID,
		"tenantId": tenantID,
		"token":   agentJWT,
	})
}

// ─── Agent WebSocket Connection ──────────────────────────────────────────────

func handleAgentConnection(w http.ResponseWriter, r *http.Request) {
	// Agent must present a valid JWT
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		http.Error(w, `{"error":"missing agent token"}`, http.StatusUnauthorized)
		return
	}

	claims, err := parseToken(tokenStr)
	if err != nil {
		http.Error(w, `{"error":"invalid agent token"}`, http.StatusUnauthorized)
		return
	}

	// Only agents can connect here
	if claims.Role != "agent" {
		http.Error(w, `{"error":"forbidden: not an agent"}`, http.StatusForbidden)
		return
	}

	agentID := claims.UserID
	tenantID := claims.TenantID

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade agent connection: %v", err)
		return
	}
	defer conn.Close()

	agentConn := &AgentConnection{ID: agentID, TenantID: tenantID, Conn: conn}
	agentsMu.Lock()
	agents[agentID] = agentConn
	agentsMu.Unlock()

	upsertAgent(tenantID, agentID)
	log.Printf("Agent connected: %s (tenant: %s)", agentID, tenantID)

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

		case "telemetry_history":
			type HistoricalPayload struct {
				Payload   string `json:"payload"`
				CreatedAt string `json:"createdAt"`
			}
			var history []HistoricalPayload
			if err := json.Unmarshal([]byte(msg.Payload), &history); err != nil {
				log.Printf("Failed to parse telemetry_history: %v", err)
				continue
			}
			for _, entry := range history {
				var t TelemetryPayload
				if err := json.Unmarshal([]byte(entry.Payload), &t); err == nil {
					db.Exec(`
						INSERT INTO telemetry (agent_id, cpu_load, total_ram, free_ram, disk_total, disk_free, recorded_at)
						VALUES ($1, $2, $3, $4, $5, $6, $7)
					`, agentID, t.CPULoad, t.TotalRAM, t.FreeRAM, t.DiskTotal, t.DiskFree, entry.CreatedAt)
				}
			}
			broadcastToFrontend(agentID, msgBytes)

		case "terminal_output":
			broadcastToFrontend(agentID, msgBytes)

		case "backup_status":
			log.Printf("Backup progress from agent %s: %s", agentID, msg.Payload)
			broadcastToFrontend(agentID, msgBytes)

		case "software_list":
			log.Printf("Software list received from agent %s", agentID)
			var softwareItems []struct {
				Name                 string `json:"name"`
				Publisher            string `json:"publisher"`
				Version              string `json:"version"`
				InstallDate          string `json:"installDate"`
				EstimatedSizeKB      int64  `json:"estimatedSizeKB"`
				QuietUninstallString string `json:"quietUninstallString"`
			}
			if err := json.Unmarshal([]byte(msg.Payload), &softwareItems); err != nil {
				log.Printf("Failed to parse software_list: %v", err)
				continue
			}

			var tenantID string
			db.QueryRow(`SELECT tenant_id FROM agents WHERE id = $1`, agentID).Scan(&tenantID)

			db.Exec(`DELETE FROM agent_software WHERE agent_id = $1 AND tenant_id = $2`, agentID, tenantID)

			for _, s := range softwareItems {
				db.Exec(`
					INSERT INTO agent_software (tenant_id, agent_id, name, publisher, version, install_date, estimated_size_kb, quiet_uninstall_string)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				`, tenantID, agentID, s.Name, s.Publisher, s.Version, s.InstallDate, s.EstimatedSizeKB, s.QuietUninstallString)
			}
			broadcastToFrontend(agentID, msgBytes)

		case "agent_log":
			var logEntry struct {
				Level   string `json:"level"`
				LogType string `json:"logType"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal([]byte(msg.Payload), &logEntry); err != nil {
				log.Printf("Failed to parse agent_log: %v", err)
				continue
			}

			var tenantID string
			db.QueryRow(`SELECT tenant_id FROM agents WHERE id = $1`, agentID).Scan(&tenantID)

			db.Exec(`
				INSERT INTO agent_logs (tenant_id, agent_id, level, log_type, message)
				VALUES ($1, $2, $3, $4, $5)
			`, tenantID, agentID, logEntry.Level, logEntry.LogType, logEntry.Message)

		case "patch_list":
			log.Printf("Patch list received from agent %s", agentID)
			var patchItems []struct {
				KbID        string `json:"kbId"`
				Name        string `json:"name"`
				Severity    string `json:"severity"`
				Description string `json:"description"`
				Installed   bool   `json:"installed"`
				InstalledAt string `json:"installedAt"`
			}
			if err := json.Unmarshal([]byte(msg.Payload), &patchItems); err != nil {
				log.Printf("Failed to parse patch_list: %v", err)
				continue
			}

			var tenantID string
			db.QueryRow(`SELECT tenant_id FROM agents WHERE id = $1`, agentID).Scan(&tenantID)

			db.Exec(`DELETE FROM agent_patches WHERE agent_id = $1 AND tenant_id = $2`, agentID, tenantID)

			for _, p := range patchItems {
				db.Exec(`
					INSERT INTO agent_patches (tenant_id, agent_id, kb_id, name, severity, description, installed, installed_at)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				`, tenantID, agentID, p.KbID, p.Name, p.Severity, p.Description, p.Installed, p.InstalledAt)
			}
			broadcastToFrontend(agentID, msgBytes)

		case "check_result":
			var result struct {
				CheckID int64  `json:"checkId"`
				Status  string `json:"status"`
				Output  string `json:"output"`
			}
			if err := json.Unmarshal([]byte(msg.Payload), &result); err != nil {
				log.Printf("Failed to parse check_result: %v", err)
				continue
			}

			db.Exec(`
				UPDATE agent_checks SET status = $1, last_output = $2, last_run = NOW()
				WHERE id = $3
			`, result.Status, result.Output, result.CheckID)
			broadcastToFrontend(agentID, msgBytes)
		}
	}
}

func handleEventsWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade event connection: %v", err)
		return
	}
	client := &ClientEventConnection{Conn: conn}
	registerEventClient(client)
	defer func() {
		unregisterEventClient(client)
		conn.Close()
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

func handleTerminalWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade frontend connection: %v", err)
		return
	}
	defer conn.Close()

	tenantID := getTenantID(r)

	sessionID := fmt.Sprintf("%p", conn)
	frontendsMu.Lock()
	frontends[sessionID] = &FrontendSession{Conn: conn, TenantID: tenantID}
	frontendsMu.Unlock()

	defer func() {
		frontendsMu.Lock()
		delete(frontends, sessionID)
		frontendsMu.Unlock()
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
			continue
		}

		agentsMu.Lock()
		agent, ok := agents[cmd.AgentID]
		agentsMu.Unlock()

		if ok && agent.TenantID == tenantID {
			agent.Mu.Lock()
			agent.Conn.WriteMessage(websocket.TextMessage, msgBytes)
			agent.Mu.Unlock()
		}
	}
}

func handleListUsers(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r)
	claims := getClaims(r)

	if claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	rows, err := db.Query(`
		SELECT id, email, username, COALESCE(full_name,''), role, is_active, COALESCE(last_login::text,''), created_at::text
		FROM users
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type UserInfo struct {
		ID        string `json:"id"`
		Email     string `json:"email"`
		Username  string `json:"username"`
		FullName  string `json:"fullName"`
		Role      string `json:"role"`
		IsActive  bool   `json:"isActive"`
		LastLogin string `json:"lastLogin"`
		CreatedAt string `json:"createdAt"`
	}

	list := []UserInfo{}
	for rows.Next() {
		var u UserInfo
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.FullName, &u.Role, &u.IsActive, &u.LastLogin, &u.CreatedAt); err != nil {
			continue
		}
		list = append(list, u)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

// ─── Agent Sub-routes ────────────────────────────────────────────────────────

func handleAgentRoutes(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasSuffix(path, "/software") && r.Method == http.MethodGet {
		handleListSoftware(w, r)
	} else if strings.HasSuffix(path, "/software/scan") && r.Method == http.MethodPost {
		handleScanSoftware(w, r)
	} else if strings.HasSuffix(path, "/notes") && r.Method == http.MethodGet {
		handleListNotes(w, r)
	} else if strings.HasSuffix(path, "/notes") && r.Method == http.MethodPost {
		handleCreateNote(w, r)
	} else if strings.HasSuffix(path, "/logs") && r.Method == http.MethodGet {
		handleListLogs(w, r)
	} else if strings.HasSuffix(path, "/audit") && r.Method == http.MethodGet {
		handleListAudit(w, r)
	} else if strings.HasSuffix(path, "/patches") && r.Method == http.MethodGet {
		handleListPatches(w, r)
	} else if strings.HasSuffix(path, "/patches/scan") && r.Method == http.MethodPost {
		handleScanPatches(w, r)
	} else if strings.HasSuffix(path, "/checks") && r.Method == http.MethodGet {
		handleListChecks(w, r)
	} else if strings.HasSuffix(path, "/checks") && r.Method == http.MethodPost {
		handleCreateCheck(w, r)
	} else {
		http.NotFound(w, r)
	}
}

func getAgentIDFromPath(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/api/agents/"), "/")
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

func handleListSoftware(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentIDFromPath(r.URL.Path)
	tenantID := getTenantID(r)

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	var total int
	db.QueryRow(`SELECT COUNT(*) FROM agent_software WHERE agent_id = $1 AND tenant_id = $2`, agentID, tenantID).Scan(&total)

	rows, err := db.Query(`
		SELECT id, name, COALESCE(publisher,''), COALESCE(version,''),
		       COALESCE(install_date,''), COALESCE(estimated_size_kb, 0),
		       COALESCE(quiet_uninstall_string,''), scanned_at::text
		FROM agent_software
		WHERE agent_id = $1 AND tenant_id = $2
		ORDER BY name ASC
		LIMIT $3 OFFSET $4
	`, agentID, tenantID, limit, offset)
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type SoftwareRow struct {
		ID                   int64  `json:"id"`
		Name                 string `json:"name"`
		Publisher            string `json:"publisher"`
		Version              string `json:"version"`
		InstallDate          string `json:"installDate"`
		EstimatedSizeKB      int64  `json:"estimatedSizeKB"`
		QuietUninstallString string `json:"quietUninstallString"`
		ScannedAt            string `json:"scannedAt"`
	}

	list := []SoftwareRow{}
	for rows.Next() {
		var s SoftwareRow
		if err := rows.Scan(&s.ID, &s.Name, &s.Publisher, &s.Version,
			&s.InstallDate, &s.EstimatedSizeKB, &s.QuietUninstallString, &s.ScannedAt); err != nil {
			continue
		}
		list = append(list, s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"items":  list,
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"hasMore": offset+limit < total,
	})
}

func handleScanSoftware(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentIDFromPath(r.URL.Path)

	agentsMu.Lock()
	agent, ok := agents[agentID]
	agentsMu.Unlock()

	if !ok {
		http.Error(w, `{"error":"agent not connected"}`, http.StatusNotFound)
		return
	}

	msg := Message{
		AgentID: agentID,
		Type:    "scan_software",
		Payload: "{}",
	}
	msgBytes, _ := json.Marshal(msg)

	agent.Mu.Lock()
	err := agent.Conn.WriteMessage(websocket.TextMessage, msgBytes)
	agent.Mu.Unlock()

	if err != nil {
		http.Error(w, `{"error":"failed to send command"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "scan initiated"})
}

// ─── Notes Handlers ──────────────────────────────────────────────────────────

func handleNoteRoutes(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	parts := strings.Split(strings.TrimPrefix(path, "/api/notes/"), "/")

	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	noteID := parts[0]

	if len(parts) == 1 && r.Method == http.MethodPut {
		handleUpdateNote(w, r, noteID)
	} else if len(parts) == 1 && r.Method == http.MethodDelete {
		handleDeleteNote(w, r, noteID)
	} else {
		http.NotFound(w, r)
	}
}

func handleListNotes(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentIDFromPath(r.URL.Path)
	tenantID := getTenantID(r)
	userID := getClaims(r).UserID

	rows, err := db.Query(`
		SELECT n.id, n.content, COALESCE(u.username, 'unknown'),
		       n.created_at::text, n.updated_at::text
		FROM agent_notes n
		LEFT JOIN users u ON n.user_id = u.id
		WHERE n.agent_id = $1 AND n.tenant_id = $2
		ORDER BY n.updated_at DESC
	`, agentID, tenantID)
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type NoteRow struct {
		ID        int64  `json:"id"`
		Content   string `json:"content"`
		UserName  string `json:"userName"`
		CreatedAt string `json:"createdAt"`
		UpdatedAt string `json:"updatedAt"`
	}

	list := []NoteRow{}
	for rows.Next() {
		var n NoteRow
		if err := rows.Scan(&n.ID, &n.Content, &n.UserName, &n.CreatedAt, &n.UpdatedAt); err != nil {
			continue
		}
		list = append(list, n)
	}

	_ = userID

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func handleCreateNote(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentIDFromPath(r.URL.Path)
	tenantID := getTenantID(r)
	userID := getClaims(r).UserID

	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Content == "" {
		http.Error(w, `{"error":"content required"}`, http.StatusBadRequest)
		return
	}

	var noteID int64
	err := db.QueryRow(`
		INSERT INTO agent_notes (tenant_id, agent_id, user_id, content)
		VALUES ($1, $2, $3, $4) RETURNING id
	`, tenantID, agentID, userID, body.Content).Scan(&noteID)
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"id": noteID})
}

func handleUpdateNote(w http.ResponseWriter, r *http.Request, noteID string) {
	tenantID := getTenantID(r)

	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Content == "" {
		http.Error(w, `{"error":"content required"}`, http.StatusBadRequest)
		return
	}

	result, err := db.Exec(`
		UPDATE agent_notes SET content = $1, updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3
	`, body.Content, noteID, tenantID)
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

func handleDeleteNote(w http.ResponseWriter, r *http.Request, noteID string) {
	tenantID := getTenantID(r)

	result, err := db.Exec(`
		DELETE FROM agent_notes WHERE id = $1 AND tenant_id = $2
	`, noteID, tenantID)
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

// ─── Logs Handlers ──────────────────────────────────────────────────────────

func handleListLogs(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentIDFromPath(r.URL.Path)
	tenantID := getTenantID(r)

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}

	level := r.URL.Query().Get("level")
	cursor := r.URL.Query().Get("cursor")

	query := `
		SELECT id, level, log_type, message, created_at::text
		FROM agent_logs
		WHERE agent_id = $1 AND tenant_id = $2
	`
	args := []interface{}{agentID, tenantID}
	argIdx := 3

	if level != "" {
		query += fmt.Sprintf(" AND level = $%d", argIdx)
		args = append(args, level)
		argIdx++
	}

	if cursor != "" {
		query += fmt.Sprintf(" AND created_at < $%d", argIdx)
		args = append(args, cursor)
		argIdx++
	}

	query += " ORDER BY created_at DESC"
	query += fmt.Sprintf(" LIMIT $%d", argIdx)
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type LogRow struct {
		ID        int64  `json:"id"`
		Level     string `json:"level"`
		LogType   string `json:"logType"`
		Message   string `json:"message"`
		CreatedAt string `json:"createdAt"`
	}

	list := []LogRow{}
	for rows.Next() {
		var l LogRow
		if err := rows.Scan(&l.ID, &l.Level, &l.LogType, &l.Message, &l.CreatedAt); err != nil {
			continue
		}
		list = append(list, l)
	}

	nextCursor := ""
	if len(list) > 0 {
		nextCursor = list[len(list)-1].CreatedAt
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"items":  list,
		"cursor": nextCursor,
		"hasMore": len(list) == limit,
	})
}

// ─── Audit Handlers ─────────────────────────────────────────────────────────

func handleListAudit(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentIDFromPath(r.URL.Path)
	tenantID := getTenantID(r)

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	var total int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE resource_id = $1 AND tenant_id = $2`, agentID, tenantID).Scan(&total)

	rows, err := db.Query(`
		SELECT id, action, COALESCE(resource_type,''), COALESCE(resource_id,''),
		       COALESCE(details, '{}'), COALESCE(ip_address::text, ''),
		       created_at::text
		FROM audit_log
		WHERE resource_id = $1 AND tenant_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`, agentID, tenantID, limit, offset)
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type AuditRow struct {
		ID           int64                  `json:"id"`
		Action       string                 `json:"action"`
		ResourceType string                 `json:"resourceType"`
		ResourceID   string                 `json:"resourceId"`
		Details      map[string]interface{} `json:"details"`
		IPAddress    string                 `json:"ipAddress"`
		CreatedAt    string                 `json:"createdAt"`
	}

	list := []AuditRow{}
	for rows.Next() {
		var a AuditRow
		var detailsJSON []byte
		if err := rows.Scan(&a.ID, &a.Action, &a.ResourceType, &a.ResourceID,
			&detailsJSON, &a.IPAddress, &a.CreatedAt); err != nil {
			continue
		}
		json.Unmarshal(detailsJSON, &a.Details)
		list = append(list, a)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"items":  list,
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"hasMore": offset+limit < total,
	})
}

// ─── Patches Handlers ───────────────────────────────────────────────────────

func handleListPatches(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentIDFromPath(r.URL.Path)
	tenantID := getTenantID(r)

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	var total int
	db.QueryRow(`SELECT COUNT(*) FROM agent_patches WHERE agent_id = $1 AND tenant_id = $2`, agentID, tenantID).Scan(&total)

	rows, err := db.Query(`
		SELECT id, COALESCE(kb_id,''), COALESCE(name,''), COALESCE(severity,''),
		       COALESCE(description,''), installed, COALESCE(install_date,''),
		       scanned_at::text
		FROM agent_patches
		WHERE agent_id = $1 AND tenant_id = $2
		ORDER BY installed DESC, severity DESC, name ASC
		LIMIT $3 OFFSET $4
	`, agentID, tenantID, limit, offset)
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type PatchRow struct {
		ID          int64  `json:"id"`
		KbID        string `json:"kbId"`
		Name        string `json:"name"`
		Severity    string `json:"severity"`
		Description string `json:"description"`
		Installed   bool   `json:"installed"`
		InstalledAt string `json:"installedAt"`
		ScannedAt   string `json:"scannedAt"`
	}

	list := []PatchRow{}
	for rows.Next() {
		var p PatchRow
		if err := rows.Scan(&p.ID, &p.KbID, &p.Name, &p.Severity,
			&p.Description, &p.Installed, &p.InstalledAt, &p.ScannedAt); err != nil {
			continue
		}
		list = append(list, p)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"items":  list,
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"hasMore": offset+limit < total,
	})
}

func handleScanPatches(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentIDFromPath(r.URL.Path)

	agentsMu.Lock()
	agent, ok := agents[agentID]
	agentsMu.Unlock()

	if !ok {
		http.Error(w, `{"error":"agent not connected"}`, http.StatusNotFound)
		return
	}

	msg := Message{
		AgentID: agentID,
		Type:    "scan_patches",
		Payload: "{}",
	}
	msgBytes, _ := json.Marshal(msg)

	agent.Mu.Lock()
	err := agent.Conn.WriteMessage(websocket.TextMessage, msgBytes)
	agent.Mu.Unlock()

	if err != nil {
		http.Error(w, `{"error":"failed to send command"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "scan initiated"})
}

// ─── Checks Handlers ───────────────────────────────────────────────────────

func handleCheckRoutes(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	parts := strings.Split(strings.TrimPrefix(path, "/api/checks/"), "/")

	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	checkID := parts[0]

	if len(parts) == 1 && r.Method == http.MethodPut {
		handleUpdateCheck(w, r, checkID)
	} else if len(parts) == 1 && r.Method == http.MethodDelete {
		handleDeleteCheck(w, r, checkID)
	} else if len(parts) == 2 && parts[1] == "run" && r.Method == http.MethodPost {
		handleRunCheck(w, r, checkID)
	} else {
		http.NotFound(w, r)
	}
}

func handleListChecks(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentIDFromPath(r.URL.Path)
	tenantID := getTenantID(r)

	rows, err := db.Query(`
		SELECT id, check_type, COALESCE(description,''), COALESCE(config, '{}'),
		       status, COALESCE(last_output,''), COALESCE(last_run::text, ''),
		       enabled
		FROM agent_checks
		WHERE agent_id = $1 AND tenant_id = $2
		ORDER BY created_at DESC
	`, agentID, tenantID)
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type CheckRow struct {
		ID          int64                  `json:"id"`
		CheckType   string                 `json:"checkType"`
		Description string                 `json:"description"`
		Config      map[string]interface{} `json:"config"`
		Status      string                 `json:"status"`
		LastOutput  string                 `json:"lastOutput"`
		LastRun     string                 `json:"lastRun"`
		Enabled     bool                   `json:"enabled"`
	}

	list := []CheckRow{}
	for rows.Next() {
		var c CheckRow
		var configJSON []byte
		if err := rows.Scan(&c.ID, &c.CheckType, &c.Description,
			&configJSON, &c.Status, &c.LastOutput, &c.LastRun, &c.Enabled); err != nil {
			continue
		}
		json.Unmarshal(configJSON, &c.Config)
		list = append(list, c)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func handleCreateCheck(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentIDFromPath(r.URL.Path)
	tenantID := getTenantID(r)

	var body struct {
		CheckType   string                 `json:"checkType"`
		Description string                 `json:"description"`
		Config      map[string]interface{} `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.CheckType == "" {
		http.Error(w, `{"error":"checkType required"}`, http.StatusBadRequest)
		return
	}

	configJSON, _ := json.Marshal(body.Config)

	var checkID int64
	err := db.QueryRow(`
		INSERT INTO agent_checks (tenant_id, agent_id, check_type, description, config)
		VALUES ($1, $2, $3, $4, $5) RETURNING id
	`, tenantID, agentID, body.CheckType, body.Description, configJSON).Scan(&checkID)
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"id": checkID})
}

func handleUpdateCheck(w http.ResponseWriter, r *http.Request, checkID string) {
	tenantID := getTenantID(r)

	var body struct {
		Description string                 `json:"description"`
		Config      map[string]interface{} `json:"config"`
		Enabled     *bool                  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	if body.Config != nil {
		configJSON, _ := json.Marshal(body.Config)
		db.Exec(`UPDATE agent_checks SET config = $1 WHERE id = $2 AND tenant_id = $3`,
			configJSON, checkID, tenantID)
	}

	if body.Enabled != nil {
		db.Exec(`UPDATE agent_checks SET enabled = $1 WHERE id = $2 AND tenant_id = $3`,
			*body.Enabled, checkID, tenantID)
	}

	if body.Description != "" {
		db.Exec(`UPDATE agent_checks SET description = $1 WHERE id = $2 AND tenant_id = $3`,
			body.Description, checkID, tenantID)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

func handleDeleteCheck(w http.ResponseWriter, r *http.Request, checkID string) {
	tenantID := getTenantID(r)

	result, err := db.Exec(`
		DELETE FROM agent_checks WHERE id = $1 AND tenant_id = $2
	`, checkID, tenantID)
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

func handleRunCheck(w http.ResponseWriter, r *http.Request, checkID string) {
	tenantID := getTenantID(r)

	var check struct {
		AgentID   string
		CheckType string
		Config    map[string]interface{}
	}

	err := db.QueryRow(`
		SELECT agent_id, check_type, COALESCE(config, '{}')
		FROM agent_checks WHERE id = $1 AND tenant_id = $2
	`, checkID, tenantID).Scan(&check.AgentID, &check.CheckType, &check.Config)
	if err != nil {
		http.Error(w, `{"error":"check not found"}`, http.StatusNotFound)
		return
	}

	agentsMu.Lock()
	agent, ok := agents[check.AgentID]
	agentsMu.Unlock()

	if !ok {
		http.Error(w, `{"error":"agent not connected"}`, http.StatusNotFound)
		return
	}

	payloadBytes, _ := json.Marshal(map[string]interface{}{
		"checkId": checkID,
		"type":    check.CheckType,
		"config":  check.Config,
	})

	msg := Message{
		AgentID: check.AgentID,
		Type:    "check_command",
		Payload: string(payloadBytes),
	}
	msgBytes, _ := json.Marshal(msg)

	agent.Mu.Lock()
	err = agent.Conn.WriteMessage(websocket.TextMessage, msgBytes)
	agent.Mu.Unlock()

	if err != nil {
		http.Error(w, `{"error":"failed to send command"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "check initiated"})
}

// ─── Telemetry Pruning ───────────────────────────────────────────────────────

func pruneOldTelemetry() {
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for range ticker.C {
			result, err := db.Exec(`DELETE FROM telemetry WHERE recorded_at < NOW() - INTERVAL '30 days'`)
			if err != nil {
				log.Printf("Telemetry pruning error: %v", err)
				continue
			}
			rows, _ := result.RowsAffected()
			if rows > 0 {
				log.Printf("Pruned %d old telemetry rows", rows)
			}
		}
	}()
}

// ─── Entry Point ──────────────────────────────────────────────────────────────

func main() {
	cfg = loadConfig()
	initDB()
	pruneOldTelemetry()

	// Public routes
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/api/auth/login", corsMiddleware(handleLogin))

	// Enrollment (agent registration)
	http.HandleFunc("/api/enroll", corsMiddleware(handleEnrollAgent))
	http.HandleFunc("/api/agents/register-token", corsMiddleware(authMiddleware(handleCreateRegistrationToken)))

	// Agent WebSocket (agent authenticates with its own JWT)
	http.HandleFunc("/agent/connect", handleAgentConnection)

	// Protected routes
	http.HandleFunc("/api/agents", corsMiddleware(authMiddleware(handleListAgents)))
	http.HandleFunc("/api/agents/detail", corsMiddleware(authMiddleware(handleGetAgent)))
	http.HandleFunc("/api/agents/telemetry", corsMiddleware(authMiddleware(handleAgentTelemetry)))
	http.HandleFunc("/api/agents/", corsMiddleware(authMiddleware(handleAgentRoutes)))
	http.HandleFunc("/api/notes/", corsMiddleware(authMiddleware(handleNoteRoutes)))
	http.HandleFunc("/api/checks/", corsMiddleware(authMiddleware(handleCheckRoutes)))
	http.HandleFunc("/api/alerts", corsMiddleware(authMiddleware(handleListAlerts)))
	http.HandleFunc("/api/alerts/detail", corsMiddleware(authMiddleware(handleGetAlert)))
	http.HandleFunc("/api/alerts/acknowledge", corsMiddleware(authMiddleware(handleAcknowledgeAlert)))
	http.HandleFunc("/api/backups", corsMiddleware(authMiddleware(handleListBackups)))
	http.HandleFunc("/api/backups/run", corsMiddleware(authMiddleware(handleRunBackup)))
	http.HandleFunc("/api/users", corsMiddleware(authMiddleware(handleListUsers)))

	// WebSocket routes
	http.HandleFunc("/terminal/ws", authMiddleware(handleTerminalWebSocket))
	http.HandleFunc("/api/events/ws", authMiddleware(handleEventsWebSocket))

	log.Printf("Backend starting on http://localhost%s", cfg.Port)
	if err := http.ListenAndServe(cfg.Port, nil); err != nil {
		log.Fatalf("Failed to start backend: %v", err)
	}
}
