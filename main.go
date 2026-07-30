package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("設定載入失敗: %v", err)
	}

	bot, err := NewBot(cfg)
	if err != nil {
		log.Fatalf("機器人初始化失敗: %v", err)
	}

	openCtx, openCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer openCancel()
	if err := bot.Open(openCtx); err != nil {
		log.Fatalf("無法連線至 Discord: %v", err)
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer closeCancel()
		bot.Close(closeCtx)
	}()

	log.Printf("機器人已啟動，於文字頻道 #%s 輸入指令、語音頻道「%s」播放音樂。按 Ctrl+C 結束。",
		cfg.TextChannelName, cfg.VoiceChannelName)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("正在關閉機器人...")
}
