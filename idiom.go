package main

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

// idiomMaxAttempts 是猜成語遊戲允許的最大猜測次數（使用 !idiom hint 提示字也會消耗一次）。
const idiomMaxAttempts = 6

// idiomEntry 是題庫中的一筆成語與其釋義提示。
type idiomEntry struct {
	Word string
	Hint string
}

// idiomList 是內建的四字成語題庫（非完整詞典，僅收錄常見成語供遊戲使用），
// Hint 是簡短的釋義，讓玩家在盲猜之外還能從語意推敲答案。
var idiomList = []idiomEntry{
	{"一帆風順", "比喻事情非常順利，毫無波折"},
	{"畫蛇添足", "比喻多此一舉，反而把事情弄糟"},
	{"守株待兔", "比喻不知變通，妄想不勞而獲"},
	{"亡羊補牢", "出了差錯後想辦法補救，避免繼續受損"},
	{"井底之蛙", "比喻見識淺薄、眼界狹小的人"},
	{"狐假虎威", "比喻依仗別人的權勢來欺壓人"},
	{"對牛彈琴", "譏笑說話不看對象，白費口舌"},
	{"掩耳盜鈴", "比喻自己欺騙自己，明明掩蓋不了卻硬要掩蓋"},
	{"杯弓蛇影", "比喻因疑神疑鬼而自己嚇自己"},
	{"畫龍點睛", "比喻在關鍵處加上一筆，使全篇更加生動傳神"},
	{"自相矛盾", "比喻言行前後不一致，互相牴觸"},
	{"塞翁失馬", "比喻禍福相依，一時的損失也許會帶來後來的好處"},
	{"刻舟求劍", "比喻拘泥成規、不知變通"},
	{"葉公好龍", "比喻表面愛好某事物，實際上並不真心喜歡"},
	{"東施效顰", "比喻不衡量自身條件，胡亂模仿別人反而出醜"},
	{"濫竽充數", "比喻沒有真才實學的人混在行家中充數"},
	{"鷸蚌相爭", "比喻雙方相持不下，讓第三者從中得利"},
	{"揠苗助長", "比喻違反事物發展規律，急於求成反而壞事"},
	{"望梅止渴", "比喻用空想或許諾來安慰自己"},
	{"完璧歸趙", "比喻把原物完好無損地歸還物主"},
	{"臥薪嘗膽", "形容刻苦自勵、發憤圖強"},
	{"破釜沉舟", "比喻下定決心，不留退路，非成功不可"},
	{"紙上談兵", "比喻空談理論，不能解決實際問題"},
	{"指鹿為馬", "比喻故意顛倒是非、混淆黑白"},
	{"三顧茅廬", "比喻真心誠意、一再邀請有才能的人"},
	{"四面楚歌", "比喻四面受敵、孤立無援的處境"},
	{"老馬識途", "比喻經驗豐富的人熟悉情況，能起引導作用"},
	{"唇亡齒寒", "比喻雙方關係密切，利害相關"},
	{"朝三暮四", "比喻反覆無常，或用花招欺騙人"},
	{"買櫝還珠", "比喻取捨不當，捨本逐末"},
	{"一箭雙鵰", "比喻一個行動達到兩個目的"},
	{"七上八下", "形容心裡忐忑不安、慌亂不定"},
	{"七嘴八舌", "形容人多口雜，議論紛紛"},
	{"九牛一毛", "比喻極大數量中微不足道的一小部分"},
	{"十全十美", "形容完美無缺、毫無缺陷"},
	{"五花八門", "形容事物種類繁多、變化多端"},
	{"五光十色", "形容色彩繽紛、多采多姿"},
	{"三心二意", "形容猶豫不決、意志不堅定"},
	{"三言兩語", "形容說話簡短扼要"},
	{"半途而廢", "比喻事情做到一半就停止，沒有堅持到底"},
	{"半信半疑", "形容有點相信又有點懷疑"},
	{"天涯海角", "比喻極其遙遠的地方"},
	{"天壤之別", "形容差異極大，如天地之別"},
	{"心花怒放", "形容內心非常高興、喜悅"},
	{"心曠神怡", "形容心情開闊、精神愉快"},
	{"手忙腳亂", "形容做事慌張、失去條理"},
	{"手舞足蹈", "形容高興到極點，忍不住手腳舞動"},
	{"引人入勝", "形容事物十分吸引人，讓人陶醉其中"},
	{"引狼入室", "比喻自己招來禍害"},
	{"文不對題", "形容文章內容跟題目不相符"},
	{"日新月異", "形容進步、變化非常快速"},
	{"井井有條", "形容做事、擺放整齊有秩序"},
	{"分道揚鑣", "比喻各走各的路，各做各的事"},
	{"化險為夷", "比喻轉危為安，把危險化解掉"},
	{"火上加油", "比喻使情況更加惡化、加劇衝突"},
	{"火冒三丈", "形容非常憤怒"},
	{"冰天雪地", "形容天氣嚴寒、到處都是冰雪"},
	{"光明正大", "形容行為正直、沒有私心"},
	{"光陰似箭", "形容時間過得飛快"},
	{"如魚得水", "比喻得到跟自己相合的環境或人，非常自在"},
	{"如坐針氈", "形容心中十分不安，像坐在插滿針的毯子上"},
	{"如虎添翼", "比喻強者又獲得助力，變得更加強大"},
	{"如釋重負", "形容放下心中負擔後感到輕鬆"},
	{"守口如瓶", "形容說話謹慎、嚴守秘密"},
	{"安居樂業", "形容生活安定、對工作感到滿意愉快"},
	{"家喻戶曉", "形容人人皆知、非常普遍地為人所知"},
	{"對症下藥", "比喻針對問題的癥結所在採取相應的解決辦法"},
	{"專心致志", "形容用心專一，不分心"},
	{"川流不息", "形容人、車等像水流一樣連續不斷"},
	{"左顧右盼", "形容東張西望，猶豫不決或四處張望"},
	{"平易近人", "形容態度謙和、容易親近"},
	{"平步青雲", "比喻順利迅速地達到很高的地位"},
	{"弄巧成拙", "比喻本想取巧，結果反而把事情弄糟"},
	{"忐忑不安", "形容心神不定、心裡不安"},
	{"忍無可忍", "形容忍耐已經到了極限，無法再忍下去"},
	{"忠言逆耳", "比喻真誠勸告的話聽起來不順耳，但對人有益"},
	{"快馬加鞭", "比喻加快速度、加緊行動"},
	{"怒髮衝冠", "形容極度憤怒的樣子"},
	{"恍然大悟", "形容突然明白過來"},
	{"惟妙惟肖", "形容描摹或模仿得非常逼真傳神"},
	{"意氣風發", "形容精神振奮、氣概豪邁"},
	{"愛不釋手", "形容非常喜愛，捨不得放下"},
	{"打草驚蛇", "比喻做事不謹慎，反而讓對方有所警覺、防備"},
	{"拋磚引玉", "比喻先發表粗淺的意見，引出別人更好的見解"},
	{"拍案叫絕", "形容對事物讚賞到極點，忍不住拍桌叫好"},
	{"掩人耳目", "比喻用虛假的言行掩蓋事實真相，欺騙他人"},
	{"揚眉吐氣", "形容擺脫壓抑後心情舒暢"},
	{"明察秋毫", "形容目光敏銳，連極細微的事物都能看得清楚"},
	{"明知故犯", "形容明明知道不對，卻故意去做"},
	{"易如反掌", "比喻事情非常容易辦到"},
	{"星羅棋布", "形容數量眾多、分布廣泛，像星星和棋子一樣散布各處"},
	{"春風得意", "形容順利如意、心情愉快的樣子"},
	{"望而生畏", "形容看見了就心生畏懼"},
	{"望塵莫及", "比喻遠遠落後，怎麼追也追不上"},
	{"未雨綢繆", "比喻事先做好準備，防患於未然"},
	{"本末倒置", "比喻把事情的主次、輕重弄反了"},
	{"東山再起", "比喻失勢或失敗後重新恢復地位"},
	{"杯水車薪", "比喻力量太小，不足以解決問題"},
	{"歡天喜地", "形容非常高興、喜悅的樣子"},
	{"熱淚盈眶", "形容感動或激動到眼眶充滿淚水"},
	{"畫地自限", "比喻自己限制自己的能力或範圍，不求進步"},
	{"痛改前非", "形容徹底悔改過去的錯誤"},
	{"百發百中", "形容射箭或做事每次都能命中目標，非常準確"},
	{"眼高手低", "形容要求標準很高，但實際能力做不到"},
	{"破鏡重圓", "比喻夫妻失散或決裂後重新團圓、和好"},
	{"稱心如意", "形容非常符合自己的心意"},
	{"興高采烈", "形容情緒高昂、非常興奮愉快"},
	{"舉一反三", "比喻從一件事類推而知道其他許多事，觸類旁通"},
	{"膾炙人口", "比喻詩文或事物受到眾人喜愛、傳誦一時"},
	{"自食其力", "形容依靠自己的勞動來養活自己"},
	{"良藥苦口", "比喻誠懇的勸告雖然不好聽，卻對人有益"},
	{"藕斷絲連", "比喻表面上斷絕了關係，實際上仍有牽連"},
	{"見異思遷", "形容意志不堅定，看到別的事物就改變心意"},
	{"談笑風生", "形容言談風趣、氣氛熱絡愉快"},
	{"貪小失大", "比喻貪圖小利，卻因此損失更大的利益"},
	{"近水樓台", "比喻因位置或關係接近而優先得到便利、好處"},
	{"適得其反", "形容結果與希望的完全相反"},
	{"錦上添花", "比喻在美好的事物上再添加美好，使其更完美"},
	{"雪中送炭", "比喻在別人急需時給予及時的幫助"},
	{"鴉雀無聲", "形容非常安靜，一點聲音都沒有"},
}

