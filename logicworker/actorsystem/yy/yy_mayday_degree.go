/**
 * @Author: LvYuMeng
 * @Date: 2025/4/18
 * @Desc: 五一祈福-累抽热度
**/

package yy

import (
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
)

type YYMayDayDegreeMgr struct {
	YYBase
}

func (s *YYMayDayDegreeMgr) ResetData() {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.YyDatas || nil == globalVar.YyDatas.MayDayBlessDegree {
		return
	}
	delete(globalVar.YyDatas.MayDayBlessDegree, s.GetId())
}

func (s *YYMayDayDegreeMgr) PlayerLogin(player iface.IPlayer) {
	s.sendPlayerData(player)
	s.s2cCrossInfo(player)
}

func (s *YYMayDayDegreeMgr) PlayerReconnect(player iface.IPlayer) {
	s.sendPlayerData(player)
	s.s2cCrossInfo(player)
}

func (s *YYMayDayDegreeMgr) sendPlayerData(player iface.IPlayer) {
	if player == nil {
		return
	}
	player.SendProto3(75, 49, &pb3.S2C_75_49{
		ActiveId: s.GetId(),
		Data:     s.packPlayerData(player),
	})
}

func (s *YYMayDayDegreeMgr) GetData() *pb3.YYMayDayBlessDegree {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}
	if globalVar.YyDatas.MayDayBlessDegree == nil {
		globalVar.YyDatas.MayDayBlessDegree = make(map[uint32]*pb3.YYMayDayBlessDegree)
	}
	if globalVar.YyDatas.MayDayBlessDegree[s.Id] == nil {
		globalVar.YyDatas.MayDayBlessDegree[s.Id] = &pb3.YYMayDayBlessDegree{}
	}

	bTData := globalVar.YyDatas.MayDayBlessDegree[s.Id]
	if nil == bTData.PlayerDatas {
		bTData.PlayerDatas = map[uint64]*pb3.MayDayBlessDegree{}
	}

	return bTData
}

func (s *YYMayDayDegreeMgr) getPlayerData(player iface.IPlayer) *pb3.MayDayBlessDegree {
	data := s.GetData()
	pData, ok := data.PlayerDatas[player.GetId()]
	if !ok {
		pData = &pb3.MayDayBlessDegree{}
		data.PlayerDatas[player.GetId()] = pData
	}
	return pData
}

func (s *YYMayDayDegreeMgr) packPlayerData(player iface.IPlayer) *pb3.MayDayBlessDegree {
	data := s.getPlayerData(player)
	return &pb3.MayDayBlessDegree{
		DrawTimes: data.DrawTimes,
		Ids:       data.Ids,
	}
}

func (s *YYMayDayDegreeMgr) s2cCrossInfo(player iface.IPlayer) {
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CYYMayDayDegreeReq, &pb3.CommonSt{
		U64Param:  player.GetId(),
		U32Param:  engine.GetPfId(),
		U32Param2: engine.GetServerId(),
		U32Param3: s.GetId(),
	})
	if err != nil {
		s.LogError("err:%v", err)
		return
	}
}

func (s *YYMayDayDegreeMgr) OnOpen() {
	s.Broadcast(75, 49, &pb3.S2C_75_49{
		ActiveId: s.GetId(),
		Data:     &pb3.MayDayBlessDegree{},
	})
	s.callCrossOpen()
}

func (s *YYMayDayDegreeMgr) OnInit() {
	if !s.IsOpen() {
		return
	}
	s.callCrossOpen()
}

func (s *YYMayDayDegreeMgr) handleDrawEvent(player iface.IPlayer, event *custom_id.ActDrawEvent) {
	conf, ok := jsondata.GetYYMayDayDegreeConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	if conf.YyId != event.ActId {
		return
	}
	playerData := s.getPlayerData(player)
	playerData.DrawTimes += event.Times
	s.sendPlayerData(player)
	s.AddDegree(conf.OnceIncDegree * event.Times)
}

func (s *YYMayDayDegreeMgr) AddDegree(score uint32) {
	s.callCrossOpen()
	data := s.GetData()
	data.Degree += score
	s.syncDegree()
}

func (s *YYMayDayDegreeMgr) syncDegree() {
	data := s.GetData()
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CYYAddMayDayDegreeDegree, &pb3.CommonSt{
		U32Param:  s.GetId(),
		U32Param2: data.Degree,
		U64Param:  utils.Make64(engine.GetServerId(), engine.GetPfId()),
	})
	if err != nil {
		s.LogError("err:%v", err)
		return
	}
}

func (s *YYMayDayDegreeMgr) callCrossOpen() {
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CYYMayDayDegreeActInfoSync, &pb3.G2CSyncMayDayDegreeInfo{
		Id:        s.GetId(),
		StartTime: s.GetOpenTime(),
		EndTime:   s.GetEndTime(),
		ConfIdx:   s.GetConfIdx(),
	})
	if err != nil {
		s.LogError("err:%v", err)
		return
	}
}

