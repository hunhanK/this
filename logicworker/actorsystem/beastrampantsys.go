/**
 * @Author: lzp
 * @Date: 2024/11/20
 * @Desc:
**/

package actorsystem

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/fightworker"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/teammgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
)

const (
	Easy = 1 // 简单
	Diff = 2 // 困难
)

type BeastRampantSys struct {
	Base
}

func (s *BeastRampantSys) OnOpen() {
	s.updateLeftTimes()
	s.updateSelDiff()
	s.reqActInfo()
}

func (s *BeastRampantSys) OnReconnect() {
	s.updateLeftTimes()
	s.updateSelDiff()
}

func (s *BeastRampantSys) OnLogin() {
	s.updateLeftTimes()
	s.updateSelDiff()
}

func (s *BeastRampantSys) OnNewDay() {
	binary := s.GetBinaryData()
	binary.BeastRampantTimes = 0
	s.updateLeftTimes()
}

func (s *BeastRampantSys) c2sSelDiff(msg *base.Message) error {
	var req pb3.C2S_17_131
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.Wrap(err)
	}

	player := s.GetOwner()
	teamId := player.GetTeamId()

	if teamId > 0 {
		teamPb, err := teammgr.GetTeamPb(teamId)
		if err != nil {
			return neterror.Wrap(err)
		}

		if teamPb.GetLeaderId() != player.GetId() {
			return neterror.ParamsInvalidError("not team leader id: %d", player.GetId())
		}

		fbSet := teammgr.GetTeamFbSetting(teamId)
		if fbSet.BRFbTData == nil {
			fbSet.BRFbTData = &pb3.BRFbTeamData{}
		}
		fbSet.BRFbTData.Diff = req.Diff
	}

	s.SendProto3(17, 131, &pb3.S2C_17_131{Diff: req.Diff})
	return nil
}

func (s *BeastRampantSys) c2sReqActInfo(msg *base.Message) error {
	var req pb3.C2S_17_133
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.Wrap(err)
	}

	s.reqActInfo()
	return nil
}

func (s *BeastRampantSys) c2sSingleEnter(msg *base.Message) error {
	var req pb3.C2S_17_136
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.Wrap(err)
	}

	if engine.FightClientExistPredicate(base.SmallCrossServer) {
		client := fightworker.GetFightClient(base.SmallCrossServer)
		if client != nil {
			return engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FBeastRampantSingleEnter, &pb3.CommonSt{
				U64Param:  s.owner.GetId(),
				U64Param2: req.Hdl,
				U32Param:  req.Diff,
			})
		}
	}
	return engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FBeastRampantSingleEnter, &pb3.CommonSt{
		U64Param:  s.owner.GetId(),
		U64Param2: req.Hdl,
		U32Param:  req.Diff,
	})
}

func (s *BeastRampantSys) reqActInfo() {
	if engine.FightClientExistPredicate(base.SmallCrossServer) {
		client := fightworker.GetFightClient(base.SmallCrossServer)
		if client != nil {
			if err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FBeastRampantActInfoReq, &pb3.BeastRampantActInfoReq{
				ActorId: s.owner.GetId(),
				PfId:    engine.GetPfId(),
				SrvId:   engine.GetServerId(),
			}); err != nil {
				s.LogError("err: %v", err)
			}
			return
		}
	}
	if err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FBeastRampantActInfoReq, &pb3.BeastRampantActInfoReq{
		ActorId: s.owner.GetId(),
		PfId:    engine.GetPfId(),
		SrvId:   engine.GetServerId(),
	}); err != nil {
		s.LogError("err: %v", err)
	}
}

