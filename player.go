package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"
)

// Manager 管理每個伺服器 (guild) 各自獨立的播放器。
type Manager struct {
	mu      sync.Mutex
	players map[snowflake.ID]*Player
}

func NewManager() *Manager {
	return &Manager{players: make(map[snowflake.ID]*Player)}
}

// Get 取得（或建立）指定伺服器的播放器。
func (m *Manager) Get(client *bot.Client, guildID, textChannelID snowflake.ID, voiceChannelName, ytDlpPath string) *Player {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.players[guildID]
	if !ok {
		p = &Player{
			client:           client,
			guildID:          guildID,
			voiceChannelName: voiceChannelName,
			ytDlpPath:        ytDlpPath,
		}
		m.players[guildID] = p
	}
	// 每次下指令都同步最新的文字頻道 ID（可能在不同頻道呼叫）
	p.mu.Lock()
	p.textChannelID = textChannelID
	p.mu.Unlock()
	return p
}

// Player 負責單一伺服器的語音連線、播放佇列與播放狀態。
type Player struct {
	mu sync.Mutex

	client           *bot.Client
	guildID          snowflake.ID
	textChannelID    snowflake.ID
	voiceChannelName string
	ytDlpPath        string

	conn voice.Conn

	queue         []*Song
	current       *Song
	source        *opusSource
	cancelCurrent context.CancelFunc

	playing       bool
	stopRequested bool

	radioActive    bool
	radioRefilling bool
	radioLabel     string
	radioPool      []radioCandidate
	radioPlayed    map[string]bool
}

// Enqueue 將歌曲加入佇列，若播放器閒置則啟動播放迴圈。
func (p *Player) Enqueue(song *Song) {
	p.mu.Lock()
	p.queue = append(p.queue, song)
	shouldStart := !p.playing
	if shouldStart {
		p.playing = true
	}
	p.mu.Unlock()

	if shouldStart {
		go p.loop()
	}
}

// QueueLength 回傳目前佇列中（不含正在播放）的歌曲數量。
func (p *Player) QueueLength() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.queue)
}

// Snapshot 回傳目前播放中的歌曲與佇列快照，用於 !queue / !nowplaying 指令。
func (p *Player) Snapshot() (current *Song, queue []*Song) {
	p.mu.Lock()
	defer p.mu.Unlock()
	queue = make([]*Song, len(p.queue))
	copy(queue, p.queue)
	return p.current, queue
}

// Skip 中斷目前播放的歌曲，播放迴圈會自動接續播放佇列中的下一首。
func (p *Player) Skip() bool {
	p.mu.Lock()
	cancel := p.cancelCurrent
	hasSong := p.current != nil
	p.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	return hasSong
}

// Stop 清空佇列、停止播放並在目前歌曲結束後離開語音頻道，並關閉電台模式。
func (p *Player) Stop() {
	p.mu.Lock()
	p.queue = nil
	p.radioActive = false
	playing := p.playing
	cancel := p.cancelCurrent
	if playing {
		p.stopRequested = true
	}
	p.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if !playing {
		p.disconnect()
	}
}

// StartRadio 啟動電台模式：記錄候選歌曲池，並立即解析、加入第一批歌曲。
// 之後播放迴圈會在佇列剩餘歌曲不多時，透過 maybeRefillRadio 自動補歌。
func (p *Player) StartRadio(ctx context.Context, label string, pool []radioCandidate) (queued int, err error) {
	p.mu.Lock()
	p.radioActive = true
	p.radioLabel = label
	p.radioPool = pool
	p.radioPlayed = make(map[string]bool)
	ytDlpPath := p.ytDlpPath
	p.mu.Unlock()

	songs, used, err := resolveCandidates(ctx, ytDlpPath, pool, map[string]bool{}, 3, "📻 "+label)
	if err != nil {
		return 0, err
	}
	if len(songs) == 0 {
		return 0, fmt.Errorf("找不到可播放的歌曲")
	}

	p.mu.Lock()
	for _, id := range used {
		p.radioPlayed[id] = true
	}
	p.mu.Unlock()

	for _, s := range songs {
		p.Enqueue(s)
	}
	return len(songs), nil
}

// maybeRefillRadio 在電台模式下，於佇列剩餘歌曲不多時於背景自動補入新歌曲。
func (p *Player) maybeRefillRadio() {
	p.mu.Lock()
	if !p.radioActive || p.radioRefilling || len(p.queue) > 2 {
		p.mu.Unlock()
		return
	}
	p.radioRefilling = true
	pool := p.radioPool
	label := p.radioLabel
	ytDlpPath := p.ytDlpPath
	played := make(map[string]bool, len(p.radioPlayed))
	for id := range p.radioPlayed {
		played[id] = true
	}
	p.mu.Unlock()

	go func() {
		songs, used, err := resolveCandidates(context.Background(), ytDlpPath, pool, played, 3, "📻 "+label)

		p.mu.Lock()
		p.radioRefilling = false
		stillActive := p.radioActive
		if stillActive {
			if len(songs) == 0 {
				// 候選池已全部播過，清空紀錄讓電台可以循環播放。
				p.radioPlayed = make(map[string]bool)
			} else {
				for _, id := range used {
					p.radioPlayed[id] = true
				}
			}
		}
		p.mu.Unlock()

		if err != nil || !stillActive || len(songs) == 0 {
			return
		}
		for _, s := range songs {
			p.Enqueue(s)
		}
		p.sendMessage(fmt.Sprintf("📻 電台「%s」已自動補入 %d 首歌曲。", label, len(songs)))
	}()
}

