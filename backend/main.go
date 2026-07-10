package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	pq "github.com/lib/pq"
	"github.com/robfig/cron/v3"
	"golang.org/x/crypto/bcrypt"

	"rmm/backend/notifier"
)

// ─── Configuration ────────────────────────────────────────────────────────────

type Config struct {
	JWTSecret        []byte
	CORSOrigins      []string
	DatabaseURL      string
	DatabaseURLAdmin string
	AdminEmail       string
	AdminPassword    string
	AgentEnrollSecret string
	LogLevel         string
	Port             string
	SMTPHost         string
	SMTPPort         string
	SMTPUser         string
	SMTPPassword     string
	SMTPFrom         string
	AlertToEmails    []string
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
		// Default: connect as `apexrmm_app` (NOSUPERUSER, NOBYPASSRLS).
		// RLS policies apply to this role.
		dbURL = "postgres://apexrmm_app:apexrmm_app_secret_2024@localhost:5432/apexrmm?sslmode=disable"
	}

	// Admin connection: connects as `apexrmm` (SUPERUSER, BYPASSRLS).
	// Used ONLY by initDB() and handleEnrollAgent(). Both call sites are
	// explicitly commented as bypass sites.
	dbAdminURL := os.Getenv("DATABASE_URL_ADMIN")
	if dbAdminURL == "" {
		dbAdminURL = "postgres://apexrmm:apexrmm_secret_2024@localhost:5432/apexrmm?sslmode=disable"
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

	smtpTo := []string{}
	if to := os.Getenv("ALERT_TO_EMAILS"); to != "" {
		for _, addr := range strings.Split(to, ",") {
			addr = strings.TrimSpace(addr)
			if addr != "" {
				smtpTo = append(smtpTo, addr)
			}
		}
	}

	return Config{
		JWTSecret:         []byte(jwtSecret),
		CORSOrigins:       corsOrigins,
		DatabaseURL:       dbURL,
		DatabaseURLAdmin:  dbAdminURL,
		AdminEmail:        adminEmail,
		AdminPassword:     adminPass,
		AgentEnrollSecret: enrollSecret,
		LogLevel:          os.Getenv("LOG_LEVEL"),
		Port:              ":8080",
		SMTPHost:          os.Getenv("SMTP_HOST"),
		SMTPPort:          os.Getenv("SMTP_PORT"),
		SMTPUser:          os.Getenv("SMTP_USER"),
		SMTPPassword:      os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:          os.Getenv("SMTP_FROM"),
		AlertToEmails:     smtpTo,
	}
}

// ─── Globals ──────────────────────────────────────────────────────────────────

var (
	cfg Config
	// db is the connection pool used by all tenant-scoped queries.
	// It connects as `apexrmm_app` (NOSUPERUSER, NOBYPASSRLS) so PostgreSQL
	// RLS policies are enforced. The Go wrapper `withTenant()` sets
	// `app.tenant_id` per transaction; the policies filter by it.
	db      *sql.DB
	// dbAdmin is the connection pool used ONLY for code paths that must
	// bypass RLS: schema initialization and the agent enrollment handler
	// (which has to create rows for a tenant it doesn't have a JWT for yet).
	// It connects as `apexrmm` (SUPERUSER, BYPASSRLS). Keep its usage
	// auditable — every call site should have a comment explaining why.
	dbAdmin *sql.DB
)

// ─── WebSocket Upgrader ──────────────────────────────────────────────────────

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
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
	ID          int64  `json:"id"`
	AgentID     string `json:"agentId"`
	Name        string `json:"name"`
	Location    string `json:"location"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	SizeBytes   int64  `json:"sizeBytes"`
	Cron        string `json:"cron"`
	ExecutedAt  string `json:"executedAt"`
	NextRunTime string `json:"nextRunTime"`
	CreatedAt   string `json:"createdAt"`
}

type BackupConfig struct {
	RepoURL       string   `json:"repoUrl"`
	SourcePaths   []string `json:"sourcePaths"`
	Cron          string   `json:"cron"`
	RetentionDays int      `json:"retentionDays"`
	Enabled       bool     `json:"enabled"`
}

// ─── Encryption Helpers ───────────────────────────────────────────────────────

var backupEncryptionKey []byte

func initBackupEncryption() {
	key := os.Getenv("BACKUP_ENCRYPTION_KEY")
	if key == "" {
		key = "rmm-default-backup-key-32bytes!" // 32 bytes for AES-256
	}
	h := sha256.Sum256([]byte(key))
	backupEncryptionKey = h[:]
}

func encryptPassword(plaintext string) ([]byte, error) {
	block, err := aes.NewCipher(backupEncryptionKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

func decryptPassword(ciphertext []byte) (string, error) {
	block, err := aes.NewCipher(backupEncryptionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
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

const (
	rateLimitUserThreshold    = 5
	rateLimitIPThreshold      = 20
	rateLimitWindow           = 15 * time.Minute
)

// rateLimitedUser returns true if the user identifier has exceeded the
// failed-attempt threshold within the window (resets on successful login).
func rateLimitedUser(identifier string) bool {
	var count int
	db.QueryRow(`
		SELECT COUNT(*) FROM login_attempts a
		WHERE a.identifier = $1 AND a.type = 'user' AND a.success = false
		AND a.attempted_at > NOW() - ($2 || ' minutes')::INTERVAL
		AND NOT EXISTS (
			SELECT 1 FROM login_attempts b
			WHERE b.identifier = $1 AND b.type = 'user' AND b.success = true
			AND b.attempted_at > a.attempted_at
		)
	`, identifier, int(rateLimitWindow.Minutes())).Scan(&count)
	return count >= rateLimitUserThreshold
}

// rateLimitedIP returns true if the IP has exceeded the failed-attempt
// threshold within the window (resets on successful login from that IP).
func rateLimitedIP(ip string) bool {
	var count int
	db.QueryRow(`
		SELECT COUNT(*) FROM login_attempts a
		WHERE a.identifier = $1 AND a.type = 'ip' AND a.success = false
		AND a.attempted_at > NOW() - ($2 || ' minutes')::INTERVAL
		AND NOT EXISTS (
			SELECT 1 FROM login_attempts b
			WHERE b.identifier = $1 AND b.type = 'ip' AND b.success = true
			AND b.attempted_at > a.attempted_at
		)
	`, ip, int(rateLimitWindow.Minutes())).Scan(&count)
	return count >= rateLimitIPThreshold
}

func recordLoginAttempt(identifier, idType string, success bool) {
	db.Exec(`INSERT INTO login_attempts (identifier, type, success) VALUES ($1, $2, $3)`,
		identifier, idType, success)
}

// pruneLoginAttempts runs every hour and removes records older than 24 hours.
func pruneLoginAttempts() {
	for {
		time.Sleep(1 * time.Hour)
		if _, err := db.Exec(`DELETE FROM login_attempts WHERE attempted_at < NOW() - INTERVAL '24 hours'`); err != nil {
			log.Printf("Login attempt pruning error: %v", err)
		}
	}
}

// ─── Database ─────────────────────────────────────────────────────────────────

func initDB() {
	// Admin pool (apexrmm, SUPERUSER, BYPASSRLS) — used ONLY for schema setup
	// and for the explicit RLS bypass in handleEnrollAgent.
	// Use `=` not `:=` so we assign to the package-level dbAdmin.
	var err error
	dbAdmin, err = sql.Open("postgres", cfg.DatabaseURLAdmin)
	if err != nil {
		log.Fatalf("Cannot open admin database connection: %v", err)
	}
	dbAdmin.SetMaxOpenConns(5)
	dbAdmin.SetMaxIdleConns(2)
	dbAdmin.SetConnMaxLifetime(5 * time.Minute)
	if err := dbAdmin.Ping(); err != nil {
		log.Fatalf("Cannot ping admin database: %v", err)
	}

	// App pool (apexrmm_app, NOSUPERUSER, NOBYPASSRLS) — used by all
	// tenant-scoped queries. RLS policies apply to this role.
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
	// Schema setup runs with the admin pool (BYPASSRLS) because
	// the app pool would not have permissions to CREATE TABLE / GRANT
	// during initial bootstrap, and RLS is meaningless before the
	// tables exist anyway.
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
		next_run_time TIMESTAMPTZ,
		created_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS agent_backup_configs (
		id BIGSERIAL PRIMARY KEY,
		tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
		agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE UNIQUE,
		repo_url VARCHAR(500) NOT NULL DEFAULT '',
		repo_password_enc BYTEA,
		source_paths TEXT[] NOT NULL DEFAULT '{}',
		cron VARCHAR(100) NOT NULL DEFAULT '0 2 * * *',
		retention_days INTEGER NOT NULL DEFAULT 30,
		enabled BOOLEAN NOT NULL DEFAULT true,
		updated_at TIMESTAMPTZ DEFAULT NOW(),
		created_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS scripts (
		id BIGSERIAL PRIMARY KEY,
		tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
		name VARCHAR(255) NOT NULL,
		description TEXT,
		command TEXT NOT NULL,
		language VARCHAR(50) NOT NULL DEFAULT 'powershell',
		created_by UUID REFERENCES users(id) ON DELETE SET NULL,
		created_at TIMESTAMPTZ DEFAULT NOW(),
		updated_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS script_executions (
		id BIGSERIAL PRIMARY KEY,
		tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
		script_id BIGINT REFERENCES scripts(id) ON DELETE SET NULL,
		agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
		executed_by UUID REFERENCES users(id) ON DELETE SET NULL,
		status VARCHAR(50) DEFAULT 'pending',
		exit_code INTEGER,
		output_truncated BOOLEAN DEFAULT false,
		output TEXT,
		duration_ms INTEGER,
		started_at TIMESTAMPTZ,
		finished_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS terminal_sessions (
		id BIGSERIAL PRIMARY KEY,
		tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
		agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
		user_id UUID NOT NULL REFERENCES users(id) ON DELETE SET NULL,
		status VARCHAR(20) NOT NULL DEFAULT 'active',
		closed_by VARCHAR(20),
		recording JSONB NOT NULL DEFAULT '[]',
		duration_sec INTEGER,
		started_at TIMESTAMPTZ DEFAULT NOW(),
		ended_at TIMESTAMPTZ,
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
	CREATE INDEX IF NOT EXISTS idx_backup_configs_tenant ON agent_backup_configs(tenant_id);
	CREATE INDEX IF NOT EXISTS idx_backup_configs_agent ON agent_backup_configs(agent_id);
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
	CREATE INDEX IF NOT EXISTS idx_scripts_tenant ON scripts(tenant_id);
	CREATE INDEX IF NOT EXISTS idx_script_executions_tenant ON script_executions(tenant_id);
	CREATE INDEX IF NOT EXISTS idx_script_executions_agent ON script_executions(agent_id);
	CREATE INDEX IF NOT EXISTS idx_script_executions_script ON script_executions(script_id);
	CREATE INDEX IF NOT EXISTS idx_terminal_sessions_tenant ON terminal_sessions(tenant_id);
	CREATE INDEX IF NOT EXISTS idx_terminal_sessions_agent ON terminal_sessions(agent_id);
	CREATE INDEX IF NOT EXISTS idx_terminal_sessions_user ON terminal_sessions(user_id);

	CREATE TABLE IF NOT EXISTS login_attempts (
		id BIGSERIAL PRIMARY KEY,
		identifier VARCHAR(255) NOT NULL,
		type VARCHAR(10) NOT NULL CHECK (type IN ('ip', 'user')),
		success BOOLEAN NOT NULL DEFAULT false,
		attempted_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_login_attempts_lookup ON login_attempts(identifier, type, attempted_at);

	ALTER TABLE alerts ADD COLUMN IF NOT EXISTS notified_at TIMESTAMPTZ;
	CREATE INDEX IF NOT EXISTS idx_alerts_notified ON alerts(notified_at) WHERE notified_at IS NOT NULL;

	GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE scripts, script_executions, terminal_sessions, agent_backup_configs, login_attempts TO apexrmm_app;
	GRANT USAGE, SELECT ON SEQUENCE scripts_id_seq, script_executions_id_seq, terminal_sessions_id_seq, agent_backup_configs_id_seq, login_attempts_id_seq TO apexrmm_app;

	ALTER TABLE agent_backup_configs ENABLE ROW LEVEL SECURITY;
	ALTER TABLE agent_backup_configs FORCE ROW LEVEL SECURITY;
	DROP POLICY IF EXISTS tenant_isolation_agent_backup_configs ON agent_backup_configs;
	CREATE POLICY tenant_isolation_agent_backup_configs ON agent_backup_configs
	  USING      (tenant_id::text = current_setting('app.tenant_id', true))
	  WITH CHECK (tenant_id::text = current_setting('app.tenant_id', true));
	`

	if _, err := dbAdmin.Exec(migration); err != nil {
		log.Fatalf("Schema migration failed: %v", err)
	}
}

