import logging
import os
import sqlite3
import time
from datetime import datetime, timezone

from models import Deposit, Note, Withdrawal, format_bytes

logger = logging.getLogger(__name__)

# Module-level variable for the shared connection, initialized by `initialize_db`
db_conn = None

DEPOSIT_TXN_TYPE = 0
WITHDRAWAL_TXN_TYPE = 1

TXN_COLUMNS = (
    "leaf_index, commitment, txn_id, txn_type, address, amount, from_nullifier"
)
FATAL_ERROR_FILE = "subscriber.fatal"
fatal_error_path = None


class TxnConflictError(RuntimeError):
    pass


def has_fatal_error() -> bool:
    return fatal_error_path is not None and os.path.exists(fatal_error_path)


def record_fatal_error(error: Exception) -> None:
    if fatal_error_path is None:
        logger.critical("Fatal subscriber error before database initialization: %s", error)
        return

    timestamp = datetime.now(timezone.utc).isoformat()
    with open(fatal_error_path, "w", encoding="utf-8") as f:
        f.write(f"{timestamp}\n{error}\n")
    logger.critical("Recorded fatal subscriber error in %s", fatal_error_path)


def update_root_and_watermark(cursor: sqlite3.Cursor, root: bytes, leaf_count: int,
                              block: int) -> None:
    cursor.execute("SELECT value, leaf_count FROM roots WHERE id = 1")
    row = cursor.fetchone()
    current_root = row[0] if row else b""
    current_leaf_count = row[1] if row else 0

    if leaf_count == current_leaf_count and current_root != root:
        raise TxnConflictError(f"conflicting root for leaf count {leaf_count}")

    if leaf_count > current_leaf_count:
        cursor.execute(
            "UPDATE roots SET value = ?, leaf_count = ? WHERE id = 1",
            (root, leaf_count),
        )

    cursor.execute("UPDATE watermark SET value = MAX(value, ?) WHERE id = 1", (block,))


def existing_txn_matches(cursor: sqlite3.Cursor, expected: tuple) -> bool:
    leaf_index = expected[0]
    txn_id = expected[2]
    cursor.execute(
        f"SELECT {TXN_COLUMNS} FROM txns WHERE leaf_index = ? OR txn_id = ?",
        (leaf_index, txn_id),
    )
    rows = cursor.fetchall()
    if len(rows) != 1:
        return False
    return rows[0] == expected


def insert_or_verify_txn(cursor: sqlite3.Cursor, expected: tuple) -> bool:
    cursor.execute(
        f"INSERT OR IGNORE INTO txns ({TXN_COLUMNS}) VALUES (?, ?, ?, ?, ?, ?, ?)",
        expected,
    )
    if cursor.rowcount == 1:
        return True

    if existing_txn_matches(cursor, expected):
        logger.info("Transaction %s at leaf index %s was already saved",
                    expected[2], expected[0])
        return False

    raise TxnConflictError(
        f"conflicting transaction for leaf index {expected[0]} or txn id {expected[2]}"
    )


def save_deposit(note: Note, deposit: Deposit, root: bytes, block: int) -> None:
    global db_conn
    cursor = db_conn.cursor()

    try:
        inserted = insert_or_verify_txn(
            cursor,
            (
                note.leaf_index,
                note.commitment,
                note.txn_id,
                DEPOSIT_TXN_TYPE,
                deposit.address,
                deposit.amount,
                None,
            ),
        )

        if inserted:
            cursor.execute("UPDATE stats SET value = value + ? WHERE key = 'total_deposits'",
                           (deposit.amount,))
            cursor.execute("UPDATE stats SET value = value + 1 WHERE key = 'count_deposits'")

        update_root_and_watermark(cursor, root, note.leaf_index + 1, block)

        db_conn.commit()  # Commit everything at once

        if inserted:
            logger.info("Saved deposit %s", deposit)
            logger.info("Saved note %s", note)
        logger.info("Tree root %s", format_bytes(root))
        logger.info("Block %s", block)

    except Exception as e:
        db_conn.rollback()  # Roll back changes on error
        logger.error("Failed to save deposit: %s", e)
        raise


def save_withdrawal(note: Note, withdrawal: Withdrawal, root: bytes, block: int) -> None:
    global db_conn
    cursor = db_conn.cursor()

    try:
        inserted = insert_or_verify_txn(
            cursor,
            (
                note.leaf_index,
                note.commitment,
                note.txn_id,
                WITHDRAWAL_TXN_TYPE,
                withdrawal.address,
                withdrawal.amount,
                withdrawal.nullifier,
            ),
        )

        if inserted:
            cursor.execute("UPDATE stats SET value = value + ? WHERE key = 'total_withdrawals'",
                           (withdrawal.amount,))

            cursor.execute("UPDATE stats SET value = value + ? WHERE key = 'total_fees'",
                           (withdrawal.fee,))

        update_root_and_watermark(cursor, root, note.leaf_index + 1, block)

        db_conn.commit()  # Commit everything at once

        if inserted:
            logger.info("Saved withdrawal %s", withdrawal)
            logger.info("Saved note %s", note)
        logger.info("Tree root %s", format_bytes(root))
        logger.info("Block %s", block)

    except Exception as e:
        db_conn.rollback()  # Roll back changes on error
        logger.error("Failed to save withdrawal: %s", e)
        raise


