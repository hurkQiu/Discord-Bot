package main

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

// lightsOutSize 是熄燈遊戲的棋盤邊長（5x5）。
const lightsOutSize = 5

// lightsOutScrambleMoves 是產生新題目時，從全暗狀態隨機按幾次燈來打亂盤面（保證有解）。
const lightsOutScrambleMoves = 18

// LightsOutGame 是單一頻道正在進行的一局熄燈遊戲。
type LightsOutGame struct {
	lit       [lightsOutSize][lightsOutSize]bool
	messageID snowflake.ID
}

// press 按下一格燈：切換自己與上下左右（不含對角、不環繞邊界）的燈光狀態。
func (g *LightsOutGame) press(r, c int) {
	toggle := func(r, c int) {
		if r >= 0 && r < lightsOutSize && c >= 0 && c < lightsOutSize {
			g.lit[r][c] = !g.lit[r][c]
		}
	}
	toggle(r, c)
	toggle(r-1, c)
	toggle(r+1, c)
	toggle(r, c-1)
	toggle(r, c+1)
}

func newLightsOutGame() *LightsOutGame {
	g := &LightsOutGame{}
	// 從全暗（已解）狀態隨機按燈打亂盤面，這樣一定存在一組解法（逆向操作）。
	for i := 0; i < lightsOutScrambleMoves; i++ {
		g.press(rand.Intn(lightsOutSize), rand.Intn(lightsOutSize))
	}
	return g
}

func (g *LightsOutGame) isSolved() bool {
	for r := 0; r < lightsOutSize; r++ {
		for c := 0; c < lightsOutSize; c++ {
			if g.lit[r][c] {
				return false
			}
		}
	}
	return true
}

func renderLightsOut(g *LightsOutGame) string {
	var sb strings.Builder
	sb.WriteString("💡 熄燈遊戲\n```\n")
	sb.WriteString("  1 2 3 4 5\n")
	sb.WriteString(" +----------+\n")
	for r := 0; r < lightsOutSize; r++ {
		sb.WriteString(fmt.Sprintf("%d|", r+1))
		for c := 0; c < lightsOutSize; c++ {
			if g.lit[r][c] {
				sb.WriteString(" O")
			} else {
				sb.WriteString(" .")
			}
		}
		sb.WriteString(" |\n")
	}
	sb.WriteString(" +----------+\n```")
	sb.WriteString("`O` 燈亮 / `.` 燈暗，目標是把全部的燈都按暗")
	return sb.String()
}

func (b *Bot) cmdLightsOut(e *events.MessageCreate, args string) {
	defer b.deleteCommandMessage(e)

	fields := strings.Fields(args)
	if len(fields) == 0 {
		b.lightsUsage(e)
		return
	}

	switch strings.ToLower(fields[0]) {
	case "new":
		b.cmdLightsNew(e)
	case "show":
		b.cmdLightsShow(e)
	case "stop", "quit", "end":
		b.cmdLightsStop(e)
	default:
		if len(fields) == 2 {
			b.cmdLightsPress(e, fields[0], fields[1])
		} else {
			b.lightsUsage(e)
		}
	}
}

func (b *Bot) lightsUsage(e *events.MessageCreate) {
	prefix := b.cfg.CommandPrefix
	b.reply(e, fmt.Sprintf(
		"**熄燈遊戲指令**\n"+
			"`%slights new` - 開始新的熄燈遊戲\n"+
			"`%slights <列> <欄>` - 按下一格燈，例如 `%slights 2 3`，會連帶切換上下左右的燈\n"+
			"`%slights show` - 重新顯示目前棋盤\n"+
			"`%slights stop` - 結束目前的熄燈遊戲",
		prefix, prefix, prefix, prefix, prefix))
}

func (b *Bot) cmdLightsNew(e *events.MessageCreate) {
	game := newLightsOutGame()

	msg, err := b.client.Rest.CreateMessage(e.ChannelID, discord.NewMessageCreate().WithContent(renderLightsOut(game)))
	if err != nil {
		b.reply(e, fmt.Sprintf("❌ 建立熄燈遊戲失敗：%v", err))
		return
	}
	game.messageID = msg.ID
	b.lights.Set(e.ChannelID, game)
}

func (b *Bot) cmdLightsShow(e *events.MessageCreate) {
	game := b.lights.Get(e.ChannelID)
	if game == nil {
		b.reply(e, fmt.Sprintf("目前沒有進行中的熄燈遊戲，用 `%slights new` 開始一局。", b.cfg.CommandPrefix))
		return
	}
	b.reply(e, renderLightsOut(game))
}

func (b *Bot) cmdLightsStop(e *events.MessageCreate) {
	if b.lights.Get(e.ChannelID) == nil {
		b.reply(e, "目前沒有進行中的熄燈遊戲。")
		return
	}
	b.lights.Delete(e.ChannelID)
	b.reply(e, "🛑 已結束目前的熄燈遊戲。")
}

func (b *Bot) cmdLightsPress(e *events.MessageCreate, rowStr, colStr string) {
	game := b.lights.Get(e.ChannelID)
	if game == nil {
		b.reply(e, fmt.Sprintf("目前沒有進行中的熄燈遊戲，用 `%slights new` 開始一局。", b.cfg.CommandPrefix))
		return
	}

	row, err1 := strconv.Atoi(rowStr)
	col, err2 := strconv.Atoi(colStr)
	if err1 != nil || err2 != nil || row < 1 || row > lightsOutSize || col < 1 || col > lightsOutSize {
		b.reply(e, fmt.Sprintf("用法：`%slights <列 1-5> <欄 1-5>`", b.cfg.CommandPrefix))
		return
	}
	game.press(row-1, col-1)

	content := renderLightsOut(game)
	if game.isSolved() {
		content += "\n🎉 恭喜全部熄燈，過關！"
		b.lights.Delete(e.ChannelID)
	}

	if _, err := b.client.Rest.UpdateMessage(e.ChannelID, game.messageID, discord.NewMessageUpdate().WithContent(content)); err != nil {
		b.reply(e, content)
	}
}
