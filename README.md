# Discord-Bot

以 Go 撰寫的 Discord 音樂機器人。在指定的文字頻道下指令，機器人會搜尋 YouTube 並在指定的語音頻道播放，並支援依歌手或語言自動接歌的電台模式。

## 功能

- `!play <關鍵字或 YouTube 網址>`：搜尋 YouTube 並播放，若已有歌曲在播放則加入佇列
- `!skip`：跳過目前歌曲
- `!pause` / `!resume`：暫停 / 繼續播放
- `!stop`：停止播放、清空佇列、關閉電台模式並離開語音頻道
- `!queue`：顯示目前播放與佇列
- `!nowplaying`：顯示目前播放進度
- `!radio artist <歌手名稱>`：啟動該歌手的電台，找到頻道後隨機播放並在佇列剩 2 首時自動補歌，持續播放直到 `!stop`
- `!radio lang <日文|英文|韓文|中文>`：啟動該語言的熱門歌曲電台，機制同上
- `!help`：顯示指令列表

機器人只會回應**指定文字頻道**（預設頻道名稱 `music`）中的指令，並自動加入**指定語音頻道**（預設頻道名稱 `music`）播放。

### 電台模式的限制（請詳閱）

- **歌手電台**：透過搜尋「`<歌手> - Topic`」找出該歌手在 YouTube 上的官方頻道，再從其上傳清單中隨機挑歌。對於有明確單一官方頻道的歌手（多數西洋歌手、日本歌手）效果不錯；但對於像「初音未來」這種由多位不同製作人共用的虛擬歌手/角色名稱，沒有單一官方頻道可以代表「全部歌曲」，準確度會下降（程式會優先挑「頻道名稱與查詢字串吻合」的結果，但仍可能找不到完全對應的頻道）。頻道上傳清單也常混雜演唱會公告、周邊商品、預告片等非歌曲內容，程式已用標題關鍵字與時長（<45 秒視為非完整歌曲）過濾，但無法保證 100% 準確。
- **語言電台**：YouTube Music 官方排行榜網址目前已失效（實測回傳 404），因此改用「該語言道地的熱門歌曲搜尋詞」持續搜尋（例如日文用「邦楽 人気曲」），效果取決於 YouTube 搜尋排序，並非真正的官方排行榜。
- 兩種電台都只在**同一次電台場次**內避免短時間重複播放同一首歌；候選歌曲池播完一輪後會清空重複紀錄、重新循環。

## 事前準備

1. **Go**：1.24 以上（因 disgo 函式庫要求）
2. **ffmpeg**：需可在 PATH 中執行 `ffmpeg`（需內建 libopus 編碼支援）
3. **yt-dlp**：需可在 PATH 中執行 `yt-dlp`

   > ⚠️ **請務必先更新 yt-dlp 到最新版本**：YouTube 常更動其網站邏輯，過舊的 yt-dlp（例如本機曾偵測到的 `2024.10.07`）會出現 `Requested format is not available` 等錯誤，導致完全無法解析一般影片。請以系統管理員權限執行：
   >
   > ```powershell
   > yt-dlp -U
   > ```
   >
   > 之後建議定期更新（例如每週執行一次）。

