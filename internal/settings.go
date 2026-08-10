package internal

import (
	"fmt"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

func (m *Module) Settings() []contracts.SettingDef {
	m.cfgMu.RLock()
	defer m.cfgMu.RUnlock()
	return []contracts.SettingDef{{
		Key: "library_dir", Label: "Library directory", Type: contracts.SettingTypeString,
		Value: m.libraryDir, Description: "Root path for audiobook library", Group: "Library",
	}}
}

func (m *Module) UpdateSetting(key, value string) error {
	m.cfgMu.Lock()
	defer m.cfgMu.Unlock()
	if key != "library_dir" {
		return fmt.Errorf("unknown setting %q", key)
	}
	m.libraryDir = value
	return nil
}