func seedAdminUser() {
	// seedAdminUser runs with the admin pool (BYPASSRLS) so it can create the
	// first tenant + admin user before any tenant context exists. After the
	// first run, the COUNT check short-circuits and no inserts happen.
	var count int
	dbAdmin.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	if count > 0 {
		return
	}

	// Create default tenant
	var tenantID string
	err := dbAdmin.QueryRow(`
		INSERT INTO tenants (name, slug) VALUES ('Default Organization', 'default')
		ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
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

	_, err = dbAdmin.Exec(`
		INSERT INTO users (tenant_id, email, username, password_hash, full_name, role)
		VALUES ($1, $2, 'admin', $3, 'Administrator', 'admin')
		ON CONFLICT (email) DO UPDATE SET
			tenant_id = EXCLUDED.tenant_id,
			password_hash = EXCLUDED.password_hash,
			full_name = EXCLUDED.full_name,
			role = EXCLUDED.role
	`, tenantID, cfg.AdminEmail, string(hash))
	if err != nil {
		log.Fatalf("Failed to seed admin user: %v", err)
	}

	log.Printf("Default tenant and admin user seeded (%s)", cfg.AdminEmail)
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

// checkRole returns true if the current user's role meets the minimum required level.
// Role hierarchy: agent(0) < viewer(1) < technician(2) < admin(3)
func checkRole(r *http.Request, minRole string) bool {
	role := r.Context().Value(roleKey).(string)
	roleLevel := map[string]int{
		"agent":     0,
		"viewer":    1,
		"technician": 2,
		"admin":     3,
	}
	return roleLevel[role] >= roleLevel[minRole]
}

func denyIfUnauthorized(w http.ResponseWriter, r *http.Request, minRole string) bool {
	if !checkRole(r, minRole) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "insufficient permissions"})
		return true
	}
	return false
}

func getClaims(r *http.Request) *Claims {
	return r.Context().Value(claimsKey).(*Claims)
}

func getTenantID(r *http.Request) string {
	return r.Context().Value(tenantKey).(string)
}

// ─── Audit Logging ────────────────────────────────────────────────────────────

func clientIP(r *http.Request) string {
	ip := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		ip = strings.Split(fwd, ",")[0]
	}
	if host, _, err := net.SplitHostPort(ip); err == nil {
		return host
	}
	return ip
}

func auditLog(tenantID, userID, action, resourceType, resourceID, ipAddress string, details map[string]interface{}) {
	if tenantID == "" {
		return
	}
	detailsJSON, _ := json.Marshal(details)
	_ = WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO audit_log (tenant_id, user_id, action, resource_type, resource_id, ip_address, details)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, tenantID, userID, action, resourceType, resourceID, ipAddress, detailsJSON)
		return err
	})
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
	if err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO agents (id, tenant_id, status, last_seen)
			VALUES ($1, $2, 'online', $3)
			ON CONFLICT(id) DO UPDATE SET
				status = 'online',
				last_seen = EXCLUDED.last_seen
		`, agentID, tenantID, now)
		return err
	}); err != nil {
		log.Printf("upsertAgent error: %v", err)
	}
	seedBackupJob(tenantID, agentID)
}

func markAgentOffline(agentID string) {
	now := time.Now().UTC()
	// markAgentOffline needs the agent's tenant_id to scope the UPDATE
	// under RLS. Look it up via the admin pool (BYPASSRLS) — this
	// is a single lookup of an internal ID, not user-facing.
	var tenantID, hostname string
	if err := dbAdmin.QueryRow(`SELECT tenant_id, COALESCE(hostname,'') FROM agents WHERE id = $1`, agentID).Scan(&tenantID, &hostname); err != nil {
		log.Printf("markAgentOffline: cannot find tenant for agent %s: %v", agentID, err)
		return
	}
	if err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE agents SET status='offline', last_seen=$1 WHERE id=$2`, now, agentID)
		return err
	}); err != nil {
		log.Printf("markAgentOffline error: %v", err)
	}

	// Dedup: skip notification if already alerted offline in last 30 min
	fingerprint := fmt.Sprintf("offline-%s", agentID)
	var recentCount int
	_ = WithTenantRead(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			SELECT COUNT(*) FROM alerts
			WHERE fingerprint = $1 AND created_at > NOW() - INTERVAL '30 minutes'
		`, fingerprint).Scan(&recentCount)
	})
	if recentCount > 0 {
		return
	}

	saveAlert(tenantID, agentID, "critical",
		fmt.Sprintf("Agent offline: %s (%s)", hostname, agentID), fingerprint)
	tryNotify("Agent Offline",
		fmt.Sprintf("Alert: Agent %s is offline.", hostname))
}

func saveTelemetry(agentID, tenantID string, t TelemetryPayload) {
	now := time.Now().UTC()

	disksJSON, _ := json.Marshal(t.Disks)

	// All writes here go through the RLS wrapper so the agent's own
	// tenant context is what PostgreSQL sees. The agent JWT already
	// carries tenantID, so we don't need to look it up.
	_ = WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`
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
			t.GPUName, t.GPUDriver, agentID); err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO telemetry (agent_id, cpu_load, total_ram, free_ram, disk_total, disk_free, recorded_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, agentID, t.CPULoad, t.TotalRAM, t.FreeRAM, t.DiskTotal, t.DiskFree, now); err != nil {
			return err
		}
		return nil
	})

	// Alert checks: each alert creation is its own transaction with RLS.
	// CPU alert: usage >= 90% sustained across 2 consecutive readings (60s).
	// This filters momentary spikes (Defender scan, build, etc.).
	if t.CPULoad >= 90 {
		var previousHigh bool
		_ = WithTenantRead(tenantID, func(tx *sql.Tx) error {
			return tx.QueryRow(`
				SELECT cpu_load >= 90 FROM telemetry
				WHERE agent_id = $1
				ORDER BY recorded_at DESC
				LIMIT 1 OFFSET 1
			`, agentID).Scan(&previousHigh)
		})
		if previousHigh {
			fingerprint := fmt.Sprintf("cpu-high-%s", agentID)
			var recentCount int
			_ = WithTenantRead(tenantID, func(tx *sql.Tx) error {
				return tx.QueryRow(`
					SELECT COUNT(*) FROM alerts
					WHERE fingerprint = $1 AND created_at > NOW() - INTERVAL '10 minutes'
				`, fingerprint).Scan(&recentCount)
			})
			if recentCount == 0 {
				saveAlert(tenantID, agentID, "warning",
					fmt.Sprintf("CPU usage critical: %.0f%% on %s", t.CPULoad, t.Hostname), fingerprint)
				tryNotify("CPU Alert",
					fmt.Sprintf("Alert: CPU usage critical at %.0f%% on %s", t.CPULoad, t.Hostname))
			}
		}
	}

	// RAM alert: less than 10% free, sustained across 2 readings.
	if t.TotalRAM > 0 {
		ramFreePercent := float64(t.FreeRAM) / float64(t.TotalRAM) * 100
		if ramFreePercent < 10 {
			var prevRamLow bool
			_ = WithTenantRead(tenantID, func(tx *sql.Tx) error {
				return tx.QueryRow(`
					SELECT (free_ram::float / NULLIF(total_ram,0) * 100) < 10
					FROM telemetry
					WHERE agent_id = $1
					ORDER BY recorded_at DESC
					LIMIT 1 OFFSET 1
				`, agentID).Scan(&prevRamLow)
			})
			if prevRamLow {
				fingerprint := fmt.Sprintf("ram-high-%s", agentID)
				var recentCount int
				_ = WithTenantRead(tenantID, func(tx *sql.Tx) error {
					return tx.QueryRow(`
						SELECT COUNT(*) FROM alerts
						WHERE fingerprint = $1 AND created_at > NOW() - INTERVAL '10 minutes'
					`, fingerprint).Scan(&recentCount)
				})
			if recentCount == 0 {
				freeGB := float64(t.FreeRAM) / 1048576
				saveAlert(tenantID, agentID, "warning",
					fmt.Sprintf("RAM usage critical: %.0f%% free (%.1f GB) on %s", ramFreePercent, freeGB, t.Hostname), fingerprint)
				tryNotify("RAM Alert",
					fmt.Sprintf("Alert: RAM usage critical — %.0f%% free (%.1f GB) on %s", ramFreePercent, freeGB, t.Hostname))
			}
			}
		}
	}

	// Disk alert: less than 10% free, sustained across 2 readings.
	if t.DiskTotal > 0 {
		diskFreePercent := float64(t.DiskFree) / float64(t.DiskTotal) * 100
		if diskFreePercent < 10 {
			var prevDiskLow bool
			_ = WithTenantRead(tenantID, func(tx *sql.Tx) error {
				return tx.QueryRow(`
					SELECT (disk_free::float / NULLIF(disk_total,0) * 100) < 10
					FROM telemetry
					WHERE agent_id = $1
					ORDER BY recorded_at DESC
					LIMIT 1 OFFSET 1
				`, agentID).Scan(&prevDiskLow)
			})
			if prevDiskLow {
				fingerprint := fmt.Sprintf("disk-high-%s", agentID)
				var recentCount int
				_ = WithTenantRead(tenantID, func(tx *sql.Tx) error {
					return tx.QueryRow(`
						SELECT COUNT(*) FROM alerts
						WHERE fingerprint = $1 AND created_at > NOW() - INTERVAL '10 minutes'
					`, fingerprint).Scan(&recentCount)
				})
			if recentCount == 0 {
				freeGB := float64(t.DiskFree) / 1073741824
				saveAlert(tenantID, agentID, "warning",
					fmt.Sprintf("Disk usage critical: %.0f%% free (%.1f GB) on %s", diskFreePercent, freeGB, t.Hostname), fingerprint)
				tryNotify("Disk Alert",
					fmt.Sprintf("Alert: Disk usage critical — %.0f%% free (%.1f GB) on %s", diskFreePercent, freeGB, t.Hostname))
			}
			}
		}
	}
}

func tryNotify(subject, body string) {
	if cfg.SMTPHost == "" || len(cfg.AlertToEmails) == 0 {
		return
	}
	go notifier.SendAlert(notifier.SMTPConfig{
		Host:       cfg.SMTPHost,
		Port:       cfg.SMTPPort,
		User:       cfg.SMTPUser,
		Password:   cfg.SMTPPassword,
		From:       cfg.SMTPFrom,
		Recipients: cfg.AlertToEmails,
	}, subject, body)
}

func saveAlert(tenantID, agentID, severity, message, fingerprint string) int64 {
	now := time.Now().UTC()
	var id int64
	if err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			INSERT INTO alerts (tenant_id, agent_id, severity, message, fingerprint, created_at)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id
		`, tenantID, agentID, severity, message, fingerprint, now).Scan(&id)
	}); err != nil {
		log.Printf("saveAlert error: %v", err)
		return 0
	}
	broadcastEvent(severity, message, agentID)
	return id
}

func seedBackupJob(tenantID, agentID string) {
	var count int
	dbAdmin.QueryRow(`SELECT COUNT(*) FROM backup_jobs WHERE agent_id = $1 AND tenant_id = $2`, agentID, tenantID).Scan(&count)
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
		dbAdmin.Exec(`
			INSERT INTO backup_jobs (tenant_id, agent_id, name, location, type, status, cron, created_at)
			VALUES ($1, $2, $3, $4, $5, 'completed', $6, $7)
		`, tenantID, agentID, j.name, j.location, j.typ, j.cron, now)
	}
}

// ─── Backup Scheduler ────────────────────────────────────────────────────────

func parseNextCronRun(cronExpr string) *time.Time {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(cronExpr)
	if err != nil {
		log.Printf("backup-scheduler: invalid cron expression '%s': %v", cronExpr, err)
		return nil
	}
	next := schedule.Next(time.Now().UTC())
	return &next
}

func startBackupScheduler() {
	log.Printf("backup-scheduler: starting (check interval: 60s)")

	// Set initial next_run_time for jobs that don't have one.
	// INTENTIONAL: runs across ALL tenants (dbAdmin bypasses RLS).
	// This is a global scheduler bootstrap — no per-tenant filter needed.
	_, err := dbAdmin.Exec(`
		UPDATE backup_jobs
		SET next_run_time = COALESCE(next_run_time, now())
		WHERE next_run_time IS NULL AND status != 'running'
	`)
	if err != nil {
		log.Printf("backup-scheduler: failed to initialize next_run_time: %v", err)
	}

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		executePendingBackupJobs()
	}
}

