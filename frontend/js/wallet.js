import algosdk from "algosdk";
import { PeraWalletConnect }  from "@perawallet/connect";

// const mainnetId = 416001;
const testnetId = 416002;

// TODO: replace with mainnetId for mainnet
const peraWallet = new PeraWalletConnect({
    chainId: testnetId
});

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
    // Reconnect to the session when the component is mounted
    peraWallet
        .reconnectSession()
        .then((accounts) => {
            peraWallet.connector?.on("disconnect", handleDisconnectWalletClick);
            updateUI(accounts);
        })
        .catch((e) => console.log(e));
}

function handleConnectWalletClick(event) {
    event.preventDefault();

    peraWallet
        .connect()
        .then((accounts) => {
            peraWallet.connector?.on("disconnect", handleDisconnectWalletClick);
            updateUI(accounts);
        })
        .catch((error) => {
            if (error?.data?.type !== "CONNECT_MODAL_CLOSED") {
                console.log(error);
            }
        });
}

function handleDisconnectWalletClick(event) {
    peraWallet.disconnect().catch((error) => {
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
        let txnsToSign = [];
        for (let i = 0; i < txns.length; i++) {
            txnsToSign.push({ txn: txns[i], signers: [] });
        }
        txnsToSign[indexTxnToSign].signers = [address];

        try {
            const txnsFromPera = await peraWallet.signTransaction([txnsToSign], address);
            const signedTxnBinary = txnsFromPera[0];
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
// window.peraWallet = peraWallet;
// window.accountAddress = accountAddress;
// window.updateUIAfterConnect = updateUI;
// window.reconnectSession = reconnectSession;
// window.handleConnectWalletClick = handleConnectWalletClick;
// window.handleDisconnectWalletClick = handleDisconnectWalletClick;