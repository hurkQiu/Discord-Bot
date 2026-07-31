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

// mineDifficulties 定義各難度的地雷數量（9x9 共 81 格）。
var mineDifficulties = map[string]int{
	"easy":   10,
	"medium": 15,
	"hard":   20,
}

// MinesweeperGame 是單一頻道正在進行的一局踩地雷。
type MinesweeperGame struct {
	mines       [9][9]bool
	revealed    [9][9]bool
	flagged     [9][9]bool
	adjacent    [9][9]int
	minesPlaced bool
	mineCount   int
	difficulty  string
	messageID   snowflake.ID
	over        bool
}

// placeMines 在排除起手格（含周圍 8 格）的情況下隨機佈雷，並計算每格周圍地雷數。
func placeMines(excludeRow, excludeCol, count int) ([9][9]bool, [9][9]int) {
	var mines [9][9]bool
	var adjacent [9][9]int

	excluded := make(map[[2]int]bool)
	for dr := -1; dr <= 1; dr++ {
		for dc := -1; dc <= 1; dc++ {
			r, c := excludeRow+dr, excludeCol+dc
			if r >= 0 && r < 9 && c >= 0 && c < 9 {
				excluded[[2]int{r, c}] = true
			}
		}
	}

	var candidates [][2]int
	for r := 0; r < 9; r++ {
		for c := 0; c < 9; c++ {
			if !excluded[[2]int{r, c}] {
				candidates = append(candidates, [2]int{r, c})
			}
		}
	}
	rand.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
	if count > len(candidates) {
		count = len(candidates)
	}
	for _, pos := range candidates[:count] {
		mines[pos[0]][pos[1]] = true
	}

	for r := 0; r < 9; r++ {
		for c := 0; c < 9; c++ {
			if mines[r][c] {
				continue
			}
			n := 0
			for dr := -1; dr <= 1; dr++ {
				for dc := -1; dc <= 1; dc++ {
					if dr == 0 && dc == 0 {
						continue
					}
					nr, nc := r+dr, c+dc
					if nr >= 0 && nr < 9 && nc >= 0 && nc < 9 && mines[nr][nc] {
						n++
					}
				}
			}
			adjacent[r][c] = n
		}
	}
	return mines, adjacent
}

// reveal 開啟一格，若周圍沒有地雷則遞迴展開相鄰空格（flood fill）。
func (g *MinesweeperGame) reveal(r, c int) {
	if r < 0 || r >= 9 || c < 0 || c >= 9 || g.revealed[r][c] || g.flagged[r][c] {
		return
	}
	g.revealed[r][c] = true
	if g.adjacent[r][c] != 0 {
		return
	}
	for dr := -1; dr <= 1; dr++ {
		for dc := -1; dc <= 1; dc++ {
			if dr != 0 || dc != 0 {
				g.reveal(r+dr, c+dc)
			}
		}
	}
}

// isCleared 檢查除了地雷以外的格子是否都已開啟（踩地雷的勝利條件）。
func (g *MinesweeperGame) isCleared() bool {
	for r := 0; r < 9; r++ {
		for c := 0; c < 9; c++ {
			if !g.mines[r][c] && !g.revealed[r][c] {
				return false
			}
		}
	}
	return true
}

// renderMinesweeper 把盤面畫成等寬字型的文字方格。showMines 為 true 時（遊戲結束）會顯示所有地雷位置。
func renderMinesweeper(g *MinesweeperGame, showMines bool) string {
	var sb strings.Builder
	status := fmt.Sprintf("難度：%s，地雷：%d", g.difficulty, g.mineCount)
	sb.WriteString(fmt.Sprintf("💣 踩地雷（%s）\n```\n", status))
	sb.WriteString("   1 2 3 4 5 6 7 8 9\n")
	sb.WriteString("  +------------------+\n")
	for r := 0; r < 9; r++ {
		sb.WriteString(fmt.Sprintf("%d |", r+1))
		for c := 0; c < 9; c++ {
			var symbol string
			switch {
			case g.flagged[r][c]:
				symbol = "F"
			case showMines && g.mines[r][c]:
				symbol = "*"
			case !g.revealed[r][c]:
				symbol = "#"
			case g.adjacent[r][c] == 0:
				symbol = "."
			default:
				symbol = strconv.Itoa(g.adjacent[r][c])
			}
			sb.WriteString(" " + symbol)
		}
		sb.WriteString(" |\n")
	}
	sb.WriteString("  +------------------+\n```")
	sb.WriteString("`#` 未開 / `F` 插旗 / `.` 空格 / 數字代表周圍地雷數 / `*` 地雷")
	return sb.String()
}

func (b *Bot) cmdMinesweeper(e *events.MessageCreate, args string) {
	defer b.deleteCommandMessage(e)

	fields := strings.Fields(args)
	if len(fields) == 0 {
		b.mineUsage(e)
		return
	}

	switch strings.ToLower(fields[0]) {
	case "new":
		difficulty := "medium"
		if len(fields) > 1 {
			difficulty = strings.ToLower(fields[1])
		}
		b.cmdMineNew(e, difficulty)
	case "show":
		b.cmdMineShow(e)
	case "stop", "quit", "end":
		b.cmdMineStop(e)
	case "flag":
		if len(fields) == 3 {
			b.cmdMineFlag(e, fields[1], fields[2])
		} else {
			b.mineUsage(e)
		}
	default:
		if len(fields) == 2 {
			b.cmdMineReveal(e, fields[0], fields[1])
		} else {
			b.mineUsage(e)
		}
	}
}

