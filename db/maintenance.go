package db

import (
	"bytes"
	"context"
	"database/sql"
	"log"
	"time"
)

// StartCleanupRoutine starts a goroutine that periodically runs cleanup.
// It returns a cancel function that can be used to stop the routine.
func StartCleanupRoutine(ctx context.Context, interval time.Duration) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// log.Println("Running cleanup of unconfirmed notes...")
				CleanupUnconfirmedNotes()
			case <-ctx.Done():
				log.Println("Cleanup routine stopped")
				return
			}
		}
	}()
	return cancel
}

// CleanupUnconfirmedNotes cleans up unconfirmed_notes rows
// For each note:
//   - It retrieves the corresponding transaction from txnsDb using txn_id
//   - If found, it checks that commitment also matches:
//   - If so, it moves it tothe notes table
//   - Otherwise, it logs an error and leaves the note unconfirmed
//   - Finally, if the note is older than 7 days, it deletes it as stale
func CleanupUnconfirmedNotes() {
	// Query all rows from unconfirmed_notes.
	rows, err := internalDb.Query(`
		SELECT id, commitment, nullifier, txn_id, created_at
		FROM unconfirmed_notes
	`)
	if err != nil {
		log.Printf("failed to query unconfirmed_notes: %v", err)
		return
	}
	defer rows.Close()

	// Expected timestamp layout in SQLite (CURRENT_TIMESTAMP format).
	const timeLayout = "2006-01-02 15:04:05"

	for rows.Next() {
		var id int
		var commitment []byte
		var nullifier []byte
		var txnID string
		var createdAt string

		if err := rows.Scan(&id, &commitment, &nullifier, &txnID, &createdAt); err != nil {
			log.Printf("failed to scan unconfirmed note id %d: %v", id, err)
			continue
		}

		// Parse the created_at timestamp.
		noteTime, err := time.Parse(timeLayout, createdAt)
		if err != nil {
			log.Printf("failed to parse created_at (%s) for note id %d: %v", createdAt, id, err)
			// If we cannot parse the timestamp, skip cleanup for this note.
			continue
		}

		// Query the transaction record by txn_id.
		var txnLeafIndex int
		var txnCommitment []byte
		err = txnsDb.QueryRow(`
			SELECT leaf_index, commitment
			FROM txns
			WHERE txn_id = ?
		`, txnID).Scan(&txnLeafIndex, &txnCommitment)

		if err == sql.ErrNoRows {
			log.Printf("No matching transaction found for unconfirmed note id %d with txn_id %s", id, txnID)
			// Cleanup: if the note wasn't processed and is older than 7 days, delete it.
			if time.Since(noteTime) > 7*24*time.Hour {
				_, err = internalDb.Exec(`DELETE FROM unconfirmed_notes WHERE id = ?`, id)
				if err != nil {
					log.Printf("failed to delete stale unconfirmed note id %d: %v", id, err)
					continue
				}
				log.Printf("Deleted stale unconfirmed note id %d (created at %s)", id,
					createdAt)
			}
		} else if err != nil {
			log.Printf("failed to query txn for unconfirmed note id %d: %v", id, err)
			continue
		} else {
			// Transaction found; verify that the commitment matches.
			if bytes.Equal(txnCommitment, commitment) {
				// Begin a transaction.
				tx, err := internalDb.Begin()
				if err != nil {
					log.Printf("failed to begin transaction for unconfirmed note id %d: %v", id, err)
					continue
				}

				// Insert the note into the notes table.
				_, err = tx.Exec(
					`INSERT INTO notes (leaf_index, commitment, nullifier, txn_id) VALUES (?, ?, ?, ?)`,
					txnLeafIndex, commitment, nullifier, txnID)
				if err != nil {
					tx.Rollback()
					log.Printf("failed to insert note for unconfirmed note id %d: %v", id, err)
					continue
				}
				// Delete the note from unconfirmed_notes.
				_, err = tx.Exec(`DELETE FROM unconfirmed_notes WHERE id = ?`, id)
				if err != nil {
					tx.Rollback()
					log.Printf("failed to delete processed unconfirmed note id %d: %v", id, err)
					continue
				}

				if err = tx.Commit(); err != nil {
					log.Printf("failed to commit transaction for unconfirmed note id %d: %v", id, err)
					continue
				}

				log.Printf("Processed unconfirmed note id %d: moved to notes with leaf_index %d", id, txnLeafIndex)
			} else {
				// Commitment mismatch: log an error and leave the note intact.
				log.Printf("Commitment mismatch for unconfirmed note id %d with txn_id %s", id, txnID)
				if err := RecordFatalMarker(
					"unconfirmed note commitment mismatch: id=%d txn_id=%s txns_leaf_index=%d",
					id, txnID, txnLeafIndex,
				); err != nil {
					log.Printf("failed to record fatal marker for unconfirmed note id %d: %v", id, err)
				}
			}
		}
	}

	if err = rows.Err(); err != nil {
		log.Printf("error iterating over unconfirmed_notes rows: %v", err)
	}
}
