# Validation Plan — RMM System

**Fecha creación:** 2026-07-03
**Última actualización:** 2026-07-05
**Estado actual:** Todos los blockers cerrados. Pendiente: cleanup manual del service Windows.
**Condición de cierre:** Archivar cuando todos los ítems 🔴 estén resueltos o marcados como N/A

---

## Pre-shipping Checklist

| # | Ítem | Estado | Bloqueado por | Quién puede ejecutar |
|---|------|--------|---------------|----------------------|
| 1 | **Laptop física + Agente como Service (`LocalSystem`)** | ✅ Cerrado | N/A | Validado 2026-07-05 |
| 2 | **Test broadcast cross-tenant (B2)** | ✅ Cerrado | N/A | Validado 2026-07-05 (RLS activo) |
| 3 | **KB regex fallback audit** | ✅ Cerrado | 10 entries sin KB filtradas (driver/firmware/Store) | N/A — resuelto |
| 4 | **Driver/firmware/Store exclusions en laptop real** | ✅ Cerrado | N/A | Validado 2026-07-05 |
| 5 | **Cleanup service Windows `ApexAgent`** | ✅ Cerrado | N/A | Ejecutado 2026-07-05 (sc stop, sc delete, Remove-Item OzyShield) |

### 1. Laptop física + Agente como Service (LocalSystem)

**Resultado de validación (2026-07-05):**

Plataforma: `DESKTOP-F5PGPTF`, Windows 11 Pro, Intel Xeon W-10885M, 32GB RAM, 2 discos NTFS (C: 1TB, D: 1TB)

Service instalado con:
```powershell
sc.exe create ApexAgent binPath= "C:\Users\User\Documents\rmm\agent\rmm-agent.exe" start= auto DisplayName= "ApexRMM Agent"
sc.exe start ApexAgent
```

Validación completada:
- ✅ Service arranca en Session 0 (proceso bajo `NT AUTHORITY\SYSTEM`)
- ✅ Credenciales persistidas en `C:\ProgramData\OzyShield\queue.db` — no requiere env vars
- ✅ Telemetry fluye cada 30s: CPU 17-49%, RAM 16GB libre, disco 1.7TB libre
- ✅ Software scan retorna 28 apps (registry `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall` accesible)
- ✅ Patches scan retorna 42 KBs (COM `IUpdateSearcher` inicializa OK)
- ✅ Cero errores en Event Viewer → Application log durante toda la validación
- ✅ Reconexión automática tras restart del backend (con backoff 5s)

**Bugs encontrados y corregidos durante la validación:**
1. `CheckOrigin` rechazaba clientes no-browser (rechazaba al agente con 403)
2. Agente no tenía flujo de enrollment (no podía obtener JWT)
3. Agente no hacía push de telemetría periódica (DB quedaba vacía)
4. `handleListPatches` usaba `install_date` en vez de `installed_at` (typo)
5. `agents[agentID]` lookup fallaba: in-memory map usa no-dashes, URL trae con-dashes (UUID)
6. `handleListAgents` retornaba ID con guiones pero el map usaba sin guiones

**Cleanup (manual, requiere admin):**
```powershell
sc.exe stop ApexAgent
sc.exe delete ApexAgent
Remove-Item -Recurse C:\ProgramData\OzyShield
```

### 2. Test broadcast cross-tenant (B2)

**Resultado de validación (2026-07-05):**

**Arquitectura de aislamiento:**
- Rol `apexrmm_app` (NOSUPERUSER, NOBYPASSRLS) usado por el `db` pool — todas las queries tenant-scoped.
- Rol `apexrmm` (SUPERUSER, BYPASSRLS) usado por `dbAdmin` pool — solo para `initDB()` y `handleEnrollAgent()`.
- RLS habilitado en 11 tablas con `FORCE ROW LEVEL SECURITY`.
- Policy: `tenant_id::text = current_setting('app.tenant_id', true)` (fail-closed si el setting no existe).
- Wrapper Go: `WithTenantRead(tenantID, fn)` y `WithTenantWrite(tenantID, fn)` — abre transacción, hace `set_config('app.tenant_id', $1, true)`, ejecuta `fn(tx)`, hace commit o rollback.
- `tenants` (catálogo raíz) sin RLS — siempre legible.