func (s *YYMayDayDegreeMgr) canRvDegreeAward(player iface.IPlayer, id uint32) (bool, error) {
	conf, ok := jsondata.GetYYMayDayDegreeConf(s.ConfName, s.ConfIdx)
	if !ok {
		return false, neterror.ConfNotFoundError("YYMayDayDegreeMgr conf is nil")
	}
	if _, ok := conf.DegreeAwards[id]; !ok {
		return false, neterror.ConfNotFoundError("YYMayDayDegreeMgr degree award conf is nil")
	}
	sData := s.getPlayerData(player)
	if pie.Uint32s(sData.Ids).Contains(id) {
		player.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return false, nil
	}
	if sData.DrawTimes < conf.DegreeAwards[id].DrawTimes {
		return false, neterror.ParamsInvalidError("total draw times not enough")
	}
	return true, nil
}

func (s *YYMayDayDegreeMgr) c2sDegreeAward(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_75_51
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	for _, v := range req.GetId() {
		if ok, err := s.canRvDegreeAward(player, v); !ok {
			return err
		}
	}

	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CYYMayDayDegreeAwardReq, &pb3.G2CMayDayDegreeAwardsReq{
		ActiveId: s.GetId(),
		AwardId:  req.GetId(),
		ActorId:  player.GetId(),
		PfId:     engine.GetPfId(),
		SrvId:    engine.GetServerId(),
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *YYMayDayDegreeMgr) revDegreeAward(player iface.IPlayer, ids []uint32, degree uint32) {
	conf, ok := jsondata.GetYYMayDayDegreeConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	var revIds []uint32
	var rewardVec []jsondata.StdRewardVec
	for _, id := range ids {
		if ok, _ := s.canRvDegreeAward(player, id); !ok {
			continue
		}
		if degree < conf.DegreeAwards[id].Degree {
			continue
		}
		sData := s.getPlayerData(player)
		sData.Ids = append(sData.Ids, id)
		revIds = append(revIds, id)
		rewardVec = append(rewardVec, conf.DegreeAwards[id].Rewards)
	}

	rewards := jsondata.MergeStdReward(rewardVec...)
	if len(rewards) > 0 {
		engine.GiveRewards(player, rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogMayDayBlessDegreeAward,
		})
	}
	if len(revIds) > 0 {
		player.SendProto3(75, 51, &pb3.S2C_75_51{
			ActiveId: s.GetId(),
			Ids:      revIds,
		})
	}
}

func onYYMayDayDegreeAwardRet(buf []byte) {
	msg := &pb3.C2GMayDayDegreeAwardsRet{}
	if err := pb3.Unmarshal(buf, msg); err != nil {
		return
	}

	player := manager.GetPlayerPtrById(msg.GetActorId())
	if nil == player {
		return
	}
	iYY := yymgr.GetYYByActId(msg.ActiveId)
	sys, ok := iYY.(*YYMayDayDegreeMgr)
	if !ok || !sys.IsOpen() {
		return
	}

	sys.revDegreeAward(player, msg.GetAwardId(), msg.GetDegree())
}

func init() {
	yymgr.RegisterYYType(yydefine.YYMayDayDegree, func() iface.IYunYing {
		return &YYMayDayDegreeMgr{}
	})

	engine.RegisterSysCall(sysfuncid.C2FYYMayDayDegreeAwardRet, onYYMayDayDegreeAwardRet)

	event.RegSysEvent(custom_id.SeCrossSrvConnSucc, func(args ...interface{}) {
		allYY := yymgr.GetAllYY(yydefine.YYMayDayDegree)
		for _, iYY := range allYY {
			if sys, ok := iYY.(*YYMayDayDegreeMgr); ok && sys.IsOpen() {
				sys.syncDegree()
			}
		}
	})

	net.RegisterGlobalYYSysProto(75, 51, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYMayDayDegreeMgr).c2sDegreeAward
	})

	event.RegActorEvent(custom_id.AeActDrawTimes, func(player iface.IPlayer, args ...interface{}) {
		if len(args) < 1 {
			return
		}

		drawEvent, ok := args[0].(*custom_id.ActDrawEvent)
		if !ok {
			return
		}
		allYY := yymgr.GetAllYY(yydefine.YYMayDayDegree)
		for _, iYY := range allYY {
			if sys, ok := iYY.(*YYMayDayDegreeMgr); ok && sys.IsOpen() {
				sys.handleDrawEvent(player, drawEvent)
			}
		}
	})

	gmevent.Register("addMayDayDegree", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 2 {
			return false
		}
		yyId := utils.AtoUint32(args[0])
		score := utils.AtoUint32(args[1])
		iYY := yymgr.GetYYByActId(yyId)
		if sys, ok := iYY.(*YYMayDayDegreeMgr); ok && sys.IsOpen() {
			sys.AddDegree(score)
		}
		return true
	}, 1)
}
