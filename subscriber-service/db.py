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
NONINSERTED_WITHDRAWAL_COLUMNS = (
    "txn_id, leaf_index, address, amount, fee, from_nullifier, block"
)
FATAL_ERROR_FILE = "subscriber.fatal"
fatal_error_path = None


class SubscriberFatalError(RuntimeError):
    pass


class TxnConflictError(SubscriberFatalError):
    pass


class TxnIntegrityError(SubscriberFatalError):
    pass


def has_fatal_error() -> bool:
    return fatal_error_path is not None and os.path.exists(fatal_error_path)


def record_fatal_error(error: Exception) -> None:
    if fatal_error_path is None:
        logger.critical("Fatal subscriber error before database initialization: %s", error)
        return

    timestamp = datetime.now(timezone.utc).isoformat()
    fd = os.open(fatal_error_path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w", encoding="utf-8") as f:
        f.write(f"{timestamp}\n{error}\n")
    os.chmod(fatal_error_path, 0o600)
    logger.critical("Recorded fatal subscriber error in %s", fatal_error_path)


def update_root_and_watermark(cursor: sqlite3.Cursor, root: bytes, leaf_count: int,
                              block: int, inserted: bool) -> None:
    cursor.execute("SELECT value, leaf_count FROM roots WHERE id = 1")
    row = cursor.fetchone()
    if row is None:
        raise TxnIntegrityError("missing root row")

    current_root = row[0]
    current_leaf_count = row[1]

    if inserted and leaf_count != current_leaf_count + 1:
        raise TxnIntegrityError(
            f"non-contiguous root update: current_leaf_count={current_leaf_count}, "
            f"new_leaf_count={leaf_count}"
        )

    if not inserted and leaf_count > current_leaf_count:
        raise TxnIntegrityError(
            f"existing transaction would advance root from {current_leaf_count} "
            f"to {leaf_count}"
        )

    if leaf_count == current_leaf_count and current_root != root:
        raise TxnConflictError(f"conflicting root for leaf count {leaf_count}")

    if leaf_count > current_leaf_count:
        cursor.execute(
            "UPDATE roots SET value = ?, leaf_count = ? WHERE id = 1",
            (root, leaf_count),
        )

    cursor.execute("UPDATE watermark SET value = MAX(value, ?) WHERE id = 1", (block,))


def validate_txn_shape(expected: tuple) -> None:
    leaf_index, commitment, txn_id, txn_type, address, amount, from_nullifier = expected
    if leaf_index < 0:
        raise TxnIntegrityError(f"negative leaf index: {leaf_index}")
    if len(commitment) != 32:
        raise TxnIntegrityError(f"invalid commitment length: {len(commitment)}")
    if not txn_id:
        raise TxnIntegrityError("empty transaction id")
    if txn_type not in (DEPOSIT_TXN_TYPE, WITHDRAWAL_TXN_TYPE):
        raise TxnIntegrityError(f"invalid transaction type: {txn_type}")
    if not address:
        raise TxnIntegrityError("empty transaction address")
    if amount < 0:
        raise TxnIntegrityError(f"negative transaction amount: {amount}")
    if txn_type == DEPOSIT_TXN_TYPE and from_nullifier is not None:
        raise TxnIntegrityError("deposit transaction has a nullifier")
    if txn_type == WITHDRAWAL_TXN_TYPE and (
        from_nullifier is None or len(from_nullifier) != 32
    ):
        raise TxnIntegrityError("withdrawal transaction has an invalid nullifier")


def validate_noninserted_withdrawal_shape(expected: tuple) -> None:
    txn_id, leaf_index, address, amount, fee, from_nullifier, block = expected
    if not txn_id:
        raise TxnIntegrityError("empty no-change withdrawal transaction id")
    if leaf_index < 0:
        raise TxnIntegrityError(f"negative no-change withdrawal leaf index: {leaf_index}")
    if not address:
        raise TxnIntegrityError("empty no-change withdrawal address")
    if amount < 0:
        raise TxnIntegrityError(f"negative no-change withdrawal amount: {amount}")
    if fee < 0:
        raise TxnIntegrityError(f"negative no-change withdrawal fee: {fee}")
    if from_nullifier is None or len(from_nullifier) != 32:
        raise TxnIntegrityError("no-change withdrawal has an invalid nullifier")
    if block < 0:
        raise TxnIntegrityError(f"negative no-change withdrawal block: {block}")


def update_stat(cursor: sqlite3.Cursor, key: str, delta: int) -> None:
    cursor.execute("UPDATE stats SET value = value + ? WHERE key = ?", (delta, key))
    if cursor.rowcount != 1:
        raise TxnIntegrityError(f"missing stats row: {key}")


def validate_database_integrity(cursor: sqlite3.Cursor | None = None) -> None:
    if cursor is None:
        if db_conn is None:
            raise TxnIntegrityError("database connection is not initialized")
        cursor = db_conn.cursor()

    cursor.execute(
        "SELECT COUNT(*), COALESCE(MIN(leaf_index), 0), COALESCE(MAX(leaf_index), -1) "
        "FROM txns"
    )
    txn_count, min_leaf_index, max_leaf_index = cursor.fetchone()
    if txn_count > 0 and (min_leaf_index != 0 or max_leaf_index != txn_count - 1):
        raise TxnIntegrityError(
            f"non-contiguous txns leaf indexes: count={txn_count}, "
            f"min={min_leaf_index}, max={max_leaf_index}"
        )

    cursor.execute("SELECT value, leaf_count FROM roots WHERE id = 1")
    root_row = cursor.fetchone()
    if root_row is None:
        raise TxnIntegrityError("missing root row")

    root, leaf_count = root_row
    if leaf_count != txn_count:
        raise TxnIntegrityError(
            f"root leaf_count {leaf_count} does not match txns count {txn_count}"
        )
    if txn_count == 0 and root != b"":
        raise TxnIntegrityError("empty txns table has a non-empty root")

    cursor.execute(
        """
        SELECT
            COALESCE(SUM(CASE WHEN txn_type = ? THEN amount ELSE 0 END), 0),
            COALESCE(SUM(CASE WHEN txn_type = ? THEN amount ELSE 0 END), 0),
            COALESCE(SUM(CASE WHEN txn_type = ? THEN 1 ELSE 0 END), 0),
            COALESCE(SUM(CASE WHEN txn_type NOT IN (?, ?) THEN 1 ELSE 0 END), 0),
            COALESCE(SUM(CASE WHEN amount < 0 THEN 1 ELSE 0 END), 0),
            COALESCE(SUM(CASE WHEN length(commitment) != 32 THEN 1 ELSE 0 END), 0),
            COALESCE(SUM(CASE WHEN txn_type = ? AND from_nullifier IS NOT NULL THEN 1 ELSE 0 END), 0),
            COALESCE(SUM(CASE WHEN txn_type = ? AND (from_nullifier IS NULL OR length(from_nullifier) != 32) THEN 1 ELSE 0 END), 0)
        FROM txns
        """,
        (
            DEPOSIT_TXN_TYPE,
            WITHDRAWAL_TXN_TYPE,
            DEPOSIT_TXN_TYPE,
            DEPOSIT_TXN_TYPE,
            WITHDRAWAL_TXN_TYPE,
            DEPOSIT_TXN_TYPE,
            WITHDRAWAL_TXN_TYPE,
        ),
    )
    (
        deposit_total,
        withdrawal_total,
        deposit_count,
        invalid_type_count,
        invalid_amount_count,
        invalid_commitment_count,
        deposit_nullifier_count,
        withdrawal_nullifier_count,
    ) = cursor.fetchone()

    cursor.execute(
        """
        SELECT
            COALESCE(SUM(amount), 0),
            COALESCE(SUM(CASE WHEN leaf_index < 0 THEN 1 ELSE 0 END), 0),
            COALESCE(SUM(CASE WHEN amount < 0 THEN 1 ELSE 0 END), 0),
            COALESCE(SUM(CASE WHEN fee < 0 THEN 1 ELSE 0 END), 0),
            COALESCE(SUM(CASE WHEN from_nullifier IS NULL OR length(from_nullifier) != 32 THEN 1 ELSE 0 END), 0),
            COALESCE(SUM(CASE WHEN txn_id = '' THEN 1 ELSE 0 END), 0),
            COALESCE(SUM(CASE WHEN address = '' THEN 1 ELSE 0 END), 0),
            COALESCE(SUM(CASE WHEN block < 0 THEN 1 ELSE 0 END), 0)
        FROM noninserted_withdrawals
        """
    )
    (
        noninserted_withdrawal_total,
        noninserted_negative_leaf_count,
        noninserted_negative_amount_count,
        noninserted_negative_fee_count,
        noninserted_invalid_nullifier_count,
        noninserted_empty_txn_id_count,
        noninserted_empty_address_count,
        noninserted_negative_block_count,
    ) = cursor.fetchone()

    invalid_counts = {
        "invalid transaction types": invalid_type_count,
        "negative transaction amounts": invalid_amount_count,
        "invalid commitment lengths": invalid_commitment_count,
        "deposits with nullifiers": deposit_nullifier_count,
        "withdrawals without 32-byte nullifiers": withdrawal_nullifier_count,
        "no-change withdrawals with negative leaf indexes": noninserted_negative_leaf_count,
        "no-change withdrawals with negative amounts": noninserted_negative_amount_count,
        "no-change withdrawals with negative fees": noninserted_negative_fee_count,
        "no-change withdrawals without 32-byte nullifiers": noninserted_invalid_nullifier_count,
        "no-change withdrawals with empty transaction ids": noninserted_empty_txn_id_count,
        "no-change withdrawals with empty addresses": noninserted_empty_address_count,
        "no-change withdrawals with negative blocks": noninserted_negative_block_count,
    }
    for label, count in invalid_counts.items():
        if count:
            raise TxnIntegrityError(f"{label}: {count}")

    cursor.execute(
        "SELECT key, value FROM stats "
        "WHERE key IN ('total_deposits', 'total_withdrawals', 'count_deposits')"
    )
    stats = dict(cursor.fetchall())
    expected_stats = {
        "total_deposits": deposit_total,
        "total_withdrawals": withdrawal_total + noninserted_withdrawal_total,
        "count_deposits": deposit_count,
    }
    for key, expected in expected_stats.items():
        actual = stats.get(key)
        if actual != expected:
            raise TxnIntegrityError(
                f"stats mismatch for {key}: stored={actual}, expected={expected}"
            )


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
    validate_txn_shape(expected)

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


def existing_noninserted_withdrawal_matches(cursor: sqlite3.Cursor, expected: tuple) -> bool:
    txn_id = expected[0]
    from_nullifier = expected[5]
    cursor.execute(
        f"SELECT {NONINSERTED_WITHDRAWAL_COLUMNS} FROM noninserted_withdrawals "
        "WHERE txn_id = ? OR from_nullifier = ?",
        (txn_id, from_nullifier),
    )
    rows = cursor.fetchall()
    if len(rows) != 1:
        return False
    return rows[0] == expected


def insert_or_verify_noninserted_withdrawal(cursor: sqlite3.Cursor,
                                            expected: tuple) -> bool:
    validate_noninserted_withdrawal_shape(expected)

    cursor.execute(
        f"INSERT OR IGNORE INTO noninserted_withdrawals "
        f"({NONINSERTED_WITHDRAWAL_COLUMNS}) VALUES (?, ?, ?, ?, ?, ?, ?)",
        expected,
    )
    if cursor.rowcount == 1:
        return True

    if existing_noninserted_withdrawal_matches(cursor, expected):
        logger.info("No-change withdrawal %s was already saved", expected[0])
        return False

    raise TxnConflictError(
        f"conflicting no-change withdrawal for txn id {expected[0]} or nullifier"
    )


def save_deposit(note: Note, deposit: Deposit, root: bytes, block: int) -> None:
    global db_conn
    cursor = db_conn.cursor()

    try:
        expected = (
            note.leaf_index,
            note.commitment,
            note.txn_id,
            DEPOSIT_TXN_TYPE,
            deposit.address,
            deposit.amount,
            None,
        )
        inserted = insert_or_verify_txn(cursor, expected)

        if inserted:
            update_stat(cursor, "total_deposits", deposit.amount)
            update_stat(cursor, "count_deposits", 1)

        update_root_and_watermark(cursor, root, note.leaf_index + 1, block, inserted)

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
        expected = (
            note.leaf_index,
            note.commitment,
            note.txn_id,
            WITHDRAWAL_TXN_TYPE,
            withdrawal.address,
            withdrawal.amount,
            withdrawal.nullifier,
        )
        inserted = insert_or_verify_txn(cursor, expected)

        if inserted:
            update_stat(cursor, "total_withdrawals", withdrawal.amount)
            update_stat(cursor, "total_fees", withdrawal.fee)

        update_root_and_watermark(cursor, root, note.leaf_index + 1, block, inserted)

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


def save_noninserted_withdrawal(txn_id: str, withdrawal: Withdrawal, root: bytes,
                                block: int) -> None:
    global db_conn
    cursor = db_conn.cursor()

    try:
        expected = (
            txn_id,
            withdrawal.leaf_index,
            withdrawal.address,
            withdrawal.amount,
            withdrawal.fee,
            withdrawal.nullifier,
            block,
        )
        inserted = insert_or_verify_noninserted_withdrawal(cursor, expected)

        if inserted:
            update_stat(cursor, "total_withdrawals", withdrawal.amount)
            update_stat(cursor, "total_fees", withdrawal.fee)

        update_root_and_watermark(cursor, root, withdrawal.leaf_index + 1, block, False)

        db_conn.commit()

        if inserted:
            logger.info("Saved no-change withdrawal %s", withdrawal)
        logger.info("Tree root %s", format_bytes(root))
        logger.info("Block %s", block)

    except Exception as e:
        db_conn.rollback()
        logger.error("Failed to save no-change withdrawal: %s", e)
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

    # Create no-change withdrawal table. These withdrawals do not insert a new
    # note leaf, but they still affect totals and need replay-safe accounting.
    create_noninserted_withdrawals_sql = """
    CREATE TABLE IF NOT EXISTS noninserted_withdrawals (
        txn_id         TEXT PRIMARY KEY,
        leaf_index     INTEGER NOT NULL,
        address        TEXT NOT NULL,
        amount         INTEGER NOT NULL,
        fee            INTEGER NOT NULL,
        from_nullifier BLOB UNIQUE NOT NULL,
        block          INTEGER NOT NULL
    ) STRICT;
    """
    cursor.execute(create_noninserted_withdrawals_sql)

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
    validate_database_integrity()
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
