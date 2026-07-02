package setting

import "strings"

var CheckSensitiveEnabled = true
var CheckSensitiveOnPromptEnabled = true

//var CheckSensitiveOnCompletionEnabled = true

// StopOnSensitiveEnabled 如果检测到敏感词，是否立刻停止生成，否则替换敏感词
var StopOnSensitiveEnabled = true

// StreamCacheQueueLength 流模式缓存队列长度，0表示无缓存
var StreamCacheQueueLength = 0

// SensitiveWords 敏感词
// var SensitiveWords []string
var SensitiveWords = []string{
	"test_sensitive",
}

// SensitiveReplacement* is intentionally separate from the legacy SensitiveWords
// blocklist so admins can enable replacement without changing block behavior.
var SensitiveReplacementEnabled = false
var SensitiveReplacementLogRetentionDays = 30

// SensitiveReplacementRules stores one replacement rule per line:
// word=>replacement, or word for the default replacement. Full-line comments
// are preserved for admins and ignored when replacement rules are evaluated.
var SensitiveReplacementRules []string

func SensitiveWordsToString() string {
	return strings.Join(SensitiveWords, "\n")
}

func SensitiveWordsFromString(s string) {
	SensitiveWords = []string{}
	sw := strings.Split(s, "\n")
	for _, w := range sw {
		w = strings.TrimSpace(w)
		if w != "" {
			SensitiveWords = append(SensitiveWords, w)
		}
	}
}

func ShouldCheckPromptSensitive() bool {
	return CheckSensitiveEnabled && CheckSensitiveOnPromptEnabled
}

func SensitiveReplacementRulesToString() string {
	return strings.Join(SensitiveReplacementRules, "\n")
}

func SensitiveReplacementRulesFromString(s string) {
	SensitiveReplacementRules = []string{}
	sw := strings.Split(s, "\n")
	for _, w := range sw {
		w = strings.TrimSpace(w)
		if w != "" {
			SensitiveReplacementRules = append(SensitiveReplacementRules, w)
		}
	}
}

func IsSensitiveReplacementRuleLine(line string) bool {
	line = strings.TrimSpace(line)
	return line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "//")
}

func ShouldReplacePromptSensitive() bool {
	if !SensitiveReplacementEnabled {
		return false
	}
	for _, rule := range SensitiveReplacementRules {
		if IsSensitiveReplacementRuleLine(rule) {
			return true
		}
	}
	return false
}

//func ShouldCheckCompletionSensitive() bool {
//	return CheckSensitiveEnabled && CheckSensitiveOnCompletionEnabled
//}
