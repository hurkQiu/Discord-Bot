package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

// bulkDeleteMaxAge 是 Discord 批次刪除 API 的限制：超過 14 天的訊息無法批次刪除，只能逐則刪除。
const bulkDeleteMaxAge = 14 * 24 * time.Hour

// hasManageMessages 檢查下指令的成員是否在該頻道擁有「管理訊息」權限。
func (b *Bot) hasManageMessages(e *events.MessageCreate) bool {
	if e.Message.Member == nil {
		return false
	}
	channel, ok := b.client.Caches.Channel(e.ChannelID)
	if !ok {
		return false
	}
	perms := b.client.Caches.MemberPermissionsInChannel(channel, *e.Message.Member)
	return perms.Has(discord.PermissionManageMessages)
}

func (b *Bot) cmdClear(e *events.MessageCreate, args string) {
	if !b.hasManageMessages(e) {
		b.reply(e, "❌ 你需要「管理訊息」權限才能清除頻道訊息。")
		return
	}

	if strings.ToLower(strings.TrimSpace(args)) != "confirm" {
		b.reply(e, fmt.Sprintf(
			"⚠️ 這會刪除**這個頻道內全部的訊息**，且無法復原。確定要清除的話，請輸入 `%sclear confirm`。",
			b.cfg.CommandPrefix,
		))
		return
	}

	b.reply(e, "🧹 開始清除頻道訊息，請稍候...")

	deleted, err := b.purgeChannel(e.ChannelID)
	if err != nil {
		b.reply(e, fmt.Sprintf("❌ 清除過程中發生錯誤（已刪除 %d 則）：%v", deleted, err))
		return
	}
	b.reply(e, fmt.Sprintf("✅ 已清除 %d 則訊息。", deleted))
}

// purgeChannel 刪除頻道內所有訊息：14 天內的訊息用批次刪除 API，較舊的訊息則逐則刪除
// （Discord 批次刪除 API 不支援超過 14 天的訊息）。
func (b *Bot) purgeChannel(channelID snowflake.ID) (int, error) {
	deleted := 0
	var before snowflake.ID
	cutoff := time.Now().Add(-bulkDeleteMaxAge)

	for {
		messages, err := b.client.Rest.GetMessages(channelID, 0, before, 0, 100)
		if err != nil {
			return deleted, err
		}
		if len(messages) == 0 {
			break
		}

		var recent, old []snowflake.ID
		for _, m := range messages {
			if m.CreatedAt.After(cutoff) {
				recent = append(recent, m.ID)
			} else {
				old = append(old, m.ID)
			}
		}
		// Discord 批次刪除 API 至少要 2 則，剩單一則就改用逐則刪除。
		if len(recent) == 1 {
			old = append(old, recent[0])
			recent = nil
		}

		if len(recent) > 0 {
			if err := b.client.Rest.BulkDeleteMessages(channelID, recent); err != nil {
				return deleted, err
			}
			deleted += len(recent)
		}
		for _, id := range old {
			if err := b.client.Rest.DeleteMessage(channelID, id); err != nil {
				return deleted, err
			}
			deleted++
			time.Sleep(300 * time.Millisecond)
		}

		before = messages[len(messages)-1].ID
		if len(messages) < 100 {
			break
		}
	}

	return deleted, nil
}