func (b *Bot) mineUsage(e *events.MessageCreate) {
	prefix := b.cfg.CommandPrefix
	b.reply(e, fmt.Sprintf(
		"**踩地雷指令**\n"+
			"`%smine new [easy|medium|hard]` - 開始新的踩地雷（預設 medium）\n"+
			"`%smine <列> <欄>` - 開啟一格，例如 `%smine 3 5`\n"+
			"`%smine flag <列> <欄>` - 插旗 / 取消插旗\n"+
			"`%smine show` - 重新顯示目前棋盤\n"+
			"`%smine stop` - 結束目前的踩地雷",
		prefix, prefix, prefix, prefix, prefix, prefix))
}

func (b *Bot) cmdMineNew(e *events.MessageCreate, difficulty string) {
	mineCount, ok := mineDifficulties[difficulty]
	if !ok {
		b.reply(e, "❌ 難度請選擇 `easy`、`medium` 或 `hard`。")
		return
	}

	game := &MinesweeperGame{difficulty: difficulty, mineCount: mineCount}

	msg, err := b.client.Rest.CreateMessage(e.ChannelID, discord.NewMessageCreate().WithContent(renderMinesweeper(game, false)))
	if err != nil {
		b.reply(e, fmt.Sprintf("❌ 建立踩地雷失敗：%v", err))
		return
	}
	game.messageID = msg.ID
	b.mines.Set(e.ChannelID, game)
}

func (b *Bot) cmdMineShow(e *events.MessageCreate) {
	game := b.mines.Get(e.ChannelID)
	if game == nil {
		b.reply(e, fmt.Sprintf("目前沒有進行中的踩地雷，用 `%smine new` 開始一局。", b.cfg.CommandPrefix))
		return
	}
	b.reply(e, renderMinesweeper(game, game.over))
}

func (b *Bot) cmdMineStop(e *events.MessageCreate) {
	if b.mines.Get(e.ChannelID) == nil {
		b.reply(e, "目前沒有進行中的踩地雷。")
		return
	}
	b.mines.Delete(e.ChannelID)
	b.reply(e, "🛑 已結束目前的踩地雷。")
}

func parseMineCoords(rowStr, colStr string) (row, col int, ok bool) {
	r, err1 := strconv.Atoi(rowStr)
	c, err2 := strconv.Atoi(colStr)
	if err1 != nil || err2 != nil || r < 1 || r > 9 || c < 1 || c > 9 {
		return 0, 0, false
	}
	return r, c, true
}

func (b *Bot) cmdMineFlag(e *events.MessageCreate, rowStr, colStr string) {
	game := b.mines.Get(e.ChannelID)
	if game == nil {
		b.reply(e, fmt.Sprintf("目前沒有進行中的踩地雷，用 `%smine new` 開始一局。", b.cfg.CommandPrefix))
		return
	}
	row, col, ok := parseMineCoords(rowStr, colStr)
	if !ok {
		b.reply(e, fmt.Sprintf("用法：`%smine flag <列 1-9> <欄 1-9>`", b.cfg.CommandPrefix))
		return
	}
	r, c := row-1, col-1
	if game.revealed[r][c] {
		b.reply(e, "❌ 這格已經開啟，不能插旗。")
		return
	}
	game.flagged[r][c] = !game.flagged[r][c]
	b.updateMineMessage(e.ChannelID, game)
}

func (b *Bot) cmdMineReveal(e *events.MessageCreate, rowStr, colStr string) {
	game := b.mines.Get(e.ChannelID)
	if game == nil {
		b.reply(e, fmt.Sprintf("目前沒有進行中的踩地雷，用 `%smine new` 開始一局。", b.cfg.CommandPrefix))
		return
	}
	if game.over {
		b.reply(e, fmt.Sprintf("這局踩地雷已經結束，用 `%smine new` 開始新的一局。", b.cfg.CommandPrefix))
		return
	}
	row, col, ok := parseMineCoords(rowStr, colStr)
	if !ok {
		b.reply(e, fmt.Sprintf("用法：`%smine <列 1-9> <欄 1-9>`", b.cfg.CommandPrefix))
		return
	}
	r, c := row-1, col-1
	if game.flagged[r][c] {
		b.reply(e, "❌ 這格已插旗，請先用 `flag` 取消插旗再開啟。")
		return
	}
	if game.revealed[r][c] {
		b.reply(e, "這格已經開啟了。")
		return
	}

	if !game.minesPlaced {
		game.mines, game.adjacent = placeMines(r, c, game.mineCount)
		game.minesPlaced = true
	}

	if game.mines[r][c] {
		game.revealed[r][c] = true
		game.over = true
		b.updateMineMessageWithSuffix(e.ChannelID, game, true, "\n💥 踩到地雷了，遊戲結束！")
		return
	}

	game.reveal(r, c)

	if game.isCleared() {
		game.over = true
		b.updateMineMessageWithSuffix(e.ChannelID, game, true, "\n🎉 恭喜排除所有地雷，過關！")
		return
	}

	b.updateMineMessage(e.ChannelID, game)
}

func (b *Bot) updateMineMessage(channelID snowflake.ID, game *MinesweeperGame) {
	b.updateMineMessageWithSuffix(channelID, game, false, "")
}

func (b *Bot) updateMineMessageWithSuffix(channelID snowflake.ID, game *MinesweeperGame, showMines bool, suffix string) {
	content := renderMinesweeper(game, showMines) + suffix
	if _, err := b.client.Rest.UpdateMessage(channelID, game.messageID, discord.NewMessageUpdate().WithContent(content)); err != nil {
		_, _ = b.client.Rest.CreateMessage(channelID, discord.NewMessageCreate().WithContent(content))
	}
}
