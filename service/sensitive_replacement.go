package service

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
)

const (
	DefaultSensitiveReplacement = "XX"
	sensitiveContextRuneRadius  = 24
)

// SensitiveReplacementRule is one normalized line from the independent
// SensitiveReplacementRules option. It intentionally does not read legacy
// SensitiveWords, so the old blocklist keeps its original meaning.
type SensitiveReplacementRule struct {
	Word        string
	Replacement string
}

// SensitiveReplacementMatch records one word/replacement pair aggregated inside
// a request; only short context snippets are stored to avoid persisting prompts.
type SensitiveReplacementMatch struct {
	Word            string
	Replacement     string
	Count           int
	OriginalContext string
	ReplacedContext string
}

type SensitiveReplacementTextResult struct {
	Changed bool
	Content string
	Matches []SensitiveReplacementMatch
}

type compiledSensitiveRule struct {
	Word          string
	Replacement   string
	LowerRunes    []rune
	NeedsBoundary bool
}

type sensitiveReplacementAccumulator struct {
	rules   []compiledSensitiveRule
	matches []SensitiveReplacementMatch
	index   map[string]int
}

// ParseSensitiveReplacementRules parses replacement rule lines:
// "word=>replacement" or "word" (defaulting to XX). Duplicate words are
// resolved by keeping the later rule, matching the admin page behavior.
func ParseSensitiveReplacementRules(lines []string) []SensitiveReplacementRule {
	rules := make([]SensitiveReplacementRule, 0, len(lines))
	indexByWord := make(map[string]int, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		word, replacement, hasReplacement := strings.Cut(line, "=>")
		word = strings.TrimSpace(word)
		replacement = strings.TrimSpace(replacement)
		if word == "" {
			continue
		}
		if !hasReplacement || replacement == "" {
			replacement = DefaultSensitiveReplacement
		}
		normalized := strings.ToLower(word)
		rule := SensitiveReplacementRule{
			Word:        word,
			Replacement: replacement,
		}
		if index, ok := indexByWord[normalized]; ok {
			rules[index] = rule
		} else {
			indexByWord[normalized] = len(rules)
			rules = append(rules, rule)
		}
	}
	return rules
}

func compileSensitiveReplacementRules(lines []string) []compiledSensitiveRule {
	parsedRules := ParseSensitiveReplacementRules(lines)
	rules := make([]compiledSensitiveRule, 0, len(parsedRules))
	for _, rule := range parsedRules {
		lowerWord := strings.ToLower(rule.Word)
		runes := []rune(lowerWord)
		if len(runes) == 0 {
			continue
		}
		rules = append(rules, compiledSensitiveRule{
			Word:          rule.Word,
			Replacement:   rule.Replacement,
			LowerRunes:    runes,
			NeedsBoundary: sensitiveRuleNeedsASCIIBoundary(rule.Word),
		})
	}
	return rules
}

func newSensitiveReplacementAccumulator() *sensitiveReplacementAccumulator {
	return &sensitiveReplacementAccumulator{
		rules: compileSensitiveReplacementRules(setting.SensitiveReplacementRules),
		index: make(map[string]int),
	}
}

func (a *sensitiveReplacementAccumulator) replace(value string) (string, bool) {
	result := replaceSensitiveTextWithRules(value, a.rules, false)
	if !result.Changed {
		return value, false
	}
	a.merge(result.Matches)
	return result.Content, true
}

func (a *sensitiveReplacementAccumulator) merge(matches []SensitiveReplacementMatch) {
	for _, match := range matches {
		key := strings.ToLower(match.Word) + "\x00" + match.Replacement
		if index, ok := a.index[key]; ok {
			a.matches[index].Count += match.Count
			continue
		}
		a.index[key] = len(a.matches)
		a.matches = append(a.matches, match)
	}
}

func ReplaceSensitiveText(text string) SensitiveReplacementTextResult {
	return replaceSensitiveTextWithRules(text, compileSensitiveReplacementRules(setting.SensitiveReplacementRules), false)
}

