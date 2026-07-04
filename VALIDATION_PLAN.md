# Validation Plan — RMM System

**Fecha creación:** 2026-07-03
**Última actualización:** 2026-07-04
**Estado actual:** Feature-complete en dev, validación de producción pendiente
**Condición de cierre:** Archivar cuando todos los ítems 🔴 estén resueltos o marcados como N/A

---

## Pre-shipping Checklist

| # | Ítem | Estado | Bloqueado por | Quién puede ejecutar |
|---|------|--------|---------------|----------------------|
| 1 | **VM Windows + Agente como Service (`LocalSystem`)** | ⏳ Pendiente | Necesita VM Windows limpia | Quien tenga acceso a VM |
| 2 | **Test broadcast cross-tenant** | ⏳ Pendiente | Requiere 2 tenants conectados | QA o dev con 2 cuentas |
| 3 | **KB regex fallback audit** | ✅ Cerrado | 10 entries sin KB filtradas (driver/firmware/Store) | N/A — resuelto |

### 1. VM Windows + Agente como Service

**Por qué es crítico:**
- `CoInitializeEx` puede comportarse diferente bajo `LocalSystem` vs proceso interactivo
- WMI queries pueden tener permisos distintos
- Si COM no inicializa como servicio, Patches y Software scan quedan inválidos

**Pasos:**
```
1. Crear VM Windows 10/11 limpia (Hyper-V, VirtualBox, o AWS)
2. Copiar agent.exe compilado
3. Instalar como Windows Service:
   sc create ApexAgent binPath= "C:\path\to\agent.exe" start= auto
   sc start ApexAgent
4. Verificar en Event Viewer → Windows Logs → Application que no hay errores de COM
5. Conectar frontend y ejecutar Patches scan
6. Comparar resultados vs. "Configuración > Apps" de Windows
```

**Resultado esperado:** Patches scan retorna KBs reales (no 0 entries)
**Si falla:** Descartar COM IUpdateSearcher, volver a registry CBS con nota de que es incompleto

### 2. Test broadcast cross-tenant

**Pasos:**
```
1. Crear 2 tenants en la DB (Tenant A, Tenant B)
2. Conectar 2 sesiones WebSocket (una por tenant)
3. Enviar evento que solo debería llegar a Tenant A
4. Verificar que Tenant B NO recibe el evento
5. Verificar en logs del backend que el filtro se ejecuta
```

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
