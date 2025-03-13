#!/usr/bin/env python3
import argparse
import sqlite3

INTERNAL_DB_PATH = "../data/internal/internal.db"
TXNS_DB_PATH = "../data/txns/txns.db"

def extract_amount(secret_note_text):
    """
    Extracts the change amount from the first 8 bytes of the secret note.
    Assumes the secret note is hex-encoded (i.e., the first 16 characters represent 8 bytes).
    """
    hex_part = secret_note_text[:16]
    if len(hex_part) < 16:
        raise ValueError("Not enough hex characters")
    amount = int(hex_part, 16)
    return amount

def follow_change_chain(current_note, txns_cursor, internal_cursor):
    """
    Iteratively follows the change deposit chain starting from the given note.
    For the current note, it checks if a withdrawal exists that spent it.
    If found, retrieves the change deposit note (using the withdrawal's leaf_index),
    extracts the change amount from its secret note (via debug_notes),
    and then checks if that change deposit note itself has been spent.
    The iteration continues until an unspent change deposit is found or the change is zero.

    Returns a tuple (change_amount, leaf_index, secret_note) if an unspent change deposit is found,
    or None if the chain ends with a fully spent deposit or a zero change amount.
    """
    while True:
        # Look for a withdrawal that spent the current note.
        txns_cursor.execute(
            "SELECT leaf_index FROM txns WHERE txn_type = 1 AND from_nullifier = ?",
            (current_note["nullifier"],)
        )
        withdrawal = txns_cursor.fetchone()
        if not withdrawal:
            # This situation shouldn't occur since this function is meant to be called with a
            # deposit that has been spent
            return None

        # Retrieve the change deposit note using the withdrawal's leaf_index.
        change_leaf = withdrawal["leaf_index"]
        internal_cursor.execute(
            "SELECT leaf_index, nullifier FROM notes WHERE leaf_index = ?",
            (change_leaf,)
        )
        change_note = internal_cursor.fetchone()
        if not change_note:
            print(f"WARNING: No note found in internal.db for change deposit at leaf_index {change_leaf}")
            return None

        # Fetch the secret note for the change deposit.
        internal_cursor.execute(
            "SELECT text FROM debug_notes WHERE leaf_index = ?",
            (change_leaf,)
        )
        debug_note = internal_cursor.fetchone()
        secret_note = debug_note["text"] if debug_note else "N/A"

        # Extract the change amount from the secret note.
        change_amount = extract_amount(secret_note)
        if change_amount <= 0:
            # The change amount is zero or invalid.
            if change_amount < 0:
                print(f"WARNING: Invalid change amount {change_amount} for leaf_index "
                      f"{change_leaf}")
            return None

        # Check if this change deposit note has been spent.
        txns_cursor.execute(
            "SELECT leaf_index FROM txns WHERE txn_type = 1 AND from_nullifier = ?",
            (change_note["nullifier"],)
        )
        next_withdrawal = txns_cursor.fetchone()
        if not next_withdrawal:
            # Unspent change deposit found.
            return (change_amount, change_leaf, secret_note)
        else:
            # This change deposit has been spent; follow the chain further.
            current_note = change_note

def main():
    parser = argparse.ArgumentParser(
        description="Query unspent deposits and unspent change deposits for an Algorand address."
    )
    parser.add_argument("address", help="Algorand address to query")
    args = parser.parse_args()
    address = args.address

    # Connect to txns.db (for deposits/withdrawals).
    txns_conn = sqlite3.connect(TXNS_DB_PATH)
    txns_conn.row_factory = sqlite3.Row
    txns_cursor = txns_conn.cursor()

    # Connect to internal.db (for notes and debug_notes).
    internal_conn = sqlite3.connect(INTERNAL_DB_PATH)
    internal_conn.row_factory = sqlite3.Row
    internal_cursor = internal_conn.cursor()

    results = []  # List to collect output records.

    # 1. Fetch deposits for the given address from txns.db.
    txns_cursor.execute("""
        SELECT leaf_index, txn_id, amount
        FROM txns
        WHERE txn_type = 0 AND address = ?
    """, (address,))
    deposits = txns_cursor.fetchall()

    for deposit in deposits:
        leaf_index = deposit["leaf_index"]
        deposit_amount = deposit["amount"]

        # 2. Retrieve the corresponding note for this deposit from internal.db's notes table.
        internal_cursor.execute("""
            SELECT leaf_index, nullifier
            FROM notes
            WHERE leaf_index = ?
        """, (leaf_index,))
        note = internal_cursor.fetchone()
        if not note:
            print(f"WARNING: No note found in internal.db for leaf_index {leaf_index}")
            continue

        # 3. Check if this deposit has been spent.
        txns_cursor.execute("""
            SELECT leaf_index
            FROM txns
            WHERE txn_type = 1 AND from_nullifier = ?
        """, (note["nullifier"],))
        withdrawal = txns_cursor.fetchone()

        if not withdrawal:
            # Deposit is unspent.
            internal_cursor.execute(
                "SELECT text FROM debug_notes WHERE leaf_index = ?",
                (leaf_index,)
            )
            debug_note = internal_cursor.fetchone()
            secret_note = debug_note["text"] if debug_note else "N/A"
            results.append({
                "type": "deposit",
                "amount": deposit_amount,
                "leaf_index": leaf_index,
                "secret_note": secret_note
            })
        else:
            # Deposit was spent; follow the change deposit chain.
            change_info = follow_change_chain(note, txns_cursor, internal_cursor)
            if change_info:
                change_amount, change_leaf, secret_note = change_info
                results.append({
                    "type": "change",
                    "amount": change_amount,
                    "leaf_index": change_leaf,
                    "secret_note": secret_note
                })
            else:
                # final change amount is zero
                continue

    # 4. Print out the results.
    if results:
        print("Unspent funds:")
        # amount is microAlgos; convert to Algos for display
        for r in results:
            algo_amount = r["amount"] / 1e6
            print(f"Type: {r['type']}, Amount: {algo_amount}, Leaf Index: {r['leaf_index']}, "
                  f"Secret Note: {r['secret_note']}")
    else:
        print("No unspent deposits or change deposits found for address", address)

    # Close the database connections.
    txns_conn.close()
    internal_conn.close()

if __name__ == "__main__":
    main()
