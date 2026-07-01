package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func GetSensitiveReplacementLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	logs, total, err := model.GetSensitiveReplacementLogs(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]dto.SensitiveReplacementLog, 0, len(logs))
	for _, log := range logs {
		items = append(items, dto.SensitiveReplacementLog{
			Id:              log.Id,
			CreatedAt:       log.CreatedAt,
			UserId:          log.UserId,
			Username:        log.Username,
			TokenId:         log.TokenId,
			TokenName:       log.TokenName,
			ModelName:       log.ModelName,
			RequestPath:     log.RequestPath,
			RequestId:       log.RequestId,
			MatchedWord:     log.MatchedWord,
			Replacement:     log.Replacement,
			Count:           log.Count,
			OriginalContext: log.OriginalContext,
			ReplacedContext: log.ReplacedContext,
		})
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}
