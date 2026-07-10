# Project: Remote Monitoring Dashboard (RMM)

## Tech Stack
- Frontend: Next.js (App Router), React 19, TypeScript, Tailwind CSS 4, Shadcn UI
- Backend: Go 1.23, PostgreSQL, JWT, WebSocket
- Agent: Go 1.23, GDI (screen), WMI (telemetry), COM (patches), SQLite (offline queue)
- Package Manager: pnpm
- Container: Docker Compose (backend + PostgreSQL)

## Development Commands
- Start dev server: `pnpm dev`
- Build project: `pnpm build`
- Start built app: `pnpm start`
- Lint code: `pnpm lint`
- Build agent: `go build -o agent.exe ./agent/`
- Build backend: `go build -o backend.exe ./backend/`

## Deployment
- Agent installer: `.\deploy\installer.ps1 -BackendUrl "https://rmm.example.com" -EnrollToken "abc..."`
- Agent uninstall: `Stop-Service OzyShieldAgent; sc.exe delete OzyShieldAgent`
- Agent log: `C:\ProgramData\OzyShield\`
- Backend logs: stdout (Docker Compose)
