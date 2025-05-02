import algosdk from "algosdk";

import { WalletManager, WalletId, NetworkId } from '@txnlab/use-wallet'

const manager = new WalletManager({
    wallets: [
        WalletId.PERA,
        WalletId.DEFLY,
        {
            id: WalletId.LUTE,
            options: {
                siteName: 'HermesVault'
            }
        }
    ],
    defaultNetwork: NetworkId.MAINNET
})

let accountAddress = "";

function updateUI(accounts) {
    const addressInput = document.querySelector('[data-wallet-address]');
    const depositButton = document.querySelector('[data-wallet-deposit-button]');
    const walletButton = document.querySelector('[data-wallet-connect-button]');
    if (accounts.length) {
        accountAddress = accounts[0];
        addressInput.value = accountAddress;
        // Trigger the blur event to trim the address in the UI
        addressInput.dispatchEvent(new Event('blur'));
        depositButton.classList.remove('hidden');
        walletButton.textContent = "Disconnect Wallet";
    } else {
        accountAddress = "";
        addressInput.value = "";
        depositButton.classList.add('hidden');
        walletButton.textContent = "Connect Wallet";
    }
}

function reconnectSession() {
    // Resume the session if the user has already connected their wallet
    manager
        .resumeSessions()
        .then(() => {
            const accounts = manager.activeWalletAddresses || [];
            updateUI(accounts);
        })
        .catch((e) => console.log(e));
}

async function handleConnect(wallet) {
    try {
        await wallet.connect()
        const accounts = manager.activeWalletAddresses || [];
        updateUI(accounts);
    } catch (error) {
        console.error('Failed to connect:', error)
    }
}

function handleConnectWalletClick() {
    // Create a modal dialog with a list of wallets
    const modal = document.createElement('dialog');
    modal.classList.add('modal');

    const container = document.createElement('div');
    container.classList.add('wallet-options');

    const header = document.createElement('div');
    header.classList.add('wallet-modal-header');

    const title = document.createElement('h2');
    title.textContent = 'Choose a Wallet';

    const closeButton = document.createElement('button');
    closeButton.innerHTML = '&times;';
    closeButton.classList.add('exit-button');
    closeButton.addEventListener('click', () => modal.close());

    header.appendChild(title);
    header.appendChild(closeButton);
    container.appendChild(header);

    for (const wallet of manager.wallets) {
        const button = document.createElement('button');
        button.innerHTML = `
        <img src="${wallet.metadata.icon}" alt="${wallet.metadata.name}" />
        <span>${wallet.metadata.name}</span>
      `;;
        button.addEventListener('click', () => {
            handleConnect(wallet);
            modal.close();
        });
        container.appendChild(button);
    }

    modal.appendChild(container);
    document.body.appendChild(modal);
    modal.showModal();

    // remove modal from DOM after it's closed
    modal.addEventListener('close', () => {
        modal.remove();
    });
}

function handleDisconnectWalletClick(event) {
    manager.disconnect().catch((error) => {
        console.log(error);
    });

    updateUI([]);
}

// trigger on connet wallet button and confirm deposit button
document.addEventListener('click', async (event) => {
    if (event.target.matches('[data-wallet-connect-button]')) {
        event.preventDefault();
        if (accountAddress) {
            handleDisconnectWalletClick(event);
        } else {
            handleConnectWalletClick(event);
        }
    }
    if (event.target.matches('[data-wallet-confirm-deposit-button]')) {
        event.preventDefault();
        const address = document.querySelector('[data-wallet-address-input]').value;
        const txnsJson = document.querySelector('[data-wallet-txnsjson-input]').value;
        const indexTxnToSign = document.querySelector(
            '[data-wallet-index-txn-to-sign-input]').value;
        const txns = decodeJsonTransactions(txnsJson);
        // let txnsToSign = [];
        // for (let i = 0; i < txns.length; i++) {
        //     txnsToSign.push({ txn: txns[i], signers: [] });
        // }
        // txnsToSign[indexTxnToSign].signers = [address];
        const txnsToSign = txns;

        try {
            const txnsFromWallet = await manager.signTransactions(txnsToSign, [parseInt(indexTxnToSign, 10)]);
            // const signedTxnBinary = txnsFromWallet[0];
            const signedTxnBinary = txnsFromWallet[parseInt(indexTxnToSign, 10)];
            const signedTxnBase64 = uint8ArrayToBase64(signedTxnBinary);
            document.querySelector('[data-wallet-signed-txn-input]').value = signedTxnBase64;
            const form = event.target.closest('form');
        event.target.disabled = true;
            htmx.trigger(form, 'submit');

        } catch (error) {
            console.log(error);
            let errorBox = document.querySelector('[data-wallet-errorBox]');
            errorBox.innerHTML = (
                "Error signing the transaction, please try again");
            htmx.trigger(errorBox, 'htmx:after-swap');
            event.target.disabled = false;
        }
    };
});

// trigger on wallet form load
document.addEventListener('htmx:load', (event) => {
    if (event.detail.elt.matches('[data-wallet]')) {
        if (!accountAddress) {
            reconnectSession();
        } else {
            updateUI([accountAddress]);
        }
    }
});

// decode a json string representing an array of transactions.
// each array element is the base64 msgpack encoding of an unsigned transaction
function decodeJsonTransactions(json) {
    const txns = JSON.parse(json);
    return txns.map(txn => algosdk.decodeUnsignedTransaction(new Uint8Array(Buffer.from(txn, 'base64'))));
}

// convert a Uint8Array to a base64 string
function uint8ArrayToBase64(uint8Array) {
    let binaryString = '';
    for (let i = 0; i < uint8Array.length; i++) {
        binaryString += String.fromCharCode(uint8Array[i]);
    }
    return btoa(binaryString);
}

// Make functions and variables accessible from the console for debugging
// window.accountAddress = accountAddress;
// window.updateUI = updateUI;
// window.reconnectSession = reconnectSession;
// window.handleConnectWalletClick = handleConnectWalletClick;
// window.handleDisconnectWalletClick = handleDisconnectWalletClick;