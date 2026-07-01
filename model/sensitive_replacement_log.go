package model

import "context"

type SensitiveReplacementLog struct {
	Id              int    `json:"id" gorm:"index:idx_sensitive_replacement_logs_created_at_id,priority:2"`
	CreatedAt       int64  `json:"created_at" gorm:"bigint;index:idx_sensitive_replacement_logs_created_at_id,priority:1"`
	UserId          int    `json:"user_id" gorm:"index;default:0"`
	Username        string `json:"username" gorm:"default:''"`
	TokenId         int    `json:"token_id" gorm:"index;default:0"`
	TokenName       string `json:"token_name" gorm:"default:''"`
	ModelName       string `json:"model_name" gorm:"index;default:''"`
	RequestPath     string `json:"request_path" gorm:"type:varchar(255);default:''"`
	RequestId       string `json:"request_id" gorm:"type:varchar(64);index;default:''"`
	MatchedWord     string `json:"matched_word" gorm:"type:text"`
	Replacement     string `json:"replacement" gorm:"type:text"`
	Count           int    `json:"count" gorm:"default:0"`
	OriginalContext string `json:"original_context" gorm:"type:text"`
	ReplacedContext string `json:"replaced_context" gorm:"type:text"`
}

func RecordSensitiveReplacementLogs(logs []*SensitiveReplacementLog) error {
	if len(logs) == 0 {
		return nil
	}
	return DB.Create(&logs).Error
}

func GetSensitiveReplacementLogs(startIdx int, num int) (logs []*SensitiveReplacementLog, total int64, err error) {
	tx := DB.Model(&SensitiveReplacementLog{})
	if err = tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err = tx.Order("created_at desc, id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	return logs, total, err
}

func CountOldSensitiveReplacementLogs(ctx context.Context, targetTimestamp int64) (int64, error) {
	var total int64
	err := DB.WithContext(ctx).Model(&SensitiveReplacementLog{}).
		Where("created_at < ?", targetTimestamp).
		Count(&total).Error
	return total, err
}

func DeleteOldSensitiveReplacementLogBatch(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
	if limit <= 0 {
		limit = 100
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	var ids []int
	if err := DB.WithContext(ctx).
		Model(&SensitiveReplacementLog{}).
		Where("created_at < ?", targetTimestamp).
		Order("created_at asc, id asc").
		Limit(limit).
		Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}

	result := DB.WithContext(ctx).Where("id IN ?", ids).Delete(&SensitiveReplacementLog{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}
