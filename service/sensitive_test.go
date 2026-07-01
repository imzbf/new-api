package service

import (
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withLegacySensitiveWords(t *testing.T, words []string) {
	t.Helper()
	oldRules := append([]string(nil), setting.SensitiveWords...)
	t.Cleanup(func() {
		setting.SensitiveWords = oldRules
	})
	setting.SensitiveWords = append([]string(nil), words...)
}

func withSensitiveReplacementRules(t *testing.T, rules []string) {
	t.Helper()
	oldRules := append([]string(nil), setting.SensitiveReplacementRules...)
	t.Cleanup(func() {
		setting.SensitiveReplacementRules = oldRules
	})
	setting.SensitiveReplacementRules = append([]string(nil), rules...)
}

func TestLegacySensitiveWordsStillBlock(t *testing.T) {
	withLegacySensitiveWords(t, []string{"legacy_block"})

	contains, words := CheckSensitiveText("please stop legacy_block now")

	require.True(t, contains)
	assert.Equal(t, []string{"legacy_block"}, words)
}

func TestParseSensitiveReplacementRules(t *testing.T) {
	rules := ParseSensitiveReplacementRules([]string{
		"",
		"色情=>净化",
		"av",
		"AV=>YY",
		"empty=>",
	})

	require.Len(t, rules, 3)
	assert.Equal(t, SensitiveReplacementRule{Word: "色情", Replacement: "净化"}, rules[0])
	assert.Equal(t, SensitiveReplacementRule{Word: "AV", Replacement: "YY"}, rules[1])
	assert.Equal(t, SensitiveReplacementRule{Word: "empty", Replacement: DefaultSensitiveReplacement}, rules[2])
}

func TestReplaceSensitiveTextBoundariesAndLongestMatch(t *testing.T) {
	withSensitiveReplacementRules(t, []string{
		"av",
		"色情=>健康",
		"bad=>SHORT",
		"badword=>LONG",
	})

	result := ReplaceSensitiveText("java av AV 色情片 badword bad")

	require.True(t, result.Changed)
	assert.Equal(t, "java XX XX 健康片 LONG SHORT", result.Content)
	assert.Equal(t, []string{"av", "色情", "badword", "bad"}, SensitiveReplacementWords(result.Matches))
	assert.Equal(t, 2, result.Matches[0].Count)
}

func TestApplySensitiveReplacementsToRequestDTOs(t *testing.T) {
	withSensitiveReplacementRules(t, []string{"secret=>MASK", "av"})

	t.Run("openai chat", func(t *testing.T) {
		request := &dto.GeneralOpenAIRequest{
			Model: "gpt-test",
			Messages: []dto.Message{
				{Role: "user", Content: "java av secret"},
				{Role: "user", Content: []any{map[string]any{"type": "text", "text": "secret"}}},
			},
			Input: []any{"secret"},
		}

		result, err := ApplySensitiveReplacementsToRequest(request)

		require.NoError(t, err)
		require.True(t, result.Changed)
		assert.Equal(t, "java XX MASK", request.Messages[0].Content)
		assert.Equal(t, "MASK", request.Messages[1].Content.([]any)[0].(map[string]any)["text"])
		assert.Equal(t, []any{"MASK"}, request.Input)
	})

	t.Run("openai responses", func(t *testing.T) {
		input := []byte(`[{"role":"user","content":[{"type":"input_text","text":"secret"}]}]`)
		instructions, err := common.Marshal("av guide")
		require.NoError(t, err)
		request := &dto.OpenAIResponsesRequest{Model: "gpt-test", Input: input, Instructions: instructions}

		result, err := ApplySensitiveReplacementsToRequest(request)

		require.NoError(t, err)
		require.True(t, result.Changed)
		assert.NotContains(t, string(request.Input), "secret")
		assert.Contains(t, string(request.Input), "MASK")
		assert.Contains(t, string(request.Instructions), "XX guide")
	})

	t.Run("claude", func(t *testing.T) {
		request := &dto.ClaudeRequest{
			Model:  "claude-test",
			System: "secret system",
			Messages: []dto.ClaudeMessage{
				{Role: "user", Content: []any{map[string]any{"type": "text", "text": "av"}}},
			},
		}

		result, err := ApplySensitiveReplacementsToRequest(request)

		require.NoError(t, err)
		require.True(t, result.Changed)
		assert.Equal(t, "MASK system", request.System)
		assert.Equal(t, "XX", request.Messages[0].Content.([]any)[0].(map[string]any)["text"])
	})

	t.Run("gemini", func(t *testing.T) {
		request := &dto.GeminiChatRequest{
			Contents: []dto.GeminiChatContent{{Parts: []dto.GeminiPart{{Text: "secret"}}}},
		}

		result, err := ApplySensitiveReplacementsToRequest(request)

		require.NoError(t, err)
		require.True(t, result.Changed)
		assert.Equal(t, "MASK", request.Contents[0].Parts[0].Text)
	})

	t.Run("embedding rerank image audio", func(t *testing.T) {
		embedding := &dto.EmbeddingRequest{Input: []any{"secret"}}
		rerank := &dto.RerankRequest{Query: "secret", Documents: []any{"av", map[string]any{"text": "secret"}}}
		image := &dto.ImageRequest{Prompt: "secret"}
		audio := &dto.AudioRequest{Input: "secret", Instructions: "av"}

		for _, request := range []dto.Request{embedding, rerank, image, audio} {
			result, err := ApplySensitiveReplacementsToRequest(request)
			require.NoError(t, err)
			require.True(t, result.Changed)
		}

		assert.Equal(t, []any{"MASK"}, embedding.Input)
		assert.Equal(t, "MASK", rerank.Query)
		assert.Equal(t, []any{"XX", map[string]any{"text": "MASK"}}, rerank.Documents)
		assert.Equal(t, "MASK", image.Prompt)
		assert.Equal(t, "MASK", audio.Input)
		assert.Equal(t, "XX", audio.Instructions)
	})
}

func TestRewriteSensitiveRequestBodyReplacesReusablePayload(t *testing.T) {
	withSensitiveReplacementRules(t, []string{"secret=>MASK"})
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := `{"model":"gpt-test","messages":[{"role":"user","content":"secret"}],"extra":"secret"}`
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	require.NoError(t, RewriteSensitiveReplacementRequestBody(c))

	storage, err := common.GetBodyStorage(c)
	require.NoError(t, err)
	rewritten, err := storage.Bytes()
	require.NoError(t, err)
	assert.NotContains(t, string(rewritten), "secret")
	assert.Contains(t, string(rewritten), "MASK")
}

func TestRewriteSensitiveMultipartRequestBody(t *testing.T) {
	withSensitiveReplacementRules(t, []string{"secret=>MASK"})
	gin.SetMode(gin.TestMode)

	var body strings.Builder
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-1"))
	require.NoError(t, writer.WriteField("prompt", "secret"))
	part, err := writer.CreateFormFile("image", "test.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("image-bytes"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/edits", strings.NewReader(body.String()))
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	require.NoError(t, RewriteSensitiveReplacementRequestBody(c))

	storage, err := common.GetBodyStorage(c)
	require.NoError(t, err)
	rewritten, err := storage.Bytes()
	require.NoError(t, err)
	assert.NotContains(t, string(rewritten), "secret")
	assert.Contains(t, string(rewritten), "MASK")
	assert.Equal(t, "MASK", c.Request.MultipartForm.Value["prompt"][0])
}

func TestSensitiveReplacementLogPagination(t *testing.T) {
	truncate(t)
	logs := []*model.SensitiveReplacementLog{
		{
			CreatedAt:       100,
			UserId:          1,
			TokenId:         2,
			ModelName:       "gpt-test",
			RequestPath:     "/v1/chat/completions",
			RequestId:       "req-old",
			MatchedWord:     "secret",
			Replacement:     "MASK",
			Count:           1,
			OriginalContext: "old secret context",
			ReplacedContext: "old MASK context",
		},
		{
			CreatedAt:       101,
			UserId:          1,
			TokenId:         2,
			ModelName:       "gpt-test",
			RequestPath:     "/v1/chat/completions",
			RequestId:       "req-new",
			MatchedWord:     "av",
			Replacement:     "XX",
			Count:           2,
			OriginalContext: "new av context",
			ReplacedContext: "new XX context",
		},
	}
	require.NoError(t, model.RecordSensitiveReplacementLogs(logs))

	page, total, err := model.GetSensitiveReplacementLogs(0, 1)

	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, page, 1)
	assert.Equal(t, "req-new", page[0].RequestId)
	assert.Equal(t, "new av context", page[0].OriginalContext)
	assert.Equal(t, "new XX context", page[0].ReplacedContext)
}
