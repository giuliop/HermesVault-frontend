#!/usr/bin/env python3

from __future__ import annotations

import argparse
import os
import re
import sqlite3
import sys
from collections import Counter
from dataclasses import dataclass
from decimal import Decimal
from pathlib import Path

DEPOSIT_TXN_TYPE = 0
WITHDRAWAL_TXN_TYPE = 1
MICROALGOS_PER_ALGO = Decimal("1000000")
ANSI_BOLD = "\033[1m"
ANSI_RESET = "\033[0m"

DEFAULT_DISPLAY_LIMIT = 20


@dataclass(frozen=True)
class TxnEntry:
    amount: int
    address: str


def parse_args() -> argparse.Namespace:
    default_db_relative = Path("txns") / "txns.db"
    default_db = Path(__file__).resolve().parent / default_db_relative

    parser = argparse.ArgumentParser(
        description="Display the largest deposits and withdrawals in txns.db.",
    )
    parser.add_argument(
        "--db",
        type=Path,
        default=default_db,
        help=f"Path to the SQLite database (default: {default_db_relative})",
    )
    parser.add_argument(
        "--limit",
        type=int,
        default=DEFAULT_DISPLAY_LIMIT,
        help=(
            "Number of deposits and withdrawals to display per column "
            f"(default: {DEFAULT_DISPLAY_LIMIT})"
        ),
    )
    parser.add_argument(
        "--color",
        choices=("auto", "always", "never"),
        default="auto",
        help="ANSI styling mode for repeated IDs (default: auto)",
    )
    return parser.parse_args()


def connect_read_only(db_path: Path) -> sqlite3.Connection:
    resolved_path = db_path.expanduser().resolve()
    uri = f"file:{resolved_path}?mode=ro"
    connection = sqlite3.connect(uri, uri=True, timeout=5.0)
    connection.row_factory = sqlite3.Row
    connection.execute("PRAGMA busy_timeout = 5000")
    return connection


def fetch_top_entries(
    connection: sqlite3.Connection,
    txn_type: int,
    limit: int,
) -> list[TxnEntry]:
    rows = connection.execute(
        """
        SELECT amount, address
        FROM txns
        WHERE txn_type = ?
        ORDER BY amount DESC
        LIMIT ?
        """,
        (txn_type, limit),
    ).fetchall()
    return [
        TxnEntry(amount=int(row["amount"]), address=row["address"])
        for row in rows
    ]


def format_algos(microalgos: int) -> str:
    algos = Decimal(microalgos) / MICROALGOS_PER_ALGO
    return f"{algos:,.6f} ALGO"


def build_address_ids(
    deposits: list[TxnEntry],
    withdrawals: list[TxnEntry],
) -> tuple[dict[str, int], set[int]]:
    address_ids: dict[str, int] = {}
    counts: Counter[int] = Counter()
    max_rows = max(len(deposits), len(withdrawals))

    for row_index in range(max_rows):
        for entries in (deposits, withdrawals):
            if row_index >= len(entries):
                continue

            address = entries[row_index].address
            if address not in address_ids:
                address_ids[address] = len(address_ids) + 1

            counts[address_ids[address]] += 1

    repeated_ids = {address_id for address_id, count in counts.items() if count > 1}
    return address_ids, repeated_ids


def render_column(
    title: str,
    entries: list[TxnEntry],
    address_ids: dict[str, int],
) -> list[str]:
    if not entries:
        return [title, "-"]

    lines = [title]
    for index, entry in enumerate(entries, start=1):
        address_id = address_ids[entry.address]
        lines.append(f"{index:>2}. {format_algos(entry.amount)} [{address_id}]")
    return lines


def build_two_column_lines(left_lines: list[str], right_lines: list[str]) -> list[str]:
    total_rows = max(len(left_lines), len(right_lines))
    left_width = max(len(line) for line in left_lines)
    lines = [
        f"{left_lines[0]:<{left_width}}  {right_lines[0]}",
        f"{'-' * left_width}  {'-' * len(right_lines[0])}",
    ]

    for row in range(1, total_rows):
        left = left_lines[row] if row < len(left_lines) else ""
        right = right_lines[row] if row < len(right_lines) else ""
        lines.append(f"{left:<{left_width}}  {right}")

    return lines


def render_address_legend(address_ids: dict[str, int]) -> list[str]:
    lines = ["", "Addresses"]
    for address, address_id in sorted(address_ids.items(), key=lambda item: item[1]):
        lines.append(f"[{address_id}] {address}")
    return lines


def should_bold_ids(color_mode: str) -> bool:
    if color_mode == "always":
        return True
    if color_mode == "never":
        return False
    return sys.stdout.isatty() and os.getenv("NO_COLOR") is None


def bold_repeated_ids(
    lines: list[str],
    repeated_ids: set[int],
    color_mode: str,
) -> list[str]:
    if not repeated_ids or not should_bold_ids(color_mode):
        return lines

    repeated_id_set = set(repeated_ids)
    pattern = re.compile(r"\[(\d+)\]")

    def repl(match: re.Match[str]) -> str:
        address_id = int(match.group(1))
        token = match.group(0)
        if address_id in repeated_id_set:
            return f"{ANSI_BOLD}{token}{ANSI_RESET}"
        return token

    return [pattern.sub(repl, line) for line in lines]


def main() -> int:
    args = parse_args()

    if args.limit <= 0:
        print("--limit must be greater than 0", file=sys.stderr)
        return 1

    if not args.db.exists():
        print(f"Database not found: {args.db}", file=sys.stderr)
        return 1

    try:
        with connect_read_only(args.db) as connection:
            deposits = fetch_top_entries(connection, DEPOSIT_TXN_TYPE, args.limit)
            withdrawals = fetch_top_entries(
                connection,
                WITHDRAWAL_TXN_TYPE,
                args.limit,
            )
    except sqlite3.Error as exc:
        print(f"Failed to read {args.db}: {exc}", file=sys.stderr)
        return 1

    address_ids, repeated_ids = build_address_ids(deposits, withdrawals)
    left_lines = render_column("Largest deposits", deposits, address_ids)
    right_lines = render_column("Largest withdrawals", withdrawals, address_ids)
    legend_lines = render_address_legend(address_ids)

    table_lines = build_two_column_lines(left_lines, right_lines)
    output_lines = table_lines + legend_lines
    output_lines = bold_repeated_ids(output_lines, repeated_ids, args.color)
    print("\n".join(output_lines))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
