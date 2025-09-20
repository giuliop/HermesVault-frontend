## Project Overview

HermesVault is a privacy-focused frontend for conducting private transactions on the Algorand blockchain. The application combines a Go backend web server with JavaScript frontend components, utilizing zero-knowledge proofs for transaction privacy.

The system consists of two processes:
1. **Go webserver**: Serves frontend and manages backend (creating zk-proofs, sending blockchain transactions)
2. **Python subscriber service**: Monitors blockchain through algod node and saves transaction data

## Architecture

This is a monorepo containing both backend (Go) and frontend (JavaScript) components:

### Backend Structure
- **Main Application**: `/main.go` - HTTP server with graceful shutdown
- **Handlers**: `/handlers/` - HTTP request handlers for deposit, withdrawal, confirmation, and stats
- **Database Layer**: `/db/` - SQLite database operations with encryption support
- **AVM Integration**: `/avm/` - Algorand Virtual Machine integration and transaction handling
- **ZKP System**: `/zkp/` - Zero-knowledge proof circuits for deposits and withdrawals
- **Models**: `/models/` - Data models for addresses, amounts, notes, and application state
- **Configuration**: `/config/` - Application configuration and smart contract definitions
- **Memory Store**: `/memstore/` - In-memory store for user session data
- **Subscriber Service**: `/subscriber-service/` - Python service to monitor blockchain and update database

### Frontend Structure
- **JavaScript Source**: `/frontend/js/` - Wallet integration, behaviors, and HTMX entry points
- **Templates**: `/frontend/templates/` - HTML templates for deposit, withdrawal, confirmation screens
- **Static Assets**: `/frontend/static/` - Bundled JavaScript, CSS, and images

### Key Technologies
- **Backend**: Go with net/http (no third-party frameworks), SQLite databases, gnark for zero-knowledge proofs, Algorand SDK
- **Frontend**: HTMX for dynamic interactions, esbuild for bundling, Algorand wallet integrations (Pera, Defly, Lute)
- **Privacy**: Zero-knowledge proofs for transaction privacy, encrypted database storage
- **External Services**: nodely.io for algod node connection
- **External JS Modules**: `@pera/connect` for wallet connections, `algosdk` for Algorand transactions, `htmx` for server-driven interactions, `htmx-ext-response-targets` for response target management

### Database Architecture

The system uses two SQLite databases:

**txns.db** (written by Python subscriber service, read by Go webserver):
- `txns` table: Transaction data with leaf_index, commitment, txn_id, txn_type, address, amount, from_nullifier
- `stats` table: Global statistics (total deposits, withdrawals, fees)
- `watermark` table: Block sync watermark with algod
- `root` table: Last merkle tree root and leaf count

**internal.db** (accessed only by Go webserver):
- `notes` table: Note data with leaf_index, commitment, encrypted nullifier, txn_id
- `unconfirmed_notes` table: Temporary storage for unconfirmed transactions

Purpose: txns.db provides merkle proof data for withdrawals; internal.db stores encrypted nullifiers for compliance. Nullifiers are encrypted with a public key stored on disk, while the private key is kept off the server to protect disclosure even if the host is compromised.

## Development Commands

### Frontend Build Commands
```bash
# Build all frontend assets
cd frontend && npm run build

# Build individual components
cd frontend && npm run build:wallet    # Wallet integration bundle
cd frontend && npm run build:behaviors # UI behaviors bundle
cd frontend && npm run build:htmx     # HTMX entry point bundle
cd frontend && npm run copy:missingcss # Copy CSS framework
```

### Go Application
```bash
# Development - run with hot reload using air
air

# Production - run directly
go run main.go

# Run tests
go test ./...
```

### Python Subscriber Service
```bash
# Development - from subscriber-service directory
pipenv run python main.py
```

### Deployment
```bash
# Production deployment
./redeploy.sh
```

### Database Operations
The application uses SQLite with automatic cleanup routines. Database files are located in `/data/` directory.

## Application Flow

1. **Deposits**: Users connect wallet → specify amount → receive secret note → confirm transaction
2. **Withdrawals**: Users provide secret note + withdrawal details → generate new secret note → confirm transaction
3. **Privacy**: All transactions use zero-knowledge proofs to maintain privacy while proving validity

## Important Notes

- Server runs on port 5555 (configurable in `/config/config.go`)
- Frontend assets must be built before running the server
- Database encryption keys are managed separately from the main codebase
- Zero-knowledge proof circuits are pre-compiled and stored in `/avm/mainnet/` and `/avm/testnet/`
- The application handles both mainnet and testnet Algorand environments
- Air configuration is in `.air.toml` for development hot reload
- Production runs on a Linode cloud server using Apache2 as reverse proxy with certbot for SSL certificates
- Systemd manages both Go webserver and Python process in production

## Security Considerations

- Secret notes are critical for fund recovery - loss results in permanent fund loss
- Database contains encrypted transaction receipts for regulatory compliance
- Frontend validates all user inputs before processing
- ZKP circuits ensure transaction privacy without revealing amounts or addresses
