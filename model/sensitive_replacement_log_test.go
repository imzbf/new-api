package model

import (
	"context"
	"testing"

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
