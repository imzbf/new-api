package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSumUsedQuotaIncludesTokenBreakdown(t *testing.T) {
	require.NoError(t, LOG_DB.Exec("DELETE FROM logs").Error)
	t.Cleanup(func() {
		require.NoError(t, LOG_DB.Exec("DELETE FROM logs").Error)
	})

	now := time.Now().Unix()
	matchingLog := func(createdAt int64, quota int, promptTokens int, completionTokens int, other string) Log {
		return Log{
			UserId:           1,
			CreatedAt:        createdAt,
			Type:             LogTypeConsume,
			Username:         "alice",
			TokenName:        "key-a",
			ModelName:        "gpt-test",
			Quota:            quota,
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			ChannelId:        7,
			Group:            "default",
			Other:            other,
		}
	}

	logs := []Log{
		matchingLog(now-10, 100, 1000, 200, common.MapToJsonStr(map[string]interface{}{
			"cache_tokens":             100,
			"cache_write_tokens":       40,
			"cache_creation_tokens":    999,
			"cache_creation_tokens_5m": 999,
			"cache_creation_tokens_1h": 999,
		})),
		matchingLog(now-9, 50, 500, 100, common.MapToJsonStr(map[string]interface{}{
			"cache_tokens":             50,
			"cache_creation_tokens":    30,
			"cache_creation_tokens_5m": 10,
			"cache_creation_tokens_1h": 15,
		})),
		matchingLog(now-8, 25, 250, 50, common.MapToJsonStr(map[string]interface{}{
			"cache_creation_tokens": 12,
		})),
		matchingLog(now-7, 10, 100, 20, "not-json"),
		matchingLog(now-6, 0, 0, 0, common.MapToJsonStr(map[string]interface{}{
			"cache_write_tokens":       0,
			"cache_creation_tokens":    999,
			"cache_creation_tokens_5m": 999,
			"cache_creation_tokens_1h": 999,
		})),
		{
			CreatedAt:        now - 5,
			Type:             LogTypeError,
			Username:         "alice",
			TokenName:        "key-a",
			ModelName:        "gpt-test",
			Quota:            1000,
			PromptTokens:     1000,
			CompletionTokens: 1000,
			ChannelId:        7,
			Group:            "default",
			Other:            common.MapToJsonStr(map[string]interface{}{"cache_tokens": 1000}),
		},
		{
			CreatedAt:        now - 4,
			Type:             LogTypeConsume,
			Username:         "bob",
			TokenName:        "key-a",
			ModelName:        "gpt-test",
			Quota:            1000,
			PromptTokens:     1000,
			CompletionTokens: 1000,
			ChannelId:        7,
			Group:            "default",
			Other:            common.MapToJsonStr(map[string]interface{}{"cache_tokens": 1000}),
		},
		matchingLog(now-120, 1000, 1000, 1000, common.MapToJsonStr(map[string]interface{}{
			"cache_tokens": 1000,
		})),
	}
	require.NoError(t, LOG_DB.Create(&logs).Error)

	stat, err := SumUsedQuota(0, now-30, now+30, "gpt-test", "alice", "key-a", 7, "default")
	require.NoError(t, err)
	assert.Equal(t, 185, stat.Quota)
	assert.Equal(t, 5, stat.Rpm)
	assert.Equal(t, 2220, stat.Tpm)
	assert.Equal(t, int64(1850), stat.PromptTokens)
	assert.Equal(t, int64(150), stat.CacheTokens)
	assert.Equal(t, int64(77), stat.CacheWriteTokens)
	assert.Equal(t, int64(370), stat.CompletionTokens)

	empty, err := SumUsedQuota(0, now-30, now+30, "missing-model", "alice", "key-a", 7, "default")
	require.NoError(t, err)
	assert.Equal(t, Stat{}, empty)
}
