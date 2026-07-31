package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/godave/golibdave"
)

// musicCommands 是只能在音樂文字頻道（TextChannelName）使用的指令集合。
var musicCommands = map[string]bool{
	"play": true, "p": true,
	"skip": true, "s": true,
	"stop": true, "leave": true,
	"queue": true, "q": true, "list": true,
	"pause": true, "resume": true,
	"nowplaying": true, "np": true,
	"radio": true,
}

// gameCommands 是只能在小遊戲文字頻道（GameChannelName）使用的指令集合。
var gameCommands = map[string]bool{
	"sudoku":      true,
	"mine":        true,
	"minesweeper": true,
	"lights":      true,
	"nonogram":    true,
	"nono":        true,
	"idiom":       true,
	"chengyu":     true,
}

// Bot 封裝 disgo client 與音樂播放邏輯。
type Bot struct {
	client   *bot.Client
	cfg      *Config
	manager  *Manager
	sudoku   *SudokuManager
	mines    *GameManager[MinesweeperGame]
	lights   *GameManager[LightsOutGame]
	nonogram *GameManager[NonogramGame]
	idiom    *GameManager[IdiomGame]
}

// NewBot 建立 disgo client 並註冊事件處理，同時啟用 DAVE（E2EE）語音加密。
func NewBot(cfg *Config) (*Bot, error) {
	b := &Bot{
		cfg:      cfg,
		manager:  NewManager(),
		sudoku:   NewSudokuManager(),
		mines:    NewGameManager[MinesweeperGame](),
		lights:   NewGameManager[LightsOutGame](),
		nonogram: NewGameManager[NonogramGame](),
		idiom:    NewGameManager[IdiomGame](),
	}

	client, err := disgo.New(cfg.BotToken,
		bot.WithGatewayConfigOpts(
			gateway.WithIntents(
				gateway.IntentGuilds,
				gateway.IntentGuildMessages,
				gateway.IntentMessageContent,
				gateway.IntentGuildVoiceStates,
			),
		),
		// 啟用 Guild/Channel/Role 快取，供 !clear 指令計算成員在頻道內的實際權限（含伺服器擁有者、身分組、頻道覆寫）之用。
		bot.WithCacheConfigOpts(
			cache.WithCaches(cache.FlagGuilds, cache.FlagChannels, cache.FlagRoles),
		),
		bot.WithEventListenerFunc(b.onReady),
		bot.WithEventListenerFunc(b.onMessageCreate),
		bot.WithVoiceManagerConfigOpts(
			voice.WithDaveSessionCreateFunc(golibdave.NewSession),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("建立 Discord client 失敗: %w", err)
	}
	b.client = client

	return b, nil
}

func (b *Bot) Open(ctx context.Context) error {
	return b.client.OpenGateway(ctx)
}

func (b *Bot) Close(ctx context.Context) {
	b.manager.mu.Lock()
	players := make([]*Player, 0, len(b.manager.players))
	for _, p := range b.manager.players {
		players = append(players, p)
	}
	b.manager.mu.Unlock()

	for _, p := range players {
		p.Stop()
	}
	b.client.Close(ctx)
}

func (b *Bot) onReady(e *events.Ready) {
	log.Printf("機器人已上線：%s", e.User.Username)
}

func (b *Bot) onMessageCreate(e *events.MessageCreate) {
	if e.Message.Author.Bot || e.GuildID == nil {
		return
	}

	channel, err := b.client.Rest.GetChannel(e.ChannelID)
	if err != nil {
		return
	}
	channelName := channel.Name()
	isMusicChannel := channelName == b.cfg.TextChannelName
	isGameChannel := channelName == b.cfg.GameChannelName
	if !isMusicChannel && !isGameChannel {
		return
	}

	if !strings.HasPrefix(e.Message.Content, b.cfg.CommandPrefix) {
		return
	}
	body := strings.TrimSpace(strings.TrimPrefix(e.Message.Content, b.cfg.CommandPrefix))
	if body == "" {
		return
	}
	fields := strings.Fields(body)
	cmd := strings.ToLower(fields[0])
	args := strings.TrimSpace(strings.TrimPrefix(body, fields[0]))

	if musicCommands[cmd] && !isMusicChannel {
		return
	}
	if gameCommands[cmd] && !isGameChannel {
		return
	}

	switch cmd {
	case "play", "p":
		go b.cmdPlay(e, args)
	case "skip", "s":
		b.cmdSkip(e)
	case "stop", "leave":
		b.cmdStop(e)
	case "queue", "q", "list":
		b.cmdQueue(e)
	case "pause":
		b.cmdPause(e)
	case "resume":
		b.cmdResume(e)
	case "nowplaying", "np":
		b.cmdNowPlaying(e)
	case "radio":
		go b.cmdRadio(e, args)
	case "sudoku":
		go b.cmdSudoku(e, args)
	case "mine", "minesweeper":
		go b.cmdMinesweeper(e, args)
	case "lights":
		go b.cmdLightsOut(e, args)
	case "nonogram", "nono":
		go b.cmdNonogram(e, args)
	case "idiom", "chengyu":
		go b.cmdIdiom(e, args)
	case "clear", "clean", "purge":
		go b.cmdClear(e, args)
	case "help":
		b.cmdHelp(e)
	}
}

func (b *Bot) player(e *events.MessageCreate) *Player {
	return b.manager.Get(b.client, *e.GuildID, e.ChannelID, b.cfg.VoiceChannelName, b.cfg.YtDlpPath)
}

func requesterName(e *events.MessageCreate) string {
	if e.Message.Member != nil {
		return e.Message.Member.EffectiveName()
	}
	return e.Message.Author.Username
}

func (b *Bot) reply(e *events.MessageCreate, content string) {
	_, _ = b.client.Rest.CreateMessage(e.ChannelID, discord.NewMessageCreate().WithContent(content))
}

func (b *Bot) cmdPlay(e *events.MessageCreate, query string) {
	if query == "" {
		b.reply(e, fmt.Sprintf("用法：`%splay <歌曲名稱或 YouTube 網址>`", b.cfg.CommandPrefix))
		return
	}

	p := b.player(e)

	song, err := resolveSong(context.Background(), b.cfg.YtDlpPath, query, requesterName(e))
	if err != nil {
		b.reply(e, fmt.Sprintf("❌ 找不到歌曲：%v", err))
		return
	}

	p.Enqueue(song)

	if pos := p.QueueLength(); pos > 0 {
		b.reply(e, fmt.Sprintf("✅ 已加入佇列（第 %d 位）：**%s**", pos, song.Title))
	}
}

func (b *Bot) cmdSkip(e *events.MessageCreate) {
	p := b.player(e)
	if p.Skip() {
		b.reply(e, "⏭️ 已跳過目前歌曲。")
	} else {
		b.reply(e, "目前沒有正在播放的歌曲。")
	}
}

func (b *Bot) cmdStop(e *events.MessageCreate) {
	p := b.player(e)
	p.Stop()
	b.reply(e, "⏹️ 已停止播放並清空佇列。")
}

func (b *Bot) cmdPause(e *events.MessageCreate) {
	p := b.player(e)
	if p.SetPaused(true) {
		b.reply(e, "⏸️ 已暫停播放。")
	} else {
		b.reply(e, "目前沒有正在播放的歌曲。")
	}
}

func (b *Bot) cmdResume(e *events.MessageCreate) {
	p := b.player(e)
	if p.SetPaused(false) {
		b.reply(e, "▶️ 已繼續播放。")
	} else {
		b.reply(e, "目前沒有正在播放的歌曲。")
	}
}

func (b *Bot) cmdNowPlaying(e *events.MessageCreate) {
	p := b.player(e)
	current, _ := p.Snapshot()
	if current == nil {
		b.reply(e, "目前沒有正在播放的歌曲。")
		return
	}
	pos := formatDuration(p.PlaybackPosition())
	total := formatDuration(current.Duration)
	b.reply(e, fmt.Sprintf("🎶 正在播放：**%s** (%s / %s)，點播者：%s", current.Title, pos, total, current.Requester))
}

func (b *Bot) cmdQueue(e *events.MessageCreate) {
	p := b.player(e)
	current, queue := p.Snapshot()

	if current == nil && len(queue) == 0 {
		b.reply(e, "目前佇列是空的。")
		return
	}

	var sb strings.Builder
	if current != nil {
		sb.WriteString(fmt.Sprintf("🎶 正在播放：**%s**（%s，點播者：%s）\n", current.Title, formatDuration(current.Duration), current.Requester))
	}
	if len(queue) > 0 {
		sb.WriteString("\n📋 佇列：\n")
		for i, s := range queue {
			sb.WriteString(fmt.Sprintf("%d. %s（%s，點播者：%s）\n", i+1, s.Title, formatDuration(s.Duration), s.Requester))
		}
	}
	b.reply(e, sb.String())
}

// helpTemplate 是 !help 的內容樣板，用 {p} 代表指令前綴。用取代字串而非 fmt.Sprintf 的
// %s 位置參數，是為了避免每次新增一行指令說明就要重新數對 %s 與參數個數。
const helpTemplate = `**音樂機器人指令**
` + "`{p}play <歌曲名稱或網址>`" + ` - 搜尋並播放 / 加入佇列
` + "`{p}skip`" + ` - 跳過目前歌曲
` + "`{p}pause`" + ` - 暫停播放
` + "`{p}resume`" + ` - 繼續播放
` + "`{p}stop`" + ` - 停止播放並離開語音頻道（含電台模式）
` + "`{p}queue`" + ` - 顯示播放佇列
` + "`{p}nowplaying`" + ` - 顯示目前播放進度
` + "`{p}radio artist <歌手名稱>`" + ` - 啟動該歌手的電台，持續隨機播放並自動接歌
` + "`{p}radio lang <日文|英文|韓文|中文>`" + ` - 啟動該語言的熱門歌曲電台

**數獨**
` + "`{p}sudoku new [easy|medium|hard]`" + ` - 開始新的數獨（預設 medium）
` + "`{p}sudoku <列> <欄> <數字>`" + ` - 填入數字，例如 ` + "`{p}sudoku 3 5 7`" + `，數字用 0 清除該格
` + "`{p}sudoku show`" + ` / ` + "`{p}sudoku stop`" + ` - 重新顯示 / 結束目前的數獨

**踩地雷**
` + "`{p}mine new [easy|medium|hard]`" + ` - 開始新的踩地雷（預設 medium）
` + "`{p}mine <列> <欄>`" + ` - 開啟一格，例如 ` + "`{p}mine 3 5`" + `
` + "`{p}mine flag <列> <欄>`" + ` - 插旗 / 取消插旗
` + "`{p}mine show`" + ` / ` + "`{p}mine stop`" + ` - 重新顯示 / 結束目前的踩地雷

**熄燈遊戲**
` + "`{p}lights new`" + ` - 開始新的熄燈遊戲
` + "`{p}lights <列> <欄>`" + ` - 按下一格燈，例如 ` + "`{p}lights 2 3`" + `，會連帶切換上下左右的燈
` + "`{p}lights show`" + ` / ` + "`{p}lights stop`" + ` - 重新顯示 / 結束目前的熄燈遊戲

**數織**
` + "`{p}nonogram new`" + ` - 開始新的數織
` + "`{p}nonogram <列> <欄>`" + ` - 切換一格的填色狀態，例如 ` + "`{p}nonogram 3 5`" + `
` + "`{p}nonogram show`" + ` / ` + "`{p}nonogram stop`" + ` - 重新顯示 / 結束目前的數織

**猜成語**
` + "`{p}idiom new`" + ` - 開始新的猜成語（六次機會）
` + "`{p}idiom <四字成語>`" + ` - 送出一次猜測，例如 ` + "`{p}idiom 一帆風順`" + `
` + "`{p}idiom show`" + ` / ` + "`{p}idiom stop`" + ` - 重新顯示 / 結束目前的猜成語

**頻道管理**
` + "`{p}clear confirm`" + ` - 清除目前頻道內的全部訊息（需要「管理訊息」權限，無法復原）
`

func (b *Bot) cmdHelp(e *events.MessageCreate) {
	help := strings.ReplaceAll(helpTemplate, "{p}", b.cfg.CommandPrefix)
	b.reply(e, help)
}

func (b *Bot) cmdRadio(e *events.MessageCreate, args string) {
	fields := strings.Fields(args)
	usage := fmt.Sprintf("用法：`%sradio artist <歌手名稱>` 或 `%sradio lang <日文|英文|韓文|中文>`", b.cfg.CommandPrefix, b.cfg.CommandPrefix)
	if len(fields) < 2 {
		b.reply(e, usage)
		return
	}

	mode := strings.ToLower(fields[0])
	target := strings.TrimSpace(strings.TrimPrefix(args, fields[0]))

	switch mode {
	case "artist":
		b.cmdRadioArtist(e, target)
	case "lang", "language":
		b.cmdRadioLang(e, target)
	default:
		b.reply(e, usage)
	}
}

func (b *Bot) cmdRadioArtist(e *events.MessageCreate, name string) {
	if name == "" {
		b.reply(e, fmt.Sprintf("用法：`%sradio artist <歌手名稱>`", b.cfg.CommandPrefix))
		return
	}

	p := b.player(e)
	ctx := context.Background()

	b.reply(e, fmt.Sprintf("🔍 正在搜尋歌手「%s」的歌曲...", name))
	pool, err := fetchMusicSearchCandidates(ctx, b.cfg.YtDlpPath, name, 40)
	if err != nil || len(pool) == 0 {
		b.reply(e, fmt.Sprintf("❌ 找不到「%s」的歌曲：%v", name, err))
		return
	}

	label := name + " 電台"
	queued, err := p.StartRadio(ctx, label, pool)
	if err != nil {
		b.reply(e, fmt.Sprintf("❌ 電台啟動失敗：%v", err))
		return
	}
	b.reply(e, fmt.Sprintf("📻 已啟動「%s」，找到 %d 首候選歌曲，已加入 %d 首到佇列，會持續自動接歌，`%sstop` 可停止。", label, len(pool), queued, b.cfg.CommandPrefix))
}

func (b *Bot) cmdRadioLang(e *events.MessageCreate, lang string) {
	if lang == "" {
		b.reply(e, fmt.Sprintf("用法：`%sradio lang <日文|英文|韓文|中文>`", b.cfg.CommandPrefix))
		return
	}

	phrase, label, ok := radioLanguagePhrase(lang)
	if !ok {
		b.reply(e, fmt.Sprintf("❌ 不支援的語言「%s」，目前支援：日文、英文、韓文、中文", lang))
		return
	}

	p := b.player(e)
	ctx := context.Background()

	b.reply(e, fmt.Sprintf("🔍 正在搜尋%s熱門歌曲...", label))
	pool, err := fetchMusicSearchCandidates(ctx, b.cfg.YtDlpPath, phrase, 40)
	if err != nil || len(pool) == 0 {
		b.reply(e, fmt.Sprintf("❌ 找不到%s歌曲：%v", label, err))
		return
	}

	radioLabel := label + "熱門電台"
	queued, err := p.StartRadio(ctx, radioLabel, pool)
	if err != nil {
		b.reply(e, fmt.Sprintf("❌ 電台啟動失敗：%v", err))
		return
	}
	b.reply(e, fmt.Sprintf("📻 已啟動「%s」，已加入 %d 首到佇列，會持續自動接歌，`%sstop` 可停止。", radioLabel, queued, b.cfg.CommandPrefix))
}