func executePendingBackupJobs() {
	// INTENTIONAL: scans ALL tenants for due jobs (dbAdmin bypasses RLS).
	// `tenant_id` is returned as a column and passed to the per-job goroutine
	// so each executeBackupJobAsync can scope its own queries to the right tenant.
	rows, err := dbAdmin.Query(`
		SELECT id, tenant_id, agent_id, name, location, type, cron
		FROM backup_jobs
		WHERE next_run_time <= NOW() AND status != 'running'
		LIMIT 10
	`)
	if err != nil {
		log.Printf("backup-scheduler: query failed: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var tenantID, agentID, name, location, typ, cronExpr string
		if err := rows.Scan(&id, &tenantID, &agentID, &name, &location, &typ, &cronExpr); err != nil {
			continue
		}

		// Mark as running
		dbAdmin.Exec(`UPDATE backup_jobs SET status = 'running' WHERE id = $1`, id)

		go executeBackupJobAsync(id, tenantID, agentID, name, location, typ, cronExpr)
	}
}

func executeBackupJobAsync(jobID int64, tenantID, agentID, name, location, typ, cronExpr string) {
	agentID = normalizeAgentID(agentID)
	log.Printf("backup-scheduler: executing job %d (%s) on agent %s", jobID, name, agentID)

	// Calculate next run time BEFORE executing
	nextRun := parseNextCronRun(cronExpr)
	if nextRun != nil {
		dbAdmin.Exec(`UPDATE backup_jobs SET next_run_time = $1 WHERE id = $2`, *nextRun, jobID)
	}

	// Check if agent is connected
	agentsMu.Lock()
	agent, ok := agents[normalizeAgentID(agentID)]
	agentsMu.Unlock()

	if !ok {
		log.Printf("backup-scheduler: agent %s not connected, marking job %d as pending", agentID, jobID)
		dbAdmin.Exec(`UPDATE backup_jobs SET status = 'pending' WHERE id = $1`, jobID)
		return
	}

	// Send backup command to agent with config
	var repoURL string
	var repoPassword string
	var sourcePaths []string

	// Explicit tenant_id filter required: dbAdmin bypasses RLS so without it
	// we could read another tenant's backup config if agent_ids ever collided.
	row := dbAdmin.QueryRow(`SELECT repo_url, repo_password_enc, source_paths FROM agent_backup_configs WHERE agent_id = $1 AND tenant_id = $2`, agentID, tenantID)
	var repoPasswordEnc []byte
	var srcPaths []string
	if err := row.Scan(&repoURL, &repoPasswordEnc, pq.Array(&srcPaths)); err == nil {
		if len(srcPaths) > 0 {
			sourcePaths = srcPaths
		}
		if len(repoPasswordEnc) > 0 {
			if pwd, err := decryptPassword(repoPasswordEnc); err == nil {
				repoPassword = pwd
			}
		}
	}
	if location != "" && len(sourcePaths) == 0 {
		sourcePaths = []string{location}
	}

	payloadBytes, _ := json.Marshal(map[string]interface{}{
		"jobId":       jobID,
		"sourcePaths": sourcePaths,
		"jobType":     typ,
		"repoUrl":     repoURL,
		"password":    repoPassword,
	})

	msg := Message{
		AgentID: agentID,
		Type:    "backup_command",
		Payload: string(payloadBytes),
	}
	msgBytes, _ := json.Marshal(msg)

	agent.Mu.Lock()
	err := agent.Conn.WriteMessage(websocket.TextMessage, msgBytes)
	agent.Mu.Unlock()

	if err != nil {
		log.Printf("backup-scheduler: failed to send backup command to agent %s: %v", agentID, err)
		dbAdmin.Exec(`UPDATE backup_jobs SET status = 'failed' WHERE id = $1`, jobID)
		return
	}

	// Update executed_at
	now := time.Now().UTC()
	dbAdmin.Exec(`UPDATE backup_jobs SET executed_at = $1 WHERE id = $2`, now, jobID)

	auditLog(tenantID, "system:scheduler", "backup.run", "backup_job", fmt.Sprintf("%d", jobID), "", map[string]interface{}{
		"agentId": agentID,
		"name":    name,
	})
}

func handleBackupStatusMessage(payload string) {
	log.Printf("backup-scheduler: progress message: %s", payload)
}

func handleBackupResultMessage(tenantID, agentID, payload string) {
	var result struct {
		JobID      int64  `json:"jobId"`
		Status     string `json:"status"`
		SizeBytes  int64  `json:"sizeBytes"`
		SnapshotID string `json:"snapshotId"`
		Error      string `json:"error"`
	}

	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		log.Printf("backup-scheduler: invalid backup_result payload: %v", err)
		return
	}

	if err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		if result.Status == "completed" {
			_, err := tx.Exec(`
				UPDATE backup_jobs SET status = 'completed', size_bytes = $1 WHERE id = $2
			`, result.SizeBytes, result.JobID)
			if err == nil {
				log.Printf("backup-scheduler: job %d completed (size: %d bytes)", result.JobID, result.SizeBytes)
			}
			return err
		}
		_, err := tx.Exec(`
			UPDATE backup_jobs SET status = 'failed' WHERE id = $1
		`, result.JobID)
		if err == nil {
			log.Printf("backup-scheduler: job %d failed: %s", result.JobID, result.Error)
		}
		return err
	}); err != nil {
		log.Printf("backup-scheduler: failed to update job %d: %v", result.JobID, err)
		return
	}

	if result.Status != "completed" {
		var hostname string
		dbAdmin.QueryRow(`SELECT COALESCE(hostname,'') FROM agents WHERE id = $1`, agentID).Scan(&hostname)
		msg := fmt.Sprintf("Backup failed on %s: %s", hostname, result.Error)
		saveAlert(tenantID, agentID, "error", msg, fmt.Sprintf("backup-fail-%s-%d", agentID, result.JobID))
		tryNotify("Backup Failure", msg)
	}
}

// ─── WebSocket Event Hub ──────────────────────────────────────────────────────

type ClientEventConnection struct {
	Conn *websocket.Conn
}

var (
	eventClients         = make(map[*ClientEventConnection]bool)
	eventClientsMu       sync.Mutex
	snapshotRequests     = make(map[string]chan string)
	snapshotRequestsMu   sync.Mutex
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
	agent, ok := agents[normalizeAgentID(agentID)]
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
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	ip := clientIP(r)

	if rateLimitedIP(ip) {
		recordLoginAttempt(ip, "ip", false)
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
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

	loginField := req.Username
	if loginField == "" {
		loginField = req.Email
	}

	if rateLimitedUser(loginField) {
		recordLoginAttempt(loginField, "user", false)
		recordLoginAttempt(ip, "ip", false)
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	var hash, userID, tenantID, role string
	err := dbAdmin.QueryRow(`
		SELECT id, tenant_id, password_hash, role FROM users
		WHERE (username = $1 OR email = $1) AND is_active = true
	`, loginField).Scan(&userID, &tenantID, &hash, &role)
	if err != nil {
		recordLoginAttempt(loginField, "user", false)
		recordLoginAttempt(ip, "ip", false)
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		recordLoginAttempt(loginField, "user", false)
		recordLoginAttempt(ip, "ip", false)
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	// Record success to reset counters for this user and IP
	recordLoginAttempt(loginField, "user", true)
	recordLoginAttempt(ip, "ip", true)

	// Update last login
	if err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE users SET last_login = NOW() WHERE id = $1`, userID)
		return err
	}); err != nil {
		log.Printf("Failed to update last_login: %v", err)
	}

	// Audit log
	auditLog(tenantID, userID, "login", "user", userID, ip, nil)

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
	if denyIfUnauthorized(w, r, "technician") { return }
	tenantID := getTenantID(r)

	list := []AgentInfo{}
	err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		rows, err := tx.Query(`
			SELECT id, hostname, os, cpu_model, cpu_load, total_ram, free_ram,
			       disk_total, disk_free, COALESCE(disks, '[]'),
			       status, COALESCE(last_seen::text, ''),
			       COALESCE(vendor,''), COALESCE(model,''), COALESCE(serial_number,''),
			       COALESCE(uptime,''), COALESCE(kernel_version,''), COALESCE(agent_version,''),
			       COALESCE(local_ip,''), COALESCE(mac_address,''), COALESCE(gateway,''),
			       COALESCE(num_cpu, 0),
			       COALESCE(gpu_name,''), COALESCE(gpu_driver,'')
			FROM agents
			ORDER BY status DESC, last_seen DESC
		`)
		if err != nil {
			return err
		}
		defer rows.Close()

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
		return nil
	})
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func handleListAlerts(w http.ResponseWriter, r *http.Request) {
	if denyIfUnauthorized(w, r, "technician") { return }
	tenantID := getTenantID(r)
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}

	agentID := r.URL.Query().Get("agent_id")

	list := []AlertRow{}
	err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		var rows *sql.Rows
		var err error

		// RLS filters by tenant_id; the explicit WHERE is no longer needed.
		// The optional `agent_id` filter stays — it's a column filter, not
		// a tenant isolation concern.
		if agentID != "" {
			rows, err = tx.Query(`
				SELECT id, agent_id, severity, message, COALESCE(created_at::text, '')
				FROM alerts
				WHERE agent_id = $1
				ORDER BY id DESC
				LIMIT $2
			`, agentID, limit)
		} else {
			rows, err = tx.Query(`
				SELECT id, agent_id, severity, message, COALESCE(created_at::text, '')
				FROM alerts
				ORDER BY id DESC
				LIMIT $1
			`, limit)
		}
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var a AlertRow
			if err := rows.Scan(&a.ID, &a.AgentID, &a.Severity, &a.Message, &a.Time); err != nil {
				continue
			}
			list = append(list, a)
		}
		return nil
	})
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
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
	err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			SELECT id, agent_id, severity, message, COALESCE(created_at::text, '')
			FROM alerts
			WHERE id = $1
		`, alertID).Scan(&a.ID, &a.AgentID, &a.Severity, &a.Message, &a.Time)
	})
	if err != nil {
		http.Error(w, `{"error":"alert not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(a)
}

func handleListTenants(w http.ResponseWriter, r *http.Request) {
	if denyIfUnauthorized(w, r, "technician") { return }
	tenantID := getTenantID(r)

	err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		rows, err := tx.Query(`
			SELECT t.id, t.name, t.slug,
			       COALESCE((SELECT COUNT(*) FROM agents a WHERE a.tenant_id = t.id), 0) AS device_count
			FROM tenants t
			WHERE t.id = $1
			ORDER BY t.name
		`, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()

		type tenantRow struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Slug        string `json:"slug"`
			DeviceCount int    `json:"deviceCount"`
		}
		list := []tenantRow{}
		for rows.Next() {
			var t tenantRow
			if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.DeviceCount); err != nil {
				continue
			}
			list = append(list, t)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
		return nil
	})
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}
}

func handleGetAgent(w http.ResponseWriter, r *http.Request) {
	if denyIfUnauthorized(w, r, "technician") { return }
	tenantID := getTenantID(r)
	agentID := r.URL.Query().Get("id")
	if agentID == "" {
		http.Error(w, `{"error":"missing id"}`, http.StatusBadRequest)
		return
	}

	var a AgentInfo
	err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			SELECT id, hostname, os, cpu_model, cpu_load, total_ram, free_ram,
			       disk_total, disk_free, status, COALESCE(last_seen::text, ''),
			       COALESCE(vendor,''), COALESCE(model,''), COALESCE(serial_number,''),
			       COALESCE(uptime,''), COALESCE(kernel_version,''), COALESCE(agent_version,''),
			       COALESCE(local_ip,''), COALESCE(mac_address,''), COALESCE(gateway,''),
			       COALESCE(num_cpu, 0)
			FROM agents
			WHERE id = $1
		`, agentID).Scan(&a.ID, &a.Hostname, &a.OS, &a.CPUModel,
			&a.CPULoad, &a.TotalRAM, &a.FreeRAM, &a.DiskTotal, &a.DiskFree,
			&a.Status, &a.LastSeen,
			&a.Vendor, &a.Model, &a.SerialNumber,
			&a.Uptime, &a.KernelVersion, &a.AgentVersion,
			&a.LocalIP, &a.MACAddress, &a.Gateway, &a.NumCPU)
	})
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

	// Verify agent belongs to this tenant (RLS-protected EXISTS check)
	var exists bool
	if err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM agents WHERE id=$1)`, agentID).Scan(&exists)
	}); err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	interval := r.URL.Query().Get("interval")

	// Aggregated mode (for charts)
	if from != "" && to != "" && interval != "" {
		type AggRow struct {
			Bucket    string  `json:"bucket"`
			CPUAvg    float64 `json:"cpu_avg"`
			RAMAvg    float64 `json:"ram_avg"`
			DiskAvg   float64 `json:"disk_avg"`
		}

		list := []AggRow{}
		if err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
			rows, err := tx.Query(`
				SELECT
					date_trunc($1, recorded_at)::text AS bucket,
					AVG(cpu_load) AS cpu_avg,
					AVG((total_ram - free_ram)::real * 100 / NULLIF(total_ram, 0)) AS ram_avg,
					AVG((disk_total - disk_free)::real * 100 / NULLIF(disk_total, 0)) AS disk_avg
				FROM telemetry
				WHERE agent_id = $2 AND recorded_at BETWEEN $3 AND $4
				GROUP BY bucket
				ORDER BY bucket
			`, interval, agentID, from, to)
			if err != nil {
				return err
			}
			defer rows.Close()

			for rows.Next() {
				var row AggRow
				if err := rows.Scan(&row.Bucket, &row.CPUAvg, &row.RAMAvg, &row.DiskAvg); err != nil {
					continue
				}
				list = append(list, row)
			}
			return nil
		}); err != nil {
			http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
		return
	}

	// Raw mode (last 100 rows, backward compatible)
	type Row struct {
		CPULoad    float64 `json:"cpuLoad"`
		TotalRAM   uint64  `json:"totalRam"`
		FreeRAM    uint64  `json:"freeRam"`
		DiskTotal  uint64  `json:"diskTotal"`
		DiskFree   uint64  `json:"diskFree"`
		RecordedAt string  `json:"recordedAt"`
	}

	list := []Row{}
	if err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		rows, err := tx.Query(`
			SELECT cpu_load, total_ram, free_ram, disk_total, disk_free, recorded_at::text
			FROM telemetry
			WHERE agent_id = $1
			ORDER BY id DESC
			LIMIT 100
		`, agentID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var row Row
			if err := rows.Scan(&row.CPULoad, &row.TotalRAM, &row.FreeRAM,
				&row.DiskTotal, &row.DiskFree, &row.RecordedAt); err != nil {
				continue
			}
			list = append(list, row)
		}
		return nil
	}); err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func handleListBackups(w http.ResponseWriter, r *http.Request) {
	if denyIfUnauthorized(w, r, "technician") { return }
	tenantID := getTenantID(r)

	list := []BackupJob{}
	err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		rows, err := tx.Query(`
			SELECT id, agent_id, COALESCE(name,''), COALESCE(location,''), type, status,
			       size_bytes, cron, COALESCE(executed_at::text,''), COALESCE(next_run_time::text,''), COALESCE(created_at::text,'')
			FROM backup_jobs
			ORDER BY id DESC
		`)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var b BackupJob
			if err := rows.Scan(&b.ID, &b.AgentID, &b.Name, &b.Location, &b.Type, &b.Status,
				&b.SizeBytes, &b.Cron, &b.ExecutedAt, &b.NextRunTime, &b.CreatedAt); err != nil {
				continue
			}
			list = append(list, b)
		}
		return nil
	})
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func handleRunBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if denyIfUnauthorized(w, r, "admin") { return }

	tenantID := getTenantID(r)
	claims := getClaims(r)

	var req struct {
		AgentID    string `json:"agentId"`
		SourcePath string `json:"sourcePath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AgentID == "" {
		http.Error(w, `{"error":"missing agentId"}`, http.StatusBadRequest)
		return
	}
	if req.SourcePath == "" {
		req.SourcePath = "/backups/manual"
	}

	now := time.Now().UTC()

	var jobID int64
	if err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM agents WHERE id=$1)`, req.AgentID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("agent not found")
		}
		return tx.QueryRow(`
			INSERT INTO backup_jobs (tenant_id, agent_id, name, location, type, status, cron, executed_at, created_at)
			VALUES ($1, $2, 'Manual-Backup', $3, 'full', 'running', '@manual', $4, $4)
			RETURNING id
		`, tenantID, req.AgentID, req.SourcePath, now).Scan(&jobID)
	}); err != nil {
		if err.Error() == "agent not found" {
			http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
		} else {
			http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		}
		return
	}

	// Dispatch immediately to the agent
	go executeBackupJobAsync(jobID, tenantID, req.AgentID, "Manual-Backup", req.SourcePath, "full", "@manual")

	auditLog(tenantID, claims.UserID, "backup.run", "agent", req.AgentID, clientIP(r), nil)
	saveAlert(tenantID, req.AgentID, "info", "Manual backup job started", "")

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"queued"}`))
}

