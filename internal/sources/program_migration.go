package sources

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Unedited migrations retain legacy conflict decisions. Editing a program
// switches comparison to its effective assignments, independently of retained
// legacy settings. Returning to the original program restores its semantics.
type ProgramMigration struct {
	ProgramHash  string `json:"program_hash"`
	SettingsHash string `json:"settings_hash"`
}

func jsonHash(value any) string {
	raw, _ := json.Marshal(value)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func migratedProgram(rule Rule) *ProgramMigration {
	return &ProgramMigration{ProgramHash: jsonHash(rule.Program), SettingsHash: jsonHash(rule.Settings)}
}

func legacyComparison(rule Rule) string {
	if rule.Program == nil {
		return jsonHash(rule.Settings)
	}
	if rule.Migration != nil && rule.Migration.ProgramHash == jsonHash(rule.Program) {
		return rule.Migration.SettingsHash
	}
	return ""
}

func attributionConflict(left, right Rule) bool {
	if left.legacyComparison != "" && right.legacyComparison != "" {
		return left.legacyComparison != right.legacyComparison
	}
	if left.Subtype != "aggregate" && left.Settings.Listener != right.Settings.Listener {
		return true
	}
	a, b := left.Settings.SharePercent, right.Settings.SharePercent
	if left.Subtype == "direct" {
		return false
	}
	if a == nil || b == nil {
		return (a == nil) != (b == nil)
	}
	return *a != *b
}