// 个人结算
func (s *BeastRampantSys) actorSettlement(st *pb3.BeastRampantSettlement) {
	id := st.Id
	diff := st.Diff
	isWin := st.IsWin
	selDiff := st.SelDiff

	conf := jsondata.GetBRFbConf()
	rConf := jsondata.GetBRRefreshRuleConf(id, diff)
	if conf == nil || rConf == nil {
		return
	}

	// 没有奖励次数,使用协助奖励
	if s.getLeftTimes() <= 0 {
		if isWin {
			if s, ok := s.owner.GetSysObj(sysdef.SiAssistance).(*AssistanceSys); ok && s.IsOpen() {
				if !s.CompileTeam() {
					s.SendProto3(17, 34, &pb3.S2C_17_34{IsSuccess: true})
				}
			}
		} else {
			s.SendProto3(17, 34, &pb3.S2C_17_34{IsSuccess: false})
		}
		return
	}

	settlePb := &pb3.FbSettlement{FbId: conf.FbId}

	// 失败结算
	if !isWin {
		settlePb.Ret = custom_id.FbSettleResultLose
		s.SendProto3(17, 254, &pb3.S2C_17_254{Settle: settlePb})
		return
	}

	// 胜利结算
	s.owner.TriggerQuestEvent(custom_id.QttKillBeast, 0, 1)
	settlePb.Ret = custom_id.FbSettleResultWin

	var rewards jsondata.StdRewardVec
	switch selDiff {
	case Easy: // 简单
		rewards = rConf.SRewards
	case Diff: // 困难
		rewards = rConf.MRewards
	}

	binary := s.GetBinaryData()
	if len(rewards) > 0 {
		// 同步使用次数
		binary.BeastRampantTimes += 1
		s.updateLeftTimes()

		engine.GiveRewards(s.owner, rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogBeastRampantSettleRewards,
		})
		settlePb.ShowAward = jsondata.StdRewardVecToPb3RewardVec(rewards)
		s.SendProto3(17, 254, &pb3.S2C_17_254{Settle: settlePb})
		s.GetOwner().TriggerEvent(custom_id.AeCompleteRetrieval, sysdef.SiBeastRampant, 1)
	}

	if binary.BeastRampantDiff < diff {
		binary.BeastRampantDiff = diff
		s.owner.TriggerEvent(custom_id.AeBeastRampantDiffChange, diff)
		s.owner.SetExtraAttr(attrdef.BeastRampantDiff, int64(binary.BeastRampantDiff))
	}
}

func (s *BeastRampantSys) updateLeftTimes() {
	leftTimes := s.getLeftTimes()
	s.owner.SetExtraAttr(attrdef.BeastRampantTimes, int64(leftTimes))
}

func (s *BeastRampantSys) updateSelDiff() {
	binary := s.GetBinaryData()
	s.owner.SetExtraAttr(attrdef.BeastRampantDiff, int64(binary.BeastRampantDiff))
}

func (s *BeastRampantSys) getLeftTimes() uint32 {
	conf := jsondata.GetBRFbConf()
	if conf == nil {
		return 0
	}

	addTimes := uint32(s.owner.GetFightAttr(attrdef.BeastRampantSettleFuBenTimesAdd))
	binary := s.GetBinaryData()
	usedTimes := binary.BeastRampantTimes

	if conf.Times+addTimes > usedTimes {
		return conf.Times + addTimes - usedTimes
	} else {
		return 0
	}
}

func onActorSettlement(player iface.IPlayer, buf []byte) {
	sys, ok := player.GetSysObj(sysdef.SiBeastRampant).(*BeastRampantSys)
	if !ok || !sys.IsOpen() {
		return
	}
	var st pb3.BeastRampantSettlement
	if err := pb3.Unmarshal(buf, &st); err != nil {
		player.LogError("unmarshal err: %v", err)
		return
	}
	sys.actorSettlement(&st)
}

// 发送活动数据
func s2cBeastRampantActInfo(buf []byte) {
	var msg pb3.BeastRampantActInfoRet
	if err := pb3.Unmarshal(buf, &msg); err != nil {
		logger.LogError("unmarshal err: %v", err)
		return
	}

	player := manager.GetPlayerPtrById(msg.ActorId)
	if player == nil {
		return
	}

	sys, ok := player.GetSysObj(sysdef.SiBeastRampant).(*BeastRampantSys)
	if !ok || !sys.IsOpen() {
		return
	}

	player.SendProto3(17, 133, &pb3.S2C_17_133{
		ActInfo: msg.ActInfo,
		PlayerInfo: &pb3.BeastRampantPlayerInfo{
			Diff:      msg.ActorDiff,
			LeftTimes: player.GetExtraAttrU32(attrdef.BeastRampantTimes),
		},
	})
}

