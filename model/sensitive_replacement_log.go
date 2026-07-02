package model

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const (
	sensitiveReplacementLogEncryptionPrefix  = "enc:v1:"
	sensitiveReplacementLogEncryptionPurpose = "\x00sensitive_replacement_log:v1"
)

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
	for _, log := range logs {
		if err := encryptSensitiveReplacementLog(log); err != nil {
			return err
		}
	}
	return DB.Create(&logs).Error
}

func GetSensitiveReplacementLogs(startIdx int, num int) (logs []*SensitiveReplacementLog, total int64, err error) {
	tx := DB.Model(&SensitiveReplacementLog{})
	if err = tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err = tx.Order("created_at desc, id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}
	if err = decryptSensitiveReplacementLogs(logs); err != nil {
		return nil, 0, err
	}
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

func encryptSensitiveReplacementLog(log *SensitiveReplacementLog) error {
	if log == nil {
		return nil
	}
	var err error
	if log.MatchedWord, err = encryptSensitiveReplacementLogText(log.MatchedWord); err != nil {
		return err
	}
	if log.Replacement, err = encryptSensitiveReplacementLogText(log.Replacement); err != nil {
		return err
	}
	if log.OriginalContext, err = encryptSensitiveReplacementLogText(log.OriginalContext); err != nil {
		return err
	}
	log.ReplacedContext, err = encryptSensitiveReplacementLogText(log.ReplacedContext)
	return err
}

func decryptSensitiveReplacementLogs(logs []*SensitiveReplacementLog) error {
	for _, log := range logs {
		if log == nil {
			continue
		}
		var err error
		if log.MatchedWord, err = decryptSensitiveReplacementLogText(log.MatchedWord); err != nil {
			return err
		}
		if log.Replacement, err = decryptSensitiveReplacementLogText(log.Replacement); err != nil {
			return err
		}
		if log.OriginalContext, err = decryptSensitiveReplacementLogText(log.OriginalContext); err != nil {
			return err
		}
		if log.ReplacedContext, err = decryptSensitiveReplacementLogText(log.ReplacedContext); err != nil {
			return err
		}
	}
	return nil
}

func encryptSensitiveReplacementLogText(value string) (string, error) {
	if value == "" || strings.HasPrefix(value, sensitiveReplacementLogEncryptionPrefix) {
		return value, nil
	}
	gcm, err := sensitiveReplacementLogGCM()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	// Prefix the nonce to the ciphertext so each field stays self-contained and
	// can be decrypted without storing extra columns.
	sealed := gcm.Seal(nonce, nonce, []byte(value), nil)
	return sensitiveReplacementLogEncryptionPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

func decryptSensitiveReplacementLogText(value string) (string, error) {
	if value == "" {
		return value, nil
	}
	if !strings.HasPrefix(value, sensitiveReplacementLogEncryptionPrefix) {
		return "", fmt.Errorf("decrypt sensitive replacement log: plaintext value is not supported")
	}
	encoded := strings.TrimPrefix(value, sensitiveReplacementLogEncryptionPrefix)
	sealed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decrypt sensitive replacement log: invalid ciphertext encoding: %w", err)
	}
	gcm, err := sensitiveReplacementLogGCM()
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(sealed) <= nonceSize {
		return "", fmt.Errorf("decrypt sensitive replacement log: ciphertext too short")
	}
	nonce := sealed[:nonceSize]
	ciphertext := sealed[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt sensitive replacement log: %w", err)
	}
	return string(plaintext), nil
}

func sensitiveReplacementLogGCM() (cipher.AEAD, error) {
	key := sha256.Sum256([]byte(common.CryptoSecret + sensitiveReplacementLogEncryptionPurpose))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
