package model

import (
	"context"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSensitiveReplacementLogCleanupCountsAndDeletesOldRows(t *testing.T) {
	truncateTables(t)
	ctx := context.Background()

	logs := []*SensitiveReplacementLog{
		{CreatedAt: 100, RequestId: "old-1", MatchedWord: "secret", Replacement: "MASK", Count: 1},
		{CreatedAt: 200, RequestId: "old-2", MatchedWord: "secret", Replacement: "MASK", Count: 1},
		{CreatedAt: 300, RequestId: "new", MatchedWord: "av", Replacement: "XX", Count: 1},
	}
	require.NoError(t, RecordSensitiveReplacementLogs(logs))

	count, err := CountOldSensitiveReplacementLogs(ctx, 250)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	deleted, err := DeleteOldSensitiveReplacementLogBatch(ctx, 250, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	count, err = CountOldSensitiveReplacementLogs(ctx, 250)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	deleted, err = DeleteOldSensitiveReplacementLogBatch(ctx, 250, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	remaining, total, err := GetSensitiveReplacementLogs(0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, remaining, 1)
	assert.Equal(t, "new", remaining[0].RequestId)
}

func TestSensitiveReplacementLogsAreEncryptedAtRest(t *testing.T) {
	truncateTables(t)

	require.NoError(t, RecordSensitiveReplacementLogs([]*SensitiveReplacementLog{{
		CreatedAt:       100,
		RequestId:       "encrypted",
		MatchedWord:     "secret",
		Replacement:     "MASK",
		OriginalContext: "before secret after",
		ReplacedContext: "before MASK after",
		Count:           1,
	}}))

	var raw SensitiveReplacementLog
	require.NoError(t, DB.Where("request_id = ?", "encrypted").First(&raw).Error)
	assert.True(t, strings.HasPrefix(raw.MatchedWord, sensitiveReplacementLogEncryptionPrefix))
	assert.NotContains(t, raw.MatchedWord, "secret")
	assert.True(t, strings.HasPrefix(raw.Replacement, sensitiveReplacementLogEncryptionPrefix))
	assert.NotContains(t, raw.OriginalContext, "secret")
	assert.NotContains(t, raw.ReplacedContext, "MASK")

	logs, total, err := GetSensitiveReplacementLogs(0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	assert.Equal(t, "secret", logs[0].MatchedWord)
	assert.Equal(t, "MASK", logs[0].Replacement)
	assert.Equal(t, "before secret after", logs[0].OriginalContext)
	assert.Equal(t, "before MASK after", logs[0].ReplacedContext)
}

func TestSensitiveReplacementLogPlaintextRowsAreHidden(t *testing.T) {
	truncateTables(t)

	require.NoError(t, DB.Create(&SensitiveReplacementLog{
		CreatedAt:       100,
		RequestId:       "legacy-plain",
		MatchedWord:     "secret",
		Replacement:     "MASK",
		OriginalContext: "legacy secret context",
		ReplacedContext: "legacy MASK context",
		Count:           1,
	}).Error)

	logs, total, err := GetSensitiveReplacementLogs(0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	assert.True(t, logs[0].DecryptFailed)
	assert.Empty(t, logs[0].MatchedWord)
	assert.Empty(t, logs[0].Replacement)
	assert.Empty(t, logs[0].OriginalContext)
	assert.Empty(t, logs[0].ReplacedContext)
}

func TestSensitiveReplacementLogDecryptFailureIsHidden(t *testing.T) {
	truncateTables(t)
	oldSecret := common.CryptoSecret
	t.Cleanup(func() {
		common.CryptoSecret = oldSecret
	})

	common.CryptoSecret = "secret-one"
	require.NoError(t, RecordSensitiveReplacementLogs([]*SensitiveReplacementLog{{
		CreatedAt:       100,
		RequestId:       "wrong-secret",
		MatchedWord:     "secret",
		Replacement:     "MASK",
		OriginalContext: "secret",
		ReplacedContext: "MASK",
		Count:           1,
	}}))

	common.CryptoSecret = "secret-two"
	logs, total, err := GetSensitiveReplacementLogs(0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	assert.True(t, logs[0].DecryptFailed)
	assert.Empty(t, logs[0].MatchedWord)
	assert.Empty(t, logs[0].Replacement)
	assert.Empty(t, logs[0].OriginalContext)
	assert.Empty(t, logs[0].ReplacedContext)
}
