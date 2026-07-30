package dto

type SensitiveReplacementLog struct {
	Id              int    `json:"id"`
	CreatedAt       int64  `json:"created_at"`
	UserId          int    `json:"user_id"`
	Username        string `json:"username"`
	TokenId         int    `json:"token_id"`
	TokenName       string `json:"token_name"`
	ModelName       string `json:"model_name"`
	RequestPath     string `json:"request_path"`
	RequestId       string `json:"request_id"`
	MatchedWord     string `json:"matched_word"`
	Replacement     string `json:"replacement"`
	Count           int    `json:"count"`
	OriginalContext string `json:"original_context"`
	ReplacedContext string `json:"replaced_context"`
	DecryptFailed   bool   `json:"decrypt_failed"`
}

type SensitiveReplacementLogPage struct {
	Page     int                       `json:"page"`
	PageSize int                       `json:"page_size"`
	Total    int                       `json:"total"`
	Items    []SensitiveReplacementLog `json:"items"`
}
