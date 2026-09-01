package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

func (m *Module) Settings() []contracts.SettingDef {
	m.cfgMu.RLock()
	defer m.cfgMu.RUnlock()
	return []contracts.SettingDef{
		{
			Key: "library_dir", Label: "Library directory", Type: contracts.SettingTypeString,
			Value: m.libraryDir, Description: "Root path for audiobook library (scanned offline)", Group: "Library",
		},
		{
			Key: "data_dir", Label: "Data directory", Type: contracts.SettingTypeString,
			Value: m.dataDir, Description: "Directory for SQLite library database (audiobooks.db)", Group: "Library",
		},
	}
}

func (m *Module) UpdateSetting(key, value string) error {
	m.cfgMu.Lock()
	defer m.cfgMu.Unlock()
	switch key {
	case "library_dir":
		value = strings.TrimSpace(value)
		if value == "" {
			return fmt.Errorf("library_dir cannot be empty")
		}
		if err := os.MkdirAll(value, 0o700); err != nil {
			return fmt.Errorf("create library dir: %w", err)
		}
		info, err := os.Stat(value)
		if err != nil {
			return fmt.Errorf("library dir: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("library_dir is not a directory: %s", value)
		}
		abs, err := filepath.Abs(value)
		if err != nil {
			return fmt.Errorf("resolve library dir: %w", err)
		}
		m.libraryDir = abs
	case "data_dir":
		return fmt.Errorf("data_dir is set at startup (AUDIOBOOKS_DATA_DIR); restart to change")
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
	return nil
}
