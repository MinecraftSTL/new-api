package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type nullableQuotaDataFixture struct {
	Id               int
	QuotaBeforeGroup *int `gorm:"column:quota_before_group"`
}

func (nullableQuotaDataFixture) TableName() string {
	return "quota_data"
}

func TestQuotaBeforeGroupColumnsAreNotNullable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	models := []interface{}{
		&Log{},
		&QuotaData{},
		&Task{},
		&Midjourney{},
	}
	require.NoError(t, db.AutoMigrate(models...))

	for _, model := range models {
		columnTypes, err := db.Migrator().ColumnTypes(model)
		require.NoError(t, err)

		found := false
		for _, columnType := range columnTypes {
			if columnType.Name() != "quota_before_group" {
				continue
			}
			found = true
			nullable, ok := columnType.Nullable()
			require.True(t, ok)
			require.False(t, nullable)
			break
		}
		require.True(t, found)
	}
}

func TestQuotaBeforeGroupInternalModelsDoNotSerialize(t *testing.T) {
	models := []interface{}{
		Task{QuotaBeforeGroup: 100},
		Midjourney{QuotaBeforeGroup: 100},
	}

	for _, model := range models {
		data, err := common.Marshal(model)
		require.NoError(t, err)
		require.NotContains(t, string(data), "quota_before_group")
	}
}

func TestQuotaBeforeGroupMigrationMakesBackfilledColumnNotNullable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&nullableQuotaDataFixture{}))

	quotaBeforeGroup := 80
	require.NoError(t, db.Create(&nullableQuotaDataFixture{
		QuotaBeforeGroup: &quotaBeforeGroup,
	}).Error)
	require.NoError(t, db.AutoMigrate(&QuotaData{}))

	columnTypes, err := db.Migrator().ColumnTypes(&QuotaData{})
	require.NoError(t, err)
	for _, columnType := range columnTypes {
		if columnType.Name() != "quota_before_group" {
			continue
		}
		nullable, ok := columnType.Nullable()
		require.True(t, ok)
		require.False(t, nullable)
		var migrated QuotaData
		require.NoError(t, db.First(&migrated).Error)
		require.Equal(t, quotaBeforeGroup, migrated.QuotaBeforeGroup)
		return
	}
	require.Fail(t, "quota_before_group column not found")
}