// SetPaused 暫停或繼續播放，回傳是否有播放中的串流可供操作。
func (p *Player) SetPaused(paused bool) bool {
	p.mu.Lock()
	src := p.source
	p.mu.Unlock()
	if src == nil {
		return false
	}
	src.SetPaused(paused)
	return true
}

// PlaybackPosition 回傳目前歌曲已播放的時間長度。
func (p *Player) PlaybackPosition() time.Duration {
	p.mu.Lock()
	src := p.source
	p.mu.Unlock()
	if src == nil {
		return 0
	}
	return src.PlaybackPosition()
}

func (p *Player) loop() {
	for {
		p.mu.Lock()
		if len(p.queue) == 0 {
			p.playing = false
			p.mu.Unlock()
			return
		}
		song := p.queue[0]
		p.queue = p.queue[1:]
		ctx, cancel := context.WithCancel(context.Background())
		p.current = song
		p.cancelCurrent = cancel
		p.mu.Unlock()

		p.maybeRefillRadio()

		if err := p.ensureVoiceConnected(); err != nil {
			p.sendMessage(fmt.Sprintf("❌ 無法加入語音頻道「%s」：%v", p.voiceChannelName, err))
			cancel()
			p.mu.Lock()
			p.current = nil
			p.cancelCurrent = nil
			p.playing = false
			p.radioActive = false
			p.queue = nil
			p.mu.Unlock()
			return
		}

		p.sendNowPlaying(song)
		p.playSong(ctx, song)
		cancel()

		p.mu.Lock()
		p.current = nil
		p.cancelCurrent = nil
		p.source = nil
		stopReq := p.stopRequested
		p.stopRequested = false
		p.mu.Unlock()

		if stopReq {
			p.mu.Lock()
			p.playing = false
			p.mu.Unlock()
			p.disconnect()
			p.sendMessage("⏹️ 已停止播放並離開語音頻道。")
			return
		}
	}
}

func (p *Player) playSong(ctx context.Context, song *Song) {
	source, err := newOpusSource(song.StreamURL)
	if err != nil {
		p.sendMessage(fmt.Sprintf("❌ 音訊啟動失敗：%v", err))
		return
	}

	p.mu.Lock()
	conn := p.conn
	p.source = source
	p.mu.Unlock()
	if conn == nil {
		source.Close()
		return
	}

	conn.SetOpusFrameProvider(source)
	defer func() {
		// 先關閉來源（喚醒任何阻塞中的 ProvideOpusFrame 並終止 ffmpeg），再卸除 provider。
		source.Close()
		conn.SetOpusFrameProvider(nil)
	}()

	select {
	case <-source.Done():
	case <-ctx.Done():
	}
}

func (p *Player) ensureVoiceConnected() error {
	p.mu.Lock()
	conn := p.conn
	p.mu.Unlock()
	if conn != nil {
		return nil
	}

	channels, err := p.client.Rest.GetGuildChannels(p.guildID)
	if err != nil {
		return fmt.Errorf("無法取得伺服器頻道列表: %w", err)
	}

	var voiceChannelID snowflake.ID
	found := false
	for _, c := range channels {
		if c.Type() == discord.ChannelTypeGuildVoice && c.Name() == p.voiceChannelName {
			voiceChannelID = c.ID()
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("找不到名為「%s」的語音頻道，請確認頻道已建立", p.voiceChannelName)
	}

	newConn := p.client.VoiceManager.CreateConn(p.guildID)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := newConn.Open(ctx, voiceChannelID, false, true); err != nil {
		return fmt.Errorf("加入語音頻道失敗: %w", err)
	}
	if err := newConn.SetSpeaking(ctx, voice.SpeakingFlagMicrophone); err != nil {
		log.Printf("[player] 設定 speaking 狀態失敗: %v", err)
	}

	p.mu.Lock()
	p.conn = newConn
	p.mu.Unlock()
	return nil
}

func (p *Player) disconnect() {
	p.mu.Lock()
	conn := p.conn
	p.conn = nil
	p.mu.Unlock()

	if conn != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		conn.Close(ctx)
	}
}

func (p *Player) sendMessage(content string) {
	p.mu.Lock()
	channelID := p.textChannelID
	p.mu.Unlock()
	if channelID == 0 {
		return
	}
	if _, err := p.client.Rest.CreateMessage(channelID, discord.NewMessageCreate().WithContent(content)); err != nil {
		log.Printf("[player] 傳送訊息失敗: %v", err)
	}
}

func (p *Player) sendNowPlaying(song *Song) {
	embed := discord.NewEmbed().
		WithTitle("🎶 正在播放").
		WithDescription(fmt.Sprintf("[%s](%s)", song.Title, song.WebpageURL)).
		WithColor(0x1DB954).
		AddField("時長", formatDuration(song.Duration), true).
		AddField("點播者", song.Requester, true)

	p.mu.Lock()
	channelID := p.textChannelID
	p.mu.Unlock()
	if channelID == 0 {
		return
	}
	if _, err := p.client.Rest.CreateMessage(channelID, discord.NewMessageCreate().WithEmbeds(embed)); err != nil {
		log.Printf("[player] 傳送 now playing 訊息失敗: %v", err)
	}
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "未知"
	}
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
