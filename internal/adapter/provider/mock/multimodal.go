package mock

import (
	"context"
	"encoding/base64"
	"fmt"
	"hash/fnv"
	"html"
	"strings"
	"time"

	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/tokencount"
)

var _ domain.MultimodalClient = (*Client)(nil)

// Vision fabricates an image-understanding answer so the console's 图片理解
// experience works without real vendor credentials.
func (c *Client) Vision(ctx context.Context, target domain.CallTarget, prompt, imageDataURL string) (domain.ChatResponse, error) {
	format, size := "unknown", len(imageDataURL)
	if strings.HasPrefix(imageDataURL, "data:image/") {
		if end := strings.Index(imageDataURL, ";"); end > 0 {
			format = imageDataURL[len("data:image/"):end]
		}
		if i := strings.Index(imageDataURL, ","); i > 0 {
			size = len(imageDataURL[i+1:]) * 3 / 4
		}
	}
	reply := fmt.Sprintf("[mock:%s/%s] 已收到一张 %s 图片（约 %.1f KB）。针对你的问题「%s」：这是 Mock 模型返回的演示结果，接入真实厂商后将返回图片内容分析。",
		target.Provider.Code, target.Model.ModelKey, format, float64(size)/1024, prompt)
	pt, ct := tokencount.Estimate(prompt)+258, tokencount.Estimate(reply)
	return domain.ChatResponse{
		ID: "mock-vision-" + fmt.Sprint(time.Now().UnixNano()), Model: target.Model.ModelKey, Created: time.Now().Unix(),
		Choices: []domain.ChatChoice{{Message: domain.ChatMessage{Role: "assistant", Content: reply}, FinishReason: "stop"}},
		Usage:   domain.Usage{PromptTokens: pt, CompletionTokens: ct, TotalTokens: pt + ct},
	}, nil
}

// GenerateImage returns a deterministic abstract SVG derived from the prompt.
func (c *Client) GenerateImage(ctx context.Context, target domain.CallTarget, prompt, size string) (domain.ImageResult, error) {
	h := fnv.New32a()
	_, _ = h.Write([]byte(prompt))
	seed := h.Sum32()
	hue := int(seed % 360)
	hue2 := (hue + 60 + int(seed>>8)%80) % 360
	caption := html.EscapeString(truncate(prompt, 28))
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="512" height="512" viewBox="0 0 512 512">
<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="hsl(%d,70%%,55%%)"/><stop offset="1" stop-color="hsl(%d,70%%,40%%)"/></linearGradient></defs>
<rect width="512" height="512" fill="url(#g)"/>
<circle cx="%d" cy="%d" r="120" fill="hsl(%d,80%%,75%%)" fill-opacity=".55"/>
<circle cx="%d" cy="%d" r="80" fill="hsl(%d,80%%,85%%)" fill-opacity=".5"/>
<rect x="0" y="440" width="512" height="72" fill="#000" fill-opacity=".35"/>
<text x="24" y="482" font-family="sans-serif" font-size="20" fill="#fff">%s</text>
<text x="488" y="30" font-family="monospace" font-size="12" fill="#fff" text-anchor="end">mock:%s</text></svg>`,
		hue, hue2, 120+int(seed%260), 140+int(seed>>4%220), hue2, 300+int(seed>>6%160), 320+int(seed>>3%140), hue,
		caption, html.EscapeString(target.Model.ModelKey))
	url := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
	return domain.ImageResult{Model: target.Model.ModelKey, Images: []string{url}}, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