**Bypass explícito (1 sitio):**
- `handleEnrollAgent` usa `dbAdmin` (BYPASSRLS) porque el agente todavía no existe (no hay JWT, no hay tenant context). El tenant se determina por el enrollment token.

**Tests ejecutados:**

| # | Test | Resultado |
|---|------|-----------|
| 1 | `GET /api/agents` como admin@A | 1 agente (DESKTOP-F5PGPTF, tenant A) ✓ |
| 2 | `GET /api/agents` como admin@B | 1 agente (B-MOCK-AGENT, tenant B) ✓ |
| 3 | admin@A hace `POST /agents/{B_id}/software/scan` | 404 ✓ |
| 4 | admin@A hace `GET /agents/{B_id}/software` | 200, lista vacía ✓ |
| 5 | admin@B hace `POST /agents/{A_id}/software/scan` | 404 (después del fix) ✓ |
| 6 | admin@B hace `GET /api/alerts` | 200, lista vacía (no ve alertas de A) ✓ |

**Bug encontrado y corregido durante la validación:**
- `handleScanSoftware` inicialmente confiaba en el in-memory `agents[agentID]` map, que es global (no per-tenant). Esto permitía a admin@B enviar comandos al WebSocket del agente de A. Fix: `WithTenantRead(tenantID, ...)` que verifica `EXISTS(SELECT 1 FROM agents WHERE id = $1)` ANTES de consultar el map.

**Tech debt identificado (no resuelto en esta sesión):**
- 7 read handlers aún usan `db.Query` directo: `handleListAlerts`, `handleListSoftware`, `handleListPatches`, `handleListNotes`, `handleListLogs`, `handleListChecks`, `handleListBackups`. Estos retornan 0 resultados para usuarios autenticados (RLS bloquea sin `SET LOCAL`).
- 3 command handlers necesitan el mismo fix que `handleScanSoftware`: `handleScanPatches`, `handleRunCheck`, `handleUninstallSoftware`.
- El agente escribe en `agent_software`, `agent_patches`, `agent_logs` desde los handlers `case "software_list":`, `case "patch_list":`, `case "agent_log":` del WebSocket loop. Estos aún no están envueltos.
- `handleCreateRegistrationToken` y `handleCreateCheck` hacen INSERTs sin wrapper.

**Impacto en B2:** El test de cross-tenant pasa con el endpoint `/api/agents` (read) y `/api/agents/{id}/software/scan` (write). Los demás endpoints necesitan refactor pero el patrón ya está validado.

**Migraciones SQL aplicadas (en orden):**
1. `sql/001_create_app_role.sql` — crea `apexrmm_app` y grants
2. `sql/002_enable_rls.sql` — `ENABLE` + `FORCE` RLS en 11 tablas + policies
3. `sql/003_fix_telemetry_and_tenants.sql` — fix de `telemetry` (no tiene `tenant_id`, usa subquery) y `tenants` (sin RLS)
4. `sql/004_create_tenant_b.sql` — Tenant B para el test
5. `sql/005_create_admin_b.sql` — admin@B
6. `sql/006_simulate_agent_b.sql` — agente simulado de Tenant B

---

## Refactorización RLS completa (Fase 1.5 cierre, 2026-07-05)

**Motivación:** el test B2 inicial validó el patrón pero dejó 27 handlers sin adaptar. Sin un refactor completo, varios endpoints seguirían retornando 0 silenciosamente.

**Alcance refactorizado (27 handlers + 4 WebSocket cases):**

| Grupo | Cantidad | Patrón aplicado |
|-------|----------|------------------|
| Read handlers | 12 | `WithTenantRead(tenantID, fn)`; `WHERE tenant_id = $N` eliminado del SQL |
| Write handlers | 8 | `WithTenantWrite(tenantID, fn)`; `RowsAffected` capturado en variable externa |
| Command handlers | 3 | `WithTenantRead EXISTS` check antes del in-memory `agents[agentID]` map |
| WebSocket cases | 4 | `WithTenantWrite(tenantID, fn)` usando el `tenantID` del JWT del agente |

**Bug crítico encontrado en WebSocket cases (shadowing):**