// filteredIdiomList 只保留四個字的成語（idiomList 目前收錄的皆為四字，
// 此處過濾是為了避免未來誤加入非四字詞條時造成猜測長度矛盾）。
var filteredIdiomList = func() []idiomEntry {
	var list []idiomEntry
	for _, entry := range idiomList {
		if len([]rune(entry.Word)) == 4 {
			list = append(list, entry)
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
	hint        string
	guessLines  []string
	revealed    [4]bool
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
	sb.WriteString(fmt.Sprintf("🀄 **猜成語**（第 %d/%d 次機會）\n", len(g.guessLines), g.maxAttempts))
	sb.WriteString(fmt.Sprintf("📖 提示：%s\n\n", g.hint))
	for i, line := range g.guessLines {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, line))
	}
	switch {
	case g.over && g.won:
		sb.WriteString(fmt.Sprintf("\n🎉 恭喜猜中！答案就是「%s」。", g.target))
	case g.over:
		sb.WriteString(fmt.Sprintf("\n💥 次數用完了，答案是「%s」。", g.target))
	default:
		sb.WriteString(fmt.Sprintf(
			"\n輸入 `%sidiom <四字成語>` 繼續猜，或用 `%sidiom hint` 花一次機會換一個字，還有 %d 次機會。🟩 字和位置都對／🟨 字對位置錯／⬛ 成語中沒有這個字。",
			prefix, prefix, g.maxAttempts-len(g.guessLines)))
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
	case "hint":
		b.cmdIdiomHint(e)
	default:
		b.cmdIdiomGuess(e, trimmed)
	}
}

