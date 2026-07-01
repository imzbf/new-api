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
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSystemTaskControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}, &model.SystemTask{}, &model.SystemTaskLock{}))
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "tester", Role: common.RoleRootUser, Status: common.UserStatusEnabled}).Error)

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func newSystemTaskCleanupRouter(role int) *gin.Engine {
	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("system-task-test"))))
	router.Use(func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("username", "tester")
		session.Set("role", role)
		session.Set("id", 1)
		session.Set("status", common.UserStatusEnabled)
		session.Set("group", "default")
		c.Next()
	})
	group := router.Group("/api/system-task")
	group.Use(middleware.RootAuth())
	group.POST("/sensitive-replacement-log-cleanup", CreateSensitiveReplacementLogCleanupSystemTask)
	return router
}

func TestCreateSensitiveReplacementLogCleanupSystemTaskRequiresRoot(t *testing.T) {
	setupSystemTaskControllerTestDB(t)

	router := newSystemTaskCleanupRouter(common.RoleAdminUser)
	req := httptest.NewRequest(http.MethodPost, "/api/system-task/sensitive-replacement-log-cleanup?target_timestamp=1000", nil)
	req.Header.Set("New-Api-User", "1")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.Success)

	var count int64
	require.NoError(t, model.DB.Model(&model.SystemTask{}).
		Where("type = ?", model.SystemTaskTypeSensitiveReplacementLogCleanup).
		Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestCreateSensitiveReplacementLogCleanupSystemTaskAsRoot(t *testing.T) {
	setupSystemTaskControllerTestDB(t)

	router := newSystemTaskCleanupRouter(common.RoleRootUser)
	req := httptest.NewRequest(http.MethodPost, "/api/system-task/sensitive-replacement-log-cleanup?target_timestamp=1000", nil)
	req.Header.Set("New-Api-User", "1")
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
