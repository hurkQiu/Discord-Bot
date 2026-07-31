package main

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

// idiomMaxAttempts 是猜成語遊戲允許的最大猜測次數。
const idiomMaxAttempts = 6

// idiomList 是內建的四字成語題庫（非完整詞典，僅收錄常見成語供遊戲使用）。
var idiomList = []string{
	"一帆風順", "畫蛇添足", "守株待兔", "亡羊補牢", "井底之蛙",
	"狐假虎威", "對牛彈琴", "掩耳盜鈴", "杯弓蛇影", "畫龍點睛",
	"自相矛盾", "塞翁失馬", "刻舟求劍", "葉公好龍", "東施效顰",
	"濫竽充數", "鷸蚌相爭", "揠苗助長", "望梅止渴", "完璧歸趙",
	"臥薪嘗膽", "破釜沉舟", "紙上談兵", "指鹿為馬", "三顧茅廬",
	"四面楚歌", "老馬識途", "唇亡齒寒", "朝三暮四", "買櫝還珠",
	"一箭雙鵰", "七上八下", "七嘴八舌", "九牛一毛", "十全十美",
	"五花八門", "五光十色", "三心二意", "三言兩語", "半途而廢",
	"半信半疑", "天涯海角", "天壤之別", "心花怒放", "心曠神怡",
	"手忙腳亂", "手舞足蹈", "引人入勝", "引狼入室", "文不對題",
	"日新月異", "井井有條", "分道揚鑣", "化險為夷", "火上加油",
	"火冒三丈", "冰天雪地", "光明正大", "光陰似箭", "如魚得水",
	"如坐針氈", "如虎添翼", "如釋重負", "守口如瓶", "安居樂業",
	"家喻戶曉", "對症下藥", "專心致志", "川流不息", "左顧右盼",
	"平易近人", "平步青雲", "弄巧成拙", "忐忑不安", "忍無可忍",
	"忠言逆耳", "快馬加鞭", "怒髮衝冠", "恍然大悟", "惟妙惟肖",
	"意氣風發", "愛不釋手", "打草驚蛇", "拋磚引玉", "拍案叫絕",
	"掩人耳目", "揚眉吐氣", "明察秋毫", "明知故犯", "易如反掌",
	"星羅棋布", "春風得意", "望而生畏", "望塵莫及", "未雨綢繆",
	"本末倒置", "東山再起", "杯水車薪", "歡天喜地", "熱淚盈眶",
	"畫地自限", "痛改前非", "百發百中", "眼高手低", "破鏡重圓",
	"稱心如意", "興高采烈", "舉一反三", "膾炙人口", "自食其力",
	"良藥苦口", "藕斷絲連", "見異思遷", "談笑風生", "貪小失大",
	"近水樓台", "適得其反", "錦上添花", "雪中送炭", "鴉雀無聲",
}

// filteredIdiomList 只保留四個字的成語（idiomList 目前收錄的皆為四字，
// 此處過濾是為了避免未來誤加入非四字詞條時造成猜測長度矛盾）。
var filteredIdiomList = func() []string {
	var list []string
	for _, idiom := range idiomList {
		if len([]rune(idiom)) == 4 {
			list = append(list, idiom)
		}
	}
	return list
}()

// idiomFeedback 是單一字元的猜測結果。
type idiomFeedback int

const (
	idiomAbsent idiomFeedback = iota
	idiomPresent
	idiomCorrect
)

// IdiomGame 是單一頻道正在進行的一局猜成語。
type IdiomGame struct {
	target      string
	guessLines  []string
	over        bool
	won         bool
	messageID   snowflake.ID
	maxAttempts int
}

// evaluateIdiomGuess 用類似 Wordle 的兩階段演算法比對每個字：先標出位置與字都對的（綠），
// 再從剩餘字元池標出字對但位置錯的（黃），正確處理重複字元的情況。
func evaluateIdiomGuess(target, guess []rune) []idiomFeedback {
	n := len(target)
	result := make([]idiomFeedback, n)
	remaining := make(map[rune]int)

	for i := 0; i < n; i++ {
		if guess[i] == target[i] {
			result[i] = idiomCorrect
		} else {
			remaining[target[i]]++
		}
	}
	for i := 0; i < n; i++ {
		if result[i] == idiomCorrect {
			continue
		}
		if remaining[guess[i]] > 0 {
			result[i] = idiomPresent
			remaining[guess[i]]--
		} else {
			result[i] = idiomAbsent
		}
	}
	return result
}

func renderIdiomGuessLine(guess []rune, fb []idiomFeedback) string {
	var sb strings.Builder
	for i, r := range guess {
		sb.WriteRune(r)
		switch fb[i] {
		case idiomCorrect:
			sb.WriteString("🟩")
		case idiomPresent:
			sb.WriteString("🟨")
		default:
			sb.WriteString("⬛")
		}
		if i < len(guess)-1 {
			sb.WriteString("　")
		}
	}
	return sb.String()
}

