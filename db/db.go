// Package db implements the database operations for the application
package db

import (
	"fmt"
	"log"

	"github.com/giuliop/HermesVault-frontend/models"

	_ "github.com/mattn/go-sqlite3"
)

func RegisterUnconfirmedNote(n *models.Note) (int64, error) {
	sql := `INSERT INTO unconfirmed_notes (
		commitment,
		nullifier,
		txn_id
		) VALUES (?, ?, ?)`
	result, err := internalDb.Exec(sql,
		n.Commitment(),
		n.Nullifier(),
		n.TxnID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to register unconfirmed note %v: %w", n, err)
	}
	leafIndex, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert ID: %w", err)
	}
	return leafIndex, nil
}

func SaveNote(n *models.Note) error {
	isNoteConfirmed := n.TxnID != models.EmptyTxnId &&
		n.LeafIndex != models.EmptyLeafIndex

	if !isNoteConfirmed {
		return fmt.Errorf("malformed confirmed note: %v", n)
	}

	sql := `INSERT INTO notes (leaf_index, commitment, txn_id, nullifier) VALUES (?, ?, ?, ?)`
	_, err := internalDb.Exec(sql, n.LeafIndex, n.Commitment(), n.TxnID, n.Nullifier())
	if err != nil {
		return fmt.Errorf("failed to insert note: %w", err)
	}

	// TODO: remove after removel of debug_notes table before MainNet
	debugSql := `INSERT INTO debug_notes (leaf_index, text) VALUES (?, ?)`
	_, err = internalDb.Exec(debugSql, n.LeafIndex, n.Text())
	if err != nil {
		return fmt.Errorf("failed to insert debug note: %w", err)
	}

	return nil
}

// GetLeafIndexByCommitment returns the leaf index of a note given its commitment
// error will be sql.ErrNoRows if no rows are returned
func GetLeafIndexByCommitment(commitment []byte) (int, error) {
	query := `SELECT leaf_index FROM txns WHERE commitment = ?`
	var index int
	err := txnsDb.QueryRow(query, commitment).Scan(&index)
	return index, err
}

// GetAllLeavesCommitments returns all leaf commitments in the database
func GetAllLeavesCommitments() ([][]byte, error) {
	query := `SELECT commitment FROM txns ORDER BY leaf_index ASC`
	rows, err := txnsDb.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var commitments [][]byte
	for rows.Next() {
		var commitment []byte
		if err := rows.Scan(&commitment); err != nil {
			return nil, err
		}
		commitments = append(commitments, commitment)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return commitments, nil
}

// GetRoot returns the Merkle root and the number of leaves in the tree
func GetRoot() (root []byte, leafCount int, err error) {
	query := `SELECT value, leaf_count FROM roots`
	err = txnsDb.QueryRow(query).Scan(&root, &leafCount)
	return
}

// DeleteUnconfirmedNote deletes an unconfirmed note from the database.
// It does not return an error if it fails
func DeleteUnconfirmedNote(id int64) {
	_, err := internalDb.Exec(`DELETE FROM unconfirmed_notes WHERE id = ?`, id)
	if err != nil {
		log.Printf("Error deleting unconfirmed note: %v", err)
	}
}

// Close closes all database connections
func Close() {
	if err := internalDb.Close(); err != nil {
		log.Printf("Error closing internalDb: %v", err)
	}
	if err := txnsDb.Close(); err != nil {
		log.Printf("Error closing txnsDb: %v", err)
	}
}

// GetStas returns the statistics from the database
func GetStats() (*models.StatData, error) {
	statsSql := `SELECT
		(SELECT value FROM stats WHERE key = 'total_deposits'),
		(SELECT value FROM stats WHERE key = 'total_withdrawals'),
		(SELECT value FROM stats WHERE key = 'total_fees'),
		(SELECT value FROM stats WHERE key = 'count_deposits')`
	var depositTotal, withdrawalTotal, feeTotal, depositCount uint64
	err := txnsDb.QueryRow(statsSql).Scan(&depositTotal, &withdrawalTotal, &feeTotal,
		&depositCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	var noteCount int
	notesSql := `SELECT COUNT(*) FROM txns`
	err = txnsDb.QueryRow(notesSql).Scan(&noteCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get note count: %w", err)
	}

	return &models.StatData{
		DepositTotal:    models.NewAmount(depositTotal),
		WithdrawalTotal: models.NewAmount(withdrawalTotal),
		FeeTotal:        models.NewAmount(feeTotal),
		DepositCount:    int(depositCount),
		NoteCount:       noteCount,
	}, nil
}
