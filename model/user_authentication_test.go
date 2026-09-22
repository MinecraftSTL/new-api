package model

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/go-redis/redis/v8"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestHardDeleteUserFailsClosedWhenAuthFenceCannotPublish(t *testing.T) {
	truncateTables(t)

	user := User{Username: "hard-delete-user", Password: "password", TelegramId: "hard-delete-telegram"}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderTelegram, user.TelegramId, user.Id)
	}))
	require.NoError(t, DB.Create(&Token{UserId: user.Id, Key: "hard-delete-token"}).Error)
	require.NoError(t, DB.Create(&TwoFA{UserId: user.Id, Secret: "secret", IsEnabled: true}).Error)
	require.NoError(t, DB.Create(&TwoFABackupCode{UserId: user.Id, CodeHash: "hash"}).Error)
	require.NoError(t, DB.Create(&PasskeyCredential{UserID: user.Id, CredentialID: "credential", PublicKey: "public-key"}).Error)
	require.NoError(t, DB.Create(&UserOAuthBinding{UserId: user.Id, ProviderId: 1, ProviderUserId: "provider-user"}).Error)
	require.NoError(t, DB.Create(&UserSession{
		SID: "hard-delete-session", UserID: user.Id, Version: 1, UserAuthVersion: 1,
		Status: UserSessionStatusActive, RefreshHash: "refresh-hash", LoginMethod: "password",
		LastActiveAt: 1, ExpiresAt: 2,
	}).Error)
	require.NoError(t, DB.Create(&AuthFlow{
		TokenHash: "hard-delete-auth-flow", Purpose: AuthFlowPurposeTwoFALogin,
		UserId: user.Id, ExpiresAt: time.Now().Add(time.Minute),
	}).Error)

	oldRedisEnabled, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("forced redis failure")
		},
		MaxRetries: -1,
	})
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RedisEnabled, common.RDB = oldRedisEnabled, oldRDB
	})

	require.Error(t, HardDeleteUserById(user.Id))

	var count int64
	require.NoError(t, DB.Unscoped().Model(&User{}).Where("id = ?", user.Id).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	for _, record := range []any{
		&Token{},
		&TwoFA{},
		&TwoFABackupCode{},
		&PasskeyCredential{},
		&UserOAuthBinding{},
		&UserSession{},
		&AuthFlow{},
		&ExternalIdentityClaim{},
	} {
		require.NoError(t, DB.Unscoped().Model(record).Where("user_id = ?", user.Id).Count(&count).Error)
		assert.EqualValues(t, 1, count)
	}
}

func TestHardDeleteUserPublishesTombstoneAndPurgesAuthenticationData(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)

	user := User{
		Username: "hard-delete-success", Password: "password", AuthVersion: 1,
		TelegramId: "hard-delete-success-telegram",
	}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderTelegram, user.TelegramId, user.Id)
	}))
	require.NoError(t, DB.Create(&Token{UserId: user.Id, Key: "hard-delete-success-token"}).Error)
	require.NoError(t, DB.Create(&TwoFA{UserId: user.Id, Secret: "secret", IsEnabled: true}).Error)
	require.NoError(t, DB.Create(&TwoFABackupCode{UserId: user.Id, CodeHash: "hash"}).Error)
	require.NoError(t, DB.Create(&PasskeyCredential{UserID: user.Id, CredentialID: "credential-success", PublicKey: "public-key"}).Error)
	require.NoError(t, DB.Create(&UserOAuthBinding{UserId: user.Id, ProviderId: 1, ProviderUserId: "provider-user-success"}).Error)
	require.NoError(t, DB.Create(&UserSession{
		SID: "hard-delete-success-session", UserID: user.Id, Version: 1, UserAuthVersion: 1,
		Status: UserSessionStatusActive, RefreshHash: "refresh-hash", LoginMethod: "password",
		LastActiveAt: 1, ExpiresAt: 2,
	}).Error)
	require.NoError(t, DB.Create(&AuthFlow{
		TokenHash: "hard-delete-success-flow", Purpose: AuthFlowPurposeTwoFALogin,
		UserId: user.Id, ExpiresAt: time.Now().Add(time.Minute),
	}).Error)
	require.NoError(t, populateUserCache(user))
	// Administrative hard deletion commonly targets an already soft-deleted
	// user; the shared version increment must therefore query unscoped.
	require.NoError(t, DB.Delete(&user).Error)

	require.NoError(t, HardDeleteUserById(user.Id))

	var count int64
	require.NoError(t, DB.Unscoped().Model(&User{}).Where("id = ?", user.Id).Count(&count).Error)
	assert.Zero(t, count)
	for _, record := range []any{
		&Token{},
		&TwoFA{},
		&TwoFABackupCode{},
		&PasskeyCredential{},
		&UserOAuthBinding{},
		&UserSession{},
		&AuthFlow{},
		&ExternalIdentityClaim{},
	} {
		require.NoError(t, DB.Unscoped().Model(record).Where("user_id = ?", user.Id).Count(&count).Error)
		assert.Zero(t, count)
	}
	assert.False(t, server.Exists(getUserAuthFenceKey(user.Id)))
	committed, err := common.RDB.Get(t.Context(), getUserAuthVersionKey(user.Id)).Result()
	require.NoError(t, err)
	assert.Equal(t, "2", committed)
	assert.False(t, server.Exists(getUserCacheKey(user.Id)))
}