func renderIdiom(g *IdiomGame, prefix string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🀄 **猜成語**（第 %d/%d 次猜測）\n\n", len(g.guessLines), g.maxAttempts))
	for i, line := range g.guessLines {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, line))
	}
	switch {
	case g.over && g.won:
		sb.WriteString(fmt.Sprintf("\n🎉 恭喜猜中！答案就是「%s」。", g.target))
	case g.over:
		sb.WriteString(fmt.Sprintf("\n💥 次數用完了，答案是「%s」。", g.target))
	default:
		sb.WriteString(fmt.Sprintf("\n輸入 `%sidiom <四字成語>` 繼續猜，還有 %d 次機會。🟩 字和位置都對／🟨 字對位置錯／⬛ 成語中沒有這個字。", prefix, g.maxAttempts-len(g.guessLines)))
	}
	return sb.String()
}

func (b *Bot) cmdIdiom(e *events.MessageCreate, args string) {
	defer b.deleteCommandMessage(e)

	trimmed := strings.TrimSpace(args)
	if trimmed == "" {
		b.idiomUsage(e)
		return
	}

	switch strings.ToLower(trimmed) {
	case "new":
		b.cmdIdiomNew(e)
	case "show":
		b.cmdIdiomShow(e)
	case "stop", "quit", "end":
		b.cmdIdiomStop(e)
	default:
		b.cmdIdiomGuess(e, trimmed)
	}
}

func (b *Bot) idiomUsage(e *events.MessageCreate) {
	prefix := b.cfg.CommandPrefix
	b.reply(e, fmt.Sprintf(
		"**猜成語指令**\n"+
			"`%sidiom new` - 開始新的猜成語（六次機會）\n"+
			"`%sidiom <四字成語>` - 送出一次猜測，例如 `%sidiom 一帆風順`\n"+
			"`%sidiom show` - 重新顯示目前猜測紀錄\n"+
			"`%sidiom stop` - 結束目前的猜成語（會公布答案）",
		prefix, prefix, prefix, prefix, prefix))
}

func (b *Bot) cmdIdiomNew(e *events.MessageCreate) {
	if len(filteredIdiomList) == 0 {
		b.reply(e, "❌ 成語題庫是空的，無法開始遊戲。")
		return
	}
	game := &IdiomGame{
		target:      filteredIdiomList[rand.Intn(len(filteredIdiomList))],
		maxAttempts: idiomMaxAttempts,
	}

	msg, err := b.client.Rest.CreateMessage(e.ChannelID, discord.NewMessageCreate().WithContent(renderIdiom(game, b.cfg.CommandPrefix)))
	if err != nil {
		b.reply(e, fmt.Sprintf("❌ 建立猜成語失敗：%v", err))
		return
	}
	game.messageID = msg.ID
	b.idiom.Set(e.ChannelID, game)
}

func (b *Bot) cmdIdiomShow(e *events.MessageCreate) {
	game := b.idiom.Get(e.ChannelID)
	if game == nil {
		b.reply(e, fmt.Sprintf("目前沒有進行中的猜成語，用 `%sidiom new` 開始一局。", b.cfg.CommandPrefix))
		return
	}
	b.reply(e, renderIdiom(game, b.cfg.CommandPrefix))
}

func (b *Bot) cmdIdiomStop(e *events.MessageCreate) {
	game := b.idiom.Get(e.ChannelID)
	if game == nil {
		b.reply(e, "目前沒有進行中的猜成語。")
		return
	}
	b.idiom.Delete(e.ChannelID)
	b.reply(e, fmt.Sprintf("🛑 已結束目前的猜成語，答案是「%s」。", game.target))
}

func (b *Bot) cmdIdiomGuess(e *events.MessageCreate, guess string) {
	game := b.idiom.Get(e.ChannelID)
	if game == nil {
		b.reply(e, fmt.Sprintf("目前沒有進行中的猜成語，用 `%sidiom new` 開始一局。", b.cfg.CommandPrefix))
		return
	}
	if game.over {
		b.reply(e, fmt.Sprintf("這局猜成語已經結束，用 `%sidiom new` 開始新的一局。", b.cfg.CommandPrefix))
		return
	}

	guessRunes := []rune(guess)
	targetRunes := []rune(game.target)
	if len(guessRunes) != len(targetRunes) {
		b.reply(e, fmt.Sprintf("❌ 請輸入 %d 個字的成語，你輸入了 %d 個字。", len(targetRunes), len(guessRunes)))
		return
	}

	fb := evaluateIdiomGuess(targetRunes, guessRunes)
	game.guessLines = append(game.guessLines, renderIdiomGuessLine(guessRunes, fb))

	if guess == game.target {
		game.over = true
		game.won = true
	} else if len(game.guessLines) >= game.maxAttempts {
		game.over = true
	}

	content := renderIdiom(game, b.cfg.CommandPrefix)
	if _, err := b.client.Rest.UpdateMessage(e.ChannelID, game.messageID, discord.NewMessageUpdate().WithContent(content)); err != nil {
		b.reply(e, content)
	}
	if game.over {
		b.idiom.Delete(e.ChannelID)
	}
}
