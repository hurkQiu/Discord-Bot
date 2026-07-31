package main

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

// sudokuDifficulties 定義各難度要挖空的格數（挖越多題目越難）。
var sudokuDifficulties = map[string]int{
	"easy":   36,
	"medium": 46,
	"hard":   54,
}

// SudokuGame 是單一頻道正在進行的一局數獨。
type SudokuGame struct {
	board      [9][9]int // 0 代表空格
	fixed      [9][9]bool
	difficulty string
	messageID  snowflake.ID
}

// SudokuManager 管理每個頻道各自獨立的數獨對局。
type SudokuManager struct {
	mu    sync.Mutex
	games map[snowflake.ID]*SudokuGame
}

func NewSudokuManager() *SudokuManager {
	return &SudokuManager{games: make(map[snowflake.ID]*SudokuGame)}
}

func (m *SudokuManager) Get(channelID snowflake.ID) *SudokuGame {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.games[channelID]
}

func (m *SudokuManager) Set(channelID snowflake.ID, game *SudokuGame) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.games[channelID] = game
}

func (m *SudokuManager) Delete(channelID snowflake.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.games, channelID)
}

// isValidPlacement 檢查在 grid[row][col] 填入 val 是否違反同列、同欄、同宮格的數獨規則。
func isValidPlacement(grid [9][9]int, row, col, val int) bool {
	for i := 0; i < 9; i++ {
		if i != col && grid[row][i] == val {
			return false
		}
		if i != row && grid[i][col] == val {
			return false
		}
	}
	boxRow, boxCol := (row/3)*3, (col/3)*3
	for r := boxRow; r < boxRow+3; r++ {
		for c := boxCol; c < boxCol+3; c++ {
			if (r != row || c != col) && grid[r][c] == val {
				return false
			}
		}
	}
	return true
}

// generateFullGrid 用回溯法產生一個隨機的完整合法數獨盤面。
func generateFullGrid() [9][9]int {
	var grid [9][9]int
	fillGrid(&grid, 0, 0)
	return grid
}

func fillGrid(grid *[9][9]int, row, col int) bool {
	if row == 9 {
		return true
	}
	nextRow, nextCol := row, col+1
	if nextCol == 9 {
		nextRow, nextCol = row+1, 0
	}
	for _, n := range rand.Perm(9) {
		val := n + 1
		if isValidPlacement(*grid, row, col, val) {
			grid[row][col] = val
			if fillGrid(grid, nextRow, nextCol) {
				return true
			}
			grid[row][col] = 0
		}
	}
	return false
}

// countSolutions 計算 grid 的解數，一旦達到 limit 就提早結束（用來確認題目解答唯一）。
func countSolutions(grid [9][9]int, limit int) int {
	count := 0
	var solve func(row, col int) bool
	solve = func(row, col int) bool {
		if row == 9 {
			count++
			return count >= limit
		}
		nextRow, nextCol := row, col+1
		if nextCol == 9 {
			nextRow, nextCol = row+1, 0
		}
		if grid[row][col] != 0 {
			return solve(nextRow, nextCol)
		}
		for val := 1; val <= 9; val++ {
			if isValidPlacement(grid, row, col, val) {
				grid[row][col] = val
				if solve(nextRow, nextCol) {
					return true
				}
				grid[row][col] = 0
			}
		}
		return false
	}
	solve(0, 0)
	return count
}

// generatePuzzle 產生指定挖空格數、且解答唯一的數獨題目。
func generatePuzzle(removeCount int) [9][9]int {
	grid := generateFullGrid()
	removed := 0
	for _, pos := range rand.Perm(81) {
		if removed >= removeCount {
			break
		}
		r, c := pos/9, pos%9
		if grid[r][c] == 0 {
			continue
		}
		backup := grid[r][c]
		grid[r][c] = 0
		if countSolutions(grid, 2) != 1 {
			grid[r][c] = backup
			continue
		}
		removed++
	}
	return grid
}

// renderSudoku 把盤面畫成等寬字型的文字方格，方便在 Discord 頻道內顯示。
func renderSudoku(board [9][9]int, difficulty string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🧩 數獨（難度：%s）\n```\n", difficulty))
	sb.WriteString("    1 2 3   4 5 6   7 8 9\n")
	sb.WriteString("  +-------+-------+-------+\n")
	for r := 0; r < 9; r++ {
		if r > 0 && r%3 == 0 {
			sb.WriteString("  +-------+-------+-------+\n")
		}
		sb.WriteString(fmt.Sprintf("%d |", r+1))
		for c := 0; c < 9; c++ {
			if c > 0 && c%3 == 0 {
				sb.WriteString(" |")
			}
			if board[r][c] == 0 {
				sb.WriteString(" .")
			} else {
				sb.WriteString(fmt.Sprintf(" %d", board[r][c]))
			}
		}
		sb.WriteString(" |\n")
	}
	sb.WriteString("  +-------+-------+-------+\n```")
	return sb.String()
}

func isBoardComplete(board [9][9]int) bool {
	for r := 0; r < 9; r++ {
		for c := 0; c < 9; c++ {
			if board[r][c] == 0 {
				return false
			}
		}
	}
	return true
}