func replaceSensitiveTextWithRules(text string, rules []compiledSensitiveRule, returnImmediately bool) SensitiveReplacementTextResult {
	result := SensitiveReplacementTextResult{Content: text}
	if text == "" || len(rules) == 0 {
		return result
	}

	originalRunes := []rune(text)
	lowerRunes := []rune(strings.ToLower(text))
	var builder strings.Builder
	builder.Grow(len(text))
	matchIndex := make(map[string]int)

	for pos := 0; pos < len(originalRunes); {
		ruleIndex, matchLen := findSensitiveRuleAt(lowerRunes, rules, pos)
		if ruleIndex == -1 {
			builder.WriteRune(originalRunes[pos])
			pos++
			continue
		}

		rule := rules[ruleIndex]
		builder.WriteString(rule.Replacement)
		result.Changed = true
		result.Matches = appendSensitiveMatch(result.Matches, matchIndex, rule, originalRunes, pos, pos+matchLen)
		pos += matchLen
		if returnImmediately {
			builder.WriteString(string(originalRunes[pos:]))
			break
		}
	}

	if result.Changed {
		result.Content = builder.String()
	}
	return result
}

func findSensitiveRuleAt(text []rune, rules []compiledSensitiveRule, pos int) (int, int) {
	bestIndex := -1
	bestLen := 0
	for i, rule := range rules {
		wordLen := len(rule.LowerRunes)
		if wordLen <= bestLen || pos+wordLen > len(text) {
			continue
		}
		if !runesEqual(text[pos:pos+wordLen], rule.LowerRunes) {
			continue
		}
		if !sensitiveBoundaryOK(text, pos, pos+wordLen, rule.NeedsBoundary) {
			continue
		}
		bestIndex = i
		bestLen = wordLen
	}
	return bestIndex, bestLen
}

