#!/usr/bin/env python3
"""
Script to encrypt existing nullifiers in the database.
This is a one-time migration script to encrypt unencrypted nullifiers.
"""

import argparse
import os
import sqlite3
import sys

import nacl.public
import nacl.utils

INTERNAL_DB_PATH = "../data/internal/internal.db"
PUBLIC_KEY_PATH = "../db/encrypt/generate-key/public_key.bin"

def load_public_key(key_path):
    """Load the public key from the file"""
    with open(key_path, 'rb') as f:
        public_key_bytes = f.read()
        if len(public_key_bytes) != nacl.public.PublicKey.SIZE:
            raise ValueError(f"Invalid public key size: {len(public_key_bytes)}")
        return nacl.public.PublicKey(public_key_bytes)

def encrypt_nullifier(nullifier, public_key):
    """
    Encrypt a nullifier using NaCl box with an ephemeral keypair
    Format: [ephemeral_public_key][nonce][ciphertext]
    """
    if nullifier is None:
        return None

    # Generate an ephemeral keypair for this encryption
    ephemeral_private_key = nacl.public.PrivateKey.generate()
    ephemeral_public_key = ephemeral_private_key.public_key

    # Generate a random nonce
    nonce = nacl.utils.random(nacl.public.Box.NONCE_SIZE)

    # Create a box for encryption
    box = nacl.public.Box(ephemeral_private_key, public_key)

    # Encrypt the nullifier
    ciphertext = box.encrypt(nullifier, nonce)

    # Format: [ephemeral public key][nonce][ciphertext]
    # Note: nacl.public.Box.encrypt already includes the nonce in its output
    # but we need a specific format to match Go implementation
    result = bytearray()
    result.extend(ephemeral_public_key.encode())  # 32 bytes
    result.extend(nonce)  # 24 bytes
    result.extend(ciphertext.ciphertext)  # variable length

    return bytes(result)

def is_nullifier_encrypted(nullifier):
    """
    Check if a nullifier is likely already encrypted
    Encrypted nullifiers are at least 56 bytes long (32 + 24) for ephemeral key + nonce
    """
    if nullifier is None:
        return True  # Treat None as "already encrypted" to skip it

    # Encrypted nullifiers will be significantly longer than original ones
    # They should be at least 32 (ephemeral public key) + 24 (nonce) = 56 bytes
    return len(nullifier) >= 56

def main():
    parser = argparse.ArgumentParser(description='Encrypt unencrypted nullifiers in the database')
    parser.add_argument('--dry-run', action='store_true',
                        help='Perform a dry run without making changes')
    args = parser.parse_args()

    db_path = INTERNAL_DB_PATH
    key_path = PUBLIC_KEY_PATH
    dry_run = args.dry_run

    if not os.path.exists(db_path):
        print(f"Error: Database file not found at {db_path}")
        return 1

    if not os.path.exists(key_path):
        print(f"Error: Public key file not found at {key_path}")
        return 1

    try:
        public_key = load_public_key(key_path)
        print(f"Loaded public key from {key_path}")
    except Exception as e:
        print(f"Error loading public key: {e}")
        return 1

    print(f"{'DRY RUN - ' if dry_run else ''}Processing database at {db_path}")

    conn = sqlite3.connect(db_path)
    conn.row_factory = sqlite3.Row
    cursor = conn.cursor()

    # Process notes table
    print("\nProcessing 'notes' table...")
    cursor.execute("SELECT leaf_index, nullifier FROM notes")
    notes = cursor.fetchall()
    notes_to_update = []

    for note in notes:
        leaf_index = note['leaf_index']
        nullifier = note['nullifier']

        if not is_nullifier_encrypted(nullifier):
            print(f"  Encrypting nullifier for note with leaf_index {leaf_index}")
            encrypted_nullifier = encrypt_nullifier(nullifier, public_key)
            notes_to_update.append((encrypted_nullifier, leaf_index))
        else:
            print(f"  Nullifier for note with leaf_index {leaf_index} already encrypted, skipping")

    # Process unconfirmed_notes table
    print("\nProcessing 'unconfirmed_notes' table...")
    cursor.execute("SELECT id, nullifier FROM unconfirmed_notes")
    unconfirmed_notes = cursor.fetchall()
    unconfirmed_notes_to_update = []

    for note in unconfirmed_notes:
        note_id = note['id']
        nullifier = note['nullifier']

        if not is_nullifier_encrypted(nullifier):
            print(f"  Encrypting nullifier for unconfirmed note with id {note_id}")
            encrypted_nullifier = encrypt_nullifier(nullifier, public_key)
            unconfirmed_notes_to_update.append((encrypted_nullifier, note_id))
        else:
            print(f"  Nullifier for unconfirmed note with id {note_id} already encrypted, skipping")

    # Update the database if not in dry run mode
    if not dry_run:
        if notes_to_update:
            print(f"\nUpdating {len(notes_to_update)} notes...")
            cursor.executemany("UPDATE notes SET nullifier = ? WHERE leaf_index = ?", notes_to_update)

        if unconfirmed_notes_to_update:
            print(f"Updating {len(unconfirmed_notes_to_update)} unconfirmed notes...")
            cursor.executemany("UPDATE unconfirmed_notes SET nullifier = ? WHERE id = ?", unconfirmed_notes_to_update)

        conn.commit()
        print("Database updates committed successfully")
    else:
        print("\nDRY RUN - No changes made to the database")
        print(f"Would have updated {len(notes_to_update)} notes and {len(unconfirmed_notes_to_update)} unconfirmed notes")

    conn.close()

    if not dry_run:
        print("\nMigration completed successfully!")
    return 0

if __name__ == "__main__":
    sys.exit(main())
