/**
 * @Author: zjj
 * @Date: 2024/9/23
 * @Desc: 协助系统
**/

package actorsystem

import (
	"encoding/json"
	"fmt"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/series"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/assistancemgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils"
)

type AssistanceSys struct {
	Base
}

func (s *AssistanceSys) getData() *pb3.AssistanceData {
	data := s.GetBinaryData()
	if data.AssistanceData == nil {
		data.AssistanceData = &pb3.AssistanceData{}
	}
	return data.AssistanceData
}

func (s *AssistanceSys) s2cInfo() {
	s.SendProto3(67, 30, &pb3.S2C_67_30{
		Data: s.getData(),
	})
}

func (s *AssistanceSys) OnReconnect() {
	s.s2cInfo()
}

func (s *AssistanceSys) OnLogin() {
	s.s2cInfo()
}

func (s *AssistanceSys) OnOpen() {
	s.s2cInfo()
}

func (s *AssistanceSys) c2sAsk(msg *base.Message) error {
	var req pb3.C2S_67_32
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}

	data := s.getData()
	owner := s.GetOwner()

	assistFightingConf := jsondata.GetAssistanceConf()
	if assistFightingConf == nil {
		return neterror.ConfNotFoundError("not found assist fighting conf")
	}

	cd := assistFightingConf.Cd

	lastAskAssistanceAt := data.LastAskAssistanceAt
	nowSec := time_util.NowSec()
	if req.MonsterId == 0 {
		return neterror.ParamsInvalidError("not monsterId")
	}

	if lastAskAssistanceAt > nowSec {
		return neterror.ParamsInvalidError("now %d, lastAskAssistanceAt %d", nowSec, lastAskAssistanceAt)
	}

	if cd != 0 && nowSec-lastAskAssistanceAt < cd {
		return neterror.ParamsInvalidError("now %d, lastAskAssistanceAt %d, cd %d", nowSec, lastAskAssistanceAt, cd)
	}

	// 如果是同一个目标 广播更新时间就行
	info := assistancemgr.GetAssistanceInfoByActorId(owner.GetId())
	if info != nil && info.MonsterId == req.MonsterId {
		info.CreatedAt = nowSec
		data.LastAskAssistanceAt = nowSec
		s.SendProto3(67, 32, &pb3.S2C_67_32{
			LastAskAssistanceAt: nowSec,
			Info:                info,
		})
		s.bro(info.ActorId, info.GuildId, 67, 33, &pb3.S2C_67_33{
			Info: info,
		})
		return nil
	}

	// 构造请求协助记录
	hdl, err := series.AllocSeries()
	if err != nil {
		return neterror.Wrap(err)
	}

	actorId := owner.GetId()
	var newEntry = &pb3.AssistanceInfo{
		Hdl:        hdl,
		ActorId:    actorId,
		CreatedAt:  nowSec,
		MonsterId:  req.MonsterId,
		GuildId:    req.GuildId,
		Name:       owner.GetName(),
		MonsterHdl: req.MonsterHdl,
	}

	// 先记录 求助失败再说
	data.LastAskAssistanceAt = nowSec
	err = owner.CallActorFunc(actorfuncid.G2FChangeAssistanceTarget, newEntry)
	if err != nil {
		return neterror.Wrap(err)
	}
	return nil
}

func (s *AssistanceSys) CompileTeam() bool {
	data := s.getData()
	owner := s.GetOwner()
	conf := jsondata.GetAssistanceConf()
	if conf == nil {
		owner.LogError("not found assist fighting conf")
		return false
	}

	if data.AssistanceTimes >= conf.Times {
		owner.LogWarn("%d %d is reach", data.AssistanceTimes, conf.Times)
		return false
	}

	data.AssistanceTimes += 1

	if len(conf.Awards) != 0 {
		engine.GiveRewards(owner, conf.Awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogCompileAssistanceTeam})
	}

	s.SendProto3(67, 41, &pb3.S2C_67_41{
		AssistanceTimes: data.AssistanceTimes,
		AwardList:       jsondata.StdRewardVecToPb3RewardVec(conf.Awards),
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogCompileAssistanceTeam, &pb3.LogPlayerCounter{
		NumArgs: uint64(data.AssistanceTimes),
	})
	return true
}

