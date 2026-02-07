/**
 * @Author: zjj
 * @Desc: 答题活动
 * @Date: 2023/11/6
 */

package activity

import (
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
)

var (
	svrType base.ServerType
)

// 选择答案
func c2sChoiceAnswer(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_31_91
	if err := msg.UnPackPb3Msg(&req); nil != err {
		logger.LogError("err:%v", err)
		return err
	}

	if svrType == 0 {
		return neterror.InternalError("not found %d srv type", svrType)
	}

	err := engine.CallFightSrvFunc(svrType, sysfuncid.G2FActAnswerChoiceAnswer, &pb3.ActAnswerChoiceAnswerSt{
		PlayerId: player.GetId(),
		Idx:      req.Idx,
		Player:   convertActAnswerJoinPlayer(player),
	})
	if err != nil {
		player.LogError("err:%v", err)
		return err
	}
	return nil
}

// 请求排行榜
func c2sReqAnswerRank(player iface.IPlayer, _ *base.Message) error {
	if svrType == 0 {
		return neterror.InternalError("not found %d srv type", svrType)
	}
	joinPlayer := convertActAnswerJoinPlayer(player)
	err := engine.CallFightSrvFunc(svrType, sysfuncid.G2FActAnswerReqAnswerRank, joinPlayer)
	if err != nil {
		player.LogError("err:%v", err)
		return err
	}
	return nil
}

// 转换
func convertActAnswerJoinPlayer(player iface.IPlayer) *pb3.ActAnswerJoinPlayer {
	return &pb3.ActAnswerJoinPlayer{
		ActorId:   player.GetId(),
		PfId:      engine.GetPfId(),
		SrvId:     engine.GetServerId(),
		Name:      player.GetName(),
		Head:      player.GetHead(),
		HeadFrame: player.GetHeadFrame(),
		Lv:        player.GetLevel(),
	}
}

// 选择答案成功
func f2gActAnswerChoiceAnswer(buf []byte) {
	var req pb3.ActAnswerRecordRet
	err := pb3.Unmarshal(buf, &req)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
	player := manager.GetPlayerPtrById(req.PlayerId)
	if player != nil {
		player.TriggerQuestEvent(custom_id.QttAnswerCompleteTimes, 0, 1)
		player.TriggerQuestEvent(custom_id.QttPartActAnswerChoice, 0, 1)
	}
	channel := chatdef.CIWorld
	if svrType != base.LocalFightServer {
		channel = chatdef.CICrossChat
	}
	player.ChannelChat(&pb3.C2S_5_1{
		Msg:     req.Msg,
		Channel: uint32(channel),
	}, false)
}

// 重新推送答题信息
func g2fActAnswerRePushInfo(player iface.IPlayer) {
	if svrType == 0 {
		player.LogDebug("答题活动未开启")
		return
	}
	joinPlayer := convertActAnswerJoinPlayer(player)
	err := engine.CallFightSrvFunc(svrType, sysfuncid.G2FActAnswerRePushInfo, joinPlayer)
	if err != nil {
		player.LogError("err:%v", err)
		return
	}
}

// 活动开启
func f2gActAnswerStart(buf []byte) {
	var req pb3.CommonSt
	err := pb3.Unmarshal(buf, &req)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
	svrType = base.ServerType(req.U32Param3)
}

// 活动关闭
func f2gActAnswerClose(_ []byte) {
	svrType = 0
}

func init() {
	net.RegisterProto(31, 91, c2sChoiceAnswer)
	net.RegisterProto(31, 92, c2sReqAnswerRank)

	engine.RegisterSysCall(sysfuncid.ActAnswerStart, f2gActAnswerStart)
	engine.RegisterSysCall(sysfuncid.ActAnswerClose, f2gActAnswerClose)
	engine.RegisterSysCall(sysfuncid.F2GActAnswerChoiceAnswer, f2gActAnswerChoiceAnswer)
	event.RegActorEventL(custom_id.AeAfterLogin, func(player iface.IPlayer, args ...interface{}) {
		g2fActAnswerRePushInfo(player)
	})
	event.RegActorEventL(custom_id.AeReconnect, func(player iface.IPlayer, args ...interface{}) {
		g2fActAnswerRePushInfo(player)
	})

}