Los cases `software_list`, `agent_log`, `patch_list` en el loop de mensajes del agente declaraban `var tenantID string` que shadoweaba el `tenantID` del JWT en `handleAgentConnection`. El lookup redundante `SELECT tenant_id FROM agents WHERE id=$1` fallaba por RLS (devolvía NULL), así que `tenantID=""`. Los INSERTs fallaban el `WITH CHECK` de la policy con `tenant_id=''`. **Estos casos estaban fallando silenciosamente desde que se aplicó RLS en la sesión anterior**, descartando todos los `software_list`/`patch_list`/`agent_log` que el agente enviaba.

Verificación post-fix: las timestamps `scanned_at` de los datos en `agent_software`/`agent_patches` saltaban del 16:35 (pre-RLS) al 21:48 (post-fix), confirmando que el fix restauró la persistencia.

**Tests E2E post-refactor (todos PASS):**

| Endpoint | Test | Resultado |
|----------|------|-----------|
| `/api/agents` | admin@A ve 1 agente (DESKTOP-F5PGPTF) | ✓ |
| `/api/agents` | admin@B ve 1 agente (B-MOCK-AGENT) | ✓ |
| `/api/alerts` | admin@A ve 2 alerts | ✓ |
| `/api/alerts?agent_id=...` | Filter funciona | ✓ |
| `/api/users` | 1 user | ✓ |
| `/api/tenants` | 1 tenant | ✓ |
| `/api/backups` | 2 backups | ✓ |
| `/api/agents/{id}/software` | 28 items | ✓ |
| `/api/agents/{id}/patches` | 42 items | ✓ |
| `/api/agents/{id}/notes` | 1 item (creado en test) | ✓ |
| `/api/agents/{id}/logs` | 1 item (post-scan agent_log) | ✓ |
| `/api/agents/{id}/checks` | 1 item (creado en test) | ✓ |
| `/api/agents/{id}/audit` | 1 item | ✓ |
| `/api/agents/telemetry?id=...` | 100 items | ✓ |
| `POST /api/agents/{id}/notes` | Crea nota | ✓ |
| `POST /api/agents/{id}/checks` | Crea check | ✓ |
| `POST /api/alerts/acknowledge` | Marca alert | ✓ (verificado en DB) |
| `POST /api/agents/{A_id}/software/scan` (admin@B) | 404 | ✓ |
| `POST /api/agents/{B_id}/software/scan` (admin@A) | 404 | ✓ |

**Tests B2 cross-tenant (8/8 PASS):**

| # | Test | Resultado |
|---|------|-----------|
| 1 | admin@A → `/api/agents` | 1 agente, todos tenant A ✓ |
| 2 | admin@B → `/api/agents` | 1 agente, todos tenant B ✓ |
| 3 | admin@A scan agente B | 404 ✓ |
| 4 | admin@B scan agente A | 404 ✓ |
| 5 | admin@A ve software de B | 0 items ✓ |
| 6 | admin@B ve software de A | 0 items ✓ |
| 7 | admin@B ve alerts | 0 (no ve de A) ✓ |
| 8 | admin@A ve 2 backups, B ve 0 | Aislamiento confirmado ✓ |

**Wrappers Go en `backend/db_tenant.go`:**

- `WithTenantRead(tenantID, fn)` — abre Tx, `set_config('app.tenant_id', $1, true)`, ejecuta fn, rollback
- `WithTenantWrite(tenantID, fn)` — igual pero commit al final
- `set_config` en lugar de `SET LOCAL` (la sintaxis `SET LOCAL foo = $1` no es válida en Postgres)

---

## Cleanup pendiente (manual, requiere admin shell)

El service `ApexAgent` y las credenciales en `C:\ProgramData\OzyShield` siguen en la máquina de desarrollo (PID 24780, Session 0). Para limpiar, abrir PowerShell **como Administrador** y correr:

```powershell
sc.exe stop ApexAgent
sc.exe delete ApexAgent
Remove-Item -Recurse -Force C:\ProgramData\OzyShield
```

**Por qué es importante hacerlo antes de la próxima sesión:** evita que la laptop aparezca como "online" en el dashboard de desarrollo, libera el puerto si queremos levantar otra versión del agente, y elimina credenciales reales de la máquina.

---

