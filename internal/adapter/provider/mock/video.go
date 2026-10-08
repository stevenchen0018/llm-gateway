package mock

import (
	"context"
	"encoding/base64"
	"fmt"
	"hash/fnv"
	"html"
	"io"
	"strings"
	"time"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

var _ domain.VideoClient = (*Client)(nil)

// mockVideoDuration is how long a mock job "renders" before completing, so
// the console's queued → running → done flow is exercised realistically.
const mockVideoDuration = 8 * time.Second

type mockVideoJob struct {
	task   domain.VideoTask
	prompt string
}

// SubmitVideo starts a fake render job. Prompts containing FailKeyword fail,
// to exercise the error path.
func (c *Client) SubmitVideo(ctx context.Context, target domain.CallTarget, req domain.VideoRequest) (domain.VideoTask, error) {
	id := fmt.Sprintf("mockvid-%d", time.Now().UnixNano())
	t := domain.VideoTask{ID: id, Model: target.Model.ModelKey, Status: domain.VideoQueued, Size: req.Size, Seconds: req.Seconds, CreatedAt: time.Now()}
	c.videos.Store(id, &mockVideoJob{task: t, prompt: req.Prompt})
	c.pruneVideos()
	return t, nil
}

func (c *Client) GetVideo(ctx context.Context, target domain.CallTarget, taskID string) (domain.VideoTask, error) {
	v, ok := c.videos.Load(taskID)
	if !ok {
		return domain.VideoTask{}, fmt.Errorf("%w: video task %s", domain.ErrNotFound, taskID)
	}
	job := v.(*mockVideoJob)
	t := job.task
	elapsed := time.Since(t.CreatedAt)
	switch {
	case elapsed < time.Second:
		t.Status, t.Progress = domain.VideoQueued, 0
	case strings.Contains(job.prompt, FailKeyword) && elapsed > 3*time.Second:
		t.Status, t.Error = domain.VideoFailed, "mock vendor: render failed (forced)"
	case elapsed < mockVideoDuration:
		t.Status, t.Progress = domain.VideoRunning, int(elapsed*100/mockVideoDuration)
	default:
		t.Status, t.Progress = domain.VideoSucceeded, 100
		t.VideoURL = renderMockVideo(job.prompt, target.Model.ModelKey, t.Size, t.Seconds)
	}
	return t, nil
}

func (c *Client) VideoContent(ctx context.Context, target domain.CallTarget, taskID string) (io.ReadCloser, string, error) {
	return nil, "", fmt.Errorf("%w: mock videos are returned inline", domain.ErrInvalidArgument)
}

// pruneVideos forgets jobs older than an hour.
func (c *Client) pruneVideos() {
	c.videos.Range(func(k, v any) bool {
		if time.Since(v.(*mockVideoJob).task.CreatedAt) > time.Hour {
			c.videos.Delete(k)
		}
		return true
	})
}

// renderMockVideo returns an animated SVG "clip" (plays in an <img>) whose
// colours and motion derive from the prompt — a stand-in for a real video.
func renderMockVideo(prompt, model, size string, seconds int) string {
	w, h := 640, 360
	if size != "" {
		var sw, sh int
		if _, err := fmt.Sscanf(size, "%dx%d", &sw, &sh); err == nil && sw > 0 && sh > 0 {
			h = w * sh / sw
		}
	}
	if seconds <= 0 {
		seconds = 5
	}
	f := fnv.New32a()
	_, _ = f.Write([]byte(prompt))
	seed := f.Sum32()
	hue := int(seed % 360)
	hue2 := (hue + 50 + int(seed>>8)%90) % 360
	caption := html.EscapeString(truncate(prompt, 30))
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%[1]d" height="%[2]d" viewBox="0 0 %[1]d %[2]d">
<defs><linearGradient id="bg" x1="0" y1="0" x2="1" y2="1">
<stop offset="0" stop-color="hsl(%[3]d,65%%,45%%)"><animate attributeName="stop-color" values="hsl(%[3]d,65%%,45%%);hsl(%[4]d,65%%,40%%);hsl(%[3]d,65%%,45%%)" dur="%[5]ds" repeatCount="indefinite"/></stop>
<stop offset="1" stop-color="hsl(%[4]d,70%%,25%%)"/></linearGradient></defs>
<rect width="100%%" height="100%%" fill="url(#bg)"/>
<circle r="%[6]d" fill="hsl(%[4]d,85%%,75%%)" fill-opacity=".55"><animateMotion dur="%[5]ds" repeatCount="indefinite" path="M%[7]d,%[8]d C%[1]d,0 0,%[2]d %[9]d,%[10]d Z"/></circle>
<circle r="%[11]d" fill="hsl(%[3]d,90%%,85%%)" fill-opacity=".45"><animateMotion dur="%[12]ds" repeatCount="indefinite" path="M%[9]d,%[8]d C0,0 %[1]d,%[2]d %[7]d,%[10]d Z"/></circle>
<rect y="%[13]d" width="100%%" height="56" fill="#000" fill-opacity=".4"/>
<text x="20" y="%[14]d" font-family="sans-serif" font-size="18" fill="#fff">%[15]s</text>
<rect x="20" y="%[16]d" width="0" height="3" fill="#fff" fill-opacity=".9"><animate attributeName="width" from="0" to="%[17]d" dur="%[5]ds" repeatCount="indefinite"/></rect>
<text x="%[18]d" y="24" font-family="monospace" font-size="12" fill="#fff" text-anchor="end">mock:%[19]s · %[5]ds</text></svg>`,
		w, h, hue, hue2, seconds, 40+int(seed%40), w/5, h/2, w*4/5, h/3, 30+int(seed>>5%30), seconds+2,
		h-56, h-28, caption, h-12, w-40, w-12, html.EscapeString(model))
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}
