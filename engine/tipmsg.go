package engine

import (
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
)

/*
@Date : 2020/4/28 0028 19:44
@Author : chenjunji
@Desc : 提示
*/

// BroadcastTipMsgById 广播提示
func BroadcastTipMsgById(tipMsgId uint32, params ...interface{}) {
	Broadcast(chatdef.CIWorld, 0, 5, 0, common.PackMsg(tipMsgId, params...), 0)
}

// CrossBroadcastTipMsgById 跨服广播提示
func CrossBroadcastTipMsgById(tipMsgId uint32, params ...interface{}) {
	if FightClientExistPredicate(base.SmallCrossServer) {
		rsp := common.PackMsg(tipMsgId, params...)
		buf, _ := pb3.Marshal(rsp)
		msg := &pb3.BroadcastTipMsgSt{
			Buf: buf,
		}
		CallFightSrvFunc(base.SmallCrossServer, sysfuncid.CrossBroadcastTipmsg, msg) // 发过去跨服广播
	} else {
		Broadcast(chatdef.CIWorld, 0, 5, 0, common.PackMsg(tipMsgId, params...), 0)
	}
}

func f2gBroadcastTipMsg(buf []byte) {
	msg := &pb3.BroadcastTipMsgSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("g2fBroadcastTipMsg %v", err)
		return
	}

	// 增加开服天数限制
	openDay := msg.GetOpenDay()
	if openDay > 0 && gshare.GetOpenServerDay() < openDay {
		return
	}

	BroadcastBuf(chatdef.CIWorld, 0, 5, 0, msg.Buf, msg.Level)
}

func f2gBroadcastCustomTipMsg(buf []byte) {
	msg := &pb3.BroadcastTipMsgSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("g2fBroadcastCustomTipMsg %v", err)
		return
	}

	// 增加开服天数限制
	openDay := msg.GetOpenDay()
	if openDay > 0 && gshare.GetOpenServerDay() < openDay {
		return
	}

	BroadcastBuf(chatdef.CIWorld, 0, 2, 208, msg.Buf, msg.Level)
}

func crossBroadcastProto(buf []byte) {
	msg := &pb3.CommonSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("crossBroadcastProto %v", err)
		return
	}

	protoH, protoL := uint16(msg.U32Param), uint16(msg.U32Param2)
	logger.LogTrace("crossBroadcastProto %d, %d", protoH, protoL)
	BroadcastBuf(chatdef.CIWorld, 0, protoH, protoL, msg.Buf, 0)
}

func redisAllServerBroadcast(buf []byte) {
	msg := &pb3.BroadcastTipMsgSt{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("g2fBroadcastTipMsg %v", err)
		return
	}
	BroadcastBuf(chatdef.CIWorld, 0, 5, 0, msg.Buf, 0)
}

func init() {
	RegisterSysCall(sysfuncid.FGBroadcastTipmsg, f2gBroadcastTipMsg)
	RegisterSysCall(sysfuncid.FGBroadcastCustomTipMsg, f2gBroadcastCustomTipMsg)
	RegisterSysCall(sysfuncid.C2GBroadcastProto, crossBroadcastProto)
}
