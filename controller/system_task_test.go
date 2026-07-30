package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSystemTaskControllerTestDB(t *testing.T, role int) string {
	t.Helper()

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	previousRedisEnabled := common.RedisEnabled

	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}, &model.SystemTask{}, &model.SystemTaskLock{}))
	accessToken := "system-task-controller-token-001"
	require.NoError(t, db.Create(&model.User{
		Id:          1,
		Username:    "tester",
		Role:        role,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AccessToken: &accessToken,
		AuthVersion: 1,
	}).Error)

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		common.RedisEnabled = previousRedisEnabled
	})
	return accessToken
}

func newSystemTaskCleanupRouter() *gin.Engine {
	router := gin.New()
	group := router.Group("/api/system-task")
	group.Use(middleware.RootAuth())
	group.POST("/sensitive-replacement-log-cleanup", CreateSensitiveReplacementLogCleanupSystemTask)
	return router
}

func TestCreateSensitiveReplacementLogCleanupSystemTaskRequiresRoot(t *testing.T) {
	accessToken := setupSystemTaskControllerTestDB(t, common.RoleAdminUser)

	router := newSystemTaskCleanupRouter()
	req := httptest.NewRequest(http.MethodPost, "/api/system-task/sensitive-replacement-log-cleanup?target_timestamp=1000", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	var resp struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.Success)
	assert.Equal(t, "AUTH_INSUFFICIENT_PRIVILEGE", resp.Code)

	var count int64
	require.NoError(t, model.DB.Model(&model.SystemTask{}).
		Where("type = ?", model.SystemTaskTypeSensitiveReplacementLogCleanup).
		Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestCreateSensitiveReplacementLogCleanupSystemTaskAsRoot(t *testing.T) {
	accessToken := setupSystemTaskControllerTestDB(t, common.RoleRootUser)

	router := newSystemTaskCleanupRouter()
	req := httptest.NewRequest(http.MethodPost, "/api/system-task/sensitive-replacement-log-cleanup?target_timestamp=1000", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Success bool                     `json:"success"`
		Data    model.SystemTaskResponse `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	assert.Equal(t, model.SystemTaskTypeSensitiveReplacementLogCleanup, resp.Data.Type)

	current, err := model.GetActiveSystemTask(model.SystemTaskTypeSensitiveReplacementLogCleanup)
	require.NoError(t, err)
	require.NotNil(t, current)
	assert.Equal(t, resp.Data.TaskID, current.TaskID)
}
