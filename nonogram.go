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

// nonogramSize 是數織棋盤的邊長（8x8）。
const nonogramSize = 8

// NonogramGame 是單一頻道正在進行的一局數織。target 是要還原出的圖案，filled 是玩家目前的填色狀態。
type NonogramGame struct {
	target    [nonogramSize][nonogramSize]bool
	filled    [nonogramSize][nonogramSize]bool
	messageID snowflake.ID
}

// nonogramClue 把一整排（列或欄）的填色狀態轉成連續填色段落的長度提示，例如 "1 3"。
func nonogramClue(line [nonogramSize]bool) []int {
	var clue []int
	run := 0
	for _, v := range line {
		if v {
			run++
		} else if run > 0 {
			clue = append(clue, run)
			run = 0
		}
	}
	if run > 0 {
		clue = append(clue, run)
	}
	if len(clue) == 0 {
		clue = []int{0}
	}
	return clue
}

func formatNonogramClue(clue []int) string {
	parts := make([]string, len(clue))
	for i, v := range clue {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, " ")
}

// generateNonogramTarget 隨機產生一張約 50% 填色比例的圖案作為題目答案。
func generateNonogramTarget() [nonogramSize][nonogramSize]bool {
	var grid [nonogramSize][nonogramSize]bool
	for r := 0; r < nonogramSize; r++ {
		for c := 0; c < nonogramSize; c++ {
			grid[r][c] = rand.Intn(2) == 0
		}
	}
	return grid
}

func (g *NonogramGame) isSolved() bool {
	return g.filled == g.target
}

func renderNonogram(g *NonogramGame) string {
	rowClues := make([][]int, nonogramSize)
	colClues := make([][]int, nonogramSize)
	for r := 0; r < nonogramSize; r++ {
		var line [nonogramSize]bool
		for c := 0; c < nonogramSize; c++ {
			line[c] = g.target[r][c]
		}
		rowClues[r] = nonogramClue(line)
	}
	for c := 0; c < nonogramSize; c++ {
		var line [nonogramSize]bool
		for r := 0; r < nonogramSize; r++ {
			line[r] = g.target[r][c]
		}
		colClues[c] = nonogramClue(line)
	}

	rowLabels := make([]string, nonogramSize)
	labelWidth := 0
	for r := 0; r < nonogramSize; r++ {
		rowLabels[r] = formatNonogramClue(rowClues[r])
		if len(rowLabels[r]) > labelWidth {
			labelWidth = len(rowLabels[r])
		}
	}

	maxColLines := 1
	for c := 0; c < nonogramSize; c++ {
		if len(colClues[c]) > maxColLines {
			maxColLines = len(colClues[c])
		}
	}

	var sb strings.Builder
	sb.WriteString("🧩 數織（Nonogram）\n```\n")

	pad := strings.Repeat(" ", labelWidth+1)
	for i := 0; i < maxColLines; i++ {
		sb.WriteString(pad)
		for c := 0; c < nonogramSize; c++ {
			need := maxColLines - i
			if len(colClues[c]) >= need {
				sb.WriteString(fmt.Sprintf(" %d", colClues[c][len(colClues[c])-need]))
			} else {
				sb.WriteString("  ")
			}
		}
		sb.WriteString("\n")
	}
	sb.WriteString(pad + strings.Repeat("-", nonogramSize*2+1) + "\n")

	for r := 0; r < nonogramSize; r++ {
		sb.WriteString(fmt.Sprintf("%*s|", labelWidth, rowLabels[r]))
		for c := 0; c < nonogramSize; c++ {
			if g.filled[r][c] {
				sb.WriteString(" #")
			} else {
				sb.WriteString(" .")
			}
		}
		sb.WriteString("\n")
	}
	sb.WriteString("```")
	sb.WriteString("上方／左側數字是每欄／每列連續填色的長度提示，`#` 已填色、`.` 未填色，依提示還原圖案")
	return sb.String()
}

func (b *Bot) cmdNonogram(e *events.MessageCreate, args string) {
	defer b.deleteCommandMessage(e)

	fields := strings.Fields(args)
	if len(fields) == 0 {
		b.nonogramUsage(e)
		return
	}

	switch strings.ToLower(fields[0]) {
	case "new":
		b.cmdNonogramNew(e)
	case "show":
		b.cmdNonogramShow(e)
	case "stop", "quit", "end":
		b.cmdNonogramStop(e)
	default:
		if len(fields) == 2 {
			b.cmdNonogramToggle(e, fields[0], fields[1])
		} else {
			b.nonogramUsage(e)
		}
	}
}

func (b *Bot) nonogramUsage(e *events.MessageCreate) {
	prefix := b.cfg.CommandPrefix
	b.reply(e, fmt.Sprintf(
		"**數織指令**\n"+
			"`%snonogram new` - 開始新的數織\n"+
			"`%snonogram <列> <欄>` - 切換一格的填色狀態，例如 `%snonogram 3 5`\n"+
			"`%snonogram show` - 重新顯示目前棋盤\n"+
			"`%snonogram stop` - 結束目前的數織",
		prefix, prefix, prefix, prefix, prefix))
}

func (b *Bot) cmdNonogramNew(e *events.MessageCreate) {
	game := &NonogramGame{target: generateNonogramTarget()}

	msg, err := b.client.Rest.CreateMessage(e.ChannelID, discord.NewMessageCreate().WithContent(renderNonogram(game)))
	if err != nil {
		b.reply(e, fmt.Sprintf("❌ 建立數織失敗：%v", err))
		return
	}
	game.messageID = msg.ID
	b.nonogram.Set(e.ChannelID, game)
}

func (b *Bot) cmdNonogramShow(e *events.MessageCreate) {
	game := b.nonogram.Get(e.ChannelID)
	if game == nil {
		b.reply(e, fmt.Sprintf("目前沒有進行中的數織，用 `%snonogram new` 開始一局。", b.cfg.CommandPrefix))
		return
	}
	b.reply(e, renderNonogram(game))
}

func (b *Bot) cmdNonogramStop(e *events.MessageCreate) {
	if b.nonogram.Get(e.ChannelID) == nil {
		b.reply(e, "目前沒有進行中的數織。")
		return
	}
	b.nonogram.Delete(e.ChannelID)
	b.reply(e, "🛑 已結束目前的數織。")
}

func (b *Bot) cmdNonogramToggle(e *events.MessageCreate, rowStr, colStr string) {
	game := b.nonogram.Get(e.ChannelID)
	if game == nil {
		b.reply(e, fmt.Sprintf("目前沒有進行中的數織，用 `%snonogram new` 開始一局。", b.cfg.CommandPrefix))
		return
	}

	row, err1 := strconv.Atoi(rowStr)
	col, err2 := strconv.Atoi(colStr)
	if err1 != nil || err2 != nil || row < 1 || row > nonogramSize || col < 1 || col > nonogramSize {
		b.reply(e, fmt.Sprintf("用法：`%snonogram <列 1-8> <欄 1-8>`", b.cfg.CommandPrefix))
		return
	}
	r, c := row-1, col-1
	game.filled[r][c] = !game.filled[r][c]

	content := renderNonogram(game)
	if game.isSolved() {
		content += "\n🎉 恭喜還原圖案，過關！"
		b.nonogram.Delete(e.ChannelID)
	}

	if _, err := b.client.Rest.UpdateMessage(e.ChannelID, game.messageID, discord.NewMessageUpdate().WithContent(content)); err != nil {
		b.reply(e, content)
	}
}