func runesEqual(a []rune, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sensitiveRuleNeedsASCIIBoundary(word string) bool {
	for _, r := range word {
		if isASCIIWordRune(r) {
			return true
		}
	}
	return false
}

func sensitiveBoundaryOK(text []rune, start int, end int, needsBoundary bool) bool {
	if !needsBoundary {
		return true
	}
	if start > 0 && isASCIIWordRune(text[start-1]) {
		return false
	}
	if end < len(text) && isASCIIWordRune(text[end]) {
		return false
	}
	return true
}

func isASCIIWordRune(r rune) bool {
	return r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func appendSensitiveMatch(matches []SensitiveReplacementMatch, index map[string]int, rule compiledSensitiveRule, originalRunes []rune, start int, end int) []SensitiveReplacementMatch {
	key := strings.ToLower(rule.Word) + "\x00" + rule.Replacement
	if matchIndex, ok := index[key]; ok {
		matches[matchIndex].Count++
		return matches
	}
	index[key] = len(matches)
	return append(matches, SensitiveReplacementMatch{
		Word:            rule.Word,
		Replacement:     rule.Replacement,
		Count:           1,
		OriginalContext: sensitiveOriginalContext(originalRunes, start, end),
		ReplacedContext: sensitiveReplacedContext(originalRunes, start, end, rule.Replacement),
	})
}

func sensitiveOriginalContext(originalRunes []rune, start int, end int) string {
	contextStart := max(start-sensitiveContextRuneRadius, 0)
	contextEnd := min(end+sensitiveContextRuneRadius, len(originalRunes))
	return string(originalRunes[contextStart:contextEnd])
}

func sensitiveReplacedContext(originalRunes []rune, start int, end int, replacement string) string {
	contextStart := max(start-sensitiveContextRuneRadius, 0)
	contextEnd := min(end+sensitiveContextRuneRadius, len(originalRunes))
	return string(originalRunes[contextStart:start]) + replacement + string(originalRunes[end:contextEnd])
}

func SensitiveReplacementWords(matches []SensitiveReplacementMatch) []string {
	words := make([]string, 0, len(matches))
	for _, match := range matches {
		words = append(words, match.Word)
	}
	return words
}

// ApplySensitiveReplacementsToRequest mutates parsed request DTO text fields
// before billing and relay forwarding. It deliberately avoids OCR/ASR and only
// touches already parsed textual fields.
func ApplySensitiveReplacementsToRequest(request dto.Request) (SensitiveReplacementTextResult, error) {
	acc := newSensitiveReplacementAccumulator()
	if len(acc.rules) == 0 || request == nil {
		return SensitiveReplacementTextResult{}, nil
	}

	var err error
	switch r := request.(type) {
	case *dto.GeneralOpenAIRequest:
		replaceGeneralOpenAIRequest(r, acc)
	case *dto.OpenAIResponsesRequest:
		err = replaceOpenAIResponsesRequest(r, acc)
	case *dto.OpenAIResponsesCompactionRequest:
		err = replaceOpenAIResponsesCompactionRequest(r, acc)
	case *dto.ClaudeRequest:
		replaceClaudeRequest(r, acc)
	case *dto.GeminiChatRequest:
		replaceGeminiChatRequest(r, acc)
	case *dto.GeminiEmbeddingRequest:
		replaceGeminiEmbeddingRequest(r, acc)
	case *dto.GeminiBatchEmbeddingRequest:
		for _, embeddingRequest := range r.Requests {
			replaceGeminiEmbeddingRequest(embeddingRequest, acc)
		}
	case *dto.EmbeddingRequest:
		r.Input, _ = replaceTextInAny(r.Input, acc, true)
	case *dto.RerankRequest:
		replaceStringField(&r.Query, acc)
		for i := range r.Documents {
			r.Documents[i], _ = replaceTextInAny(r.Documents[i], acc, true)
		}
	case *dto.AudioRequest:
		replaceStringField(&r.Input, acc)
		replaceStringField(&r.Instructions, acc)
		r.RefText, _, err = replaceRawJSONString(r.RefText, acc)
	case *dto.ImageRequest:
		replaceStringField(&r.Prompt, acc)
	}
	if err != nil {
		return SensitiveReplacementTextResult{}, err
	}
	return SensitiveReplacementTextResult{
		Changed: len(acc.matches) > 0,
		Matches: acc.matches,
	}, nil
}

func replaceGeneralOpenAIRequest(r *dto.GeneralOpenAIRequest, acc *sensitiveReplacementAccumulator) {
	r.Prompt, _ = replaceTextInAny(r.Prompt, acc, true)
	r.Prefix, _ = replaceTextInAny(r.Prefix, acc, true)
	r.Suffix, _ = replaceTextInAny(r.Suffix, acc, true)
	r.Input, _ = replaceTextInAny(r.Input, acc, true)
	replaceStringField(&r.Instruction, acc)
	for i := range r.Messages {
		replaceOpenAIMessageContent(&r.Messages[i], acc)
	}
	for i := range r.Tools {
		replaceStringField(&r.Tools[i].Function.Description, acc)
	}
}

func replaceOpenAIMessageContent(message *dto.Message, acc *sensitiveReplacementAccumulator) {
	if message == nil || message.Content == nil {
		return
	}
	switch content := message.Content.(type) {
	case string:
		if next, changed := acc.replace(content); changed {
			message.SetStringContent(next)
		}
	case []any:
		if replaceMediaContentItems(content, acc) {
			message.Content = content
		}
	case []dto.MediaContent:
		changed := false
		for i := range content {
			if content[i].Type == dto.ContentTypeText {
				if replaceStringField(&content[i].Text, acc) {
					changed = true
				}
			}
		}
		if changed {
			message.SetMediaContent(content)
		}
	}
}

func replaceMediaContentItems(items []any, acc *sensitiveReplacementAccumulator) bool {
	changed := false
	for _, item := range items {
		contentMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		contentType, _ := contentMap["type"].(string)
		if contentType == dto.ContentTypeText {
			if text, ok := contentMap["text"].(string); ok {
				if next, didReplace := acc.replace(text); didReplace {
					contentMap["text"] = next
					changed = true
				}
			}
		}
	}
	return changed
}

func replaceOpenAIResponsesRequest(r *dto.OpenAIResponsesRequest, acc *sensitiveReplacementAccumulator) error {
	var err error
	r.Input, _, err = replaceResponsesRawInput(r.Input, acc)
	if err != nil {
		return err
	}
	r.Instructions, _, err = replaceRawJSONString(r.Instructions, acc)
	if err != nil {
		return err
	}
	r.Prompt, _, err = replaceRawJSONString(r.Prompt, acc)
	return err
}

func replaceOpenAIResponsesCompactionRequest(r *dto.OpenAIResponsesCompactionRequest, acc *sensitiveReplacementAccumulator) error {
	var err error
	r.Input, _, err = replaceResponsesRawInput(r.Input, acc)
	if err != nil {
		return err
	}
	r.Instructions, _, err = replaceRawJSONString(r.Instructions, acc)
	return err
}

func replaceResponsesRawInput(raw []byte, acc *sensitiveReplacementAccumulator) ([]byte, bool, error) {
	if len(raw) == 0 {
		return raw, false, nil
	}
	if common.GetJsonType(raw) == "string" {
		return replaceRawJSONString(raw, acc)
	}
	var value any
	if err := common.Unmarshal(raw, &value); err != nil {
		return nil, false, err
	}
	if !replaceResponsesInputValue(value, acc) {
		return raw, false, nil
	}
	data, err := common.Marshal(value)
	return data, err == nil, err
}

func replaceResponsesInputValue(value any, acc *sensitiveReplacementAccumulator) bool {
	changed := false
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			if replaceResponsesInputValue(item, acc) {
				changed = true
			}
		}
	case map[string]any:
		contentType, _ := v["type"].(string)
		if text, ok := v["text"].(string); ok && (contentType == "" || strings.Contains(contentType, "text")) {
			if next, didReplace := acc.replace(text); didReplace {
				v["text"] = next
				changed = true
			}
		}
		if content, ok := v["content"].(string); ok {
			if next, didReplace := acc.replace(content); didReplace {
				v["content"] = next
				changed = true
			}
		}
		if content, ok := v["content"].([]any); ok {
			for _, item := range content {
				if replaceResponsesInputValue(item, acc) {
					changed = true
				}
			}
		}
	}
	return changed
}

