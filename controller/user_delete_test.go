package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDeleteUserSoftDeletesAndRevokesSessions(t *testing.T) {
	db := setupManageUserTestDB(t)
	user := createUserWithActiveSession(t, db, "admin-delete-user", "admin-delete-session")

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/user/%d", user.Id), nil)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", user.Id)}}
	c.Set("id", 9999)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "root-operator")

	DeleteUser(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	assertUserSoftDeletedAndSessionRevoked(t, db, user.Id, "admin-delete-session")
}

func TestDeleteSelfSoftDeletesAndRevokesSessions(t *testing.T) {
	db := setupManageUserTestDB(t)
	user := createUserWithActiveSession(t, db, "self-delete-user", "self-delete-session")

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/user/self", nil)
	c.Set("id", user.Id)

	DeleteSelf(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	assertUserSoftDeletedAndSessionRevoked(t, db, user.Id, "self-delete-session")
}

func createUserWithActiveSession(t *testing.T, db *gorm.DB, username string, sid string) model.User {
	t.Helper()
	user := model.User{
		Username: username, Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(&user).Error)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.UserSession{
		SID: sid, UserID: user.Id, Version: 1, UserAuthVersion: 1,
		Status: model.UserSessionStatusActive, RefreshHash: "refresh-" + sid, LoginMethod: "password",
		LastActiveAt: now, ExpiresAt: now + 3600,
	}).Error)
	return user
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
