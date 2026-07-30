package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// radioCandidate 是從 yt-dlp --flat-playlist 取得的輕量候選歌曲（尚未解析播放網址）。
type radioCandidate struct {
	ID    string
	Title string
}

// radioLanguagePhrase 將語言關鍵字對應到該語言道地的熱門歌曲搜尋詞。
// 由於 YouTube Music 官方排行榜網址目前已失效（實測回傳 404），
// 這裡改用「該語言本身」的搜尋詞，比直接用中文字面翻譯更貼近該語言的原生熱門內容。
func radioLanguagePhrase(input string) (phrase, label string, ok bool) {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "日文", "日語", "jp", "japanese":
		return "邦楽 人気曲", "日文", true
	case "英文", "en", "english":
		return "popular english songs", "英文", true
	case "韓文", "韓語", "kr", "korean":
		return "인기 케이팝", "韓文", true
	case "中文", "華語", "zh", "chinese", "mandarin":
		return "華語流行歌曲", "中文", true
	default:
		return "", "", false
	}
}

// flatPlaylistEntry 對應 yt-dlp --flat-playlist -j 每行輸出的 JSON。
type flatPlaylistEntry struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	IEKey    string  `json:"ie_key"`
	Duration float64 `json:"duration"`
}

// runYtDlpJSONLines 執行 yt-dlp 並回傳每行（每個結果一行）JSON 字串。
func runYtDlpJSONLines(ctx context.Context, ytDlpPath string, extraArgs []string, target string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	args := append(append([]string{}, extraArgs...), "--no-warnings", "-j", target)
	cmd := exec.CommandContext(ctx, ytDlpPath, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("yt-dlp 執行失敗: %s", msg)
	}

	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

// fetchMusicSearchCandidates 透過 music.youtube.com 搜尋歌曲。
// YouTube Music 的搜尋結果本身就經過「這是不是音樂」的分類，其中真正的歌曲/影片項目
// 標記為 ie_key == "Youtube"，其餘（頻道、專輯、播放清單卡片等）標記為 "YoutubeTab"。
// 只保留前者，可以有效避開一般 YouTube 搜尋常見的公告影片、周邊宣傳、demo 等非歌曲內容。
func fetchMusicSearchCandidates(ctx context.Context, ytDlpPath, query string, limit int) ([]radioCandidate, error) {
	target := "https://music.youtube.com/search?q=" + url.QueryEscape(query)
	lines, err := runYtDlpJSONLines(ctx, ytDlpPath, []string{"--flat-playlist", "--playlist-end", strconv.Itoa(limit)}, target)
	if err != nil {
		return nil, err
	}
	return parseCandidates(lines), nil
}

func parseCandidates(lines []string) []radioCandidate {
	var candidates []radioCandidate
	for _, line := range lines {
		var e flatPlaylistEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil || e.ID == "" {
			continue
		}
		if e.IEKey != "Youtube" {
			continue // 略過頻道／專輯／播放清單等非單曲項目
		}
		if isLikelyNonMusic(e.Title) {
			continue
		}
		if e.Duration > 0 && e.Duration > maxRadioSongDuration.Seconds() {
			continue // 時長過長，多半是全專輯/精選輯混音，而非單曲
		}
		candidates = append(candidates, radioCandidate{ID: e.ID, Title: e.Title})
	}
	return candidates
}

// nonMusicTitleKeywords 是常見「非歌曲本體」上傳的標題關鍵字（預告、周邊、公告等），
// 用來在候選池階段先過濾掉，減少誤播到非音樂內容的機率。這只是啟發式規則，無法涵蓋所有情況。
var nonMusicTitleKeywords = []string{
	"trailer", "teaser", "announcement", "goods", "merch", "reaction",
	"behind the scenes", "interview", "shorts", "full album", "megamix",
	"予告", "告知", "特典", "グッズ", "発売決定", "特典映像", "特番", "情報",
	"デモンストレーション",
	"預告", "公告", "花絮", "周邊", "週邊",
}

func isLikelyNonMusic(title string) bool {
	lower := strings.ToLower(title)
	for _, kw := range nonMusicTitleKeywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// minRadioSongDuration / maxRadioSongDuration 用來過濾「太短（預告/宣傳片段）」
// 或「太長（全專輯、精選輯混音）」而不像單曲的內容。
const (
	minRadioSongDuration = 45 * time.Second
	maxRadioSongDuration = 15 * time.Minute
)

// resolveCandidates 從候選池中隨機挑選（略過 exclude 中已播放過的）並逐一解析為可播放的 Song，
// 直到湊滿 need 首或候選池耗盡為止。回傳實際解析成功的歌曲與其對應的影片 ID。
func resolveCandidates(ctx context.Context, ytDlpPath string, pool []radioCandidate, exclude map[string]bool, need int, requester string) (songs []*Song, usedIDs []string, err error) {
	shuffled := make([]radioCandidate, len(pool))
	copy(shuffled, pool)
	rand.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })

	for _, c := range shuffled {
		if len(songs) >= need {
			break
		}
		if c.ID == "" || exclude[c.ID] {
			continue
		}
		watchURL := "https://www.youtube.com/watch?v=" + c.ID
		song, resolveErr := resolveSong(ctx, ytDlpPath, watchURL, requester)
		if resolveErr != nil {
			continue // 該影片可能不可播放（會員限定、已下架等），略過繼續嘗試下一首
		}
		if song.Duration > 0 && (song.Duration < minRadioSongDuration || song.Duration > maxRadioSongDuration) {
			continue // 再次以精確時長把關（flat-playlist 的時長資訊有時不準確）
		}
		songs = append(songs, song)
		usedIDs = append(usedIDs, c.ID)
	}
	return songs, usedIDs, nil
}