func replaceClaudeRequest(r *dto.ClaudeRequest, acc *sensitiveReplacementAccumulator) {
	replaceStringField(&r.Prompt, acc)
	if r.IsStringSystem() {
		system := r.GetStringSystem()
		if next, changed := acc.replace(system); changed {
			r.SetStringSystem(next)
		}
	} else {
		system := r.ParseSystem()
		if replaceClaudeMediaMessages(system, acc) {
			r.System = system
		}
	}

	for i := range r.Messages {
		replaceClaudeMessageContent(&r.Messages[i], acc)
	}
}

func replaceClaudeMessageContent(message *dto.ClaudeMessage, acc *sensitiveReplacementAccumulator) {
	if message == nil || message.Content == nil {
		return
	}
	switch content := message.Content.(type) {
	case string:
		if next, changed := acc.replace(content); changed {
			message.SetStringContent(next)
		}
	case []any:
		if replaceClaudeContentItems(content, acc) {
			message.SetContent(content)
		}
	case []dto.ClaudeMediaMessage:
		if replaceClaudeMediaMessages(content, acc) {
			message.SetContent(content)
		}
	}
}

func replaceClaudeContentItems(items []any, acc *sensitiveReplacementAccumulator) bool {
	changed := false
	for _, item := range items {
		contentMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		contentType, _ := contentMap["type"].(string)
		if contentType == dto.ContentTypeText {
			if text, ok := contentMap["text"].(string); ok {
				if next, didReplace := acc.replace(text); didReplace {
					contentMap["text"] = next
					changed = true
				}
			}
			continue
		}
		if contentType == "tool_result" {
			if content, ok := contentMap["content"].(string); ok {
				if next, didReplace := acc.replace(content); didReplace {
					contentMap["content"] = next
					changed = true
				}
			}
		}
	}
	return changed
}

func replaceClaudeMediaMessages(items []dto.ClaudeMediaMessage, acc *sensitiveReplacementAccumulator) bool {
	changed := false
	for i := range items {
		if items[i].Text != nil {
			text := items[i].GetText()
			if next, didReplace := acc.replace(text); didReplace {
				items[i].SetText(next)
				changed = true
			}
		}
	}
	return changed
}

func replaceGeminiChatRequest(r *dto.GeminiChatRequest, acc *sensitiveReplacementAccumulator) {
	if r == nil {
		return
	}
	for i := range r.Contents {
		replaceGeminiContent(&r.Contents[i], acc)
	}
	for i := range r.Requests {
		replaceGeminiChatRequest(&r.Requests[i], acc)
	}
	if r.SystemInstructions != nil {
		replaceGeminiContent(r.SystemInstructions, acc)
	}
}

