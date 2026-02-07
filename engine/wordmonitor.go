package engine

import (
	"jjyz/base/custom_id"
	"jjyz/base/wordmonitor"
	"jjyz/gameserver/engine/wordmonitoroption"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"

	"github.com/gzjjyz/logger"
)

type WordMonitorCallBack func(word *wordmonitor.Word) error

var WordMonitorDispatcher = make(map[wordmonitor.OpCode]WordMonitorCallBack)

func RegWordMonitorOpCodeHandler(code wordmonitor.OpCode, cb WordMonitorCallBack) {
	if nil == cb {
		logger.LogStack("RegWordMonitorOpCodeHandler cb func is nil")
		return
	}
	if _, exist := WordMonitorDispatcher[code]; exist {
		logger.LogStack("RegWordMonitorOpCodeHandler repeated. code=%d", code)
		return
	}
	WordMonitorDispatcher[code] = cb
}

func SendWordMonitor(typ wordmonitor.WordType, opcode wordmonitor.OpCode, content string, opts ...wordmonitoroption.Option) {
	word := &wordmonitor.Word{
		Type:    typ,
		OpCode:  opcode,
		Content: content,
	}

	for _, opt := range opts {
		opt(word)
	}
	gshare.SendSDkMsg(custom_id.GMsgWordMonitor, word)
}

func onWordMonitorRet(args ...interface{}) {
	if len(args) < 1 {
		return
	}
	word, ok := args[0].(*wordmonitor.Word)
	if !ok {
		return
	}
	if cb := WordMonitorDispatcher[word.OpCode]; nil != cb {
		if err := cb(word); nil != err {
			logger.LogError("word monitor ret handler error! %v", err)
		}
	}
}

func init() {
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		gshare.RegisterGameMsgHandler(custom_id.GMsgWordMonitorRet, onWordMonitorRet)
	})
}
