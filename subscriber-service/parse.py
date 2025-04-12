import base64
from typing import Tuple

from algosdk import abi

DEPOSIT_SIGNATURE = "deposit(byte[32][],byte[32][],address)(uint64,byte[32])"
WITHDRAW_SIGNATURE = ("withdraw(byte[32][],byte[32][],account,account,bool)(uint64,byte[32])")

depositFilterName = "deposit"
withdrawFilterName = "withdraw"

def txn_result(result: str) -> Tuple[int, bytes]:
    """
    Parse the leaf index and tree root from the result of a transaction (the log message)
    The arc4 return signature is (uint64,byte[32]), leaf index and tree root respectively
    """
    decoded = base64.b64decode(result)

    if len(decoded) != 44: # 4 bytes prefix + 8 bytes uint + 32 bytes byte[32]
        raise ValueError(f"Decoded log has invalid length {len(decoded)}; expected 44 bytes")

    # Discard the 4-byte prefix.
    payload = decoded[4:]

    leaf_index_bytes = payload[:8]
    tree_root_bytes = payload[8:]

    leaf_index = int.from_bytes(leaf_index_bytes, byteorder="big")

    return leaf_index, tree_root_bytes


def deposit_args(args: list[str]) -> Tuple[bytes, str, int]:
    """
    Return (commitment, address, amount) from the arguments of a deposit transaction.
    The arc4 arg signature is (byte[32][],byte[32][],address), where the args are:
      - byte[32][] -> zk proof
      - byte[32][] -> zk public inputs:
                        - amount
                        - commitment
      - address    -> address sending the deposit
    """
    # args[0] is the method selector
    public_inputs_arg = args[2]
    public_inputs = base64.b64decode(public_inputs_arg)

    # the first two bytes encode the length of the byte[32] dynamic array
    amountBytes = get_Byte32(public_inputs, 0)
    amount = int.from_bytes(amountBytes, byteorder="big")

    commitmentBytes = get_Byte32(public_inputs, 1)

    address_arg = args[3]
    address_bytes = base64.b64decode(address_arg)
    address_abi_type : abi.AddressType = abi.ABIType.from_string("address")
    address = address_abi_type.decode(address_bytes)

    return commitmentBytes, address, amount


def withdraw_args(args: list[str], accounts: list[str]
                  ) -> Tuple[bytes, bytes, bytes, int, int]:
    """
    Return (commitment, withdrawal_address, nullifier, amount) from the arguments and accounts of a
    withdraw transaction.
    The arc4 arg signature is (byte[32][],byte[32][],account,account, bool), where the args are:
      - byte[32][] -> zk proof
      - byte[32][] -> zk public inputs:
                        - recipient_mod
                        - withdrawal_amount
                        - fee
                        - commitment
                        - nullifier
                        - merkle_root
      - account    -> account receiving the fee
      - account    -> account receiving the withdrawal
      - bool       -> no_change
    """
    # args[0] is the method selector
    public_inputs_arg = args[2]
    public_inputs = base64.b64decode(public_inputs_arg)

    commitment = get_Byte32(public_inputs, 3)
    nullifier = get_Byte32(public_inputs, 4)

    amount_bytes = get_Byte32(public_inputs, 1)
    amount = int.from_bytes(amount_bytes, byteorder="big")

    fee_bytes = get_Byte32(public_inputs, 2)
    fee = int.from_bytes(fee_bytes, byteorder="big")

    withdrawal_account_arg = args[4]
    withdrawal_account_pos_bytes = base64.b64decode(withdrawal_account_arg)
    # we subtract 1 because the index is 1-based but the array is 0-based
    withdrawal_address_pos = int.from_bytes(withdrawal_account_pos_bytes, byteorder="big") - 1
    withdrawal_address = accounts[withdrawal_address_pos]

    return commitment, withdrawal_address, nullifier, amount, fee

def get_Byte32(array: bytes, pos: int) -> bytes:
    """
    Return the byte[32] at position pos from an arc4 array of byte[32]
    """
    # first 2 bytes encode the length of the array
    start = 2 + pos*32
    return array[start:start+32]