func replaceGeminiEmbeddingRequest(r *dto.GeminiEmbeddingRequest, acc *sensitiveReplacementAccumulator) {
	if r == nil {
		return
	}
	replaceGeminiContent(&r.Content, acc)
	replaceStringField(&r.Title, acc)
}

func replaceGeminiContent(content *dto.GeminiChatContent, acc *sensitiveReplacementAccumulator) {
	if content == nil {
		return
	}
	for i := range content.Parts {
		replaceStringField(&content.Parts[i].Text, acc)
	}
}

func replaceTextInAny(value any, acc *sensitiveReplacementAccumulator, recurseMaps bool) (any, bool) {
	if value == nil {
		return value, false
	}
	changed := false
	switch v := value.(type) {
	case string:
		next, didReplace := acc.replace(v)
		return next, didReplace
	case []any:
		for i := range v {
			next, didReplace := replaceTextInAny(v[i], acc, recurseMaps)
			if didReplace {
				v[i] = next
				changed = true
			}
		}
		return v, changed
	case []string:
		for i := range v {
			if replaceStringField(&v[i], acc) {
				changed = true
			}
		}
		return v, changed
	case map[string]any:
		if !recurseMaps {
			return value, false
		}
		for key, item := range v {
			next, didReplace := replaceTextInAny(item, acc, true)
			if didReplace {
				v[key] = next
				changed = true
			}
		}
		return v, changed
	default:
		return value, false
	}
}

func replaceStringField(value *string, acc *sensitiveReplacementAccumulator) bool {
	if value == nil || *value == "" {
		return false
	}
	next, changed := acc.replace(*value)
	if changed {
		*value = next
	}
	return changed
}

func replaceRawJSONString(raw []byte, acc *sensitiveReplacementAccumulator) ([]byte, bool, error) {
	if len(raw) == 0 || common.GetJsonType(raw) != "string" {
		return raw, false, nil
	}
	var value string
	if err := common.Unmarshal(raw, &value); err != nil {
		return nil, false, err
	}
	next, changed := acc.replace(value)
	if !changed {
		return raw, false, nil
	}
	data, err := common.Marshal(next)
	return data, err == nil, err
}

// RewriteSensitiveReplacementRequestBody updates the reusable body cache so
// pass-through mode sees the same replacements as typed relay DTOs.
func RewriteSensitiveReplacementRequestBody(c *gin.Context) error {
	if c == nil || c.Request == nil {
		return nil
	}
	contentType := c.Request.Header.Get("Content-Type")
	switch {
	case strings.Contains(contentType, gin.MIMEMultipartPOSTForm):
		return rewriteMultipartSensitiveReplacementBody(c)
	case strings.Contains(contentType, gin.MIMEPOSTForm):
		return rewriteFormSensitiveReplacementBody(c)
	case strings.HasPrefix(contentType, "application/json"):
		return rewriteJSONSensitiveReplacementBody(c)
	default:
		return nil
	}
}

func rewriteJSONSensitiveReplacementBody(c *gin.Context) error {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return err
	}
	body, err := storage.Bytes()
	if err != nil {
		return err
	}
	var value any
	if err := common.Unmarshal(body, &value); err != nil {
		return err
	}
	acc := newSensitiveReplacementAccumulator()
	if !replaceJSONStringValues(value, acc) {
		return nil
	}
	nextBody, err := common.Marshal(value)
	if err != nil {
		return err
	}
	return resetReusableBody(c, nextBody)
}

func replaceJSONStringValues(value any, acc *sensitiveReplacementAccumulator) bool {
	changed := false
	switch v := value.(type) {
	case string:
		return false
	case []any:
		for i := range v {
			if text, ok := v[i].(string); ok {
				if next, didReplace := acc.replace(text); didReplace {
					v[i] = next
					changed = true
				}
				continue
			}
			if replaceJSONStringValues(v[i], acc) {
				changed = true
			}
		}
	case map[string]any:
		for key, item := range v {
			if text, ok := item.(string); ok {
				if next, didReplace := acc.replace(text); didReplace {
					v[key] = next
					changed = true
				}
				continue
			}
			if replaceJSONStringValues(item, acc) {
				changed = true
			}
		}
	}
	return changed
}

