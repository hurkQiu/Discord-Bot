package main

import (
	"sync"

	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

// GameManager 管理每個頻道各自獨立的小遊戲對局（踩地雷、熄燈、數織、猜成語共用）。
type GameManager[T any] struct {
	mu    sync.Mutex
	games map[snowflake.ID]*T
}

func NewGameManager[T any]() *GameManager[T] {
	return &GameManager[T]{games: make(map[snowflake.ID]*T)}
}

func (m *GameManager[T]) Get(channelID snowflake.ID) *T {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.games[channelID]
}

func (m *GameManager[T]) Set(channelID snowflake.ID, game *T) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.games[channelID] = game
}

func (m *GameManager[T]) Delete(channelID snowflake.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.games, channelID)
}

// deleteCommandMessage 刪除玩家送出的指令訊息，避免每下一次指令就在頻道多留一則訊息。
func (b *Bot) deleteCommandMessage(e *events.MessageCreate) {
	_ = b.client.Rest.DeleteMessage(e.ChannelID, e.Message.ID)
}