func (s *AssistanceSys) Compile(hdl uint64, isAsk bool, actorIds ...uint64) {
	data := s.getData()
	owner := s.GetOwner()
	conf := jsondata.GetAssistanceConf()
	if conf == nil {
		owner.LogError("not found assist fighting conf")
		return
	}

	entry := assistancemgr.GetAssistanceInfo(hdl)
	if entry == nil {
		owner.LogWarn("not found %d assist fighting info", hdl)
		return
	}

	var logId pb3.LogId
	var awards jsondata.StdRewardVec

	switch {
	case isAsk:
		logId = pb3.LogId_LogCompileAssistance
		if data.AskAssistanceTimes < conf.AskTimes {
			data.AskAssistanceTimes += 1
			awards = conf.AskAwards
		}
	default:
		logId = pb3.LogId_LogCompileAssistanceTarget
		if data.AssistanceTimes < conf.Times {
			data.AssistanceTimes += 1
			awards = conf.Awards
		}
	}

	if len(awards) != 0 {
		engine.GiveRewards(owner, awards, common.EngineGiveRewardParam{LogId: logId})
	}

	var playerList []*pb3.PlayerDataBase
	for _, actorId := range actorIds {
		getData := manager.GetData(actorId, gshare.ActorDataBase)
		if getData != nil {
			baseData, ok := getData.(*pb3.PlayerDataBase)
			if ok {
				playerList = append(playerList, baseData)
			}
		}
	}

	s.SendProto3(67, 34, &pb3.S2C_67_34{
		AssistanceTimes:    data.AssistanceTimes,
		AskAssistanceTimes: data.AskAssistanceTimes,

		Info:       entry,
		PlayerList: playerList,
	})

	if isAsk {
		assistancemgr.DelAssistanceInfo(hdl)
		// 广播最新的求助信息
		s2cAssistanceList(nil)
	}

	logworker.LogPlayerBehavior(owner, logId, &pb3.LogPlayerCounter{
		NumArgs: uint64(data.AssistanceTimes),
		StrArgs: fmt.Sprintf("%d_%v", hdl, isAsk),
	})
}

func (s *AssistanceSys) CancelAssistance() {
	hdl := s.owner.GetExtraAttr(attrdef.AskAssistance)
	if hdl == 0 {
		hdl = s.owner.GetExtraAttr(attrdef.HelpAssistance)
	}

	if hdl > 0 {
		err := s.owner.CallActorFunc(actorfuncid.G2FLeaveAssistance, &pb3.CommonSt{U64Param: uint64(hdl)})
		if err != nil {
			s.LogError("err: %v", err)
		}
	}
}

func (s *AssistanceSys) c2sEnter(msg *base.Message) error {
	var req pb3.C2S_67_35
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}

	entry := assistancemgr.GetAssistanceInfo(req.Hdl)
	if entry == nil {
		return neterror.ParamsInvalidError("not found %d assist fighting info", req.Hdl)
	}

	fbConf := jsondata.GetFbConf(entry.FbId)
	if fbConf == nil {
		return neterror.ParamsInvalidError("not found %d fb info", entry.FbId)
	}

	// 同仙盟校验
	if entry.GuildId != 0 && s.GetOwner().GetGuildId() != entry.GuildId {
		s.GetOwner().SendTipMsg(tipmsgid.TpGuildNotContainSelf)
		return neterror.ParamsInvalidError("not same %d guild", entry.GuildId)
	}

	owner := s.GetOwner()
	// 玩家当前不在这个副本中
	if !(owner.GetFbId() == entry.FbId && owner.GetSceneId() == entry.SceneId) {
		switch fbConf.HdlId {
		case fubendef.EnterWorldBoss:
			obj := owner.GetSysObj(sysdef.SiWorldBoss)
			if obj == nil || !obj.IsOpen() {
				return neterror.SysNotExistError("%d not open", sysdef.SiWorldBoss)
			}
			err = obj.(*WorldBossSys).enter(entry.SceneId)
		case fubendef.EnterMythicalBeastLand:
			obj := owner.GetSysObj(sysdef.SiMythicalBeastLand)
			if obj == nil || !obj.IsOpen() {
				return neterror.SysNotExistError("%d not open", sysdef.SiMythicalBeastLand)
			}
			err = obj.(*MythicalBeastLandSys).enter(entry.SceneId, !entry.IsCross)
		}
	}
	if err != nil {
		return neterror.InternalError("enter fight srv failed err: %s", err)
	}

	err = owner.CallActorFunc(actorfuncid.G2FJoinAssistanceTarget, entry)
	if err != nil {
		return neterror.InternalError("enter fight srv failed err: %s", err)
	}

	return nil
}

func (s *AssistanceSys) c2sList(_ *base.Message) error {
	s2cAssistanceList(s.owner)
	return nil
}

