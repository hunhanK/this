package wordmonitoroption

import (
	wm "github.com/gzjjyz/wordmonitor"
	"jjyz/base/wordmonitor"
)

type Option func(*wordmonitor.Word)

func WithRawData(data interface{}) Option {
	return func(word *wordmonitor.Word) {
		word.Data = data
	}
}

func WithDitchId(id uint32) Option {
	return func(word *wordmonitor.Word) {
		word.DitchId = id
	}
}

func WithPlayerId(id uint64) Option {
	return func(word *wordmonitor.Word) {
		word.PlayerId = id
	}
}

func WithCommonData(data *wm.CommonData) Option {
	return func(word *wordmonitor.Word) {
		word.ChatBaseData = data
	}
}