// ─── Backup Config ────────────────────────────────────────────────────────────

func handleGetBackupConfig(w http.ResponseWriter, r *http.Request) {
	if denyIfUnauthorized(w, r, "admin") { return }
	tenantID := getTenantID(r)
	agentID := getAgentIDFromPath(r.URL.Path)

	var cfg BackupConfig
	err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		row := tx.QueryRow(`
			SELECT COALESCE(repo_url,''), COALESCE(source_paths,'{}'::text[]), COALESCE(cron,'0 2 * * *'),
			       COALESCE(retention_days,30), COALESCE(enabled,true)
			FROM agent_backup_configs WHERE agent_id = $1
		`, agentID)
		return row.Scan(&cfg.RepoURL, pq.Array(&cfg.SourcePaths), &cfg.Cron, &cfg.RetentionDays, &cfg.Enabled)
	})
	if err == sql.ErrNoRows {
		cfg = BackupConfig{Cron: "0 2 * * *", RetentionDays: 30, Enabled: true}
	} else if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}

func handleUpdateBackupConfig(w http.ResponseWriter, r *http.Request) {
	if denyIfUnauthorized(w, r, "admin") { return }
	tenantID := getTenantID(r)
	agentID := getAgentIDFromPath(r.URL.Path)

	var req struct {
		RepoURL       string   `json:"repoUrl"`
		RepoPassword  string   `json:"repoPassword"`
		SourcePaths   []string `json:"sourcePaths"`
		Cron          string   `json:"cron"`
		RetentionDays int      `json:"retentionDays"`
		Enabled       bool     `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if req.Cron == "" {
		req.Cron = "0 2 * * *"
	}
	if req.RetentionDays <= 0 {
		req.RetentionDays = 30
	}
	if req.SourcePaths == nil {
		req.SourcePaths = []string{}
	}

	var passwordEnc []byte
	if req.RepoPassword != "" {
		var err error
		passwordEnc, err = encryptPassword(req.RepoPassword)
		if err != nil {
			http.Error(w, `{"error":"encryption failed"}`, http.StatusInternalServerError)
			return
		}
	}

	err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		var existingID int64
		err := tx.QueryRow(`SELECT id FROM agent_backup_configs WHERE agent_id = $1`, agentID).Scan(&existingID)
		if err == sql.ErrNoRows {
			_, err = tx.Exec(`
				INSERT INTO agent_backup_configs (tenant_id, agent_id, repo_url, repo_password_enc, source_paths, cron, retention_days, enabled)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			`, tenantID, agentID, req.RepoURL, passwordEnc, pq.Array(req.SourcePaths), req.Cron, req.RetentionDays, req.Enabled)
		} else if err != nil {
			return err
		} else {
			_, err = tx.Exec(`
				UPDATE agent_backup_configs SET repo_url = $1,
					repo_password_enc = COALESCE($2, repo_password_enc),
					source_paths = $3, cron = $4, retention_days = $5, enabled = $6, updated_at = NOW()
				WHERE id = $7
			`, req.RepoURL, passwordEnc, pq.Array(req.SourcePaths), req.Cron, req.RetentionDays, req.Enabled, existingID)
		}
		return err
	})
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ─── Agent Backup Jobs ────────────────────────────────────────────────────────

func handleListAgentBackups(w http.ResponseWriter, r *http.Request) {
	if denyIfUnauthorized(w, r, "technician") { return }
	tenantID := getTenantID(r)
	agentID := getAgentIDFromPath(r.URL.Path)

	list := []BackupJob{}
	err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		rows, err := tx.Query(`
			SELECT id, agent_id, COALESCE(name,''), COALESCE(location,''), type, status,
			       size_bytes, cron, COALESCE(executed_at::text,''), COALESCE(next_run_time::text,''), COALESCE(created_at::text,'')
			FROM backup_jobs WHERE agent_id = $1
			ORDER BY id DESC LIMIT 50
		`, agentID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var b BackupJob
			if err := rows.Scan(&b.ID, &b.AgentID, &b.Name, &b.Location, &b.Type, &b.Status,
				&b.SizeBytes, &b.Cron, &b.ExecutedAt, &b.NextRunTime, &b.CreatedAt); err != nil {
				continue
			}
			list = append(list, b)
		}
		return nil
	})
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func handleCreateAgentBackupJob(w http.ResponseWriter, r *http.Request) {
	if denyIfUnauthorized(w, r, "admin") { return }
	tenantID := getTenantID(r)
	agentID := getAgentIDFromPath(r.URL.Path)

	var req struct {
		Name     string `json:"name"`
		Location string `json:"location"`
		Type     string `json:"type"`
		Cron     string `json:"cron"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Location == "" {
		http.Error(w, `{"error":"name and location required"}`, http.StatusBadRequest)
		return
	}
	if req.Type == "" {
		req.Type = "full"
	}
	if req.Cron == "" {
		req.Cron = "0 2 * * *"
	}

	var jobID int64
	err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			INSERT INTO backup_jobs (tenant_id, agent_id, name, location, type, status, cron, created_at)
			VALUES ($1, $2, $3, $4, $5, 'completed', $6, NOW())
			RETURNING id
		`, tenantID, agentID, req.Name, req.Location, req.Type, req.Cron).Scan(&jobID)
	})
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"id": jobID, "status": "created"})
}

func handleRunAgentBackup(w http.ResponseWriter, r *http.Request) {
	if denyIfUnauthorized(w, r, "admin") { return }
	tenantID := getTenantID(r)
	claims := getClaims(r)
	agentID := getAgentIDFromPath(r.URL.Path)

	var req struct {
		SourcePath string `json:"sourcePath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.SourcePath == "" {
		req.SourcePath = "/backups/manual"
	}
	if req.SourcePath == "" {
		req.SourcePath = "/backups/manual"
	}

	now := time.Now().UTC()
	var jobID int64
	err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			INSERT INTO backup_jobs (tenant_id, agent_id, name, location, type, status, cron, executed_at, created_at)
			VALUES ($1, $2, 'Manual-Backup', $3, 'full', 'running', '@manual', $4, $4)
			RETURNING id
		`, tenantID, agentID, req.SourcePath, now).Scan(&jobID)
	})
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	go executeBackupJobAsync(jobID, tenantID, agentID, "Manual-Backup", req.SourcePath, "full", "@manual")

	auditLog(tenantID, claims.UserID, "backup.run", "agent", agentID, clientIP(r), nil)
	saveAlert(tenantID, agentID, "info", "Manual backup started from device page", "")

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"queued"}`))
}

// ─── Delete Backup Job ────────────────────────────────────────────────────────

func handleDeleteBackupJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if denyIfUnauthorized(w, r, "admin") { return }
	tenantID := getTenantID(r)

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/backup-jobs/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, `{"error":"missing job id"}`, http.StatusBadRequest)
		return
	}
	jobID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid job id"}`, http.StatusBadRequest)
		return
	}

	err = WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM backup_jobs WHERE id = $1`, jobID)
		return err
	})
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"deleted"}`))
}

// ─── Snapshot List & Restore ──────────────────────────────────────────────────

func handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if denyIfUnauthorized(w, r, "technician") { return }
	tenantID := getTenantID(r)
	agentID := getAgentIDFromPath(r.URL.Path)

	agentsMu.Lock()
	agentConn, ok := agents[normalizeAgentID(agentID)]
	agentsMu.Unlock()
	if !ok {
		http.Error(w, `{"error":"agent not connected"}`, http.StatusServiceUnavailable)
		return
	}

	var repoURL, repoPassword string
	var passwordEnc []byte
	_ = WithTenantRead(tenantID, func(tx *sql.Tx) error {
		row := tx.QueryRow(`SELECT repo_url, repo_password_enc FROM agent_backup_configs WHERE agent_id = $1`, agentID)
		if err := row.Scan(&repoURL, &passwordEnc); err == nil && len(passwordEnc) > 0 {
			if pwd, err := decryptPassword(passwordEnc); err == nil {
				repoPassword = pwd
			}
		}
		return nil
	})

	payloadBytes, _ := json.Marshal(map[string]interface{}{
		"repoUrl":  repoURL,
		"password": repoPassword,
	})
	msg := Message{AgentID: agentID, Type: "list_snapshots", Payload: string(payloadBytes)}
	msgBytes, _ := json.Marshal(msg)

	agentConn.Mu.Lock()
	err := agentConn.Conn.WriteMessage(websocket.TextMessage, msgBytes)
	agentConn.Mu.Unlock()
	if err != nil {
		http.Error(w, `{"error":"failed to send to agent"}`, http.StatusInternalServerError)
		return
	}

	ch := make(chan string, 1)
	snapshotRequestsMu.Lock()
	snapshotRequests[normalizeAgentID(agentID)] = ch
	snapshotRequestsMu.Unlock()

	select {
	case result := <-ch:
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(result))
	case <-time.After(120 * time.Second):
		snapshotRequestsMu.Lock()
		delete(snapshotRequests, normalizeAgentID(agentID))
		snapshotRequestsMu.Unlock()
		http.Error(w, `{"error":"timeout waiting for snapshot list"}`, http.StatusGatewayTimeout)
	}
}

func handleRestoreSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if denyIfUnauthorized(w, r, "admin") { return }
	tenantID := getTenantID(r)
	claims := getClaims(r)
	agentID := getAgentIDFromPath(r.URL.Path)

	var req struct {
		SnapshotID  string `json:"snapshotId"`
		Destination string `json:"destination"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SnapshotID == "" || req.Destination == "" {
		http.Error(w, `{"error":"snapshotId and destination required"}`, http.StatusBadRequest)
		return
	}

	agentsMu.Lock()
	agentConn, ok := agents[normalizeAgentID(agentID)]
	agentsMu.Unlock()
	if !ok {
		http.Error(w, `{"error":"agent not connected"}`, http.StatusServiceUnavailable)
		return
	}

	var repoURL, repoPassword string
	var passwordEnc []byte
	_ = WithTenantRead(tenantID, func(tx *sql.Tx) error {
		row := tx.QueryRow(`SELECT repo_url, repo_password_enc FROM agent_backup_configs WHERE agent_id = $1`, agentID)
		if err := row.Scan(&repoURL, &passwordEnc); err == nil && len(passwordEnc) > 0 {
			if pwd, err := decryptPassword(passwordEnc); err == nil {
				repoPassword = pwd
			}
		}
		return nil
	})

	payloadBytes, _ := json.Marshal(map[string]interface{}{
		"snapshotId":  req.SnapshotID,
		"destination": req.Destination,
		"repoUrl":     repoURL,
		"password":    repoPassword,
	})
	msg := Message{AgentID: agentID, Type: "restore_command", Payload: string(payloadBytes)}
	msgBytes, _ := json.Marshal(msg)

	agentConn.Mu.Lock()
	err := agentConn.Conn.WriteMessage(websocket.TextMessage, msgBytes)
	agentConn.Mu.Unlock()
	if err != nil {
		http.Error(w, `{"error":"failed to send to agent"}`, http.StatusInternalServerError)
		return
	}

	auditLog(tenantID, claims.UserID, "backup.restore", "agent", agentID, clientIP(r), map[string]interface{}{
		"snapshotId":  req.SnapshotID,
		"destination": req.Destination,
	})

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"restore started"}`))
}

func handleAcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if denyIfUnauthorized(w, r, "technician") { return }

	tenantID := getTenantID(r)
	claims := getClaims(r)

	var req struct {
		AlertID int64 `json:"alertId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	var rowsAffected int64
	if err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		result, err := tx.Exec(`
			UPDATE alerts SET acknowledged = true, acknowledged_by = $1, acknowledged_at = NOW()
			WHERE id = $2
		`, claims.UserID, req.AlertID)
		if err != nil {
			return err
		}
		rowsAffected, _ = result.RowsAffected()
		return nil
	}); err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	if rowsAffected == 0 {
		http.Error(w, `{"error":"alert not found"}`, http.StatusNotFound)
		return
	}

	auditLog(tenantID, claims.UserID, "alert.acknowledge", "alert", fmt.Sprintf("%d", req.AlertID), clientIP(r), nil)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"acknowledged"}`))
}

// ─── Enrollment Handlers ──────────────────────────────────────────────────────

// ─── Scripts ───────────────────────────────────────────────────────────────────

func handleScripts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleListScripts(w, r)
	case http.MethodPost:
		handleCreateScript(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func handleListScripts(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r)
	if denyIfUnauthorized(w, r, "technician") { return }

	type Script struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Command     string `json:"command"`
		Language    string `json:"language"`
		CreatedBy   string `json:"createdBy"`
		CreatedAt   string `json:"createdAt"`
		UpdatedAt   string `json:"updatedAt"`
	}

	scripts := []Script{}
	err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		rows, err := tx.Query(`
			SELECT id, name, COALESCE(description,''), command, language, COALESCE(created_by::text,''), created_at::text, updated_at::text
			FROM scripts
			ORDER BY created_at DESC
		`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s Script
			if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.Command, &s.Language, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt); err != nil {
				return err
			}
			scripts = append(scripts, s)
		}
		return nil
	})
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(scripts)
}