func (s *AssistanceSys) c2sCancel(msg *base.Message) error {
	var req pb3.C2S_67_37
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}
	err = s.GetOwner().CallActorFunc(actorfuncid.G2FLeaveAssistance, &pb3.CommonSt{
		U64Param: req.Hdl,
	})
	if err != nil {
		return neterror.InternalError("enter fight srv failed err: %s", err)
	}
	return nil
}

func (s *AssistanceSys) bro(actorId, guildId uint64, sysId, cmdId uint16, msg pb3.Message) {
	buffer, err := pb3.Marshal(msg)
	if err != nil {
		s.LogError("assistanceSys bro err:%v", err)
		return
	}
	manager.AllOnlinePlayerDo(func(actor iface.IPlayer) {
		if guildId != 0 && guildId != actor.GetGuildId() {
			return
		}
		if actor.GetId() == actorId {
			return
		}
		actor.SendProtoBuffer(sysId, cmdId, buffer)
	})
}

func s2cAssistanceList(player iface.IPlayer) {
	if player != nil {
		resp := &pb3.S2C_67_31{
			List: assistancemgr.GetListByWorldAndGuild(player.GetGuildId()),
		}
		player.SendProto3(67, 31, resp)
		return
	}
	manager.AllOnlinePlayerDo(func(actor iface.IPlayer) {
		resp := &pb3.S2C_67_31{
			List: assistancemgr.GetListByWorldAndGuild(actor.GetGuildId()),
		}
		actor.SendProto3(67, 31, resp)
	})
}

func onAssistanceNewDay(player iface.IPlayer, _ ...interface{}) {
	obj := player.GetSysObj(sysdef.SiAssistance)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys := obj.(*AssistanceSys)
	data := sys.getData()
	data.AskAssistanceTimes = 0
	data.AssistanceTimes = 0
}

func onPlayerJoinTeam(player iface.IPlayer, _ ...interface{}) {
	obj := player.GetSysObj(sysdef.SiAssistance)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys := obj.(*AssistanceSys)
	sys.CancelAssistance()
}

func f2gChangeAssistanceTargetRet(player iface.IPlayer, buf []byte) {
	obj := player.GetSysObj(sysdef.SiAssistance)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys := obj.(*AssistanceSys)

	var newEntry pb3.AssistanceInfo
	err := pb3.Unmarshal(buf, &newEntry)
	if err != nil {
		player.LogError("err:%v", err)
		return
	}

	data := sys.getData()
	assistancemgr.AppendNewAssistanceInfo(newEntry.ActorId, &newEntry)
	player.SendProto3(67, 32, &pb3.S2C_67_32{
		LastAskAssistanceAt: data.LastAskAssistanceAt,
		Info:                &newEntry,
	})

	// 广播新增一个协助
	sys.bro(newEntry.ActorId, newEntry.GuildId, 67, 33, &pb3.S2C_67_33{
		Info: &newEntry,
	})

	marshal, _ := json.Marshal(&newEntry)
	logworker.LogPlayerBehavior(player, pb3.LogId_LogAskAssistance, &pb3.LogPlayerCounter{
		NumArgs: uint64(data.AskAssistanceTimes),
		StrArgs: string(marshal),
	})
}

func f2gDelAssistanceTargetRet(player iface.IPlayer, buf []byte) {
	obj := player.GetSysObj(sysdef.SiAssistance)
	if obj == nil || !obj.IsOpen() {
		return
	}

	var info pb3.CommonSt
	err := pb3.Unmarshal(buf, &info)
	if err != nil {
		player.LogError("err:%v", err)
		return
	}

	assistancemgr.DelAssistanceInfo(info.U64Param)
	player.SendProto3(67, 37, &pb3.S2C_67_37{
		Hdl: info.U64Param,
	})
	s2cAssistanceList(nil)
}

func f2gJoinAssistanceTargetRet(player iface.IPlayer, buf []byte) {
	obj := player.GetSysObj(sysdef.SiAssistance)
	if obj == nil || !obj.IsOpen() {
		return
	}

	var entry pb3.AssistanceInfo
	err := pb3.Unmarshal(buf, &entry)
	if err != nil {
		player.LogError("err:%v", err)
		return
	}

	player.SendProto3(67, 35, &pb3.S2C_67_35{
		Hdl:  entry.Hdl,
		Info: &entry,
	})

	marshal, _ := json.Marshal(&entry)
	logworker.LogPlayerBehavior(player, pb3.LogId_LogJoinAssistance, &pb3.LogPlayerCounter{
		NumArgs: entry.Hdl,
		StrArgs: string(marshal),
	})
}

