# HermesVault Frontend Development Guide

## Commands
- Build Go project: `go build -o ./.tmp/main .`
- Run server (dev mode): `go run main.go -dev`
- Run server (prod): `go run main.go`
- Development server with hot reload: `air` (requires air installed)
- Frontend build: `cd frontend && npm run build`
- Subscriber service: `cd subscriber-service && pipenv run python main.py`
- Redeploy in production: `./redeploy.sh`

## Code Style
- **Imports**: Group standard library, then external packages, then project packages
- **Error Handling**: Use structured error types with specific error categories
- **Type Definitions**: Define custom types in models/
- **Naming**: Use CamelCase for exported symbols, camelCase for internal ones
- **Comments**: Document package-level declarations and complex logic
- **Error Returns**: Return early on errors, wrap errors with context
- **Formatting**: Use `gofmt` or `goimports`
- **Variables**: Use short variable names for short scopes
- **Project Structure**: Follow standard Go project layout patterns

## Git Workflow
- **Commit Messages**: Use descriptive commit messages without Claude attribution
- **Commit Format**: Start with a concise summary, then a blank line, followed by details if needed

## Tech Stack
- Frontend: esbuild, htmx, missing.css, Pera wallet integration (@perawallet/connect)
- Backend: Go 1.23, SQLite (two databases: txns.db and internal.db)
- ZKP: gnark for zero-knowledge proof generation
- Blockchain: Algorand SDK, runs with TestNet

## Architecture
- Web server (Go): Serves frontend, manages backend (creates ZKP and blockchain txns)
- Subscriber service (Python): Monitors blockchain and updates txns.db
- Algod node: Connects to Algorand blockchain to broadcast transactions