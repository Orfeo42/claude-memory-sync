package postgres

import (
	"time"

	"claude-memory-sync/internal/settings"
)

type settingRow struct {
	Key       string    `db:"key"`
	Value     string    `db:"value"`
	UpdatedAt time.Time `db:"updated_at"`
	UpdatedBy string    `db:"updated_by"`
}

type versionRow struct {
	Value     *string   `db:"value"`
	Deleted   bool      `db:"deleted"`
	CreatedAt time.Time `db:"created_at"`
	CreatedBy string    `db:"created_by"`
}

type writeArgs struct {
	Key        string  `db:"key"`
	Value      string  `db:"value"`
	AuditValue *string `db:"audit_value"`
	Deleted    bool    `db:"deleted"`
	Actor      string  `db:"actor"`
}

type keyArgs struct {
	Key string `db:"key"`
}

type historyArgs struct {
	Key   string `db:"key"`
	Limit int    `db:"limit"`
}

func (r settingRow) toSetting() settings.Setting {
	value := r.Value
	return settings.Setting{
		Key:       r.Key,
		Value:     &value,
		Set:       true,
		UpdatedAt: r.UpdatedAt,
		UpdatedBy: r.UpdatedBy,
	}
}

func (r versionRow) toVersion() settings.Version {
	return settings.Version{
		Value:     r.Value,
		Deleted:   r.Deleted,
		CreatedAt: r.CreatedAt,
		CreatedBy: r.CreatedBy,
	}
}