func rewriteFormSensitiveReplacementBody(c *gin.Context) error {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return err
	}
	body, err := storage.Bytes()
	if err != nil {
		return err
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return err
	}
	acc := newSensitiveReplacementAccumulator()
	changed := false
	for key, items := range values {
		for i := range items {
			if next, didReplace := acc.replace(items[i]); didReplace {
				items[i] = next
				changed = true
			}
		}
		values[key] = items
	}
	if !changed {
		return nil
	}
	c.Request.PostForm = values
	return resetReusableBody(c, []byte(values.Encode()))
}

func rewriteMultipartSensitiveReplacementBody(c *gin.Context) error {
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return err
	}
	acc := newSensitiveReplacementAccumulator()
	changed := false
	for key, values := range form.Value {
		for i := range values {
			if next, didReplace := acc.replace(values[i]); didReplace {
				values[i] = next
				changed = true
			}
		}
		form.Value[key] = values
	}
	if !changed {
		return nil
	}

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	for key, values := range form.Value {
		for _, value := range values {
			if err := writer.WriteField(key, value); err != nil {
				return err
			}
		}
	}
	for fieldName, files := range form.File {
		for _, fileHeader := range files {
			if err := copyMultipartFile(writer, fieldName, fileHeader); err != nil {
				return err
			}
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}

	contentType := writer.FormDataContentType()
	c.Request.Header.Set("Content-Type", contentType)
	c.Set("_original_multipart_ct", contentType)
	c.Request.MultipartForm = form
	c.Request.PostForm = url.Values(form.Value)
	return resetReusableBody(c, requestBody.Bytes())
}

func copyMultipartFile(writer *multipart.Writer, fieldName string, fileHeader *multipart.FileHeader) error {
	file, err := fileHeader.Open()
	if err != nil {
		return err
	}
	defer file.Close()

	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, fileHeader.Filename))
	if contentType := fileHeader.Header.Get("Content-Type"); contentType != "" {
		partHeader.Set("Content-Type", contentType)
	}
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return err
	}
	_, err = io.Copy(part, file)
	return err
}

func resetReusableBody(c *gin.Context, body []byte) error {
	if storage, exists := c.Get(common.KeyBodyStorage); exists && storage != nil {
		if bs, ok := storage.(common.BodyStorage); ok {
			_ = bs.Close()
		}
	}
	storage, err := common.CreateBodyStorage(body)
	if err != nil {
		return err
	}
	c.Set(common.KeyBodyStorage, storage)
	c.Set(common.KeyRequestBody, body)
	if _, err := storage.Seek(0, io.SeekStart); err != nil {
		return err
	}
	c.Request.Body = io.NopCloser(storage)
	c.Request.ContentLength = int64(len(body))
	c.Request.Header.Set("Content-Length", strconv.Itoa(len(body)))
	return nil
}

func RecordSensitiveReplacementLogs(c *gin.Context, modelName string, matches []SensitiveReplacementMatch) {
	if c == nil || len(matches) == 0 {
		return
	}
	now := common.GetTimestamp()
	requestPath := ""
	if c.Request != nil && c.Request.URL != nil {
		requestPath = c.Request.URL.Path
	}
	logs := make([]*model.SensitiveReplacementLog, 0, len(matches))
	for _, match := range matches {
		logs = append(logs, &model.SensitiveReplacementLog{
			CreatedAt:       now,
			UserId:          common.GetContextKeyInt(c, constant.ContextKeyUserId),
			Username:        common.GetContextKeyString(c, constant.ContextKeyUserName),
			TokenId:         common.GetContextKeyInt(c, constant.ContextKeyTokenId),
			TokenName:       c.GetString("token_name"),
			ModelName:       modelName,
			RequestPath:     requestPath,
			RequestId:       c.GetString(common.RequestIdKey),
			MatchedWord:     match.Word,
			Replacement:     match.Replacement,
			Count:           match.Count,
			OriginalContext: match.OriginalContext,
			ReplacedContext: match.ReplacedContext,
		})
	}
	if err := model.RecordSensitiveReplacementLogs(logs); err != nil {
		logger.LogError(c, "failed to record sensitive replacement logs: "+err.Error())
	}
}
