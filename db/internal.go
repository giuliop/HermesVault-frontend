package db

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/giuliop/HermesVault-frontend/config"

	_ "github.com/mattn/go-sqlite3"
)

var (
	// txnsDb is populated by the subscriber service reading txns from algod
	txnsDb *sql.DB
	// internalDb is populated by the frontend to store additional notes data
	internalDb *sql.DB
)
var (
	internalDbPath = config.InternalDbPath
	txnsDbPath     = config.TxnsDbPath
)

func init() {
	if err := initializeInternalDB(); err != nil {
		log.Fatalf("failed to initialize internal database: %v", err)
	}
	if err := initializeTxnsDB(); err != nil {
		log.Fatalf("failed to initialize transactions database: %v", err)
	}
}

// initializeTxnsDB opens a connection to the txnsDb in read-only mode
func initializeTxnsDB() error {
	var err error
	// Open connection in read-only mode using DSN parameters.
	dsn := fmt.Sprintf("file:%s?mode=ro", txnsDbPath)
	txnsDb, err = sql.Open("sqlite3", dsn)
	if err != nil {
		return fmt.Errorf("failed to open transactions database in read-only mode: %w", err)
	}
	// Set busy timeout to 5000ms (5 seconds)
	_, err = txnsDb.Exec("PRAGMA busy_timeout = 5000")
	if err != nil {
		return fmt.Errorf("failed to set busy timeout on transactions database: %w", err)
	}
	log.Println("Transactions database (read-only) initialized successfully")
	return nil
}

// InitializeDB initializes the internalDb with WAL mode and creates necessary tables
// (if they don't exist already)
func initializeInternalDB() error {
	var err error
	internalDb, err = sql.Open("sqlite3", internalDbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// The unconfirmed_notes table stores notes that the frontend has not received confirmation
	// for yet form the blockchain. Once the txn inserting the note is confirmed, it is
	// removed from this table and added to the notes table.
	createTables := `
	CREATE TABLE IF NOT EXISTS notes (
		leaf_index INTEGER PRIMARY KEY,			-- note ndex in onchain merkle tree
		commitment BLOB NOT NULL,          		-- note Value in onchain merkle tree
		nullifier BLOB,                         -- note nullifier
		txn_id TEXT UNIQUE NOT NULL 			-- id of first group txn that inserted the note
	) STRICT;

	CREATE TABLE IF NOT EXISTS unconfirmed_notes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		commitment BLOB NOT NULL,          		-- note Value in onchain merkle tree
		nullifier BLOB,                         -- note nullifier
		txn_id TEXT UNIQUE NOT NULL, 			-- id of first group txn that will insert note
		created_at TEXT DEFAULT CURRENT_TIMESTAMP
	) STRICT;
	`
	// Only for use in TestNet
	// CREATE TABLE IF NOT EXISTS debug_notes (
	// 	leaf_index INTEGER PRIMARY KEY,
	// 	text TEXT NOT NULL,
	// 	FOREIGN KEY(leaf_index) REFERENCES notes(leaf_index) ON DELETE CASCADE
	// ) STRICT;

	// Enable WAL
	_, err = internalDb.Exec("PRAGMA journal_mode = WAL")
	if err != nil {
		return fmt.Errorf("failed to enable WAL: %w", err)
	}
	// Set busy timeout to 5000ms (5 seconds) to reduce "database is locked" errors
	_, err = internalDb.Exec("PRAGMA busy_timeout = 5000")
	if err != nil {
		return fmt.Errorf("failed to set busy timeout: %w", err)
	}
	_, err = internalDb.Exec(createTables)
	if err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	log.Println("Internal database initialized successfully")
	return nil
}