func f2gLeaveAssistanceRet(player iface.IPlayer, buf []byte) {
	var st pb3.CommonSt
	err := pb3.Unmarshal(buf, &st)
	if err != nil {
		player.LogError("err:%v", err)
		return
	}

	obj := player.GetSysObj(sysdef.SiAssistance)
	if obj == nil || !obj.IsOpen() {
		return
	}
	player.SendProto3(67, 37, &pb3.S2C_67_37{
		Hdl: st.U64Param,
	})
}

func f2gAssistanceCompleteRet(player iface.IPlayer, buf []byte) {
	var st pb3.CommonSt
	err := pb3.Unmarshal(buf, &st)
	if err != nil {
		player.LogError("err:%v", err)
		return
	}

	obj := player.GetSysObj(sysdef.SiAssistance)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys := obj.(*AssistanceSys)
	sys.Compile(st.U64Param2, st.U64Param == player.GetId(), st.U64ListParam...)
}

func init() {
	RegisterSysClass(sysdef.SiAssistance, func() iface.ISystem {
		return &AssistanceSys{}
	})
	event.RegActorEvent(custom_id.AeNewDay, onAssistanceNewDay)
	event.RegActorEvent(custom_id.AePlayerJoinTeam, onPlayerJoinTeam)

	engine.RegisterActorCallFunc(playerfuncid.F2GChangeAssistanceTargetRet, f2gChangeAssistanceTargetRet)
	engine.RegisterActorCallFunc(playerfuncid.F2GDelAssistanceTargetRet, f2gDelAssistanceTargetRet)
	engine.RegisterActorCallFunc(playerfuncid.F2GJoinAssistanceTargetRet, f2gJoinAssistanceTargetRet)
	engine.RegisterActorCallFunc(playerfuncid.F2GLeaveAssistanceRet, f2gLeaveAssistanceRet)
	engine.RegisterActorCallFunc(playerfuncid.F2GAssistanceCompleteRet, f2gAssistanceCompleteRet)

	net.RegisterSysProtoV2(67, 31, sysdef.SiAssistance, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*AssistanceSys).c2sList
	})
	net.RegisterSysProtoV2(67, 32, sysdef.SiAssistance, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*AssistanceSys).c2sAsk
	})
	net.RegisterSysProtoV2(67, 35, sysdef.SiAssistance, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*AssistanceSys).c2sEnter
	})
	net.RegisterSysProtoV2(67, 37, sysdef.SiAssistance, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*AssistanceSys).c2sCancel
	})
	initAssistanceGm()
}

func initAssistanceGm() {
	gmevent.Register("AssistanceSys.getList", func(player iface.IPlayer, args ...string) bool {
		s2cAssistanceList(player)
		return true
	}, 1)
	gmevent.Register("AssistanceSys.CompileTeam", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiAssistance)
		if obj == nil || !obj.IsOpen() {
			return false
		}
		sys := obj.(*AssistanceSys)
		sys.CompileTeam()
		return true
	}, 1)
	gmevent.Register("AssistanceSys.c2sAsk", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiAssistance)
		if obj == nil || !obj.IsOpen() {
			return false
		}
		sys := obj.(*AssistanceSys)
		sys.getData().LastAskAssistanceAt = 0
		msg := base.NewMessage()
		msg.SetCmd(67<<8 | 32)
		err := msg.PackPb3Msg(&pb3.C2S_67_32{
			MonsterId: utils.AtoUint32(args[0]),
			GuildId:   utils.AtoUint64(args[1]),
		})
		if err != nil {
			player.LogError(err.Error())
			return false
		}
		player.DoNetMsg(67, 32, msg)
		return true
	}, 1)
	gmevent.Register("AssistanceSys.c2sEnter", func(player iface.IPlayer, args ...string) bool {
		msg := base.NewMessage()
		msg.SetCmd(67<<8 | 35)
		err := msg.PackPb3Msg(&pb3.C2S_67_35{
			Hdl: utils.AtoUint64(args[0]),
		})
		if err != nil {
			player.LogError(err.Error())
			return false
		}
		player.DoNetMsg(67, 35, msg)
		return true
	}, 1)
	gmevent.Register("AssistanceSys.c2sCancel", func(player iface.IPlayer, args ...string) bool {
		msg := base.NewMessage()
		msg.SetCmd(67<<8 | 37)
		err := msg.PackPb3Msg(&pb3.C2S_67_37{
			Hdl: utils.AtoUint64(args[0]),
		})
		if err != nil {
			player.LogError(err.Error())
			return false
		}
		player.DoNetMsg(67, 37, msg)
		return true
	}, 1)
}
