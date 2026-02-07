package mq

import (
	"errors"
	"jjyz/base/pb3"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

type Handler func(data []byte) error

var (
	HandlerIsNil    = errors.New("handler is nil")
	OpCodeIsInvalid = errors.New("opcode is invalid")
)

var (
	handlers = [pb3.GameServerNatsOpCode_Max]Handler{}
)

func RegisterMQHandler(opCode pb3.GameServerNatsOpCode, handler Handler) {
	if nil == handler {
		logger.LogStack("handler is nil! opcode=%s", opCode.String())
		return
	}
	if opCode > pb3.GameServerNatsOpCode_Max {
		logger.LogStack("mq opcode=%s is invalid", opCode.String())
		return
	}
	if nil != handlers[opCode] {
		logger.LogStack("mq opcode=%s is repeated", opCode.String())
		return
	}
	handlers[opCode] = handler
	logger.LogInfo("mqHandler %s registed", opCode.String())
}

func OnMQMessage(opCode pb3.GameServerNatsOpCode, data []byte) error {
	logger.LogInfo("on mq message, opCode:%s", opCode.String())
	if opCode > pb3.GameServerNatsOpCode_Max {
		logger.LogStack("mq opcode=%s is invalid", opCode.String())
		return OpCodeIsInvalid
	}

	var err error
	utils.ProtectRun(func() {
		if handler := handlers[opCode]; nil != handler {
			err = handler(data)
			return
		}
		err = HandlerIsNil
	})
	return err
}