4. **DAVE 語音加密（CGO 編譯環境）**：Discord 自 2026 年 3 月起強制所有語音連線必須支援 DAVE（E2EE）加密協議，本專案透過 [disgo](https://github.com/disgoorg/disgo) + [godave](https://github.com/disgoorg/godave)（Discord 官方 libdave 的 Go 綁定）實作，需要：
   - **MinGW-w64 GCC**（CGO 編譯需要 C 編譯器）。Windows 上可用 winget 安裝：
     ```powershell
     winget install --id BrechtSanders.WinLibs.POSIX.UCRT -e
     ```
   - **libdave 原生函式庫**：執行 godave 官方安裝腳本（會從 `discord/libdave` 官方 GitHub Releases 下載預編譯的 `libdave.dll`）：
     ```powershell
     Set-ExecutionPolicy RemoteSigned -Scope Process
     Invoke-WebRequest -Uri "https://raw.githubusercontent.com/disgoorg/godave/master/scripts/libdave_install.ps1" -OutFile libdave_install.ps1
     .\libdave_install.ps1 v1.1.0
     ```
   - 上述兩個安裝步驟都會自動把必要路徑寫入使用者層級的 `PATH`／`PKG_CONFIG_PATH`。此外還需手動設定 **`PKG_CONFIG`** 環境變數指向 WinLibs 附的 `pkgconf.exe`（因為 cgo 預設呼叫的指令名稱是 `pkg-config`，但 WinLibs 只提供 `pkgconf.exe`）：
     ```powershell
     [Environment]::SetEnvironmentVariable("PKG_CONFIG", "<WinLibs 安裝路徑>\mingw64\bin\pkgconf.exe", "User")
     ```
   - 設定完成後**請開一個新的終端機視窗**讓環境變數生效，之後就能直接 `go build` / `go run .`，不需要每次手動設變數。
   - 編譯出的執行檔在執行時需要能找到 `libdave.dll`：只要 libdave 安裝路徑（`%LOCALAPPDATA%\libdave\bin`）在 PATH 中即可（安裝腳本已自動加入），或者也可以把該資料夾內的 `libdave.dll` 複製到與 `bot.exe` 相同的目錄下。

5. **Discord Bot**：已在 [Discord Developer Portal](https://discord.com/developers/applications) 建立好應用程式與 Bot，並取得 Bot Token。
   - 在 Bot 設定頁開啟 **MESSAGE CONTENT INTENT**（此為 Privileged Intent，必須手動開啟，否則機器人讀不到指令內容）。
   - 邀請機器人加入伺服器時，至少需要以下權限：檢視頻道、傳送訊息、嵌入連結、連接語音頻道、在語音頻道發言。
   - 於伺服器中建立文字頻道與語音頻道，名稱皆為 `music`（或依 `.env` 自訂名稱）。

## 安裝與設定

```powershell
git clone <此專案位址>
cd Discord-Bot
copy .env.example .env
```

編輯 `.env`，填入你的 Bot Token：

```
DISCORD_BOT_TOKEN=你的BotToken
COMMAND_PREFIX=!
TEXT_CHANNEL_NAME=music
VOICE_CHANNEL_NAME=music
YTDLP_PATH=yt-dlp
```

## 執行

```powershell
go run .
```

或編譯後執行：

```powershell
go build -o bot.exe .
./bot.exe
```

按 `Ctrl+C` 結束。

## 疑難排解

- **機器人沒反應**：確認訊息是否發在 `TEXT_CHANNEL_NAME` 指定的文字頻道，且已開啟 MESSAGE CONTENT INTENT。
- **`!play` 一直回報找不到歌曲**：多半是 yt-dlp 版本過舊，執行 `yt-dlp -U` 更新（需系統管理員權限）。
- **加入語音頻道失敗**：確認伺服器中確實存在名稱為 `VOICE_CHANNEL_NAME` 的語音頻道，且機器人有「連接」「發言」權限。
- **機器人能進語音頻道但完全沒聲音，主控台出現 `4017: E2EE/DAVE protocol required`**：代表編譯時沒有正確啟用 DAVE（CGO 未生效或 `voice.WithDaveSessionCreateFunc` 未設定），請確認「事前準備」第 4 點的 GCC / libdave / `PKG_CONFIG` 都已正確安裝與設定，並用**新開的終端機**重新 `go build`。
- **`go build` 出現 `pkg-config: exec: "pkg-config": executable file not found`**：`PKG_CONFIG` 環境變數沒設定或路徑錯誤，請參考「事前準備」第 4 點重新設定並開新終端機。
- **執行時找不到 `libdave.dll`**：確認 `%LOCALAPPDATA%\libdave\bin` 在 PATH 中，或直接把 `libdave.dll` 複製到 `bot.exe` 旁邊。
- **`!radio artist` 找到不相關的歌手/頻道**：多半是該名稱在 YouTube 上沒有單一明確的官方頻道（見上方「電台模式的限制」），可換更精確的官方藝名再試一次。
- **聲音斷斷續續**：通常是機器人所在主機網路頻寬或延遲問題，可嘗試調整 `opus_provider.go` 中 ffmpeg 參數的 `-b:a`（位元率）。
