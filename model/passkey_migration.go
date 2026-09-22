package model

import (
	"fmt"

	"gorm.io/gorm"
)

const passkeyUserIndex = "idx_passkey_credentials_user_id"

// migratePasskeyMultiCredentials removes the legacy one-passkey-per-user
// constraint before AutoMigrate applies the current model.
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
		if index.Name() != passkeyUserIndex {
			continue
		}
		unique, known := index.Unique()
		if !known {
			return fmt.Errorf("inspect passkey user index uniqueness")
		}
		if unique {
			if err := db.Migrator().DropIndex(&PasskeyCredential{}, passkeyUserIndex); err != nil {
				return fmt.Errorf("drop legacy passkey user index: %w", err)
			}
		}
		return nil
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
