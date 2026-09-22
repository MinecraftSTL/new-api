package model

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"gorm.io/gorm"
)

const (
	MaxPasskeysPerUser  = 16
	PasskeyNameMaxRunes = 64
)

var (
	ErrPasskeyNotFound         = errors.New("passkey credential not found")
	ErrPasskeyLimitReached     = errors.New("\u6bcf\u4e2a\u7528\u6237\u6700\u591a\u53ef\u4ee5\u7ed1\u5b9a 16 \u4e2a Passkey")
	ErrPasskeyNameInvalid      = errors.New("Passkey \u540d\u79f0\u5fc5\u987b\u4e3a 1 \u5230 64 \u4e2a\u5b57\u7b26")
	ErrPasskeyCredentialExists = errors.New("\u8be5 Passkey \u5df2\u7ed1\u5b9a")
)

type PasskeyCredential struct {
	ID              int            `json:"id" gorm:"primaryKey"`
	UserID          int            `json:"user_id" gorm:"index:idx_passkey_credentials_user_id;not null"`
	Name            string         `json:"name" gorm:"type:varchar(64)"`
	RPID            *string        `json:"rp_id,omitempty" gorm:"column:rp_id;type:varchar(253)"`
	CredentialID    string         `json:"credential_id" gorm:"type:varchar(512);uniqueIndex;not null"` // base64 encoded
	PublicKey       string         `json:"public_key" gorm:"type:text;not null"`                        // base64 encoded
	AttestationType string         `json:"attestation_type" gorm:"type:varchar(255)"`
	AAGUID          string         `json:"aaguid" gorm:"type:varchar(512)"` // base64 encoded
	SignCount       uint32         `json:"sign_count" gorm:"default:0"`
	CloneWarning    bool           `json:"clone_warning"`
	UserPresent     bool           `json:"user_present"`
	UserVerified    bool           `json:"user_verified"`
	BackupEligible  bool           `json:"backup_eligible"`
	BackupState     bool           `json:"backup_state"`
	Transports      string         `json:"transports" gorm:"type:text"`
	Attachment      string         `json:"attachment" gorm:"type:varchar(32)"`
	LastUsedAt      *time.Time     `json:"last_used_at"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `json:"-" gorm:"index"`
}

func NormalizePasskeyName(name string) (string, error) {
	normalized := strings.TrimSpace(name)
	if normalized == "" || utf8.RuneCountInString(normalized) > PasskeyNameMaxRunes {
		return "", ErrPasskeyNameInvalid
	}
	for _, character := range normalized {
		if unicode.IsControl(character) {
			return "", ErrPasskeyNameInvalid
		}
	}
	return normalized, nil
}

func (p *PasskeyCredential) TransportList() []protocol.AuthenticatorTransport {
	if p == nil || strings.TrimSpace(p.Transports) == "" {
		return nil
	}
	var transports []string
	if err := common.Unmarshal([]byte(p.Transports), &transports); err != nil {
		return nil
	}
	result := make([]protocol.AuthenticatorTransport, 0, len(transports))
	for _, transport := range transports {
		result = append(result, protocol.AuthenticatorTransport(transport))
	}
	return result
}

func (p *PasskeyCredential) SetTransports(list []protocol.AuthenticatorTransport) {
	if len(list) == 0 {
		p.Transports = ""
		return
	}
	stringList := make([]string, len(list))
	for i, transport := range list {
		stringList[i] = string(transport)
	}
	encoded, err := common.Marshal(stringList)
	if err != nil {
		return
	}
	p.Transports = string(encoded)
}

func (p *PasskeyCredential) ToWebAuthnCredential() webauthn.Credential {
	flags := webauthn.CredentialFlags{
		UserPresent:    p.UserPresent,
		UserVerified:   p.UserVerified,
		BackupEligible: p.BackupEligible,
		BackupState:    p.BackupState,
	}

	credID, _ := base64.StdEncoding.DecodeString(p.CredentialID)
	pubKey, _ := base64.StdEncoding.DecodeString(p.PublicKey)
	aaguid, _ := base64.StdEncoding.DecodeString(p.AAGUID)

	return webauthn.Credential{
		ID:              credID,
		PublicKey:       pubKey,
		AttestationType: p.AttestationType,
		Transport:       p.TransportList(),
		Flags:           flags,
		Authenticator: webauthn.Authenticator{
			AAGUID:       aaguid,
			SignCount:    p.SignCount,
			CloneWarning: p.CloneWarning,
			Attachment:   protocol.AuthenticatorAttachment(p.Attachment),
		},
	}
}

func NewPasskeyCredentialFromWebAuthn(userID int, credential *webauthn.Credential) *PasskeyCredential {
	if credential == nil {
		return nil
	}
	passkey := &PasskeyCredential{
		UserID:          userID,
		CredentialID:    base64.StdEncoding.EncodeToString(credential.ID),
		PublicKey:       base64.StdEncoding.EncodeToString(credential.PublicKey),
		AttestationType: credential.AttestationType,
		AAGUID:          base64.StdEncoding.EncodeToString(credential.Authenticator.AAGUID),
		SignCount:       credential.Authenticator.SignCount,
		CloneWarning:    credential.Authenticator.CloneWarning,
		UserPresent:     credential.Flags.UserPresent,
		UserVerified:    credential.Flags.UserVerified,
		BackupEligible:  credential.Flags.BackupEligible,
		BackupState:     credential.Flags.BackupState,
		Attachment:      string(credential.Authenticator.Attachment),
	}
	passkey.SetTransports(credential.Transport)
	return passkey
}

func GetPasskeysByUserID(userID int) ([]*PasskeyCredential, error) {
	if userID == 0 {
		return nil, ErrPasskeyNotFound
	}
	var credentials []*PasskeyCredential
	if err := DB.Where("user_id = ?", userID).
		Order("created_at DESC").
		Order("id DESC").
		Find(&credentials).Error; err != nil {
		return nil, err
	}
	return credentials, nil
}

func GetPasskeyByIDForUser(userID, passkeyID int) (*PasskeyCredential, error) {
	if userID <= 0 || passkeyID <= 0 {
		return nil, ErrPasskeyNotFound
	}
	var credential PasskeyCredential
	if err := DB.Where("user_id = ? AND id = ?", userID, passkeyID).First(&credential).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPasskeyNotFound
		}
		return nil, err
	}
	return &credential, nil
}

func GetPasskeyByCredentialID(credentialID []byte) (*PasskeyCredential, error) {
	if len(credentialID) == 0 {
		return nil, ErrPasskeyNotFound
	}

	credIDStr := base64.StdEncoding.EncodeToString(credentialID)
	var credential PasskeyCredential
	if err := DB.Where("credential_id = ?", credIDStr).First(&credential).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPasskeyNotFound
		}
		return nil, err
	}
	return &credential, nil
}

// UpdatePasskeyAssertionState persists only fields produced by a successful
// assertion. Registration identity and the display name are immutable here.
func UpdatePasskeyAssertionState(userID int, credential *webauthn.Credential, lastUsedAt time.Time, rpID string) error {
	if userID <= 0 || credential == nil || len(credential.ID) == 0 || lastUsedAt.IsZero() || rpID == "" {
		return fmt.Errorf("Passkey \u4fdd\u5b58\u5931\u8d25\uff0c\u8bf7\u91cd\u8bd5")
	}
	credentialID := base64.StdEncoding.EncodeToString(credential.ID)
	passkeyOptionMutex.Lock()
	defer passkeyOptionMutex.Unlock()
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := validatePasskeyRPIDWithTx(tx, rpID); err != nil {
			return err
		}
		var stored PasskeyCredential
		if err := lockForUpdate(tx).Where("user_id = ? AND credential_id = ?", userID, credentialID).First(&stored).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPasskeyNotFound
			}
			return err
		}
		if stored.RPID != nil && *stored.RPID != "" && *stored.RPID != rpID {
			return system_setting.ErrPasskeyRPIDUnavailable
		}
		result := tx.Model(&PasskeyCredential{}).
			Where("user_id = ? AND credential_id = ?", userID, credentialID).
			Where("rp_id IS NULL OR rp_id = ? OR rp_id = ?", "", rpID).
			Updates(map[string]any{
				"rp_id":           rpID,
				"sign_count":      credential.Authenticator.SignCount,
				"clone_warning":   credential.Authenticator.CloneWarning,
				"user_present":    credential.Flags.UserPresent,
				"user_verified":   credential.Flags.UserVerified,
				"backup_eligible": credential.Flags.BackupEligible,
				"backup_state":    credential.Flags.BackupState,
				"last_used_at":    lastUsedAt,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrPasskeyNotFound
		}
		return nil
	})
}

func createPasskeyCredentialWithTx(tx *gorm.DB, credential *PasskeyCredential) error {
	var existing PasskeyCredential
	err := tx.Where("credential_id = ?", credential.CredentialID).First(&existing).Error
	if err == nil {
		return ErrPasskeyCredentialExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return tx.Create(credential).Error
}

// CreatePasskeyCredentialWithAuthVersion is reserved for account/authentication
// changes. Assertion updates must use UpdatePasskeyAssertionState.
func CreatePasskeyCredentialWithAuthVersion(credential *PasskeyCredential) error {
	return createPasskeyCredentialWithAuthVersion(credential, nil)
}

func RegisterPasskeyForSession(identity AuthSessionIdentity, credential *PasskeyCredential) error {
	return createPasskeyCredentialWithAuthVersion(credential, &identity)
}

func createPasskeyCredentialWithAuthVersion(credential *PasskeyCredential, identity *AuthSessionIdentity) error {
	if credential == nil || credential.UserID <= 0 {
		return fmt.Errorf("Passkey \u4fdd\u5b58\u5931\u8d25\uff0c\u8bf7\u91cd\u8bd5")
	}
	normalizedName, err := NormalizePasskeyName(credential.Name)
	if err != nil {
		return err
	}
	credential.Name = normalizedName

	passkeyOptionMutex.Lock()
	defer passkeyOptionMutex.Unlock()
	if err := DB.Transaction(func(tx *gorm.DB) error {
		if identity != nil && (credential.RPID == nil || *credential.RPID == "") {
			return system_setting.ErrPasskeyRPIDUnavailable
		}
		if credential.RPID != nil && *credential.RPID != "" {
			if err := validatePasskeyRPIDWithTx(tx, *credential.RPID); err != nil {
				return err
			}
		}
		if identity != nil {
			if identity.UserID != credential.UserID {
				return ErrUserSessionInactive
			}
			if err := ValidateAuthSessionWithTx(tx, *identity); err != nil {
				return err
			}
		}
		var lockedUser User
		if err := lockForUpdate(tx).Select("id").First(&lockedUser, credential.UserID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUserSessionInactive
			}
			return err
		}
		var count int64
		if err := tx.Model(&PasskeyCredential{}).Where("user_id = ?", credential.UserID).Count(&count).Error; err != nil {
			return err
		}
		if count >= MaxPasskeysPerUser {
			return ErrPasskeyLimitReached
		}
		if _, err := IncrementUserAuthVersionWithTx(tx, credential.UserID); err != nil {
			return err
		}
		return createPasskeyCredentialWithTx(tx, credential)
	}); err != nil {
		return err
	}
	return PublishUserAuthCache(credential.UserID)
}

func RenamePasskeyForSession(identity AuthSessionIdentity, passkeyID int, name string) (*PasskeyCredential, error) {
	normalizedName, err := NormalizePasskeyName(name)
	if err != nil {
		return nil, err
	}
	if passkeyID <= 0 {
		return nil, ErrPasskeyNotFound
	}
	var updated PasskeyCredential
	if err := DB.Transaction(func(tx *gorm.DB) error {
		if err := ValidateAuthSessionWithTx(tx, identity); err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where("user_id = ? AND id = ?", identity.UserID, passkeyID).First(&updated).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPasskeyNotFound
			}
			return err
		}
		updated.Name = normalizedName
		return tx.Model(&updated).Update("name", normalizedName).Error
	}); err != nil {
		return nil, err
	}
	return &updated, nil
}

func DeletePasskeysByUserIDWithAuthVersion(userID int) error {
	if userID <= 0 {
		return fmt.Errorf("\u5220\u9664\u5931\u8d25\uff0c\u8bf7\u91cd\u8bd5")
	}
	if err := DB.Transaction(func(tx *gorm.DB) error {
		var credentials []PasskeyCredential
		if err := lockForUpdate(tx).Where("user_id = ?", userID).Find(&credentials).Error; err != nil {
			return err
		}
		if len(credentials) == 0 {
			return ErrPasskeyNotFound
		}
		if _, err := IncrementUserAuthVersionWithTx(tx, userID); err != nil {
			return err
		}
		result := tx.Unscoped().Where("user_id = ?", userID).Delete(&PasskeyCredential{})
		if result.Error != nil {
			return result.Error
		}
		return nil
	}); err != nil {
		return err
	}
	return PublishUserAuthCache(userID)
}

func DeletePasskeyForSession(identity AuthSessionIdentity, passkeyID int) error {
	if identity.UserID <= 0 || passkeyID <= 0 {
		return ErrPasskeyNotFound
	}
	if err := DB.Transaction(func(tx *gorm.DB) error {
		if err := ValidateAuthSessionWithTx(tx, identity); err != nil {
			return err
		}
		var credential PasskeyCredential
		if err := lockForUpdate(tx).Where("user_id = ? AND id = ?", identity.UserID, passkeyID).First(&credential).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPasskeyNotFound
			}
			return err
		}
		if _, err := IncrementUserAuthVersionWithTx(tx, identity.UserID); err != nil {
			return err
		}
		result := tx.Unscoped().Delete(&credential)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrPasskeyNotFound
		}
		return nil
	}); err != nil {
		return err
	}
	return PublishUserAuthCache(identity.UserID)
}

// GetPasskeyByUserID returns the most recently created credential. New code
// should use GetPasskeysByUserID or GetPasskeyByIDForUser.
func GetPasskeyByUserID(userID int) (*PasskeyCredential, error) {
	credentials, err := GetPasskeysByUserID(userID)
	if err != nil {
		return nil, err
	}
	if len(credentials) == 0 {
		return nil, ErrPasskeyNotFound
	}
	return credentials[0], nil
}

// UpsertPasskeyCredentialWithAuthVersion preserves the legacy single-credential
// call shape. New authentication flows must use CreatePasskeyCredentialWithAuthVersion.
func UpsertPasskeyCredentialWithAuthVersion(credential *PasskeyCredential) error {
	if credential != nil && credential.Name == "" {
		credential.Name = "Default"
	}
	return CreatePasskeyCredentialWithAuthVersion(credential)
}

// DeletePasskeyByUserIDWithAuthVersion preserves the administrator reset API.
func DeletePasskeyByUserIDWithAuthVersion(userID int) error {
	return DeletePasskeysByUserIDWithAuthVersion(userID)
}