func handleCreateScript(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r)
	claims := getClaims(r)
	if denyIfUnauthorized(w, r, "admin") { return }

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Command     string `json:"command"`
		Language    string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Command == "" {
		http.Error(w, `{"error":"name and command are required"}`, http.StatusBadRequest)
		return
	}
	if req.Language == "" {
		req.Language = "powershell"
	}

	var scriptID int64
	if err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			INSERT INTO scripts (tenant_id, name, description, command, language, created_by)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id
		`, tenantID, req.Name, req.Description, req.Command, req.Language, claims.UserID).Scan(&scriptID)
	}); err != nil {
		log.Printf("handleCreateScript: %v", err)
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	auditLog(tenantID, claims.UserID, "script.create", "script", fmt.Sprintf("%d", scriptID), clientIP(r), map[string]interface{}{
		"name": req.Name,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int64{"id": scriptID})
}

func handleScriptRoutes(w http.ResponseWriter, r *http.Request) {
	// Path: /api/scripts/{id} or /api/scripts/{id}/run
	path := strings.TrimPrefix(r.URL.Path, "/api/scripts/")
	parts := strings.Split(path, "/")

	if len(parts) == 2 && parts[1] == "run" {
		handleRunScript(w, r, parts[0])
		return
	}

	// GET, PUT, DELETE /api/scripts/{id}
	scriptID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid script id"}`, http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		handleGetScript(w, r, scriptID)
	case http.MethodPut:
		handleUpdateScript(w, r, scriptID)
	case http.MethodDelete:
		handleDeleteScript(w, r, scriptID)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func handleGetScript(w http.ResponseWriter, r *http.Request, scriptID int64) {
	tenantID := getTenantID(r)
	if denyIfUnauthorized(w, r, "technician") { return }

	var s struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Command     string `json:"command"`
		Language    string `json:"language"`
		CreatedBy   string `json:"createdBy"`
		CreatedAt   string `json:"createdAt"`
		UpdatedAt   string `json:"updatedAt"`
	}
	err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			SELECT id, name, COALESCE(description,''), command, language, COALESCE(created_by::text,''), created_at::text, updated_at::text
			FROM scripts WHERE id = $1
		`, scriptID).Scan(&s.ID, &s.Name, &s.Description, &s.Command, &s.Language, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
	})
	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"script not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

func handleUpdateScript(w http.ResponseWriter, r *http.Request, scriptID int64) {
	tenantID := getTenantID(r)
	claims := getClaims(r)
	if denyIfUnauthorized(w, r, "admin") { return }

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Command     string `json:"command"`
		Language    string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		result, err := tx.Exec(`
			UPDATE scripts SET name = $1, description = $2, command = $3, language = $4, updated_at = NOW()
			WHERE id = $5
		`, req.Name, req.Description, req.Command, req.Language, scriptID)
		if err != nil {
			return err
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			return sql.ErrNoRows
		}
		return nil
	})
	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"script not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	auditLog(tenantID, claims.UserID, "script.update", "script", fmt.Sprintf("%d", scriptID), clientIP(r), nil)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"updated"}`))
}

func handleDeleteScript(w http.ResponseWriter, r *http.Request, scriptID int64) {
	tenantID := getTenantID(r)
	claims := getClaims(r)
	if denyIfUnauthorized(w, r, "admin") { return }

	err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		result, err := tx.Exec(`DELETE FROM scripts WHERE id = $1`, scriptID)
		if err != nil {
			return err
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			return sql.ErrNoRows
		}
		return nil
	})
	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"script not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	auditLog(tenantID, claims.UserID, "script.delete", "script", fmt.Sprintf("%d", scriptID), clientIP(r), nil)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"deleted"}`))
}

func handleRunScript(w http.ResponseWriter, r *http.Request, scriptIDStr string) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if denyIfUnauthorized(w, r, "technician") { return }

	tenantID := getTenantID(r)
	claims := getClaims(r)

	scriptID, err := strconv.ParseInt(scriptIDStr, 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid script id"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		AgentID string `json:"agentId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AgentID == "" {
		http.Error(w, `{"error":"missing agentId"}`, http.StatusBadRequest)
		return
	}

	// Fetch script details inside a transaction
	var scriptName, command, language string
	if err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			SELECT name, command, language FROM scripts WHERE id = $1
		`, scriptID).Scan(&scriptName, &command, &language)
	}); err == sql.ErrNoRows {
		http.Error(w, `{"error":"script not found"}`, http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	// Create execution record
	now := time.Now().UTC()
	var executionID int64
	if err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		// Verify agent belongs to this tenant
		var exists bool
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM agents WHERE id=$1)`, req.AgentID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("agent not found")
		}
		return tx.QueryRow(`
			INSERT INTO script_executions (tenant_id, script_id, agent_id, executed_by, status, started_at)
			VALUES ($1, $2, $3, $4, 'running', $5)
			RETURNING id
		`, tenantID, scriptID, req.AgentID, claims.UserID, now).Scan(&executionID)
	}); err != nil {
		if err.Error() == "agent not found" {
			http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
		} else {
			http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		}
		return
	}

	// Send to agent via WebSocket
	agentID := normalizeAgentID(req.AgentID)
	agentsMu.Lock()
	agent, ok := agents[agentID]
	agentsMu.Unlock()

	if !ok {
		// Mark execution as failed
		WithTenantWrite(tenantID, func(tx *sql.Tx) error {
			_, err := tx.Exec(`UPDATE script_executions SET status = 'failed', finished_at = NOW() WHERE id = $1`, executionID)
			return err
		})
		http.Error(w, `{"error":"agent not connected"}`, http.StatusServiceUnavailable)
		return
	}

	payloadBytes, _ := json.Marshal(map[string]interface{}{
		"scriptId":        scriptID,
		"executionId":     executionID,
		"command":         command,
		"language":        language,
		"timeoutSeconds":  15,
		"maxOutputBytes":  65536,
	})

	msg := Message{
		AgentID: agentID,
		Type:    "script_command",
		Payload: string(payloadBytes),
	}
	msgBytes, _ := json.Marshal(msg)

	agent.Mu.Lock()
	err = agent.Conn.WriteMessage(websocket.TextMessage, msgBytes)
	agent.Mu.Unlock()
	if err != nil {
		log.Printf("Failed to send script_command to agent %s: %v", agentID, err)
		WithTenantWrite(tenantID, func(tx *sql.Tx) error {
			_, err := tx.Exec(`UPDATE script_executions SET status = 'failed', finished_at = NOW() WHERE id = $1`, executionID)
			return err
		})
		http.Error(w, `{"error":"failed to send command to agent"}`, http.StatusInternalServerError)
		return
	}

	auditLog(tenantID, claims.UserID, "script.run", "script", fmt.Sprintf("%d", scriptID), clientIP(r), map[string]interface{}{
		"executionId": executionID,
		"agentId":     req.AgentID,
		"command":     command,
		"language":    language,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"executionId": executionID,
		"status":      "running",
	})
}

func handleScriptResultMessage(tenantID, agentID, payload string) {
	var result struct {
		ExecutionID     int64  `json:"executionId"`
		Status          string `json:"status"`
		ExitCode        int    `json:"exitCode"`
		Output          string `json:"output"`
		OutputTruncated bool   `json:"outputTruncated"`
		DurationMs      int    `json:"durationMs"`
		Error           string `json:"error"`
	}

	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		log.Printf("script_exec: invalid script_result payload: %v", err)
		return
	}

	if err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			UPDATE script_executions SET status = $1, exit_code = $2, output = $3, output_truncated = $4, duration_ms = $5, finished_at = NOW()
			WHERE id = $6
		`, result.Status, result.ExitCode, result.Output, result.OutputTruncated, result.DurationMs, result.ExecutionID)
		return err
	}); err != nil {
		log.Printf("script_exec: failed to update execution %d: %v", result.ExecutionID, err)
	}
}

func handleListScriptExecutions(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r)
	if denyIfUnauthorized(w, r, "technician") { return }

	type Execution struct {
		ID              int64  `json:"id"`
		ScriptID        int64  `json:"scriptId"`
		ScriptName      string `json:"scriptName"`
		AgentID         string `json:"agentId"`
		AgentHostname   string `json:"agentHostname"`
		ExecutedBy      string `json:"executedBy"`
		Status          string `json:"status"`
		ExitCode        int    `json:"exitCode"`
		Output          string `json:"output"`
		OutputTruncated bool   `json:"outputTruncated"`
		DurationMs      int    `json:"durationMs"`
		StartedAt       string `json:"startedAt"`
		FinishedAt      string `json:"finishedAt"`
	}

	limit := 50
	offset := 0
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 200 {
		limit = l
	}
	if o, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && o >= 0 {
		offset = o
	}
	scriptFilter := r.URL.Query().Get("scriptId")
	agentFilter := r.URL.Query().Get("agentId")

	executions := []Execution{}
	err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		query := `
			SELECT se.id, se.script_id, COALESCE(s.name,''), se.agent_id, COALESCE(a.hostname,''), COALESCE(se.executed_by::text,''),
				se.status, COALESCE(se.exit_code,0), COALESCE(se.output,''), COALESCE(se.output_truncated,false), COALESCE(se.duration_ms,0),
				COALESCE(se.started_at::text,''), COALESCE(se.finished_at::text,'')
			FROM script_executions se
			LEFT JOIN scripts s ON se.script_id = s.id
			LEFT JOIN agents a ON se.agent_id = a.id
			WHERE 1=1
		`
		args := []interface{}{}
		argIdx := 1
		if scriptFilter != "" {
			sid, err := strconv.ParseInt(scriptFilter, 10, 64)
			if err == nil {
				query += fmt.Sprintf(" AND se.script_id = $%d", argIdx)
				args = append(args, sid)
				argIdx++
			}
		}
		if agentFilter != "" {
			query += fmt.Sprintf(" AND se.agent_id = $%d", argIdx)
			args = append(args, agentFilter)
			argIdx++
		}
		query += fmt.Sprintf(" ORDER BY se.created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
		args = append(args, limit, offset)

		rows, err := tx.Query(query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e Execution
			if err := rows.Scan(&e.ID, &e.ScriptID, &e.ScriptName, &e.AgentID, &e.AgentHostname, &e.ExecutedBy,
				&e.Status, &e.ExitCode, &e.Output, &e.OutputTruncated, &e.DurationMs,
				&e.StartedAt, &e.FinishedAt); err != nil {
				return err
			}
			executions = append(executions, e)
		}
		return nil
	})
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(executions)
}

func handleCreateRegistrationToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if denyIfUnauthorized(w, r, "admin") { return }

	tenantID := getTenantID(r)
	claims := getClaims(r)

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
	if err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			INSERT INTO registration_tokens (tenant_id, token_hash, label, created_by, expires_at, max_uses)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id
		`, tenantID, tokenHash, req.Label, claims.UserID, expiresAt, req.MaxUses).Scan(&tokenID)
	}); err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	auditLog(tenantID, claims.UserID, "token.create", "registration_token", tokenID, clientIP(r), nil)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":     rawToken,
		"tokenId":   tokenID,
		"expiresAt": expiresAt.Format(time.RFC3339),
	})
}

