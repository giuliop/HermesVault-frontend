#!/usr/bin/env python3

from __future__ import annotations

import argparse
import sqlite3
import sys
from dataclasses import dataclass
from pathlib import Path

DEPOSIT_TXN_TYPE = 0
WITHDRAWAL_TXN_TYPE = 1


@dataclass(frozen=True)
class Counts:
    deposits: int = 0
    withdrawals: int = 0
    unknown: int = 0

    @property
    def total(self) -> int:
        return self.deposits + self.withdrawals + self.unknown


def parse_args() -> argparse.Namespace:
    data_dir = Path(__file__).resolve().parent
    default_txns_db = data_dir / "txns" / "txns.db"
    default_internal_db = data_dir / "internal" / "internal.db"

    parser = argparse.ArgumentParser(
        description=(
            "Compare deposit/withdrawal counts in data/txns/txns.db and "
            "data/internal/internal.db."
        )
    )
    parser.add_argument(
        "--txns-db",
        type=Path,
        default=default_txns_db,
        help=f"Path to txns.db (default: {default_txns_db})",
    )
    parser.add_argument(
        "--internal-db",
        type=Path,
        default=default_internal_db,
        help=f"Path to internal.db (default: {default_internal_db})",
    )
    return parser.parse_args()


def connect_read_only(db_path: Path) -> sqlite3.Connection:
    resolved_path = db_path.expanduser().resolve()
    uri = f"file:{resolved_path}?mode=ro"
    connection = sqlite3.connect(uri, uri=True, timeout=5.0)
    connection.row_factory = sqlite3.Row
    connection.execute("PRAGMA busy_timeout = 5000")
    return connection


def fetch_txns_counts(connection: sqlite3.Connection) -> Counts:
    counts = {DEPOSIT_TXN_TYPE: 0, WITHDRAWAL_TXN_TYPE: 0}
    rows = connection.execute(
        """
        SELECT txn_type, COUNT(*) AS count
        FROM txns
        GROUP BY txn_type
        """
    ).fetchall()

    unknown = 0
    for row in rows:
        txn_type = row["txn_type"]
        count = int(row["count"])
        if txn_type in counts:
            counts[txn_type] = count
        else:
            unknown += count

    return Counts(
        deposits=counts[DEPOSIT_TXN_TYPE],
        withdrawals=counts[WITHDRAWAL_TXN_TYPE],
        unknown=unknown,
    )


def fetch_internal_counts(connection: sqlite3.Connection, txns_db_path: Path, table: str) -> Counts:
    if table not in {"notes", "unconfirmed_notes"}:
        raise ValueError(f"unsupported table: {table}")

    connection.execute("ATTACH DATABASE ? AS txns_db", (str(txns_db_path.resolve()),))
    try:
        rows = connection.execute(
            f"""
            SELECT tx.txn_type, COUNT(*) AS count
            FROM {table} AS i
            LEFT JOIN txns_db.txns AS tx
              ON tx.txn_id = i.txn_id
            GROUP BY tx.txn_type
            """
        ).fetchall()
    finally:
        connection.execute("DETACH DATABASE txns_db")

    counts = Counts()
    for row in rows:
        txn_type = row["txn_type"]
        count = int(row["count"])
        if txn_type == DEPOSIT_TXN_TYPE:
            counts = Counts(
                deposits=counts.deposits + count,
                withdrawals=counts.withdrawals,
                unknown=counts.unknown,
            )
        elif txn_type == WITHDRAWAL_TXN_TYPE:
            counts = Counts(
                deposits=counts.deposits,
                withdrawals=counts.withdrawals + count,
                unknown=counts.unknown,
            )
        else:
            counts = Counts(
                deposits=counts.deposits,
                withdrawals=counts.withdrawals,
                unknown=counts.unknown + count,
            )
    return counts


def render_counts(label: str, counts: Counts) -> list[str]:
    lines = [
        label,
        f"  deposits:    {counts.deposits}",
        f"  withdrawals: {counts.withdrawals}",
        f"  total:       {counts.total}",
    ]
    if counts.unknown:
        lines.append(f"  unmatched:   {counts.unknown}")
    return lines


def main() -> int:
    args = parse_args()

    for db_path in (args.txns_db, args.internal_db):
        if not db_path.exists():
            print(f"Database not found: {db_path}", file=sys.stderr)
            return 1

    try:
        with connect_read_only(args.txns_db) as txns_connection:
            txns_counts = fetch_txns_counts(txns_connection)

        with connect_read_only(args.internal_db) as internal_connection:
            notes_counts = fetch_internal_counts(internal_connection, args.txns_db, "notes")
            unconfirmed_counts = fetch_internal_counts(
                internal_connection,
                args.txns_db,
                "unconfirmed_notes",
            )
    except sqlite3.Error as exc:
        print(f"Failed to read database: {exc}", file=sys.stderr)
        return 1

    lines: list[str] = []
    lines.extend(render_counts("txns.db", txns_counts))
    lines.append("")
    lines.extend(render_counts("internal.db notes", notes_counts))
    lines.append("")
    lines.extend(render_counts("internal.db unconfirmed_notes", unconfirmed_counts))
    lines.append("")
    lines.append("Differences vs txns.db")
    lines.append(f"  notes deposits missing:    {txns_counts.deposits - notes_counts.deposits}")
    lines.append(
        f"  notes withdrawals missing: {txns_counts.withdrawals - notes_counts.withdrawals}"
    )

    print("\n".join(lines))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
