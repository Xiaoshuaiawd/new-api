package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ModerationRequest OpenAI Moderation API 请求结构
type ModerationRequest struct {
	Input string `json:"input"`
}

// ModerationCategories 审核类别
type ModerationCategories struct {
	Sexual                 bool `json:"sexual"`
	Hate                   bool `json:"hate"`
	Harassment             bool `json:"harassment"`
	SelfHarm               bool `json:"self-harm"`
	SexualMinors           bool `json:"sexual/minors"`
	HateThreatening        bool `json:"hate/threatening"`
	ViolenceGraphic        bool `json:"violence/graphic"`
	SelfHarmIntent         bool `json:"self-harm/intent"`
	SelfHarmInstructions   bool `json:"self-harm/instructions"`
	HarassmentThreatening  bool `json:"harassment/threatening"`
	Violence               bool `json:"violence"`
}

// ModerationCategoryScores 审核类别分数
type ModerationCategoryScores struct {
	Sexual                 float64 `json:"sexual"`
	Hate                   float64 `json:"hate"`
	Harassment             float64 `json:"harassment"`
	SelfHarm               float64 `json:"self-harm"`
	SexualMinors           float64 `json:"sexual/minors"`
	HateThreatening        float64 `json:"hate/threatening"`
	ViolenceGraphic        float64 `json:"violence/graphic"`
	SelfHarmIntent         float64 `json:"self-harm/intent"`
	SelfHarmInstructions   float64 `json:"self-harm/instructions"`
	HarassmentThreatening  float64 `json:"harassment/threatening"`
	Violence               float64 `json:"violence"`
}

// ModerationResult 单个审核结果
type ModerationResult struct {
	Flagged        bool                     `json:"flagged"`
	Categories     ModerationCategories     `json:"categories"`
	CategoryScores ModerationCategoryScores `json:"category_scores"`
}

// ModerationResponse OpenAI Moderation API 响应结构
type ModerationResponse struct {
	ID      string             `json:"id"`
	Model   string             `json:"model"`
	Results []ModerationResult `json:"results"`
}

// ModerationError 审核错误信息
type ModerationError struct {
	Flagged       bool
	ViolatedRules []string
	Message       string
}

// CheckContentModeration 检查内容是否违规
// apiKey: OpenAI API密钥
// baseURL: API基础URL（可选，默认使用OpenAI官方地址）
// content: 要审核的内容
func CheckContentModeration(apiKey string, baseURL string, content string) (*ModerationError, error) {
	if content == "" {
		return nil, nil
	}

	// 设置默认的API地址
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	// 构建请求
	reqBody := ModerationRequest{
		Input: content,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal moderation request failed: %w", err)
	}

	url := baseURL + "/v1/moderations"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create moderation request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	// 发送请求
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("moderation request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read moderation response failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("moderation API returned status %d: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var moderationResp ModerationResponse
	if err := json.Unmarshal(body, &moderationResp); err != nil {
		return nil, fmt.Errorf("unmarshal moderation response failed: %w", err)
	}

	// 检查结果
	if len(moderationResp.Results) == 0 {
		return nil, errors.New("moderation response contains no results")
	}

	result := moderationResp.Results[0]
	if !result.Flagged {
		return nil, nil
	}

	// 收集违规类别
	violatedRules := collectViolatedCategories(result.Categories)

	return &ModerationError{
		Flagged:       true,
		ViolatedRules: violatedRules,
		Message:       buildModerationErrorMessage(violatedRules),
	}, nil
}

// collectViolatedCategories 收集违规的类别
func collectViolatedCategories(categories ModerationCategories) []string {
	var violated []string

	if categories.Sexual {
		violated = append(violated, "sexual")
	}
	if categories.Hate {
		violated = append(violated, "hate")
	}
	if categories.Harassment {
		violated = append(violated, "harassment")
	}
	if categories.SelfHarm {
		violated = append(violated, "self-harm")
	}
	if categories.SexualMinors {
		violated = append(violated, "sexual/minors")
	}
	if categories.HateThreatening {
		violated = append(violated, "hate/threatening")
	}
	if categories.ViolenceGraphic {
		violated = append(violated, "violence/graphic")
	}
	if categories.SelfHarmIntent {
		violated = append(violated, "self-harm/intent")
	}
	if categories.SelfHarmInstructions {
		violated = append(violated, "self-harm/instructions")
	}
	if categories.HarassmentThreatening {
		violated = append(violated, "harassment/threatening")
	}
	if categories.Violence {
		violated = append(violated, "violence")
	}

	return violated
}

// buildModerationErrorMessage 构建审核错误消息
func buildModerationErrorMessage(violatedRules []string) string {
	if len(violatedRules) == 0 {
		return "当前内容已被审核拒绝"
	}
	return fmt.Sprintf("当前内容已被审核拒绝，违反规则：%s", strings.Join(violatedRules, ", "))
}

// ExtractTextFromMessages 从消息中提取文本内容用于审核
func ExtractTextFromMessages(combineText string) string {
	if combineText == "" {
		return ""
	}
	// 限制审核文本长度，避免请求过大
	maxLen := 32000
	if len(combineText) > maxLen {
		combineText = combineText[:maxLen]
	}
	return combineText
}