// BYPASS RLS: handleEnrollAgent runs against the admin pool because
// the agent does not yet exist (no JWT, no tenant context). The tenant
// is determined by the enrollment token, which is verified inside this
// handler. RLS would block the SELECT on registration_tokens and the
// INSERT on agents in this scenario. The security model: a valid token
// from one tenant can only create an agent for that same tenant.
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

	err := dbAdmin.QueryRow(`
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

	// Check if an agent with the same hostname already exists in this tenant.
	// If so, reuse it instead of creating a duplicate. This prevents the same
	// machine from appearing as multiple devices when re-enrolling.
	var agentID string
	var existingAgentID string
	err = dbAdmin.QueryRow(`
		SELECT id FROM agents
		WHERE tenant_id = $1 AND hostname = $2
		LIMIT 1
	`, tenantID, req.Info.Hostname).Scan(&existingAgentID)

	now := time.Now().UTC()

	if err == nil && existingAgentID != "" {
		// Reuse existing agent — update metadata and bring back online.
		agentID = existingAgentID
		_, err = dbAdmin.Exec(`
			UPDATE agents SET
				os = $1, cpu_model = $2, status = 'online',
				last_seen = $3, enrollment_token_id = $4
			WHERE id = $5
		`, req.Info.OS, req.Info.CPUModel, now, tokenID, agentID)
		if err != nil {
			http.Error(w, `{"error":"failed to update agent"}`, http.StatusInternalServerError)
			return
		}
		log.Printf("Re-enrolled existing agent %s (hostname=%s)", agentID, req.Info.Hostname)
	} else {
		// No existing agent — create a new one.
		agentID, err = generateRandomToken(16)
		if err != nil {
			http.Error(w, `{"error":"failed to generate agent ID"}`, http.StatusInternalServerError)
			return
		}

		_, err = dbAdmin.Exec(`
			INSERT INTO agents (id, tenant_id, enrollment_token_id, hostname, os, cpu_model, status, last_seen, enrolled_at)
			VALUES ($1, $2, $3, $4, $5, $6, 'online', $7, $7)
		`, agentID, tenantID, tokenID, req.Info.Hostname, req.Info.OS, req.Info.CPUModel, now)
		if err != nil {
			http.Error(w, `{"error":"failed to register agent"}`, http.StatusInternalServerError)
			return
		}
		log.Printf("Enrolled new agent %s (hostname=%s)", agentID, req.Info.Hostname)

		// Increment token use count only for new agents.
		dbAdmin.Exec(`UPDATE registration_tokens SET use_count = use_count + 1, used_at = NOW() WHERE id = $1`, tokenID)
	}

	auditLog(tenantID, "", "agent.enroll", "agent", agentID, clientIP(r), map[string]interface{}{
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
	agents[normalizeAgentID(agentID)] = agentConn
	agentsMu.Unlock()

	upsertAgent(tenantID, agentID)
	log.Printf("Agent connected: %s (tenant: %s)", agentID, tenantID)

	defer func() {
		agentsMu.Lock()
		delete(agents, normalizeAgentID(agentID))
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
			saveTelemetry(agentID, tenantID, t)
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
			// telemetry has no tenant_id column; its RLS policy uses a
			// subquery against agents. RLS itself does the tenant check
			// via that subquery — we just need to be inside a transaction
			// with the agent's tenant context set.
			if err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
				for _, entry := range history {
					var t TelemetryPayload
					if err := json.Unmarshal([]byte(entry.Payload), &t); err != nil {
						continue
					}
					if _, err := tx.Exec(`
						INSERT INTO telemetry (agent_id, cpu_load, total_ram, free_ram, disk_total, disk_free, recorded_at)
						VALUES ($1, $2, $3, $4, $5, $6, $7)
					`, agentID, t.CPULoad, t.TotalRAM, t.FreeRAM, t.DiskTotal, t.DiskFree, entry.CreatedAt); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				log.Printf("telemetry_history write error: %v", err)
			}
			broadcastToFrontend(agentID, msgBytes)

		case "terminal_output":
			// Append output to the active terminal session recording.
			// The frontend also receives this via broadcastToFrontend,
			// but the recording must be written server-side since the
			// agent's terminal_output never crosses handleTerminalWebSocket.
			outputEvent, _ := json.Marshal(map[string]interface{}{
				"t":    0,
				"type": "terminal_output",
				"data": msg.Payload,
			})
			WithTenantWrite(tenantID, func(tx *sql.Tx) error {
				_, e := tx.Exec(`
					UPDATE terminal_sessions
					SET recording = recording || $1::jsonb
					WHERE agent_id = $2 AND status = 'active' AND tenant_id = $3
				`, string(outputEvent), agentID, tenantID)
				return e
			})
			broadcastToFrontend(agentID, msgBytes)

		case "backup_status":
			log.Printf("Backup progress from agent %s: %s", agentID, msg.Payload)
			handleBackupStatusMessage(msg.Payload)
			broadcastToFrontend(agentID, msgBytes)

		case "backup_result":
			log.Printf("Backup result from agent %s", agentID)
			handleBackupResultMessage(tenantID, agentID, msg.Payload)
			broadcastToFrontend(agentID, msgBytes)

		case "snapshot_list":
			log.Printf("Snapshot list from agent %s", agentID)
			snapshotRequestsMu.Lock()
			if ch, ok := snapshotRequests[normalizeAgentID(agentID)]; ok {
				ch <- msg.Payload
				delete(snapshotRequests, normalizeAgentID(agentID))
			}
			snapshotRequestsMu.Unlock()
			broadcastToFrontend(agentID, msgBytes)

		case "restore_status":
			log.Printf("Restore progress from agent %s: %s", agentID, msg.Payload)
			broadcastToFrontend(agentID, msgBytes)

		case "restore_result":
			log.Printf("Restore result from agent %s", agentID)
			broadcastToFrontend(agentID, msgBytes)

		case "screen_frame":
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

			// BUGFIX: the original code did `var tenantID string` here,
			// which shadowed the outer tenantID (from the JWT at the
			// top of handleAgentConnection). The shadowed variable was
			// populated by an RLS-blocked lookup (returns NULL), so the
			// subsequent DELETE/INSERTs used an empty tenant_id, which
			// failed the policy's WITH CHECK. This was silently dropping
			// every software_list message from the agent since RLS was
			// enabled. Use the outer tenantID directly.
			if err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
				if _, err := tx.Exec(
					`DELETE FROM agent_software WHERE agent_id = $1`, agentID,
				); err != nil {
					return err
				}
				for _, s := range softwareItems {
					if _, err := tx.Exec(`
						INSERT INTO agent_software (tenant_id, agent_id, name, publisher, version, install_date, estimated_size_kb, quiet_uninstall_string)
						VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
					`, tenantID, agentID, s.Name, s.Publisher, s.Version, s.InstallDate, s.EstimatedSizeKB, s.QuietUninstallString); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				log.Printf("software_list write error: %v", err)
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

			// Same shadowing fix as software_list above.
			if err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
				_, err := tx.Exec(`
					INSERT INTO agent_logs (tenant_id, agent_id, level, log_type, message)
					VALUES ($1, $2, $3, $4, $5)
				`, tenantID, agentID, logEntry.Level, logEntry.LogType, logEntry.Message)
				return err
			}); err != nil {
				log.Printf("agent_log write error: %v", err)
			}

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

			// Same shadowing fix as software_list above.
			if err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
				if _, err := tx.Exec(
					`DELETE FROM agent_patches WHERE agent_id = $1`, agentID,
				); err != nil {
					return err
				}
				for _, p := range patchItems {
					if _, err := tx.Exec(`
						INSERT INTO agent_patches (tenant_id, agent_id, kb_id, name, severity, description, installed, installed_at)
						VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
					`, tenantID, agentID, p.KbID, p.Name, p.Severity, p.Description, p.Installed, p.InstalledAt); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				log.Printf("patch_list write error: %v", err)
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

			_ = WithTenantWrite(tenantID, func(tx *sql.Tx) error {
				_, err := tx.Exec(`
					UPDATE agent_checks SET status = $1, last_output = $2, last_run = NOW()
					WHERE id = $3
				`, result.Status, result.Output, result.CheckID)
				return err
			})
			broadcastToFrontend(agentID, msgBytes)

		case "software_uninstall_result":
			log.Printf("Software uninstall result from agent %s: %s", agentID, msg.Payload)
			broadcastToFrontend(agentID, msgBytes)

		case "script_result":
			log.Printf("Script result from agent %s", agentID)
			handleScriptResultMessage(tenantID, agentID, msg.Payload)
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
	tenantID := getTenantID(r)
	claims := getClaims(r)
	if denyIfUnauthorized(w, r, "technician") { return }
	agentID := r.URL.Query().Get("id")
	if agentID == "" {
		http.Error(w, `{"error":"missing agent id"}`, http.StatusBadRequest)
		return
	}
	log.Printf("terminal: ws open, agent=%s user=%s", agentID, claims.UserID)

	// Resolve normalized agent ID (stripped dashes) to canonical form.
	normalized := normalizeAgentID(agentID)
	agentsMu.Lock()
	agentConn, agentOK := agents[normalized]
	agentsMu.Unlock()
	if !agentOK {
		http.Error(w, `{"error":"agent not found or offline"}`, http.StatusNotFound)
		return
	}
	if agentConn.TenantID != tenantID {
		http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
		return
	}

	// ─── Create terminal session ──────────────────────────────────────────
	var sessionID int64
	if err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			INSERT INTO terminal_sessions (tenant_id, agent_id, user_id, status)
			VALUES ($1, $2, $3, 'active')
			RETURNING id
		`, tenantID, agentID, claims.UserID).Scan(&sessionID)
	}); err != nil {
		log.Printf("terminal: failed to create session: %v", err)
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	auditLog(tenantID, claims.UserID, "terminal.open", "terminal_session",
		fmt.Sprintf("%d", sessionID), clientIP(r), map[string]interface{}{
			"agentId": agentID,
		})

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("terminal: upgrade failed: %v", err)
		WithTenantWrite(tenantID, func(tx *sql.Tx) error {
			_, e := tx.Exec(`UPDATE terminal_sessions SET status='failed', ended_at=NOW() WHERE id=$1`, sessionID)
			return e
		})
		return
	}
	defer conn.Close()

	// Register this frontend WebSocket so broadcastToFrontend can deliver
	// terminal_output from the agent to this specific frontend connection.
	feSessionID := fmt.Sprintf("terminal-%d", sessionID)
	frontendsMu.Lock()
	frontends[feSessionID] = &FrontendSession{Conn: conn, TenantID: tenantID}
	frontendsMu.Unlock()
	defer func() {
		frontendsMu.Lock()
		delete(frontends, feSessionID)
		frontendsMu.Unlock()
	}()

	startedAt := time.Now()
	var recordingMu sync.Mutex
	recording := make([]map[string]interface{}, 0, 1024)
	flushRecording := func() {
		recordingMu.Lock()
		events := recording
		recording = make([]map[string]interface{}, 0, 1024)
		recordingMu.Unlock()
		if len(events) == 0 {
			return
		}
		eventsJSON, _ := json.Marshal(events)
		WithTenantWrite(tenantID, func(tx *sql.Tx) error {
			_, e := tx.Exec(
				`UPDATE terminal_sessions SET recording = recording || $1::jsonb WHERE id=$2`,
				string(eventsJSON), sessionID)
			return e
		})
	}

	// Periodic recording flush + session timeouts.
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	lastActivity := time.Now()
	const inactiveTimeout = 30 * time.Minute
	const hardMaxDuration = 4 * time.Hour

	closeSession := func(status, closedBy string) {
		flushRecording()
		durationSec := int(time.Since(startedAt).Seconds())
		WithTenantWrite(tenantID, func(tx *sql.Tx) error {
			_, e := tx.Exec(`
				UPDATE terminal_sessions
				SET status=$1, closed_by=$2, duration_sec=$3, ended_at=NOW()
				WHERE id=$4
			`, status, closedBy, durationSec, sessionID)
			return e
		})
		auditLog(tenantID, claims.UserID, "terminal.close", "terminal_session",
			fmt.Sprintf("%d", sessionID), clientIP(r), map[string]interface{}{
				"status":    status,
				"closedBy":  closedBy,
				"duration":  durationSec,
				"agentId":   agentID,
			})
		// Notify frontend and agent.
		reason := ""
		switch closedBy {
		case "inactivity":
			reason = "timeout (30 min inactivity)"
		case "max_duration":
			reason = "timeout (4 hour max duration)"
		default:
			reason = closedBy
		}
		closePayload, _ := json.Marshal(map[string]string{
			"agentId": agentID,
			"type":    "terminal_closed",
			"payload": reason,
		})
		conn.WriteMessage(websocket.TextMessage, closePayload)
		agentConn.Mu.Lock()
		agentConn.Conn.WriteMessage(websocket.TextMessage, closePayload)
		agentConn.Mu.Unlock()
	}

	// Read messages in a goroutine so the ticker can fire while idle.
	type wsMsg struct {
		data []byte
		err  error
	}
	msgCh := make(chan wsMsg, 1)
	go func() {
		for {
			_, msgBytes, err := conn.ReadMessage()
			msgCh <- wsMsg{data: msgBytes, err: err}
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-ticker.C:
			flushRecording()
			if time.Since(lastActivity) > inactiveTimeout {
				log.Printf("terminal: session %d inactive, closing", sessionID)
				closeSession("timeout", "inactivity")
				return
			}
			if time.Since(startedAt) > hardMaxDuration {
				log.Printf("terminal: session %d exceeded 4h max, closing", sessionID)
				closeSession("timeout", "max_duration")
				return
			}

		case wm := <-msgCh:
			if wm.err != nil {
				flushRecording()
				durationSec := int(time.Since(startedAt).Seconds())
				WithTenantWrite(tenantID, func(tx *sql.Tx) error {
					_, e := tx.Exec(`
						UPDATE terminal_sessions SET status='closed', duration_sec=$1, ended_at=NOW()
						WHERE id=$2 AND status='active'
					`, durationSec, sessionID)
					return e
				})
				auditLog(tenantID, claims.UserID, "terminal.close", "terminal_session",
					fmt.Sprintf("%d", sessionID), clientIP(r), map[string]interface{}{
						"status":   "closed",
						"duration": durationSec,
						"agentId":  agentID,
					})
				closePayload, _ := json.Marshal(map[string]string{
					"agentId": agentID,
					"type":    "terminal_closed",
					"payload": "disconnected",
				})
				if agentConn != nil && agentConn.Conn != nil {
					agentConn.Mu.Lock()
					err := agentConn.Conn.WriteMessage(websocket.TextMessage, closePayload)
					agentConn.Mu.Unlock()
					if err != nil {
						log.Printf("terminal: failed to send terminal_closed to agent: %v", err)
					} else {
						log.Printf("terminal: sent terminal_closed to agent %s", agentID)
					}
				}
				return
			}

			var cmd struct {
				AgentID string `json:"agentId"`
				Type    string `json:"type"`
				Payload string `json:"payload"`
			}
			if err := json.Unmarshal(wm.data, &cmd); err != nil {
				continue
			}
			if cmd.AgentID != agentID {
				continue
			}

			lastActivity = time.Now()
			elapsedMs := int(time.Since(startedAt).Milliseconds())

			recordingMu.Lock()
			if cmd.Type == "terminal_input" || cmd.Type == "terminal_output" {
				recording = append(recording, map[string]interface{}{
					"t":    elapsedMs,
					"type": cmd.Type,
					"data": cmd.Payload,
				})
			}
			recordingMu.Unlock()

			agentConn.Mu.Lock()
			agentConn.Conn.WriteMessage(websocket.TextMessage, wm.data)
			agentConn.Mu.Unlock()
		}
	}
}

// ─── Remote Screen WebSocket ─────────────────────────────────────────────────

