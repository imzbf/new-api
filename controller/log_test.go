package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type logStatsResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Quota            int   `json:"quota"`
		Rpm              int   `json:"rpm"`
		Tpm              int   `json:"tpm"`
		PromptTokens     int64 `json:"prompt_tokens"`
		CacheTokens      int64 `json:"cache_tokens"`
		CacheWriteTokens int64 `json:"cache_write_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
	} `json:"data"`
}

func setupLogStatsControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMainDatabaseType := common.MainDatabaseType()
	oldLogDatabaseType := common.LogDatabaseType()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	model.DB = db
	model.LOG_DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	gin.SetMode(gin.TestMode)

	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.SetDatabaseTypes(oldMainDatabaseType, oldLogDatabaseType)
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestLogStatsEndpointsExposeTokenBreakdown(t *testing.T) {
	db := setupLogStatsControllerTestDB(t)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&[]model.Log{
		{
			CreatedAt:        now - 5,
			Type:             model.LogTypeConsume,
			Username:         "alice",
			Quota:            100,
			PromptTokens:     1000,
			CompletionTokens: 200,
			Other: common.MapToJsonStr(map[string]interface{}{
				"cache_tokens":       100,
				"cache_write_tokens": 40,
			}),
		},
		{
			CreatedAt:        now - 4,
			Type:             model.LogTypeConsume,
			Username:         "bob",
			Quota:            999,
			PromptTokens:     999,
			CompletionTokens: 999,
			Other: common.MapToJsonStr(map[string]interface{}{
				"cache_tokens":       999,
				"cache_write_tokens": 999,
			}),
		},
	}).Error)

	tests := []struct {
		name    string
		path    string
		prepare func(*gin.Context)
		handler func(*gin.Context)
	}{
		{
			name:    "admin stats",
			path:    fmt.Sprintf("/api/log/stat?username=alice&start_timestamp=%d&end_timestamp=%d", now-30, now+30),
			prepare: func(_ *gin.Context) {},
			handler: GetLogsStat,
		},
		{
			name: "self stats",
			path: fmt.Sprintf("/api/log/self/stat?start_timestamp=%d&end_timestamp=%d", now-30, now+30),
			prepare: func(c *gin.Context) {
				c.Set("username", "alice")
			},
			handler: GetLogsSelfStat,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, test.path, nil)
			test.prepare(ctx)

			test.handler(ctx)

			require.Equal(t, http.StatusOK, recorder.Code)
			var response logStatsResponse
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.True(t, response.Success)
			assert.Equal(t, 100, response.Data.Quota)
			assert.Equal(t, 1, response.Data.Rpm)
			assert.Equal(t, 1200, response.Data.Tpm)
			assert.Equal(t, int64(1000), response.Data.PromptTokens)
			assert.Equal(t, int64(100), response.Data.CacheTokens)
			assert.Equal(t, int64(40), response.Data.CacheWriteTokens)
			assert.Equal(t, int64(200), response.Data.CompletionTokens)
		})
	}
}