**✅ Cleanup ejecutado 2026-07-05:**
- `sc.exe stop ApexAgent` → STATE: 1 STOPPED
- `sc.exe delete ApexAgent` → `[SC] DeleteService CORRECTO`
- `Remove-Item -Recurse -Force C:\ProgramData\OzyShield` → success
- Verificado: Get-Service retorna vacío, Test-Path retorna False, no rmm-agent processes running.

### 3. KB regex fallback (cerrado)

Patrón: `regexp.MustCompile(\`KB(\d+)\`)` extrae KB del título del update.

En laptop real validado 2026-07-05: 4 KBs únicos detectados (2267602, 4052623, 5094126, 890830), todos son updates legítimos de Windows Update. Cero drivers/firmware/Store updates en el resultado.

Mecanismo de filtrado: en `agent/main.go`, después de aplicar el regex, si el KB no se extrae (string vacío), el entry se descarta y se loguea.

### 4. Driver/firmware/Store exclusions (cerrado)

Las entradas sin KB (driver/firmware/Store updates) son filtradas por el regex antes de insertarse en la DB. Por lo tanto, el tab Patches en la UI nunca las muestra.

---

## Completado en esta sesión

### ✅ Seguridad

| Feature | Detalle |
|---------|---------|
| **Broadcast tenant filtering** | `broadcastToFrontend()` filtra por `tenantID` |
| **RBAC enforcement** | 3 roles (admin/technician/agent), `denyIfUnauthorized()` en 15 endpoints |
| **Audit log** | Registra acciones destructivas (uninstall, backup, etc.) |

### ✅ Alerting

| Feature | Detalle |
|---------|---------|
| **CPU threshold** | `>= 90%` con dedup 10 min |
| **RAM threshold** | `< 10% free` con dedup 10 min |
| **Disk threshold** | `< 10% free` con dedup 10 min |

### ✅ Acciones remotas

| Feature | Detalle |
|---------|---------|
| **Software uninstall** | Admin-only, confirmación, audit log, timeout 5min |
| **Health checks** | Sidecar con `go + context.WithTimeout` por check |
| **Terminal remoto** | WebSocket bidireccional con xterm.js |

### ✅ Backups

| Feature | Detalle |
|---------|---------|
| **Scheduler** | Goroutine cada 60s, ejecuta según cron |
| **Cron parsing** | `robfig/cron/v3` — expresiones estándar |
| **Agent execution** | Kopia sidecar con reporting de resultado |

### ✅ Device Detail Tabs (9 tabs)

| Tab | Fuente de datos |
|-----|-----------------|
| Summary | Agent telemetry + GPU + disks |
| Software | Registry scan (no Win32_Product) |
| Patches | COM IUpdateSearcher + regex KB |
| Checks | CRUD + sidecar execution |
| Notes | CRUD con auto-save |
| Assets | WMI key-value |
| Debug | Agent logs (cursor pagination) |
| Audit | Audit log (offset pagination) |

---

## Deuda Técnica conocida

| # | Ítem | Severidad | Notas |
|---|------|-----------|-------|
| 1 | Monitoring charts | Media | Telemetry existe pero sin visualización. 2-4h |
| 2 | Multi-instance backups | Baja | Scheduler puede ejecutar 2x si backend escala. Necesita distributed lock |
| 3 | User management UI | Baja | No hay página para crear/editar usuarios |
| 4 | Agent auto-update | Baja | Versión es constante estática |
| 5 | CI/CD pipeline | Baja | Todo manual hoy |
| 6 | DB migrations versionadas | Baja | SQL inline en initDB() |

---

## Infra del proyecto

| Componente | Estado |
|------------|--------|
| Backend | Go 1.23, PostgreSQL, JWT, WebSocket |
| Agent | Go 1.23, WMI, COM, Kopia, SQLite |
| Frontend | Next.js 15, React 19, Tailwind 4, Shadcn |
| Deploy | docker-compose.yml, Dockerfiles |

---

## Condiciones de cierre del documento

Este archivo se archiva cuando:
1. VM validation esté completa (ítem #1 del Pre-shipping)
2. Broadcast cross-tenant test pase (ítem #2)
3. Los ítems de deuda técnica estén en backlog con fecha estimada

**No archivar si:** Solo se completaron features de código y no se ejecutó la validación en VM real.