func handleScreenWebSocket(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r)
	claims := getClaims(r)
	if denyIfUnauthorized(w, r, "technician") { return }
	agentID := r.URL.Query().Get("id")
	if agentID == "" {
		http.Error(w, `{"error":"missing agent id"}`, http.StatusBadRequest)
		return
	}
	log.Printf("screen: ws open, agent=%s user=%s", agentID, claims.UserID)

	// Verify agent exists and belongs to this tenant
	normalized := normalizeAgentID(agentID)
	agentsMu.Lock()
	agentConn, agentOK := agents[normalized]
	agentsMu.Unlock()
	if !agentOK {
		http.Error(w, `{"error":"agent not found or offline"}`, http.StatusNotFound)
		return
	}
	if agentConn.TenantID != tenantID {
		http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
		return
	}

	auditLog(tenantID, claims.UserID, "screen.open", "agent", agentID, clientIP(r), map[string]interface{}{
		"agentId": agentID,
	})

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("screen: upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Register frontend session so broadcastToFrontend delivers screen_frame
	feSessionID := fmt.Sprintf("screen-%s-%s", agentID, claims.UserID)
	frontendsMu.Lock()
	frontends[feSessionID] = &FrontendSession{Conn: conn, TenantID: tenantID}
	frontendsMu.Unlock()
	defer func() {
		frontendsMu.Lock()
		delete(frontends, feSessionID)
		frontendsMu.Unlock()
		auditLog(tenantID, claims.UserID, "screen.close", "agent", agentID, clientIP(r), map[string]interface{}{
			"agentId": agentID,
		})
	}()

	// Send screen_start to agent to begin capture with defaults
	startPayload, _ := json.Marshal(Message{
		AgentID: agentID,
		Type:    "screen_start",
		Payload: `{"quality":50,"fps":5}`,
	})
	agentConn.Mu.Lock()
	if err := agentConn.Conn.WriteMessage(websocket.TextMessage, startPayload); err != nil {
		agentConn.Mu.Unlock()
		log.Printf("screen: failed to send screen_start to agent %s: %v", agentID, err)
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"screen_error","payload":"agent unavailable"}`))
		return
	}
	agentConn.Mu.Unlock()

	// Read loop: relay screen_input events from frontend to agent
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			log.Printf("screen: frontend disconnected, agent=%s", agentID)
			break
		}

		// Parse to extract agentId and type, relay to agent
		var frontendMsg struct {
			AgentID string `json:"agentId"`
			Type    string `json:"type"`
			Payload string `json:"payload"`
		}
		if err := json.Unmarshal(data, &frontendMsg); err != nil {
			continue
		}
		if frontendMsg.AgentID != agentID {
			continue
		}

		agentConn.Mu.Lock()
		agentConn.Conn.WriteMessage(websocket.TextMessage, data)
		agentConn.Mu.Unlock()
	}

	// Send screen_stop to agent when frontend disconnects
	stopPayload, _ := json.Marshal(Message{
		AgentID: agentID,
		Type:    "screen_stop",
	})
	agentConn.Mu.Lock()
	agentConn.Conn.WriteMessage(websocket.TextMessage, stopPayload)
	agentConn.Mu.Unlock()
}

func handleUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleListUsers(w, r)
	case http.MethodPost:
		handleCreateUser(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func handleListUsers(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r)
	if denyIfUnauthorized(w, r, "admin") { return }

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
	err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		rows, err := tx.Query(`
			SELECT id, email, username, COALESCE(full_name,''), role, is_active, COALESCE(last_login::text,''), created_at::text
			FROM users
			ORDER BY created_at DESC
		`)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var u UserInfo
			if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.FullName, &u.Role, &u.IsActive, &u.LastLogin, &u.CreatedAt); err != nil {
				continue
			}
			list = append(list, u)
		}
		return nil
	})
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func handleCreateUser(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r)
	if denyIfUnauthorized(w, r, "admin") { return }

	var body struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
		FullName string `json:"fullName"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if body.Email == "" || body.Username == "" || body.Password == "" {
		http.Error(w, `{"error":"email, username, and password are required"}`, http.StatusBadRequest)
		return
	}

	if len(body.Password) < 8 {
		http.Error(w, `{"error":"password must be at least 8 characters"}`, http.StatusBadRequest)
		return
	}

	role := body.Role
	if role != "admin" && role != "technician" && role != "viewer" {
		role = "technician"
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, `{"error":"failed to hash password"}`, http.StatusInternalServerError)
		return
	}

	var userID string
	err = WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			INSERT INTO users (tenant_id, email, username, password_hash, full_name, role)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id
		`, tenantID, body.Email, body.Username, string(hash), body.FullName, role).Scan(&userID)
	})
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			http.Error(w, `{"error":"email or username already exists"}`, http.StatusConflict)
			return
		}
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": userID})
}

func handleUserRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/users/")
	if path == "" || path == "/" {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(path, "/")
	userID := parts[0]

	if userID == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodPut:
		handleUpdateUser(w, r, userID)
	case http.MethodDelete:
		handleDeleteUser(w, r, userID)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func handleUpdateUser(w http.ResponseWriter, r *http.Request, userID string) {
	tenantID := getTenantID(r)
	claims := getClaims(r)

	isAdmin := claims.Role == "admin"
	isSelf := claims.UserID == userID

	if !isAdmin && !isSelf {
		http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
		return
	}

	var body struct {
		Email    *string `json:"email"`
		Username *string `json:"username"`
		FullName *string `json:"fullName"`
		Role     *string `json:"role"`
		Password *string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Whitelist: non-admin callers can only update fullName
	if !isAdmin {
		body = struct {
			Email    *string `json:"email"`
			Username *string `json:"username"`
			FullName *string `json:"fullName"`
			Role     *string `json:"role"`
			Password *string `json:"password"`
		}{FullName: body.FullName}
	}

	err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		// Build dynamic SET clause
		sets := []string{}
		args := []interface{}{}
		argIdx := 1

		if body.Email != nil {
			sets = append(sets, fmt.Sprintf("email = $%d", argIdx))
			args = append(args, *body.Email)
			argIdx++
		}
		if body.Username != nil {
			sets = append(sets, fmt.Sprintf("username = $%d", argIdx))
			args = append(args, *body.Username)
			argIdx++
		}
		if body.FullName != nil {
			sets = append(sets, fmt.Sprintf("full_name = $%d", argIdx))
			args = append(args, *body.FullName)
			argIdx++
		}
		if body.Role != nil {
			role := *body.Role
			if role != "admin" && role != "technician" && role != "viewer" {
				role = "technician"
			}
			sets = append(sets, fmt.Sprintf("role = $%d", argIdx))
			args = append(args, role)
			argIdx++
		}
		if body.Password != nil {
			hash, err := bcrypt.GenerateFromPassword([]byte(*body.Password), bcrypt.DefaultCost)
			if err != nil {
				return fmt.Errorf("hash error: %w", err)
			}
			sets = append(sets, fmt.Sprintf("password_hash = $%d", argIdx))
			args = append(args, string(hash))
			argIdx++
		}

		if len(sets) == 0 {
			return nil
		}

		sets = append(sets, "updated_at = NOW()")
		args = append(args, userID)

		query := fmt.Sprintf(`
			UPDATE users SET %s
			WHERE id = $%d
		`, strings.Join(sets, ", "), argIdx)

		result, err := tx.Exec(query, args...)
		if err != nil {
			if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
				return fmt.Errorf("duplicate: %w", err)
			}
			return err
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			return fmt.Errorf("not found")
		}
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		} else if strings.Contains(err.Error(), "duplicate") {
			http.Error(w, `{"error":"email or username already exists"}`, http.StatusConflict)
		} else if strings.Contains(err.Error(), "hash error") {
			http.Error(w, `{"error":"failed to hash password"}`, http.StatusInternalServerError)
		} else {
			http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

func handleDeleteUser(w http.ResponseWriter, r *http.Request, userID string) {
	tenantID := getTenantID(r)
	claims := getClaims(r)

	if denyIfUnauthorized(w, r, "admin") { return }

	if claims.UserID == userID {
		http.Error(w, `{"error":"cannot delete yourself"}`, http.StatusBadRequest)
		return
	}

	// Last-admin guard: count admins in this tenant before deleting
	var adminCount int
	err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM users WHERE tenant_id = $1 AND role = 'admin'`, tenantID).Scan(&adminCount)
	})
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	// Check if target user is an admin
	var targetRole string
	err2 := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT role FROM users WHERE id = $1`, userID).Scan(&targetRole)
	})
	if err2 != nil {
		http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		return
	}

	if targetRole == "admin" && adminCount <= 1 {
		http.Error(w, `{"error":"cannot delete the last admin of the tenant"}`, http.StatusBadRequest)
		return
	}

	err = WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		result, err := tx.Exec(`DELETE FROM users WHERE id = $1`, userID)
		if err != nil {
			return err
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			return fmt.Errorf("not found")
		}
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		} else {
			http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

// ─── Agent Sub-routes ────────────────────────────────────────────────────────

func handleAgentRoutes(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasSuffix(path, "/software") && r.Method == http.MethodGet {
		if denyIfUnauthorized(w, r, "technician") { return }
		handleListSoftware(w, r)
	} else if strings.HasSuffix(path, "/software/scan") && r.Method == http.MethodPost {
		if denyIfUnauthorized(w, r, "admin") { return }
		handleScanSoftware(w, r)
	} else if strings.Contains(path, "/software/") && strings.HasSuffix(path, "/uninstall") && r.Method == http.MethodPost {
		if denyIfUnauthorized(w, r, "admin") { return }
		handleUninstallSoftware(w, r)
	} else if strings.HasSuffix(path, "/notes") && r.Method == http.MethodGet {
		if denyIfUnauthorized(w, r, "technician") { return }
		handleListNotes(w, r)
	} else if strings.HasSuffix(path, "/notes") && r.Method == http.MethodPost {
		if denyIfUnauthorized(w, r, "technician") { return }
		handleCreateNote(w, r)
	} else if strings.HasSuffix(path, "/logs") && r.Method == http.MethodGet {
		if denyIfUnauthorized(w, r, "technician") { return }
		handleListLogs(w, r)
	} else if strings.HasSuffix(path, "/audit") && r.Method == http.MethodGet {
		if denyIfUnauthorized(w, r, "technician") { return }
		handleListAudit(w, r)
	} else if strings.HasSuffix(path, "/patches") && r.Method == http.MethodGet {
		if denyIfUnauthorized(w, r, "technician") { return }
		handleListPatches(w, r)
	} else if strings.HasSuffix(path, "/patches/scan") && r.Method == http.MethodPost {
		if denyIfUnauthorized(w, r, "admin") { return }
		handleScanPatches(w, r)
	} else if strings.HasSuffix(path, "/checks") && r.Method == http.MethodGet {
		if denyIfUnauthorized(w, r, "technician") { return }
		handleListChecks(w, r)
	} else if strings.HasSuffix(path, "/checks") && r.Method == http.MethodPost {
		if denyIfUnauthorized(w, r, "admin") { return }
		handleCreateCheck(w, r)
	} else if strings.HasSuffix(path, "/backup-config") && r.Method == http.MethodGet {
		handleGetBackupConfig(w, r)
	} else if strings.HasSuffix(path, "/backup-config") && r.Method == http.MethodPut {
		handleUpdateBackupConfig(w, r)
	} else if strings.HasSuffix(path, "/backups/run") && r.Method == http.MethodPost {
		handleRunAgentBackup(w, r)
	} else if strings.HasSuffix(path, "/backups/snapshots") && r.Method == http.MethodPost {
		handleListSnapshots(w, r)
	} else if strings.HasSuffix(path, "/backups/restore") && r.Method == http.MethodPost {
		handleRestoreSnapshot(w, r)
	} else if strings.HasSuffix(path, "/backups") && r.Method == http.MethodGet {
		handleListAgentBackups(w, r)
	} else if strings.HasSuffix(path, "/backups") && r.Method == http.MethodPost {
		handleCreateAgentBackupJob(w, r)
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

func normalizeAgentID(s string) string {
	return strings.ReplaceAll(s, "-", "")
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
	var total int
	err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		// RLS filters by tenant_id; the explicit `AND tenant_id = $2`
		// is removed to avoid the "filter that looks explicit but
		// depends on RLS" inconsistency.
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM agent_software WHERE agent_id = $1`,
			agentID,
		).Scan(&total); err != nil {
			return err
		}
		rows, err := tx.Query(`
			SELECT id, name, COALESCE(publisher,''), COALESCE(version,''),
			       COALESCE(install_date,''), COALESCE(estimated_size_kb, 0),
			       COALESCE(quiet_uninstall_string,''), scanned_at::text
			FROM agent_software
			WHERE agent_id = $1
			ORDER BY name ASC
			LIMIT $2 OFFSET $3
		`, agentID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var s SoftwareRow
			if err := rows.Scan(&s.ID, &s.Name, &s.Publisher, &s.Version,
				&s.InstallDate, &s.EstimatedSizeKB, &s.QuietUninstallString, &s.ScannedAt); err != nil {
				continue
			}
			list = append(list, s)
		}
		return nil
	})
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
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
	if denyIfUnauthorized(w, r, "admin") { return }
	agentID := normalizeAgentID(getAgentIDFromPath(r.URL.Path))
	tenantID := getTenantID(r)

	// CRITICAL: verify the agent exists in the caller's tenant BEFORE
	// looking up the in-memory WebSocket map. The map is global, not
	// per-tenant, so without this check an admin from Tenant B could
	// send a scan command to an agent belonging to Tenant A.
	var exists bool
	if err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1)`, agentID).Scan(&exists)
	}); err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, `{"error":"agent not found in this tenant"}`, http.StatusNotFound)
		return
	}

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
		if denyIfUnauthorized(w, r, "admin") { return }
		handleUpdateNote(w, r, noteID)
	} else if len(parts) == 1 && r.Method == http.MethodDelete {
		if denyIfUnauthorized(w, r, "admin") { return }
		handleDeleteNote(w, r, noteID)
	} else {
		http.NotFound(w, r)
	}
}

func handleListNotes(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentIDFromPath(r.URL.Path)
	tenantID := getTenantID(r)
	userID := getClaims(r).UserID

	type NoteRow struct {
		ID        int64  `json:"id"`
		Content   string `json:"content"`
		UserName  string `json:"userName"`
		CreatedAt string `json:"createdAt"`
		UpdatedAt string `json:"updatedAt"`
	}

	list := []NoteRow{}
	err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		// Both agent_notes and users are RLS-protected; the JOIN works
		// because RLS applies to both tables independently.
		rows, err := tx.Query(`
			SELECT n.id, n.content, COALESCE(u.username, 'unknown'),
			       n.created_at::text, n.updated_at::text
			FROM agent_notes n
			LEFT JOIN users u ON n.user_id = u.id
			WHERE n.agent_id = $1
			ORDER BY n.updated_at DESC
		`, agentID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var n NoteRow
			if err := rows.Scan(&n.ID, &n.Content, &n.UserName, &n.CreatedAt, &n.UpdatedAt); err != nil {
				continue
			}
			list = append(list, n)
		}
		return nil
	})
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
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
	if err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			INSERT INTO agent_notes (tenant_id, agent_id, user_id, content)
			VALUES ($1, $2, $3, $4) RETURNING id
		`, tenantID, agentID, userID, body.Content).Scan(&noteID)
	}); err != nil {
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

	var rowsAffected int64
	if err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		result, err := tx.Exec(`
			UPDATE agent_notes SET content = $1, updated_at = NOW()
			WHERE id = $2
		`, body.Content, noteID)
		if err != nil {
			return err
		}
		rowsAffected, _ = result.RowsAffected()
		return nil
	}); err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	if rowsAffected == 0 {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

func handleDeleteNote(w http.ResponseWriter, r *http.Request, noteID string) {
	tenantID := getTenantID(r)

	var rowsAffected int64
	if err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		result, err := tx.Exec(`
			DELETE FROM agent_notes WHERE id = $1
		`, noteID)
		if err != nil {
			return err
		}
		rowsAffected, _ = result.RowsAffected()
		return nil
	}); err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	if rowsAffected == 0 {
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

	// Build the query with tenant filter removed (RLS handles it).
	// The optional `level` and `cursor` filters are still column-level.
	query := `
		SELECT id, level, log_type, message, created_at::text
		FROM agent_logs
		WHERE agent_id = $1
	`
	args := []interface{}{agentID}
	argIdx := 2

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

	type LogRow struct {
		ID        int64  `json:"id"`
		Level     string `json:"level"`
		LogType   string `json:"logType"`
		Message   string `json:"message"`
		CreatedAt string `json:"createdAt"`
	}

	list := []LogRow{}
	err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		rows, err := tx.Query(query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var l LogRow
			if err := rows.Scan(&l.ID, &l.Level, &l.LogType, &l.Message, &l.CreatedAt); err != nil {
				continue
			}
			list = append(list, l)
		}
		return nil
	})
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
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
	var total int
	err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM audit_log WHERE resource_id = $1`,
			agentID,
		).Scan(&total); err != nil {
			return err
		}
		rows, err := tx.Query(`
			SELECT id, action, COALESCE(resource_type,''), COALESCE(resource_id,''),
			       COALESCE(details, '{}'), COALESCE(ip_address::text, ''),
			       created_at::text
			FROM audit_log
			WHERE resource_id = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`, agentID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

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
		return nil
	})
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
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

func handleListGlobalAudit(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r)

	limit := 50
	offset := 0
	action := r.URL.Query().Get("action")
	resourceType := r.URL.Query().Get("resourceType")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

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

	type GlobalAuditRow struct {
		ID           int64                  `json:"id"`
		UserID       string                 `json:"userId"`
		Action       string                 `json:"action"`
		ResourceType string                 `json:"resourceType"`
		ResourceID   string                 `json:"resourceId"`
		Details      map[string]interface{} `json:"details"`
		IPAddress    string                 `json:"ipAddress"`
		CreatedAt    string                 `json:"createdAt"`
	}

	where := "true"
	args := []interface{}{}
	argIdx := 1

	if action != "" {
		where += fmt.Sprintf(" AND action = $%d", argIdx)
		args = append(args, action)
		argIdx++
	}
	if resourceType != "" {
		where += fmt.Sprintf(" AND resource_type = $%d", argIdx)
		args = append(args, resourceType)
		argIdx++
	}
	if from != "" {
		where += fmt.Sprintf(" AND created_at >= $%d", argIdx)
		args = append(args, from)
		argIdx++
	}
	if to != "" {
		where += fmt.Sprintf(" AND created_at <= $%d", argIdx)
		args = append(args, to)
		argIdx++
	}

	list := []GlobalAuditRow{}
	var total int

	err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM audit_log WHERE %s`, where)
		if err := tx.QueryRow(countSQL, args...).Scan(&total); err != nil {
			return err
		}

		querySQL := fmt.Sprintf(`
			SELECT id, COALESCE(user_id::text,''), action, COALESCE(resource_type,''),
			       COALESCE(resource_id,''), COALESCE(details, '{}'),
			       COALESCE(ip_address::text, ''), created_at::text
			FROM audit_log
			WHERE %s
			ORDER BY created_at DESC
			LIMIT $%d OFFSET $%d
		`, where, argIdx, argIdx+1)
		queryArgs := append(args, limit, offset)

		rows, err := tx.Query(querySQL, queryArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var a GlobalAuditRow
			var detailsJSON []byte
			if err := rows.Scan(&a.ID, &a.UserID, &a.Action, &a.ResourceType,
				&a.ResourceID, &detailsJSON, &a.IPAddress, &a.CreatedAt); err != nil {
				continue
			}
			json.Unmarshal(detailsJSON, &a.Details)
			list = append(list, a)
		}
		return nil
	})
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
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
	var total int
	err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM agent_patches WHERE agent_id = $1`,
			agentID,
		).Scan(&total); err != nil {
			return err
		}
		rows, err := tx.Query(`
			SELECT id, COALESCE(kb_id,''), COALESCE(name,''), COALESCE(severity,''),
			       COALESCE(description,''), installed, COALESCE(installed_at,''),
			       scanned_at::text
			FROM agent_patches
			WHERE agent_id = $1
			ORDER BY installed DESC, severity DESC, name ASC
			LIMIT $2 OFFSET $3
		`, agentID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var p PatchRow
			if err := rows.Scan(&p.ID, &p.KbID, &p.Name, &p.Severity,
				&p.Description, &p.Installed, &p.InstalledAt, &p.ScannedAt); err != nil {
				continue
			}
			list = append(list, p)
		}
		return nil
	})
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
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
	if denyIfUnauthorized(w, r, "admin") { return }
	agentID := normalizeAgentID(getAgentIDFromPath(r.URL.Path))
	tenantID := getTenantID(r)

	// RLS-protected EXISTS check before consulting the in-memory map.
	// Without this, an admin from Tenant B could send a scan command to
	// an agent belonging to Tenant A. (See handleScanSoftware for the
	// original bug that motivated this pattern.)
	var exists bool
	if err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1)`, agentID).Scan(&exists)
	}); err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, `{"error":"agent not found in this tenant"}`, http.StatusNotFound)
		return
	}

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
		if denyIfUnauthorized(w, r, "admin") { return }
		handleUpdateCheck(w, r, checkID)
	} else if len(parts) == 1 && r.Method == http.MethodDelete {
		if denyIfUnauthorized(w, r, "admin") { return }
		handleDeleteCheck(w, r, checkID)
	} else if len(parts) == 2 && parts[1] == "run" && r.Method == http.MethodPost {
		if denyIfUnauthorized(w, r, "admin") { return }
		handleRunCheck(w, r, checkID)
	} else {
		http.NotFound(w, r)
	}
}

