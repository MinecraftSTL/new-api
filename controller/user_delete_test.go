package controller

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Deleting a managed user is a sensitive administrative change: the handler
// consumes an admin.user.delete proof and still soft-deletes the account, so the
// audit trail and the session revocation stay observable.
func TestDeleteUserSoftDeletesAndRevokesSessions(t *testing.T) {
	_, identity, target := setupAdminUserTest(t)
	require.NoError(t, model.DB.Create(&model.UserSession{
		SID: "admin-delete-session", UserID: target.Id, Version: 1, UserAuthVersion: target.AuthVersion,
		Status: model.UserSessionStatusActive, RefreshHash: "refresh-admin-delete-session",
		LoginMethod: "password", LastActiveAt: 1, ExpiresAt: 1 << 40,
	}).Error)
	proof := issueSecurityEnrollmentProof(t, identity, service.VerificationOperation{
		Scope:   service.VerificationScopeAdminUserDelete,
		Context: []byte(fmt.Sprintf(`{"user_id":%d}`, target.Id)),
	}, service.VerificationMethodPassword)

	response := adminUserRequest(http.MethodDelete, fmt.Sprintf("/api/user/%d", target.Id), "", proof, identity, common.RoleRootUser,
		gin.Params{{Key: "id", Value: fmt.Sprint(target.Id)}}, DeleteUser)

	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"success":true`)
	assertUserSoftDeletedAndSessionRevoked(t, model.DB, target.Id, "admin-delete-session")
}

func TestDeleteSelfSoftDeletesAndRevokesSessions(t *testing.T) {
	user, identity := setupSecurityEnrollmentTest(t)
	proof := issueSecurityEnrollmentProof(t, identity, service.VerificationOperation{
		Scope: service.VerificationScopeAccountDelete,
	}, service.VerificationMethodPassword)

	response := securityEnrollmentRequest("DELETE", "/api/user/self", "", proof, identity, DeleteSelf)

	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"success":true`)
	assertUserSoftDeletedAndSessionRevoked(t, model.DB, user.Id, identity.SessionID)
}

func assertUserSoftDeletedAndSessionRevoked(t *testing.T, db *gorm.DB, userID int, sid string) {
	t.Helper()
	var visibleCount int64
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", userID).Count(&visibleCount).Error)
	assert.Zero(t, visibleCount)

	var deleted model.User
	require.NoError(t, db.Unscoped().First(&deleted, userID).Error)
	assert.True(t, deleted.DeletedAt.Valid)
	assert.EqualValues(t, 2, deleted.AuthVersion)

	var session model.UserSession
	require.NoError(t, db.First(&session, "sid = ?", sid).Error)
	assert.Equal(t, model.UserSessionStatusRevoked, session.Status)
	assert.Equal(t, "user_deleted", session.RevokedReason)
}
