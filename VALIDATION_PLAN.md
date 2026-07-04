# Validation Plan — RMM Feature: Device Detail Tabs

**Fecha creación:** 2026-07-03
**Estado actual:** Completo en dev, validación de producción pendiente
**Condición de cierre:** Archivar cuando todos los ítems 🔴 estén resueltos o marcados como N/A

---

## Pre-shipping Checklist

Ítems que bloquean considerar la feature "lista para producción".

| # | Ítem | Bloqueado por | Quién puede ejecutar | Cuándo estimado |
|---|------|---------------|----------------------|-----------------|
| 1 | **VM Windows + Agente como Service (`LocalSystem`)** | Necesita VM Windows limpia (no dev machine) | Quien tenga acceso a VM | Antes de cerrar feature |
| 2 | **Auditar 26/50 entries sin KB en Patches** | Requiere análisis de patrones en entries fallidas | Mismo dev que hizo el fix | Próxima sesión |
| 3 | **Test broadcast cross-tenant con sesiones activas** | Requiere 2 usuarios/admins de tenants distintos conectados simultáneamente | QA o dev con 2 cuentas | Próxima sesión |
| 4 | **Verificar reconexión forzada post-fix de seguridad** | Si sesiones WebSocket activas previas al fix no se reconectan, el bug de cross-tenant persiste hasta reconexión natural | Mismo dev que hizo el fix | Próxima sesión |

### Detalle de cada ítem

#### 1. VM Windows + Agente como Service (`LocalSystem`)

**Por qué es crítico:**
- `CoInitializeEx` puede comportarse diferente bajo `LocalSystem` vs proceso interactivo
- WMI queries pueden tener permisos distintos
- Si COM no inicializa como servicio, toda la feature de Patches queda inválida

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

#### 2. Auditar 26/50 entries sin KB

**Contexto:** De 50 entries históricas, 39 tienen KB extraído del título por regex, 11 no.

**De las 11 sin KB identificado:**
- Driver updates (HP, Intel, NVIDIA) — 5 entries
- WinAppRuntime updates — 5 entries
- Firmware updates — 1 entry

**Pasos:**
```
1. Revisar las 11 entries sin KB en kb_debug output
2. Determinar si son entries que deberían aparecer en patch management
3. Si sí: implementar extracción alternativa (vendor ID, version number)
4. Si no: filtrar explícitamente (no mostrar entries sin KB como "unknown")
5. Agregar logging: "KB fallback used for entry: <title>" para diagnóstico futuro
```

**Objetivo:** Reducir el 22% de entradas huérfanas a <5%, o filtrarlas explícitamente

#### 3. Test broadcast cross-tenant

**Contexto:** Fix de seguridad (Ítem 0) agregó filtro `tenantID` en `broadcastToFrontend()`

**Pasos:**
```
1. Crear 2 tenants en la DB (Tenant A, Tenant B)
2. Conectar 2 sesiones WebSocket (una por tenant)
3. Enviar evento que solo debería llegar a Tenant A
4. Verificar que Tenant B NO recibe el evento
5. Verificar en logs del backend que el filtro se ejecuta
```

**Herramienta:** Podría ser un script simple con `wscat` o un test de integración

#### 4. Verificar reconexión post-fix

**Riesgo:** Si hay sesiones WebSocket activas ANTES del fix de seguridad, esas sesiones podrían seguir enviando datos cross-tenant hasta que se reconecten naturalmente.

**Pasos:**
```
1. Conectar frontend al backend (versión sin fix)
2. Aplicar fix de seguridad
3. NO recargar el frontend
4. Intentar enviar mensaje con agentId de otro tenant
5. Verificar que el backend lo bloquea
6. Si no lo bloquea: forzar reconexión en el frontend al detectar versión del servidor
```

**Solución probable:** No requiere acción — el fix se aplica en el handler del servidor, no en el cliente. Las sesiones existentes seguirán usando el handler actualizado en la siguiente llamada.

---

## Deuda Técnica (post-shipping)

No bloquea producción pero debe resolverse antes de escalar.

| # | Ítem | Severidad | Notas |
|---|------|-----------|-------|
| 5 | Conectar `recentAlerts`/`recentBackups` a datos reales | Media | Hoy: badge "not implemented" explícito |
| 6 | `cmd.Process.Kill()` explícito en checks sidecar | Baja | `exec.CommandContext` ya mata proceso en Windows |
| 7 | Evaluar si Assets necesita endpoint propio a futuro | Baja | Solo si crece más allá de lo que `GET /api/agents` puede manejar |

---

## Inventario de Infra — Preguntas Abiertas

**Distinción de origen:**
- ✅ **Confirmado por conversación** — Verificado en código o en esta sesión de desarrollo
- ❓ **Asunción RMM típica** — Estándar de la industria, pero NO confirmado que exista en este proyecto

### Tabla resumen

| Área | Estado | Preguntas a resolver |
|------|--------|----------------------|
| **Alerting real** | ✅ Completo | Motor de alertas: CPU ≥ 90%, RAM < 10% free, Disk < 10% free. Todos con dedup (10 min). |
| **Backup jobs** | ✅ Implementado | Scheduler goroutine cada 60s, ejecuta jobs según cron, envía backup_command al agent. Agent ejecuta Kopia sidecar. DB actualiza con resultado. |
| **Onboarding de agentes** | ✅ Parcial | Tabla `registration_tokens` existe. ¿Flujo de instalación documentado? ¿Rotación de tokens? ¿Expiración? |
| **RBAC / permisos granulares** | ✅ Implementado | 3 roles: admin, technician, agent. `denyIfUnauthorized()` aplica en todos los endpoints. Viewer postergado. |
| **Rate limiting** | ❓ | Con 20+ endpoints, ¿hay límite por tenant o por usuario? |
| **Observabilidad del backend** | ❓ | ¿Métricas, logs estructurados, tracing del backend mismo (no solo `agent_logs`)? |
| **Migraciones de DB** | ❓ | Con 13 tablas, ¿hay sistema versionado (golang-migrate, atlas) o se corre SQL a mano? |
| **CI/CD** | ❓ | ¿Pipeline que corra `go build`/`pnpm build` + tests automáticos, o todo manual? |
| **Backups Postgres** | ❓ | ¿Backup automatizado del backend DB? ¿Disaster recovery? |
| **Escalado WebSocket** | ❓ | `broadcastToFrontend` itera sesiones en memoria — ¿backend corre en una sola instancia hoy? |

### Nota sobre las asunciones

Los ítems marcados ❓ son preguntas legítimas, no gaps confirmados. Si alguno ya está resuelto, eliminarlo de esta lista. Si no está resuelto, probablemente es más urgente que features nuevas de UI.

---

## Mejoras Incrementales

Sobre lo ya construido, no urgentes.

| Ítem | Nota |
|------|------|
| Migrar Summary tab a WebSocket | Cuando el resto de la app tenga el patrón, no antes |
| Cursor pagination — validar bajo escritura concurrente | Pendiente de sesión de testing |
| Software scan — comparar conteo vs. "Configuración > Apps" | <definir método de comparación> |

---

## Condiciones de cierre del documento

Este archivo se archiva cuando:
1. Los 4 ítems 🔴 del Pre-shipping Checklist estén resueltos o marcados N/A
2. Los 3 ítems 🟠 de deuda técnica estén en backlog con fecha estimada
3. El inventario 🟡 tenga respuestas claras (resuelto o confirmado que no existe)

**No archivar si:** Solo se completaron los ítems de código y no se ejecutó la validación en VM real.
