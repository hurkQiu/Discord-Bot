package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config 保存機器人執行所需的所有設定值，皆透過環境變數 (或 .env 檔) 設定。
type Config struct {
	BotToken         string // Discord Bot Token
	CommandPrefix    string // 文字指令前綴，例如 "!"
	TextChannelName  string // 允許下指令的文字頻道名稱
	VoiceChannelName string // 機器人加入播放音樂的語音頻道名稱
	YtDlpPath        string // yt-dlp 執行檔路徑或名稱
}

// LoadConfig 讀取 .env (若存在) 與環境變數並回傳設定。
func LoadConfig() (*Config, error) {
	// .env 不存在也沒關係，直接改用系統環境變數
	_ = godotenv.Load()

	token := os.Getenv("DISCORD_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("環境變數 DISCORD_BOT_TOKEN 未設定，請於 .env 中提供 Discord Bot Token")
	}

	cfg := &Config{
		BotToken:         token,
		CommandPrefix:    envOrDefault("COMMAND_PREFIX", "!"),
		TextChannelName:  envOrDefault("TEXT_CHANNEL_NAME", "music"),
		VoiceChannelName: envOrDefault("VOICE_CHANNEL_NAME", "music"),
		YtDlpPath:        envOrDefault("YTDLP_PATH", "yt-dlp"),
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
