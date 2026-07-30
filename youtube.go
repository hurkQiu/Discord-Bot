package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Song 代表一首已經解析完成、可直接交給 ffmpeg 播放的歌曲。
type Song struct {
	Title      string
	WebpageURL string
	StreamURL  string // ffmpeg 可直接讀取的音訊串流網址
	Duration   time.Duration
	Requester  string
}

// ytDlpEntry 對應 yt-dlp -j 輸出 JSON 中我們需要的欄位。
type ytDlpEntry struct {
	Title      string  `json:"title"`
	WebpageURL string  `json:"webpage_url"`
	Duration   float64 `json:"duration"`
	URL        string  `json:"url"`
}

// resolveSong 依查詢字串（可以是關鍵字或 YouTube 網址）呼叫 yt-dlp，
// 回傳可直接播放的音訊串流資訊。
func resolveSong(ctx context.Context, ytDlpPath, query, requester string) (*Song, error) {
	target := query
	if !isURL(query) {
		target = "ytsearch1:" + query
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, ytDlpPath,
		"-f", "bestaudio/best",
		"--no-playlist",
		"--no-warnings",
		"-j",
		target,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("yt-dlp 搜尋失敗: %s", msg)
	}

	// -j 每行輸出一筆結果，搜尋時只取第一行
	firstLine := strings.SplitN(strings.TrimSpace(stdout.String()), "\n", 2)[0]
	if firstLine == "" {
		return nil, fmt.Errorf("找不到符合的 YouTube 影片")
	}

	var entry ytDlpEntry
	if err := json.Unmarshal([]byte(firstLine), &entry); err != nil {
		return nil, fmt.Errorf("解析 yt-dlp 輸出失敗: %w", err)
	}
	if entry.URL == "" {
		return nil, fmt.Errorf("找不到可播放的音訊串流")
	}

	return &Song{
		Title:      entry.Title,
		WebpageURL: entry.WebpageURL,
		StreamURL:  entry.URL,
		Duration:   time.Duration(entry.Duration * float64(time.Second)),
		Requester:  requester,
	}, nil
}

func isURL(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}