func (b *Bot) cmdSudoku(e *events.MessageCreate, args string) {
	// 每個子指令都只是為了更新同一則棋盤訊息，指令本身處理完就刪掉，
	// 避免每填一格就在頻道留下一則訊息、洗版聊天室。
	defer func() {
		_ = b.client.Rest.DeleteMessage(e.ChannelID, e.Message.ID)
	}()

	fields := strings.Fields(args)
	if len(fields) == 0 {
		b.sudokuUsage(e)
		return
	}

	switch strings.ToLower(fields[0]) {
	case "new":
		difficulty := "medium"
		if len(fields) > 1 {
			difficulty = strings.ToLower(fields[1])
		}
		b.cmdSudokuNew(e, difficulty)
	case "show":
		b.cmdSudokuShow(e)
	case "stop", "quit", "end":
		b.cmdSudokuStop(e)
	default:
		if len(fields) == 3 {
			b.cmdSudokuSet(e, fields[0], fields[1], fields[2])
		} else {
			b.sudokuUsage(e)
		}
	}
}

func (b *Bot) sudokuUsage(e *events.MessageCreate) {
	prefix := b.cfg.CommandPrefix
	b.reply(e, fmt.Sprintf(
		"**數獨指令**\n"+
			"`%ssudoku new [easy|medium|hard]` - 開始新的數獨（預設 medium）\n"+
			"`%ssudoku <列> <欄> <數字>` - 例如 `%ssudoku 3 5 7`，數字用 0 清除該格\n"+
			"`%ssudoku show` - 重新顯示目前棋盤\n"+
			"`%ssudoku stop` - 結束目前的數獨",
		prefix, prefix, prefix, prefix, prefix))
}

func (b *Bot) cmdSudokuNew(e *events.MessageCreate, difficulty string) {
	removeCount, ok := sudokuDifficulties[difficulty]
	if !ok {
		b.reply(e, "❌ 難度請選擇 `easy`、`medium` 或 `hard`。")
		return
	}

	board := generatePuzzle(removeCount)
	game := &SudokuGame{board: board, difficulty: difficulty}
	for r := 0; r < 9; r++ {
		for c := 0; c < 9; c++ {
			game.fixed[r][c] = board[r][c] != 0
		}
	}

	msg, err := b.client.Rest.CreateMessage(e.ChannelID, discord.NewMessageCreate().WithContent(renderSudoku(game.board, game.difficulty)))
	if err != nil {
		b.reply(e, fmt.Sprintf("❌ 建立數獨失敗：%v", err))
		return
	}
	game.messageID = msg.ID
	b.sudoku.Set(e.ChannelID, game)
}

func (b *Bot) cmdSudokuShow(e *events.MessageCreate) {
	game := b.sudoku.Get(e.ChannelID)
	if game == nil {
		b.reply(e, fmt.Sprintf("目前沒有進行中的數獨，用 `%ssudoku new` 開始一局。", b.cfg.CommandPrefix))
		return
	}
	b.reply(e, renderSudoku(game.board, game.difficulty))
}

func (b *Bot) cmdSudokuStop(e *events.MessageCreate) {
	if b.sudoku.Get(e.ChannelID) == nil {
		b.reply(e, "目前沒有進行中的數獨。")
		return
	}
	b.sudoku.Delete(e.ChannelID)
	b.reply(e, "🛑 已結束目前的數獨。")
}

func (b *Bot) cmdSudokuSet(e *events.MessageCreate, rowStr, colStr, valStr string) {
	game := b.sudoku.Get(e.ChannelID)
	if game == nil {
		b.reply(e, fmt.Sprintf("目前沒有進行中的數獨，用 `%ssudoku new` 開始一局。", b.cfg.CommandPrefix))
		return
	}

	row, err1 := strconv.Atoi(rowStr)
	col, err2 := strconv.Atoi(colStr)
	val, err3 := strconv.Atoi(valStr)
	if err1 != nil || err2 != nil || err3 != nil || row < 1 || row > 9 || col < 1 || col > 9 || val < 0 || val > 9 {
		b.reply(e, fmt.Sprintf("用法：`%ssudoku <列 1-9> <欄 1-9> <數字 0-9>`（0 清除該格）", b.cfg.CommandPrefix))
		return
	}
	r, c := row-1, col-1

	if game.fixed[r][c] {
		b.reply(e, "❌ 這格是題目原本的數字，不能修改。")
		return
	}

	if val != 0 {
		if !isValidPlacement(game.board, r, c, val) {
			b.reply(e, fmt.Sprintf("❌ 第 %d 列、第 %d 欄不能填 %d（跟同列/同欄/同宮格衝突）。", row, col, val))
			return
		}
	}
	game.board[r][c] = val

	content := renderSudoku(game.board, game.difficulty)
	if isBoardComplete(game.board) {
		content += "\n🎉 恭喜完成數獨！"
		b.sudoku.Delete(e.ChannelID)
	}

	if _, err := b.client.Rest.UpdateMessage(e.ChannelID, game.messageID, discord.NewMessageUpdate().WithContent(content)); err != nil {
		b.reply(e, content)
	}
}
