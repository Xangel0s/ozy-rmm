-- ApexRMM Multi-Tenant Schema
-- Migration 001: Initial schema with tenant isolation

-- Tenants (client organizations)
CREATE TABLE IF NOT EXISTS tenants (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          VARCHAR(255) NOT NULL,
    slug          VARCHAR(100) UNIQUE NOT NULL,
    plan          VARCHAR(50) DEFAULT 'standard',
    max_agents    INTEGER DEFAULT 100,
    created_at    TIMESTAMPTZ DEFAULT NOW(),
    updated_at    TIMESTAMPTZ DEFAULT NOW()
);

-- Users with RBAC and tenant association
CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID REFERENCES tenants(id) ON DELETE CASCADE,
    email         VARCHAR(255) UNIQUE NOT NULL,
    username      VARCHAR(100) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    full_name     VARCHAR(255),
    role          VARCHAR(50) NOT NULL DEFAULT 'technician',
    is_active     BOOLEAN DEFAULT true,
    last_login    TIMESTAMPTZ,
    failed_attempts INTEGER DEFAULT 0,
    locked_until  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ DEFAULT NOW(),
    updated_at    TIMESTAMPTZ DEFAULT NOW()
);

-- Registration tokens for agent enrollment
CREATE TABLE IF NOT EXISTS registration_tokens (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    token_hash    VARCHAR(255) NOT NULL,
    label         VARCHAR(255),
    created_by    UUID REFERENCES users(id),
    expires_at    TIMESTAMPTZ NOT NULL,
    used_at       TIMESTAMPTZ,
    max_uses      INTEGER DEFAULT 1,
    use_count     INTEGER DEFAULT 0,
    is_revoked    BOOLEAN DEFAULT false,
    created_at    TIMESTAMPTZ DEFAULT NOW()
);

-- Agents with cryptographic identity
CREATE TABLE IF NOT EXISTS agents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    enrollment_token_id UUID REFERENCES registration_tokens(id),
    hostname        VARCHAR(255),
    os              VARCHAR(255),
    cpu_model       VARCHAR(255),
    cpu_load        REAL DEFAULT 0,
    total_ram       BIGINT DEFAULT 0,
    free_ram        BIGINT DEFAULT 0,
    disk_total      BIGINT DEFAULT 0,
    disk_free       BIGINT DEFAULT 0,
    status          VARCHAR(50) DEFAULT 'offline',
    agent_version   VARCHAR(50),
    last_seen       TIMESTAMPTZ,
    enrolled_at     TIMESTAMPTZ DEFAULT NOW(),
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Telemetry time-series
CREATE TABLE IF NOT EXISTS telemetry (
    id            BIGSERIAL PRIMARY KEY,
    agent_id      UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    cpu_load      REAL,
    total_ram     BIGINT,
    free_ram      BIGINT,
    disk_total    BIGINT,
    disk_free     BIGINT,
    recorded_at   TIMESTAMPTZ DEFAULT NOW()
);

-- Alerts with deduplication window
CREATE TABLE IF NOT EXISTS alerts (
    id            BIGSERIAL PRIMARY KEY,
    tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    agent_id      UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    severity      VARCHAR(20) NOT NULL,
    message       TEXT,
    acknowledged  BOOLEAN DEFAULT false,
    acknowledged_by UUID REFERENCES users(id),
    acknowledged_at TIMESTAMPTZ,
    fingerprint   VARCHAR(64),
    created_at    TIMESTAMPTZ DEFAULT NOW()
);

-- Backup jobs
CREATE TABLE IF NOT EXISTS backup_jobs (
    id            BIGSERIAL PRIMARY KEY,
    tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    agent_id      UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    name          VARCHAR(255),
    location      VARCHAR(500),
    type          VARCHAR(50) DEFAULT 'full',
    status        VARCHAR(50) DEFAULT 'pending',
    size_bytes    BIGINT DEFAULT 0,
    cron          VARCHAR(100) DEFAULT '0 2 * * *',
    executed_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ DEFAULT NOW()
);

-- Audit log for all mutations
CREATE TABLE IF NOT EXISTS audit_log (
    id            BIGSERIAL PRIMARY KEY,
    tenant_id     UUID REFERENCES tenants(id) ON DELETE SET NULL,
    user_id       UUID REFERENCES users(id) ON DELETE SET NULL,
    action        VARCHAR(100) NOT NULL,
    resource_type VARCHAR(100),
    resource_id   VARCHAR(255),
    details       JSONB,
    ip_address    INET,
    created_at    TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for performance
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
