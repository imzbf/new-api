package relay

import (
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func sanitizeOutboundSensitiveReplacementJSON(c *gin.Context, jsonData []byte) ([]byte, *types.NewAPIError) {
	if !setting.ShouldReplacePromptSensitive() {
		return jsonData, nil
	}
	sanitized, result, err := service.SanitizeSensitiveReplacementJSONBytes(jsonData)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	service.MergeSensitiveReplacementMatches(c, result.Matches)
	return sanitized, nil
}
