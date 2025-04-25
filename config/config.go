package config

import (
	"bufio"
	"log"
	"os"
	"strings"
	"time"
)

// webserver constants
const (
	CacheControl = "public, max-age=600" // 600 sec = 10 min
	Port         = "5555"
)

// other constants
const (
	// Number of characters to highlight displaying long strings, e.g. addresses
	NumCharsToHighlight = 5

	// Number of rounds to wait for a transaction to be confirmed
	WaitRounds = 30

	// Interval between internal db cleanup runs
	CleanupInterval = 10 * time.Minute // 10 minutes
)

// Frontend fees
var (
	// The frontend withdrawal fee is determined by dividing the withdrawal amount
	// by this divisor. If zero, there is no frontend fee
	FrontendWithDrawalFeeDivisor = uint64(0)
)

// file paths
var (
	AppSetupDirPath string
	InternalDbPath  string
	TxnsDbPath      string
	AlgodPath       string
	AlgodToken      string
)

func init() {
	env, err := LoadEnv("config/.env")
	if err != nil {
		log.Fatalf("failed to load env: %v", err)
	}

	AppSetupDirPath = env["AppSetupDirPath"]
	InternalDbPath = env["InternalDbPath"]
	TxnsDbPath = env["TxnsDbPath"]
	AlgodPath = env["AlgodPath"]
	AlgodToken = env["AlgodToken"]
}

// LoadEnv reads a set of key-value pairs from a file and returns them as a map
// Each line in the file can be in one of the following formats:
// - key=value
// - # comment
// - empty line
func LoadEnv(filename string) (map[string]string, error) {
	envMap := make(map[string]string)

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments starting with # or //
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			log.Printf("Malformed line in env file: %s\n", line)
			continue // Skip malformed lines
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove surrounding quotes if any
		value = strings.Trim(value, `"'`)

		envMap[key] = value
	}

	return envMap, scanner.Err()
}
