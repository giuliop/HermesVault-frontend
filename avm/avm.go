// package avm provides functionalities to interact with the smart contracts onchain
package avm

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/giuliop/HermesVault-frontend/config"

	"github.com/algorand/go-algorand-sdk/v2/abi"
	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
)

type algodConfig struct {
	URL   string
	Token string
}

var client *algod.Client

func init() {
	if config.AlgodPath == "" {
		client = devnetAlgodClient()
		return
	}

	var err error
	conf := &algodConfig{}

	if strings.Contains(config.AlgodPath, "http") {
		conf.URL = config.AlgodPath
		conf.Token = config.AlgodToken
	} else {
		conf, err = readAlgodConfigFromDir(config.AlgodPath)
		if err != nil {
			log.Fatalf("failed to read algod config: %v", err)
		}
	}

	client, err = algod.MakeClient(
		conf.URL,
		conf.Token,
	)
	if err != nil {
		log.Fatalf("Failed to create algod client: %v", err)
	}
}

func AlgodClient() *algod.Client {
	return client
}

func CompileTealFromFile(tealPath string) ([]byte, error) {
	algodClient := AlgodClient()

	teal, err := os.ReadFile(tealPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s from file: %v", tealPath, err)
	}

	result, err := algodClient.TealCompile(teal).Do(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to compile %s: %v", tealPath, err)
	}
	binary, err := base64.StdEncoding.DecodeString(result.Result)
	if err != nil {
		log.Fatalf("failed to decode approval program: %v", err)
	}

	return binary, nil
}

// GetBalance returns the balance net of MBR of the given address in microAlgos
func GetNetBalance(address string) (uint64, error) {
	algodClient := AlgodClient()

	accountInfo, err := algodClient.AccountInformation(address).Do(context.Background())
	if err != nil {
		return 0, fmt.Errorf("failed to get account information: %v", err)
	}
	return accountInfo.Amount - accountInfo.MinBalance, nil
}

// abiEncode encodes arg into its abi []byte representation
func abiEncode(arg any, abiTypeName string) ([]byte, error) {
	abiType, err := abi.TypeOf(abiTypeName)
	if err != nil {
		return nil, fmt.Errorf("failed to get abi type: %v", err)
	}
	abiArg, err := abiType.Encode(arg)
	if err != nil {
		return nil, fmt.Errorf("failed to encode noChange: %v", err)
	}
	return abiArg, nil
}

// readAlgodConfigFromDir reads the algod URL and token from the given directory
func readAlgodConfigFromDir(dir string) (*algodConfig, error) {
	urlPath := filepath.Join(dir, "algod.net")
	urlBytes, err := os.ReadFile(urlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read algod url: %v", err)
	}
	url := strings.TrimSpace(string(urlBytes))
	if strings.HasPrefix(string(url), "[::]") {
		// we have something like [::]:port_num, replace it with localhost
		index := strings.LastIndex(string(url), ":")
		port := url[index+1:]
		url = "localhost:" + port
	}
	tokenPath := filepath.Join(dir, "algod.token")
	tokenBytes, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read algod token: %v", err)
	}
	return &algodConfig{
		URL:   "http://" + url,
		Token: strings.TrimSpace(string(tokenBytes)),
	}, nil
}