func handleListChecks(w http.ResponseWriter, r *http.Request) {
	agentID := getAgentIDFromPath(r.URL.Path)
	tenantID := getTenantID(r)

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
	err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		rows, err := tx.Query(`
			SELECT id, check_type, COALESCE(description,''), COALESCE(config, '{}'),
			       status, COALESCE(last_output,''), COALESCE(last_run::text, ''),
			       enabled
			FROM agent_checks
			WHERE agent_id = $1
			ORDER BY created_at DESC
		`, agentID)
		if err != nil {
			return err
		}
		defer rows.Close()

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
		return nil
	})
	if err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
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
	if err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			INSERT INTO agent_checks (tenant_id, agent_id, check_type, description, config)
			VALUES ($1, $2, $3, $4, $5) RETURNING id
		`, tenantID, agentID, body.CheckType, body.Description, configJSON).Scan(&checkID)
	}); err != nil {
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

	if err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		if body.Config != nil {
			configJSON, _ := json.Marshal(body.Config)
			if _, err := tx.Exec(`UPDATE agent_checks SET config = $1 WHERE id = $2`, configJSON, checkID); err != nil {
				return err
			}
		}
		if body.Enabled != nil {
			if _, err := tx.Exec(`UPDATE agent_checks SET enabled = $1 WHERE id = $2`, *body.Enabled, checkID); err != nil {
				return err
			}
		}
		if body.Description != "" {
			if _, err := tx.Exec(`UPDATE agent_checks SET description = $1 WHERE id = $2`, body.Description, checkID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

func handleDeleteCheck(w http.ResponseWriter, r *http.Request, checkID string) {
	tenantID := getTenantID(r)

	var rowsAffected int64
	if err := WithTenantWrite(tenantID, func(tx *sql.Tx) error {
		result, err := tx.Exec(`DELETE FROM agent_checks WHERE id = $1`, checkID)
		if err != nil {
			return err
		}
		rowsAffected, _ = result.RowsAffected()
		return nil
	}); err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}

	if rowsAffected == 0 {
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

	if err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			SELECT agent_id, check_type, COALESCE(config, '{}')
			FROM agent_checks WHERE id = $1
		`, checkID).Scan(&check.AgentID, &check.CheckType, &check.Config)
	}); err != nil {
		http.Error(w, `{"error":"check not found"}`, http.StatusNotFound)
		return
	}

	agentsMu.Lock()
	agent, ok := agents[normalizeAgentID(check.AgentID)]
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
	writeErr := agent.Conn.WriteMessage(websocket.TextMessage, msgBytes)
	agent.Mu.Unlock()

	if writeErr != nil {
		http.Error(w, `{"error":"failed to send command"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "check initiated"})
}

// ─── Software Uninstall ──────────────────────────────────────────────────────

func handleUninstallSoftware(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if denyIfUnauthorized(w, r, "admin") { return }

	tenantID := getTenantID(r)
	claims := getClaims(r)

	// Extract agentID and softwareID from path: /api/agents/{agentID}/software/{softwareID}/uninstall
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/api/agents/")
	parts := strings.Split(path, "/")
	if len(parts) < 3 || parts[1] != "software" || parts[2] == "" || (len(parts) > 3 && parts[3] != "uninstall") {
		http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
		return
	}
	agentID := normalizeAgentID(parts[0])
	softwareID := parts[2]

	// Verify agent belongs to this tenant (RLS-protected EXISTS check).
	// Without `WithTenantRead`, this lookup would happen on a connection
	// without `app.tenant_id` set, so the query would return 0 rows
	// (and the in-memory map would let the command through to another
	// tenant's agent — see handleScanSoftware for that bug).
	var exists bool
	if err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1)`, agentID).Scan(&exists)
	}); err != nil {
		http.Error(w, `{"error":"DB error"}`, http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
		return
	}

	// Get the quiet uninstall string (RLS filters by tenant_id).
	var uninstallString string
	if err := WithTenantRead(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`
			SELECT COALESCE(quiet_uninstall_string, '')
			FROM agent_software WHERE id = $1 AND agent_id = $2
		`, softwareID, agentID).Scan(&uninstallString)
	}); err != nil {
		http.Error(w, `{"error":"software not found"}`, http.StatusNotFound)
		return
	}
	if uninstallString == "" {
		http.Error(w, `{"error":"no uninstall string available"}`, http.StatusBadRequest)
		return
	}

	// Get software name for audit log (RLS-protected).
	var softwareName string
	_ = WithTenantRead(tenantID, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT name FROM agent_software WHERE id = $1`, softwareID).Scan(&softwareName)
	})

	// Check if agent is connected
	agentsMu.Lock()
	agent, ok := agents[agentID]
	agentsMu.Unlock()

	if !ok {
		http.Error(w, `{"error":"agent not connected"}`, http.StatusNotFound)
		return
	}

	// Send uninstall command to agent
	payloadBytes, _ := json.Marshal(map[string]interface{}{
		"softwareId":      softwareID,
		"softwareName":    softwareName,
		"uninstallString": uninstallString,
	})

	msg := Message{
		AgentID: agentID,
		Type:    "software_uninstall_command",
		Payload: string(payloadBytes),
	}
	msgBytes, _ := json.Marshal(msg)

	agent.Mu.Lock()
	writeErr := agent.Conn.WriteMessage(websocket.TextMessage, msgBytes)
	agent.Mu.Unlock()

	if writeErr != nil {
		http.Error(w, `{"error":"failed to send command"}`, http.StatusInternalServerError)
		return
	}

	auditLog(tenantID, claims.UserID, "software.uninstall", "software", softwareID, clientIP(r), map[string]interface{}{
		"agentId": agentID,
		"name":    softwareName,
	})

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"uninstalling"}`))
}

// ─── Telemetry Pruning ───────────────────────────────────────────────────────

func pruneOldTelemetry() {
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for range ticker.C {
			result, err := dbAdmin.Exec(`DELETE FROM telemetry WHERE recorded_at < NOW() - INTERVAL '30 days'`)
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
	initBackupEncryption()
	initDB()
	pruneOldTelemetry()
	go pruneLoginAttempts()
	go startBackupScheduler()

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
	http.HandleFunc("/api/backup-jobs/", corsMiddleware(authMiddleware(handleDeleteBackupJob)))
	http.HandleFunc("/api/users", corsMiddleware(authMiddleware(handleUsers)))
	http.HandleFunc("/api/users/", corsMiddleware(authMiddleware(handleUserRoutes)))
	http.HandleFunc("/api/tenants", corsMiddleware(authMiddleware(handleListTenants)))
	http.HandleFunc("/api/audit", corsMiddleware(authMiddleware(handleListGlobalAudit)))
	http.HandleFunc("/api/scripts", corsMiddleware(authMiddleware(handleScripts)))
	http.HandleFunc("/api/scripts/", corsMiddleware(authMiddleware(handleScriptRoutes)))
	http.HandleFunc("/api/script-executions", corsMiddleware(authMiddleware(handleListScriptExecutions)))

	// WebSocket routes
	http.HandleFunc("/terminal/ws", corsMiddleware(authMiddleware(handleTerminalWebSocket)))
	http.HandleFunc("/screen/ws", corsMiddleware(authMiddleware(handleScreenWebSocket)))
	http.HandleFunc("/api/events/ws", authMiddleware(handleEventsWebSocket))

	log.Printf("Backend starting on http://localhost%s", cfg.Port)
	if err := http.ListenAndServe(cfg.Port, nil); err != nil {
		log.Fatalf("Failed to start backend: %v", err)
	}
}