func handleBeastRampantTeamSettlement(buf []byte) {
	var msg pb3.BeastRampantSettlement
	if err := pb3.Unmarshal(buf, &msg); err != nil {
		logger.LogError("unmarshal err: %v", err)
		return
	}

	teamId := msg.TeamId
	teamPb, err := teammgr.GetTeamPb(teamId)
	if err != nil {
		return
	}

	for _, mem := range teamPb.Members {
		playerId := mem.PlayerInfo.Id
		player := manager.GetPlayerPtrById(playerId)
		if player == nil {
			continue
		}

		player.GetSysObj(sysdef.SiBeastRampant)
		sys, ok := player.GetSysObj(sysdef.SiBeastRampant).(*BeastRampantSys)
		if !ok || !sys.IsOpen() {
			continue
		}

		sys.actorSettlement(&msg)
	}
}

func init() {
	RegisterSysClass(sysdef.SiBeastRampant, func() iface.ISystem {
		return &BeastRampantSys{}
	})

	engine.RegisterActorCallFunc(playerfuncid.BeastRampantActorSettlement, onActorSettlement)

	engine.RegisterSysCall(sysfuncid.F2GBeastRampantActInfoRet, s2cBeastRampantActInfo)
	engine.RegisterSysCall(sysfuncid.F2GBeastRampantTeamSettlement, handleBeastRampantTeamSettlement)

	net.RegisterSysProto(17, 131, sysdef.SiBeastRampant, (*BeastRampantSys).c2sSelDiff)
	net.RegisterSysProto(17, 133, sysdef.SiBeastRampant, (*BeastRampantSys).c2sReqActInfo)
	net.RegisterSysProto(17, 136, sysdef.SiBeastRampant, (*BeastRampantSys).c2sSingleEnter)

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		sys, ok := player.GetSysObj(sysdef.SiBeastRampant).(*BeastRampantSys)
		if !ok || !sys.IsOpen() {
			return
		}
		sys.OnNewDay()
	})

	event.RegActorEvent(custom_id.AeRegFightAttrChange, func(actor iface.IPlayer, args ...interface{}) {
		if len(args) < 1 {
			return
		}
		attrType, ok := args[0].(uint32)
		if !ok {
			return
		}
		if attrType != attrdef.BeastRampantSettleFuBenTimesAdd {
			return
		}
		sys, ok := actor.GetSysObj(sysdef.SiBeastRampant).(*BeastRampantSys)
		if !ok || !sys.IsOpen() {
			return
		}
		sys.updateLeftTimes()
	})

	event.RegActorEvent(custom_id.AeEnterTeamFb, func(player iface.IPlayer, args ...interface{}) {
		if len(args) < 1 {
			return
		}
		sys, ok := player.GetSysObj(sysdef.SiBeastRampant).(*BeastRampantSys)
		if !ok || !sys.IsOpen() {
			return
		}
		conf := jsondata.GetBRFbConf()
		if conf == nil {
			return
		}
		fbId := args[0].(uint32)
		if fbId == conf.FbId {
			player.TriggerEvent(custom_id.AeDailyMissionComplete, custom_id.DailyMissionBeastRampantTimes)
		}
	})

	gmevent.Register("beastrampant.diff", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		diff := utils.AtoUint32(args[0])
		player.GetBinaryData().BeastRampantDiff = diff
		player.SetExtraAttr(attrdef.BeastRampantDiff, int64(player.GetBinaryData().BeastRampantDiff))
		player.TriggerEvent(custom_id.AeBeastRampantDiffChange, diff)
		return true
	}, 1)
}