func (b *Bot) idiomUsage(e *events.MessageCreate) {
	prefix := b.cfg.CommandPrefix
	b.reply(e, fmt.Sprintf(
		"**猜成語指令**\n"+
			"`%sidiom new` - 開始新的猜成語（會先給一句釋義提示，共六次機會）\n"+
			"`%sidiom <四字成語>` - 送出一次猜測，例如 `%sidiom 一帆風順`\n"+
			"`%sidiom hint` - 花一次機會，讓機器人隨機公布一個字的位置與內容\n"+
			"`%sidiom show` - 重新顯示目前猜測紀錄\n"+
			"`%sidiom stop` - 結束目前的猜成語（會公布答案）",
		prefix, prefix, prefix, prefix, prefix, prefix))
}

func (b *Bot) cmdIdiomNew(e *events.MessageCreate) {
	if len(filteredIdiomList) == 0 {
		b.reply(e, "❌ 成語題庫是空的，無法開始遊戲。")
		return
	}
	entry := filteredIdiomList[rand.Intn(len(filteredIdiomList))]
	game := &IdiomGame{
		target:      entry.Word,
		hint:        entry.Hint,
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

// cmdIdiomHint 花玩家一次機會，隨機公布一個尚未提示過的字（位置與內容）。
func (b *Bot) cmdIdiomHint(e *events.MessageCreate) {
	game := b.idiom.Get(e.ChannelID)
	if game == nil {
		b.reply(e, fmt.Sprintf("目前沒有進行中的猜成語，用 `%sidiom new` 開始一局。", b.cfg.CommandPrefix))
		return
	}
	if game.over {
		b.reply(e, fmt.Sprintf("這局猜成語已經結束，用 `%sidiom new` 開始新的一局。", b.cfg.CommandPrefix))
		return
	}

	var candidates []int
	for i, done := range game.revealed {
		if !done {
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 0 {
		b.reply(e, "所有字都已經提示過了，請直接用 `idiom <四字成語>` 猜猜看。")
		return
	}

	idx := candidates[rand.Intn(len(candidates))]
	game.revealed[idx] = true
	targetRunes := []rune(game.target)
	game.guessLines = append(game.guessLines, fmt.Sprintf("🔎 提示：第 %d 個字是「%c」", idx+1, targetRunes[idx]))

	if len(game.guessLines) >= game.maxAttempts {
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
