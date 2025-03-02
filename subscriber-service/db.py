import logging
import sqlite3
import time

from models import Deposit, Note, Withdrawal, format_bytes

logger = logging.getLogger(__name__)

# Module-level variable for the shared connection, initialized by `initialize_db`
db_conn = None

DEPOSIT_TXN_TYPE = 0
WITHDRAWAL_TXN_TYPE = 1

def save_deposit(note: Note, deposit: Deposit, root: bytes, block: int) -> None:
    txns_table_sql = """
    INSERT INTO txns (leaf_index, commitment, txn_id, txn_type, address, amount, from_nullifier)
    VALUES (?, ?, ?, ?, ?, ?, ?)
    """
    global db_conn
    cursor = db_conn.cursor()

    try:
        cursor.execute(
            txns_table_sql,
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
        cursor.execute("UPDATE stats SET value = value + ? WHERE key = 'total_deposits'",
                       (deposit.amount,))
        cursor.execute("UPDATE stats SET value = value + 1 WHERE key = 'count_deposits'")

        cursor.execute("UPDATE roots SET value = ?, leaf_count = ? WHERE id = 1",
                          (root, note.leaf_index + 1))

        cursor.execute("UPDATE watermark SET value = ? WHERE id = 1", (block,))

        db_conn.commit()  # Commit everything at once

        logger.info("Saved deposit %s", deposit)
        logger.info("Saved note %s", note)
        logger.info("Tree root %s", format_bytes(root))
        logger.info("Block %s", block)

    except Exception as e:
        db_conn.rollback()  # Roll back changes on error
        logger.error("Failed to save deposit: %s", e)
        raise


def save_withdrawal(note: Note, withdrawal: Withdrawal, root: bytes, block: int) -> None:
    txns_table_sql = """
    INSERT INTO txns (leaf_index, commitment, txn_id, txn_type, address, amount, from_nullifier)
    VALUES (?, ?, ?, ?, ?, ?, ?)
    """
    global db_conn
    cursor = db_conn.cursor()

    try:
        cursor.execute(
            txns_table_sql,
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
        cursor.execute("UPDATE stats SET value = value + ? WHERE key = 'total_withdrawals'",
                       (withdrawal.amount,))

        cursor.execute("UPDATE stats SET value = value + ? WHERE key = 'total_fees'",
                       (withdrawal.fee,))

        cursor.execute("UPDATE roots SET value = ?, leaf_count = ? WHERE id = 1",
                          (root, note.leaf_index + 1))

        cursor.execute("UPDATE watermark SET value = ? WHERE id = 1", (block,))

        db_conn.commit()  # Commit everything at once

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
    global db_conn
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