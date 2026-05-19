package db

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/giuliop/HermesVault-frontend/config"
)

const fatalMarkerFile = "subscriber.fatal"

func FatalMarkerPath() string {
	return filepath.Join(filepath.Dir(config.TxnsDbPath), fatalMarkerFile)
}

func HasFatalMarker() (bool, error) {
	if _, err := os.Stat(FatalMarkerPath()); err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, err
	}
}

func RecordFatalMarker(format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	content := fmt.Sprintf("%s\n%s\n", time.Now().UTC().Format(time.RFC3339Nano), message)
	return os.WriteFile(FatalMarkerPath(), []byte(content), 0644)
}
