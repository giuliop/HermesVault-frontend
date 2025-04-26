# [Hermes Vault](https://github.com/giuliop/HermesVault)
This is the frontend code for HermesVault, an application for private transaction on the Algorand blockchain.

Access it at [hermesvault.org](https://hermesvault.org)


## How to use

### Deposits

Deposits can be accesses from the `Deposit` tab.

Click the `Connect Wallet` button if your wallet is not connected already, at the moment only [Pera](https://perawallet.app/) wallet is supported. 

Once connected, you'll see your connected address in `From` field. Type in the amount of algo you want to deposit (<u>minimum 1 algo</u>) and press `Deposit`.

A confirmation screen will appear with the secret note <b>THAT YOU MUST SAVE</b> to withdraw your tokens in the future. You will have to paste it back in the appropriate section to proceed, to make sure you have copied it.

IF YOU LOSE THAT NOTE, NOBODY WILL BE ABLE TO HELP YOU RETRIEVE YOUR TOKENS !

Now click the `Confirm` button and you will be asked to open your Pera wallet and authorize the transaction. Note that the transaction fee will be 0.042 algo since it is a "heavy" transaction group which requires a lot of computation on the AVM to validate the zero knowledge proof involved.

If all goes well, you will get a success confirmation message. Otherwise you will get an error message explaining what went wrong.

### Withdrawals

Withdrawals can be accessed from the `Withdraw` tab.

Fill the `Amount` field with the amount you wish to withdraw, the `Address` field with the address you with to receive the withdrawal, and the `Note` field with the secret note you received when you deposited or made a withdrawal in the past.

The address receiving the tokens is not paying any transaction fee, so it can be a new, zero-balance account.

The deposit you are withdrawing from (identified by your secret note) will be reduced by the amount withdrawn and a transaction fee of 0.0753 algo to the Algorand blockchain.
What is left will be automatically inserted in the contract as a new deposit with a new secret note that will be shown to you in the next screen.

As with deposits, before the withdrawal transaction takes place, you will be asked to save the new secret note and prove you did by pasting it back in the appropriate section.
Click `Confirm` and if all goes well, you will get a success confirmation message. Otherwise you will get an error message explaining what went wrong.

### Fees
The frontend does not charge any fees.

The only fees to be covered are the algorand blockchain fees, in particular:
* For deposit, 0.056 algo will be paid in transaction fees by the signer
* For withdrawals, 0.0753 algo will be paid in trasaction fees by the application and taken from the original deposit, allowing a zero balance account to withdraw

### Privacy and security

While the HermesVault smart contracts are fully permissionless and decentralized, this frontend is a hosted website and a centralized entity, so it is subject to the laws and regulations of the jurisdiction it operates in.

For that purpose, the frontend stores receipts that could be used to link back specific withdrawals to the original deposits if so compelled by law enforcement. These receitps are encrypted with a secret key not stored on the server, so even if the database is compromised or leaked this information is safe.

In any case, the frontend can NEVER access users' funds, which are always 100% controlled by the users only. That's why if you lose your secret note nobody can help you retrieve your tokens.

There are three ways you can lose your funds:
1) You lose your secret note
2) Your device is compromised with malware that steals your secret note
3) The frontend is hacked and it serves you malicious code to steal your secret note
