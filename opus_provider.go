package main

import (
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jonas747/ogg"
)

// opusSource 透過 ffmpeg 將串流網址即時轉碼成 Ogg/Opus，
// 並實作 disgo 的 voice.OpusFrameProvider 介面（ProvideOpusFrame + Close）。
type opusSource struct {
	cmd *exec.Cmd
	dec *ogg.PacketDecoder

	skip   int // 略過 ogg/opus 串流開頭的 metadata 封包
	frames atomic.Int64

	mu     sync.Mutex
	cond   *sync.Cond
	paused bool
	closed bool

	done     chan struct{}
	doneOnce sync.Once
}

// newOpusSource 啟動 ffmpeg 子行程，將 streamURL 轉碼為 48kHz/stereo 的 Ogg/Opus 串流。
func newOpusSource(streamURL string) (*opusSource, error) {
	args := []string{
		"-reconnect", "1",
		"-reconnect_at_eof", "1",
		"-reconnect_streamed", "1",
		"-reconnect_delay_max", "2",
		"-i", streamURL,
		"-map", "0:a",
		"-acodec", "libopus",
		"-f", "ogg",
		"-vbr", "on",
		"-compression_level", "10",
		"-ar", "48000",
		"-ac", "2",
		"-b:a", "96000",
		"-application", "audio",
		"-frame_duration", "20",
		"-packet_loss", "1",
		"pipe:1",
	}

	cmd := exec.Command("ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	s := &opusSource{
		cmd:  cmd,
		dec:  ogg.NewPacketDecoder(ogg.NewDecoder(stdout)),
		skip: 2, // ogg/opus 串流前兩個封包是 OpusHead / OpusTags，非音訊資料
		done: make(chan struct{}),
	}
	s.cond = sync.NewCond(&s.mu)
	return s, nil
}

// SetPaused 暫停或繼續供應音框；暫停時 ProvideOpusFrame 會阻塞，ffmpeg 行程持續執行但不消耗輸出。
func (s *opusSource) SetPaused(paused bool) {
	s.mu.Lock()
	s.paused = paused
	s.mu.Unlock()
	s.cond.Broadcast()
}

// ProvideOpusFrame 實作 voice.OpusFrameProvider。
func (s *opusSource) ProvideOpusFrame() ([]byte, error) {
	s.mu.Lock()
	for s.paused && !s.closed {
		s.cond.Wait()
	}
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return nil, io.EOF
	}

	packet, _, err := s.dec.Decode()
	if err != nil {
		s.markDone()
		return nil, io.EOF
	}
	if s.skip > 0 {
		s.skip--
		return s.ProvideOpusFrame()
	}
	s.frames.Add(1)
	return packet, nil
}

// Done 會在串流結束（正常播畢、發生錯誤或被 Close）時關閉。
func (s *opusSource) Done() <-chan struct{} {
	return s.done
}

// PlaybackPosition 回傳目前已播放的時間長度（每個 opus 音框為 20ms）。
func (s *opusSource) PlaybackPosition() time.Duration {
	return time.Duration(s.frames.Load()) * 20 * time.Millisecond
}

func (s *opusSource) markDone() {
	s.doneOnce.Do(func() { close(s.done) })
}

// Close 實作 voice.OpusFrameProvider，終止 ffmpeg 行程並釋放資源。
func (s *opusSource) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.cond.Broadcast()

	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	_ = s.cmd.Wait()
	s.markDone()
}