def initialize_db(db_file: str) -> None:
    """
    Initialize the txn.db SQLite database with WAL mode, a busy timeout,
    and create the necessary tables if they don't exist. This function sets
    a module-level variable `db_conn` that will be used by other functions.
    """
    global db_conn, fatal_error_path
    fatal_error_path = os.path.join(os.path.dirname(db_file), FATAL_ERROR_FILE)
    db_conn = sqlite3.connect(db_file, timeout=5.0)
    cursor = db_conn.cursor()

    # Enable Write-Ahead Logging (WAL) and verify it
    cursor.execute("PRAGMA journal_mode = WAL;")
    mode = cursor.fetchone()
    if mode is None or mode[0].lower() != "wal":
        logger.warning("WAL mode not enabled, current mode: %s", mode)

    # Set busy timeout to 5000ms (5 seconds)
    cursor.execute("PRAGMA busy_timeout = 5000;")

    # Create txns table
    create_table_sql = """
    CREATE TABLE IF NOT EXISTS txns (
        leaf_index     INTEGER PRIMARY KEY,	 -- inserted note index in onchain merkle tree
        commitment     BLOB NOT NULL,        -- inserted note value in onchain merkle tree
        txn_id         TEXT UNIQUE NOT NULL, -- id of txn that inserted note (1st in group)
        txn_type       INTEGER NOT NULL, 	 -- 0 for deposits, 1 for withdrawal
        address        TEXT NOT NULL,        -- address making deposit or withdrawal
        amount         INTEGER NOT NULL,     -- amount deposited or withdrawn
        from_nullifier BLOB     			 -- spent note nullifier (NULL for deposits)
    ) STRICT;
    """
    cursor.execute(create_table_sql)

    # Create stats table
    create_stats_table_sql = """
    CREATE TABLE IF NOT EXISTS stats (
        key   TEXT PRIMARY KEY,
        value INTEGER
    ) STRICT;
    """
    cursor.execute(create_stats_table_sql)

    # Insert initial stats keys if they don't exist
    stats_keys = [
        ("total_deposits", 0),
        ("total_withdrawals", 0),
        ("total_fees", 0),
        ("count_deposits", 0)
    ]
    cursor.executemany(
        "INSERT OR IGNORE INTO stats (key, value) VALUES (?, ?)",
        stats_keys,
    )

    # Create block sync watermark table
    create_watermark_table_sql = """
    CREATE TABLE IF NOT EXISTS watermark (
        id INTEGER PRIMARY KEY CHECK (id = 1),
        value INTEGER NOT NULL
    ) STRICT;
    """
    cursor.execute(create_watermark_table_sql)

    # Initialize watermark to 0 if not already present
    cursor.execute("INSERT OR IGNORE INTO watermark (id, value) VALUES (1, 0)")

    # Create root table
    create_root_table_sql = """
    CREATE TABLE IF NOT EXISTS roots (
        id INTEGER PRIMARY KEY CHECK (id = 1),
        value BLOB NOT NULL,
        leaf_count INTEGER NOT NULL
    ) STRICT;
    """
    cursor.execute(create_root_table_sql)

    # Initialize root to empty bytes if not already present
    cursor.execute("INSERT OR IGNORE INTO roots (id, value, leaf_count) VALUES (1, x'', 0)")

    # Create index on commitment for faster lookups
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_txns_commitment ON txns(commitment)")

    db_conn.commit()
    logger.warning("Database initialized successfully")

def get_watermark() -> int:
    """
    Retrieve the current watermark from the DB.
    """
    global db_conn
    cursor = db_conn.cursor()
    cursor.execute("SELECT value FROM watermark WHERE id = 1")
    row = cursor.fetchone()
    return row[0]

def set_watermark(new_watermark: int) -> None:
    """
    Persist the new watermark value into the DB.
    """
    global db_conn
    cursor = db_conn.cursor()
    cursor.execute("INSERT OR REPLACE INTO watermark (id, value) VALUES (1, ?)", (new_watermark,))
    db_conn.commit()

def retry(operation, retries=3, initial_backoff=1, factor=3):
    """
    Calls the operation (a callable) and retries it up to `retries` times on transient errors.
    Transient errors are identified here as sqlite3.OperationalError.
    """
    backoff = initial_backoff
    for attempt in range(1, retries+1):
        try:
            return operation()
        except sqlite3.OperationalError as e:
            if attempt == retries:
                logger.error("Max retries reached, operation failed: %s", e)
                raise
            logger.warning("Transient error (attempt %d/%d): %s. Retrying in %d seconds",
                           attempt, retries, e, backoff)
            time.sleep(backoff)
            backoff *= factor
