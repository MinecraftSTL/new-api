package model

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// migratePasskeyMultiCredentials removes legacy single-credential uniqueness
// on passkey_credentials.user_id before AutoMigrate applies the current model.
// Index names can be rewritten by PostgreSQL imports, so match the definition
// (one unique, non-primary index over user_id) instead of one hard-coded name.
func migratePasskeyMultiCredentials(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("migrate passkey credentials: database is nil")
	}
	if !db.Migrator().HasTable(&PasskeyCredential{}) {
		return nil
	}
	indexes, err := db.Migrator().GetIndexes(&PasskeyCredential{})
	if err != nil {
		return fmt.Errorf("inspect passkey indexes: %w", err)
	}
	for _, index := range indexes {
		columns := index.Columns()
		if len(columns) != 1 || !strings.EqualFold(columns[0], "user_id") {
			continue
		}
		primary, known := index.PrimaryKey()
		if !known {
			return fmt.Errorf("inspect passkey user index %q primary key", index.Name())
		}
		if primary {
			continue
		}
		unique, known := index.Unique()
		if !known {
			return fmt.Errorf("inspect passkey user index %q uniqueness", index.Name())
		}
		if !unique {
			continue
		}
		if err := db.Migrator().DropIndex(&PasskeyCredential{}, index.Name()); err != nil {
			return fmt.Errorf("drop legacy passkey user index %q: %w", index.Name(), err)
		}
	}
	return nil
}

func backfillPasskeyNames(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("backfill passkey names: database is nil")
	}
	if !db.Migrator().HasTable(&PasskeyCredential{}) {
		return nil
	}
	sql := "UPDATE passkey_credentials SET name = 'Default' WHERE name IS NULL OR name = ''"
	if err := db.Exec(sql).Error; err != nil {
		return fmt.Errorf("backfill passkey names: %w", err)
	}
	return nil
}