func TestIncrementFailedAttemptsCountsConcurrentFailures(t *testing.T) {
	truncateTables(t)

	user := User{Username: "twofa-cas-user", Password: "password"}
	require.NoError(t, DB.Create(&user).Error)
	twoFA := TwoFA{UserId: user.Id, Secret: "secret", IsEnabled: true}
	require.NoError(t, DB.Create(&twoFA).Error)

	const attempts = 4
	errs := make(chan error, attempts)
	var wg sync.WaitGroup
	for range attempts {
		wg.Go(func() {
			errs <- (&TwoFA{Id: twoFA.Id}).IncrementFailedAttempts()
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	var reloaded TwoFA
	require.NoError(t, DB.First(&reloaded, twoFA.Id).Error)
	assert.Equal(t, attempts, reloaded.FailedAttempts)
}

func TestValidateBackupCodeCanOnlySucceedOnce(t *testing.T) {
	truncateTables(t)

	const code = "ABCD-1234"
	user := User{Id: 123, Username: "backup-code-user", Password: "password", AuthVersion: 1}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Create(&TwoFA{UserId: user.Id, Secret: "secret", IsEnabled: false}).Error)
	hashedCode, err := common.HashBackupCode(code)
	require.NoError(t, err)
	require.NoError(t, DB.Create(&TwoFABackupCode{UserId: user.Id, CodeHash: hashedCode}).Error)

	const attempts = 2
	results := make(chan bool, attempts)
	errs := make(chan error, attempts)
	var wg sync.WaitGroup
	for range attempts {
		wg.Go(func() {
			valid, err := ValidateBackupCode(123, code)
			results <- valid
			errs <- err
		})
	}
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	wins := 0
	for valid := range results {
		if valid {
			wins++
		}
	}
	assert.Equal(t, 1, wins)

	remaining, err := GetUnusedBackupCodeCount(123)
	require.NoError(t, err)
	assert.Zero(t, remaining)
}

func TestPendingTwoFASetupAPIsRejectEnabledFactor(t *testing.T) {
	truncateTables(t)

	user := User{Username: "enabled-twofa-guard", Password: "password", Status: common.UserStatusEnabled, AuthVersion: 1}
	require.NoError(t, DB.Create(&user).Error)
	twoFA := TwoFA{UserId: user.Id, Secret: "secret", IsEnabled: true}
	require.NoError(t, DB.Create(&twoFA).Error)

	session := UserSession{SID: "enabled-factor-session", UserID: user.Id, Version: 1, UserAuthVersion: 1, Status: UserSessionStatusActive, ExpiresAt: time.Now().Add(time.Hour).Unix()}
	require.NoError(t, DB.Create(&session).Error)
	identity := AuthSessionIdentity{UserID: user.Id, SessionID: session.SID, UserAuthVersion: 1, SessionVersion: 1}
	authorization := &AuthFlowAuthorization{AuthSessionIdentity: identity, ProofID: 1, Scope: "2fa.setup", ContextHash: "setup-context", Method: "password"}
	_, err := CreateTwoFAEnrollment(identity, authorization, "replacement-secret", []string{"ABCD-1234"}, time.Now().Add(time.Minute))
	require.ErrorIs(t, err, ErrTwoFAAlreadyEnabled)

	var stored TwoFA
	require.NoError(t, DB.First(&stored, twoFA.Id).Error)
	assert.True(t, stored.IsEnabled)
	var backupCodeCount int64
	require.NoError(t, DB.Model(&TwoFABackupCode{}).Where("user_id = ?", user.Id).Count(&backupCodeCount).Error)
	assert.Zero(t, backupCodeCount)
}

func TestSecurityFactorMutationsAdvanceUserAuthVersion(t *testing.T) {
	truncateTables(t)

	user := User{
		Username:    "security-factor-version-user",
		Password:    "password",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
	}
	require.NoError(t, DB.Create(&user).Error)
	session := UserSession{SID: "factor-version-session", UserID: user.Id, Version: 1, UserAuthVersion: 1, Status: UserSessionStatusActive, ExpiresAt: time.Now().Add(time.Hour).Unix()}
	require.NoError(t, DB.Create(&session).Error)
	identity := AuthSessionIdentity{UserID: user.Id, SessionID: session.SID, UserAuthVersion: 1, SessionVersion: 1}
	authorization := &AuthFlowAuthorization{AuthSessionIdentity: identity, ProofID: 1, Scope: "2fa.setup", ContextHash: "setup-context", Method: "password"}
	token, err := CreateTwoFAEnrollment(identity, authorization, "JBSWY3DPEHPK3PXP", []string{"ABCD-1234"}, time.Now().Add(time.Minute))
	require.NoError(t, err)
	code, err := totp.GenerateCode("JBSWY3DPEHPK3PXP", time.Now())
	require.NoError(t, err)
	require.NoError(t, EnableTwoFAEnrollment(identity, token, code))
	assertUserAuthVersion(t, user.Id, 2)
	assert.ErrorIs(t, EnableTwoFAEnrollment(identity, token, code), ErrTwoFASetupInvalid)
	assertUserAuthVersion(t, user.Id, 2)
	require.NoError(t, ReplaceBackupCodesWithAuthVersion(user.Id, []string{"ABCD-1234"}))
	assertUserAuthVersion(t, user.Id, 3)
	require.NoError(t, DisableTwoFAWithAuthVersion(user.Id))
	assertUserAuthVersion(t, user.Id, 4)

	credential := &PasskeyCredential{UserID: user.Id, CredentialID: "credential-id", PublicKey: "public-key"}
	require.NoError(t, UpsertPasskeyCredentialWithAuthVersion(credential))
	assertUserAuthVersion(t, user.Id, 5)
	require.NoError(t, DeletePasskeyByUserIDWithAuthVersion(user.Id))
	assertUserAuthVersion(t, user.Id, 6)
}

func TestUpdatePasskeyAssertionStateCannotRewriteRegistrationIdentity(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	previousSettings := *system_setting.GetPasskeySettings()
	domainKeys := map[string]any{"key": []string{"ServerAddress", "passkey.rp_id", "passkey.legacy_rp_ids", "passkey.origins"}}
	var previousOptions []Option
	require.NoError(t, DB.Where(domainKeys).Find(&previousOptions).Error)
	t.Cleanup(func() {
		*system_setting.GetPasskeySettings() = previousSettings
		require.NoError(t, DB.Where(domainKeys).Delete(&Option{}).Error)
		if len(previousOptions) > 0 {
			require.NoError(t, DB.Create(&previousOptions).Error)
		}
	})
	*system_setting.GetPasskeySettings() = system_setting.PasskeySettings{RPID: "example.com", Origins: "https://example.com"}
	for _, option := range []Option{{Key: "passkey.rp_id", Value: "example.com"}, {Key: "passkey.legacy_rp_ids", Value: ""}, {Key: "passkey.origins", Value: "https://example.com"}} {
		require.NoError(t, DB.Save(&option).Error)
	}

	user := User{Username: "passkey-assertion-state", Password: "password", AuthVersion: 1}
	require.NoError(t, DB.Create(&user).Error)
	credentialID := []byte("stable-credential-id")
	stored := PasskeyCredential{
		UserID:          user.Id,
		CredentialID:    base64.StdEncoding.EncodeToString(credentialID),
		PublicKey:       "original-public-key",
		AttestationType: "packed",
		AAGUID:          "original-aaguid",
		SignCount:       1,
		Transports:      `["usb"]`,
		Attachment:      "platform",
	}
	require.NoError(t, DB.Create(&stored).Error)
	usedAt := time.Now().UTC().Truncate(time.Second)
	validated := &webauthn.Credential{
		ID:              credentialID,
		PublicKey:       []byte("replacement-public-key"),
		AttestationType: "none",
		Flags: webauthn.CredentialFlags{
			UserPresent:    true,
			UserVerified:   true,
			BackupEligible: true,
			BackupState:    true,
		},
		Authenticator: webauthn.Authenticator{
			AAGUID:       []byte("replacement-aaguid"),
			SignCount:    8,
			CloneWarning: true,
		},
	}
	require.NoError(t, UpdatePasskeyAssertionState(user.Id, validated, usedAt, "example.com"))

	var updated PasskeyCredential
	require.NoError(t, DB.First(&updated, stored.ID).Error)
	assert.Equal(t, stored.CredentialID, updated.CredentialID)
	assert.Equal(t, stored.PublicKey, updated.PublicKey)
	assert.Equal(t, stored.AttestationType, updated.AttestationType)
	assert.Equal(t, stored.AAGUID, updated.AAGUID)
	assert.Equal(t, stored.Transports, updated.Transports)
	assert.Equal(t, stored.Attachment, updated.Attachment)
	assert.EqualValues(t, 8, updated.SignCount)
	assert.True(t, updated.CloneWarning)
	assert.True(t, updated.UserPresent)
	assert.True(t, updated.UserVerified)
	assert.True(t, updated.BackupEligible)
	assert.True(t, updated.BackupState)
	require.NotNil(t, updated.LastUsedAt)
	assert.Equal(t, usedAt.Unix(), updated.LastUsedAt.Unix())

	validated.ID = []byte("another-credential")
	assert.ErrorIs(t, UpdatePasskeyAssertionState(user.Id, validated, usedAt, "example.com"), ErrPasskeyNotFound)
}

func assertUserAuthVersion(t *testing.T, userID int, expected int64) {
	t.Helper()
	var version int64
	require.NoError(t, DB.Model(&User{}).Where("id = ?", userID).Select("auth_version").Scan(&version).Error)
	assert.Equal(t, expected, version)
}

func TestPasskeyMultiCredentialLifecycle(t *testing.T) {
	truncateTables(t)

	user := User{Username: "multi-passkey-user", Password: "password", AffCode: "multi-passkey-aff", Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, DB.Create(&user).Error)
	for i := range MaxPasskeysPerUser {
		credential := &PasskeyCredential{
			UserID:       user.Id,
			Name:         "Duplicate",
			CredentialID: base64.StdEncoding.EncodeToString([]byte{byte(i + 1)}),
			PublicKey:    "public-key",
		}
		require.NoError(t, CreatePasskeyCredentialWithAuthVersion(credential))
	}
	assertUserAuthVersion(t, user.Id, 17)

	credentials, err := GetPasskeysByUserID(user.Id)
	require.NoError(t, err)
	require.Len(t, credentials, MaxPasskeysPerUser)

	tooMany := &PasskeyCredential{UserID: user.Id, Name: "Too many", CredentialID: base64.StdEncoding.EncodeToString([]byte("too-many")), PublicKey: "public-key"}
	require.ErrorIs(t, CreatePasskeyCredentialWithAuthVersion(tooMany), ErrPasskeyLimitReached)

	require.NoError(t, DB.First(&user, user.Id).Error)
	session := UserSession{SID: "multi-passkey-session", UserID: user.Id, Version: 1, UserAuthVersion: user.AuthVersion, Status: UserSessionStatusActive, ExpiresAt: time.Now().Add(time.Hour).Unix()}
	require.NoError(t, DB.Create(&session).Error)
	identity := AuthSessionIdentity{UserID: user.Id, SessionID: session.SID, UserAuthVersion: user.AuthVersion, SessionVersion: session.Version}

	renamed, err := RenamePasskeyForSession(identity, credentials[0].ID, "  Shared name  ")
	require.NoError(t, err)
	assert.Equal(t, "Shared name", renamed.Name)
	assertUserAuthVersion(t, user.Id, 17)

	otherUser := User{Username: "other-multi-passkey-user", Password: "password", AffCode: "other-passkey-aff", Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, DB.Create(&otherUser).Error)
	otherCredential := &PasskeyCredential{UserID: otherUser.Id, Name: "Other", CredentialID: base64.StdEncoding.EncodeToString([]byte("other-credential")), PublicKey: "public-key"}
	require.NoError(t, DB.Create(otherCredential).Error)
	_, err = RenamePasskeyForSession(identity, otherCredential.ID, "not allowed")
	require.ErrorIs(t, err, ErrPasskeyNotFound)

	require.NoError(t, DeletePasskeyForSession(identity, credentials[0].ID))
	assertUserAuthVersion(t, user.Id, 18)
	remaining, err := GetPasskeysByUserID(user.Id)
	require.NoError(t, err)
	require.Len(t, remaining, MaxPasskeysPerUser-1)

	lastUser := User{Username: "last-passkey-user", Password: "password", AffCode: "last-passkey-aff", Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, DB.Create(&lastUser).Error)
	lastCredential := &PasskeyCredential{UserID: lastUser.Id, Name: "Only", CredentialID: base64.StdEncoding.EncodeToString([]byte("last-credential")), PublicKey: "public-key"}
	require.NoError(t, CreatePasskeyCredentialWithAuthVersion(lastCredential))
	require.NoError(t, DB.First(&lastUser, lastUser.Id).Error)
	lastSession := UserSession{SID: "last-passkey-session", UserID: lastUser.Id, Version: 1, UserAuthVersion: lastUser.AuthVersion, Status: UserSessionStatusActive, ExpiresAt: time.Now().Add(time.Hour).Unix()}
	require.NoError(t, DB.Create(&lastSession).Error)
	lastIdentity := AuthSessionIdentity{UserID: lastUser.Id, SessionID: lastSession.SID, UserAuthVersion: lastUser.AuthVersion, SessionVersion: lastSession.Version}
	require.NoError(t, DeletePasskeyForSession(lastIdentity, lastCredential.ID))
	lastCredentials, err := GetPasskeysByUserID(lastUser.Id)
	require.NoError(t, err)
	assert.Empty(t, lastCredentials)
}

func TestPasskeyMultiCredentialMigrationPreservesLegacyCredential(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Migrator().DropTable(&PasskeyCredential{}))
	require.NoError(t, DB.Exec(`CREATE TABLE passkey_credentials (
		id integer PRIMARY KEY,
		user_id integer NOT NULL,
		rp_id varchar(253),
		credential_id varchar(512) NOT NULL,
		public_key text NOT NULL,
		attestation_type varchar(255),
		aa_guid varchar(512),
		sign_count integer DEFAULT 0,
		clone_warning numeric,
		user_present numeric,
		user_verified numeric,
		backup_eligible numeric,
		backup_state numeric,
		transports text,
		attachment varchar(32),
		last_used_at datetime,
		created_at datetime,
		updated_at datetime,
		deleted_at datetime
	)`).Error)
	require.NoError(t, DB.Exec("CREATE UNIQUE INDEX idx_passkey_credentials_user_id ON passkey_credentials(user_id)").Error)
	require.NoError(t, DB.Exec("CREATE UNIQUE INDEX idx_passkey_credentials_credential_id ON passkey_credentials(credential_id)").Error)
	require.NoError(t, DB.Exec("INSERT INTO passkey_credentials (id, user_id, credential_id, public_key) VALUES (?, ?, ?, ?)", 1, 9, "legacy-credential", "key").Error)

	runMigration := func() {
		require.NoError(t, migratePasskeyMultiCredentials(DB))
		require.NoError(t, DB.AutoMigrate(&PasskeyCredential{}))
		require.NoError(t, backfillPasskeyNames(DB))
	}
	runMigration()

	var migrated PasskeyCredential
	require.NoError(t, DB.First(&migrated, 1).Error)
	assert.Equal(t, "Default", migrated.Name)

	second := &PasskeyCredential{UserID: 9, Name: "Second", CredentialID: "second-credential", PublicKey: "key"}
	require.NoError(t, DB.Create(second).Error)
	runMigration()

	var renamed PasskeyCredential
	require.NoError(t, DB.First(&renamed, second.ID).Error)
	assert.Equal(t, "Second", renamed.Name)
	assert.Error(t, DB.Create(&PasskeyCredential{UserID: 9, Name: "Duplicate credential", CredentialID: "legacy-credential", PublicKey: "key"}).Error, "legacy duplicate should fail")
}

type legacyPasskeyCredential struct {
	ID              int     `gorm:"primaryKey"`
	UserID          int     `gorm:"uniqueIndex:idx_passkey_credentials_user_id;not null"`
	RPID            *string `gorm:"column:rp_id;type:varchar(253)"`
	CredentialID    string  `gorm:"type:varchar(512);uniqueIndex;not null"`
	PublicKey       string  `gorm:"type:text;not null"`
	AttestationType string  `gorm:"type:varchar(255)"`
	AAGUID          string  `gorm:"type:varchar(512)"`
	SignCount       uint32  `gorm:"default:0"`
	CloneWarning    bool
	UserPresent     bool
	UserVerified    bool
	BackupEligible  bool
	BackupState     bool
	Transports      string `gorm:"type:text"`
	Attachment      string `gorm:"type:varchar(32)"`
	LastUsedAt      *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt `gorm:"index"`
}

func TestPasskeyMultiCredentialMigrationDialects(t *testing.T) {
	dialect := strings.ToLower(strings.TrimSpace(os.Getenv("TEST_SECURITY_DIALECT")))
	if dialect == "" || dialect == "sqlite" {
		t.Skip("set TEST_SECURITY_DIALECT and the matching DSN to run")
	}
	dsn := os.Getenv("TEST_" + strings.ToUpper(dialect) + "_DSN")
	require.NotEmpty(t, dsn, "dialect DSN must be set")
	var dialector gorm.Dialector
	switch dialect {
	case "mysql":
		dialector = mysql.Open(dsn)
	case "postgres":
		dialector = postgres.Open(dsn)
	default:
		t.Fatalf("unsupported dialect %q", dialect)
	}
	db, err := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Migrator().DropTable("passkey_credentials")
		_ = sqlDB.Close()
	})
	require.NoError(t, db.Migrator().DropTable("passkey_credentials"))
	require.NoError(t, db.Table("passkey_credentials").AutoMigrate(&legacyPasskeyCredential{}))
	rpID := "example.com"
	require.NoError(t, db.Table("passkey_credentials").Create(&legacyPasskeyCredential{
		UserID:       9,
		RPID:         &rpID,
		CredentialID: "legacy-credential",
		PublicKey:    "key",
	}).Error)

	runMigration := func() {
		require.NoError(t, migratePasskeyMultiCredentials(db))
		require.NoError(t, db.AutoMigrate(&PasskeyCredential{}))
		require.NoError(t, backfillPasskeyNames(db))
	}
	runMigration()

	var migrated PasskeyCredential
	require.NoError(t, db.Where("credential_id = ?", "legacy-credential").First(&migrated).Error)
	assert.Equal(t, "Default", migrated.Name)

	second := &PasskeyCredential{UserID: 9, Name: "Second", CredentialID: "second-credential", PublicKey: "key"}
	require.NoError(t, db.Create(second).Error)
	runMigration()
	var renamed PasskeyCredential
	require.NoError(t, db.First(&renamed, second.ID).Error)
	assert.Equal(t, "Second", renamed.Name)
	assert.Error(t, db.Create(&PasskeyCredential{UserID: 9, Name: "Duplicate", CredentialID: "legacy-credential", PublicKey: "key"}).Error)
}
