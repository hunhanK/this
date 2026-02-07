/**
 * @Author: LvYuMeng
 * @Date: 2025/5/12
 * @Desc:
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/divinerealmmgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"math/bits"
)

type DivineRealmSys struct {
	Base
}

func (s *DivineRealmSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *DivineRealmSys) OnReconnect() {
	s.s2cInfo()
}

func (s *DivineRealmSys) OnOpen() {
	s.s2cInfo()
}

func (s *DivineRealmSys) onNewDay() {
	data := s.getData()
	data.PartiBits = 0
}

func (s *DivineRealmSys) s2cInfo() {
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CDivineRealmActorInfoReq, &pb3.G2CDivineRealmActorInfoReq{
		PfId:    engine.GetPfId(),
		SrvId:   engine.GetServerId(),
		ActorId: s.owner.GetId(),
	})
	if nil != err {
		logger.LogError("err:%v", err)
	}
}

func (s *DivineRealmSys) c2sAssemble(msg *base.Message) error {
	var req pb3.C2S_77_6
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	conf := jsondata.GetDivineRealmConquerConf()
	if nil == conf {
		return neterror.ConfNotFoundError("conf is nil")
	}

	rank := manager.GRankMgrIns.GetRankByType(gshare.RankTypePower)
	if nil == rank {
		return neterror.ParamsInvalidError("not in rank")
	}

	if conf.AssembleFightValCond < rank.GetRankById(s.owner.GetId()) {
		s.owner.SendTipMsg(tipmsgid.TpDivineRealmConveneLimit, conf.AssembleFightValCond)
		return nil
	}

	err = engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CDivineRealmAssemble, &pb3.G2CDivineRealmAssemble{
		SceneId:   req.StrongholdId,
		MonsterId: req.MonsterId,
		Camp:      uint32(s.owner.GetSmallCrossCamp()),
		X:         req.X,
		Y:         req.Y,
		PfId:      engine.GetPfId(),
		SrvId:     engine.GetServerId(),
		ActorId:   s.owner.GetId(),
	})

	if nil != err {
		return err
	}

	return nil
}

func (s *DivineRealmSys) c2sEnter(msg *base.Message) error {
	var req pb3.C2S_77_10
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	err = s.owner.EnterFightSrv(base.SmallCrossServer, fubendef.EnterDivineRealmConquerFb, &pb3.DivineRealmConquerEnterReq{
		SceneId: req.GetStrongholdId(),
	})

	if err != nil {
		return err
	}

	return nil
}

func (s *DivineRealmSys) c2sAssembleTransfer(msg *base.Message) error {
	var req pb3.C2S_77_13
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	err = s.owner.EnterFightSrv(base.SmallCrossServer, fubendef.EnterDivineRealmConquerFb, &pb3.DivineRealmConquerEnterReq{
		IsAssemble: true,
	})

	if err != nil {
		return err
	}

	return nil
}

func (s *DivineRealmSys) c2sPersonalAwards(msg *base.Message) error {
	var req pb3.C2S_77_12
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	err = engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CDivineRealmPersonalAwards, &pb3.G2CDivineRealmPersonalAwards{
		PfId:    engine.GetPfId(),
		SrvId:   engine.GetServerId(),
		ActorId: s.owner.GetId(),
	})
	if nil != err {
		return err
	}
	return nil
}

func (s *DivineRealmSys) c2sStrongholdAwards(msg *base.Message) error {
	var req pb3.C2S_77_14
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	err = engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CDivineRealmStrongholdAwards, &pb3.G2CDivineRealmStrongholdAwards{
		PfId:         engine.GetPfId(),
		SrvId:        engine.GetServerId(),
		ActorId:      s.owner.GetId(),
		StrongholdId: req.StrongholdId,
		Camp:         uint32(s.owner.GetSmallCrossCamp()),
	})
	if nil != err {
		return err
	}

	return nil
}

func (s *DivineRealmSys) c2sBoxData(msg *base.Message) error {
	var req pb3.C2S_77_20
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	mgr := divinerealmmgr.GetMgr()
	s.SendProto3(77, 20, &pb3.S2C_77_20{Data: mgr.GetBoxData()})
	return nil
}

func (s *DivineRealmSys) c2sBuy(msg *base.Message) error {
	var req pb3.C2S_77_21
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	mgr := divinerealmmgr.GetMgr()
	boxMap := mgr.GetBoxData()

	boxData, ok := boxMap[req.Id]
	if !ok {
		return neterror.ParamsInvalidError("id:%d, divineRealm box data nil", req.Id)
	}
	if boxData.IsBuy {
		return neterror.ParamsInvalidError("id:%d, divineRealm box has bought", req.Id)
	}

	conf := jsondata.GetDivineRealmBoxConf(boxData.BoxType)
	if conf == nil {
		return neterror.ConfNotFoundError("boxType:%d config not found", boxData.BoxType)
	}

	if !s.owner.DeductMoney(boxData.MoneyType, int64(boxData.Price), common.ConsumeParams{
		LogId: pb3.LogId_LogDivineRealmBoxBuyConsume,
	}) {
		return neterror.ParamsInvalidError("consume error")
	}

	boxData.IsBuy = true
	boxData.PlayerId = s.owner.GetId()

	// 给奖励
	if len(conf.Rewards) > 0 {
		engine.GiveRewards(s.owner, conf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogDivineRealmBoxBuyRewards})
	}

	// 分红
	mgr.GiveDividends(boxData)

	s.owner.ChannelChat(&pb3.C2S_5_1{
		Channel:     chatdef.CIWorld,
		ContentType: chatdef.ContentDivineRealmDividendRank,
		Params:      fmt.Sprintf("%d", boxData.Id),
	}, false)

	s.SendProto3(77, 21, &pb3.S2C_77_21{BoxData: boxData})
	return nil
}

func (s *DivineRealmSys) c2sGetDividendPreview(msg *base.Message) error {
	var req pb3.C2S_77_23
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}

	mgr := divinerealmmgr.GetMgr()
	boxMap := mgr.GetBoxData()

	boxData, ok := boxMap[req.BoxId]
	if !ok {
		s.owner.SendTipMsg(tipmsgid.MsgExpireTip)
		return neterror.ParamsInvalidError("boxId:%d not exist", req.BoxId)
	}
	pkMsg := &pb3.S2C_77_23{
		BoxId:     boxData.Id,
		Round:     boxData.Round,
		Dividends: mgr.PackBoxDividends(boxData.Id),
	}
	s.SendProto3(77, 23, pkMsg)
	return nil
}

func (s *DivineRealmSys) getData() *pb3.DivineRealmInfo {
	binary := s.GetBinaryData()
	if nil == binary.DivineRealmInfo {
		binary.DivineRealmInfo = &pb3.DivineRealmInfo{}
	}
	return binary.DivineRealmInfo
}

func (s *DivineRealmSys) getDailyPartiTimes() uint32 {
	data := s.getData()
	return uint32(bits.OnesCount32(data.PartiBits))
}

func (s *DivineRealmSys) syncPartiBits(bits uint32) {
	data := s.getData()
	partiBits := data.PartiBits
	data.PartiBits |= bits
	if partiBits != data.PartiBits {
		s.owner.TriggerQuestEvent(custom_id.QttDivineRealmParticipate, 0, 1)
	}
}

func c2fDivineRealmPreInfoReq(buf []byte) {
	conf := jsondata.GetDivineRealmConquerConf()
	if nil == conf {
		return
	}

	monsterLevel := uint32(manager.GRankMgrIns.GetAverageScoreByInterval(gshare.RankTypeLevel, 1, int(conf.LevelRank)))
	auctionLevel := uint32(manager.GRankMgrIns.GetAverageScoreByInterval(gshare.RankTypeLevel, 1, int(conf.TopLevelRange)))
	auctionCircle := uint32(manager.GRankMgrIns.GetAverageScoreByInterval(gshare.RankTypeBoundary, 1, int(conf.TopCircleRange)))

	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CDivineRealmPreInfoRet, &pb3.G2CDivineRealmPreInfoRet{
		PfId:          engine.GetPfId(),
		SrvId:         engine.GetServerId(),
		MonsterLevel:  monsterLevel,
		AuctionLevel:  auctionLevel,
		AuctionCircle: auctionCircle,
	})

	if nil != err {
		logger.LogError("err:%v", err)
		return
	}
}

func c2gDivineRealmReceiveStrongholdAward(buf []byte) {
	var req pb3.CommonSt
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogError("unmarshal failed %v", err)
		return
	}

	actorId := req.U64Param

	engine.SendPlayerMessage(actorId, gshare.OfflineCompleteDivineRealmOpenBoxQuest, &pb3.CommonSt{U32Param: 1})
}

func c2gDivineRealmSyncScore(buf []byte) {
	var req pb3.C2GDivineRealmSyncScore
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogError("unmarshal failed %v", err)
		return
	}

	globalVar := gshare.GetStaticVar()
	if nil == globalVar.DivineRealmCrossData {
		globalVar.DivineRealmCrossData = &pb3.DivineRealmCrossData{}
	}
	d := make(map[uint64]*pb3.DivineRealmPlayer)
	for actorId, pData := range req.PDatas {
		if !gshare.IsActorInThisServer(actorId) {
			continue
		}
		d[actorId] = pData
	}
	globalVar.DivineRealmCrossData.PDatas = d
	globalVar.DivineRealmCrossData.SyncTime = time_util.NowSec()
}

func syncDivineRealmScoreToCross(args ...interface{}) {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.DivineRealmCrossData {
		return
	}
	if !time_util.IsSameDay(globalVar.DivineRealmCrossData.SyncTime, time_util.NowSec()) {
		globalVar.DivineRealmCrossData = nil
		return
	}

	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CDivineRealmSyncScore, &pb3.G2CDivineRealmSyncScore{
		PDatas: globalVar.DivineRealmCrossData.PDatas,
	})

	if nil != err {
		logger.LogError("err:%v", err)
		return
	}
	globalVar.DivineRealmCrossData = nil
}

func offlineCompleteDivineRealmOpenBoxQuest(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.CommonSt)
	if !ok {
		return
	}

	count := st.U32Param
	sys, ok := player.GetSysObj(sysdef.SiDivineRealmConquer).(*DivineRealmSys)
	if !ok || !sys.IsOpen() {
		return
	}

	player.TriggerQuestEvent(custom_id.QttDivineRealmStrongholdAwards, 0, int64(count))
}

func init() {
	RegisterSysClass(sysdef.SiDivineRealmConquer, func() iface.ISystem {
		return &DivineRealmSys{}
	})

	engine.RegisterSysCall(sysfuncid.C2FDivineRealmPreInfoReq, c2fDivineRealmPreInfoReq)
	engine.RegisterSysCall(sysfuncid.C2GDivineRealmReceiveStrongholdAward, c2gDivineRealmReceiveStrongholdAward)
	engine.RegisterSysCall(sysfuncid.C2GDivineRealmSyncScore, c2gDivineRealmSyncScore)

	event.RegSysEvent(custom_id.SeNewDayArrive, func(args ...interface{}) {
		gshare.GetStaticVar().DivineRealmCrossData = nil
	})

	event.RegSysEvent(custom_id.SeCrossSrvConnSucc, syncDivineRealmScoreToCross)

	net.RegisterSysProtoV2(77, 6, sysdef.SiDivineRealmConquer, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DivineRealmSys).c2sAssemble
	})
	net.RegisterSysProtoV2(77, 10, sysdef.SiDivineRealmConquer, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DivineRealmSys).c2sEnter
	})
	net.RegisterSysProtoV2(77, 12, sysdef.SiDivineRealmConquer, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DivineRealmSys).c2sPersonalAwards
	})
	net.RegisterSysProtoV2(77, 13, sysdef.SiDivineRealmConquer, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DivineRealmSys).c2sAssembleTransfer
	})
	net.RegisterSysProtoV2(77, 14, sysdef.SiDivineRealmConquer, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DivineRealmSys).c2sStrongholdAwards
	})
	net.RegisterSysProtoV2(77, 20, sysdef.SiDivineRealmConquer, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DivineRealmSys).c2sBoxData
	})
	net.RegisterSysProtoV2(77, 21, sysdef.SiDivineRealmConquer, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DivineRealmSys).c2sBuy
	})
	net.RegisterSysProtoV2(77, 23, sysdef.SiDivineRealmConquer, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*DivineRealmSys).c2sGetDividendPreview
	})

	engine.RegisterMessage(gshare.OfflineCompleteDivineRealmOpenBoxQuest, func() pb3.Message {
		return &pb3.CommonSt{}
	}, offlineCompleteDivineRealmOpenBoxQuest)

	event.RegActorEvent(custom_id.AeNewDay, func(player iface.IPlayer, args ...interface{}) {
		sys, ok := player.GetSysObj(sysdef.SiDivineRealmConquer).(*DivineRealmSys)
		if !ok || !sys.IsOpen() {
			return
		}
		sys.onNewDay()
	})

	engine.RegisterActorCallFunc(playerfuncid.C2GDivineRealmSyncParti, func(player iface.IPlayer, buf []byte) {
		var msg pb3.CommonSt
		if err := pb3.Unmarshal(buf, &msg); err != nil {
			return
		}

		sys, ok := player.GetSysObj(sysdef.SiDivineRealmConquer).(*DivineRealmSys)
		if !ok || !sys.IsOpen() {
			return
		}

		sys.syncPartiBits(msg.U32Param)
	})

	gmevent.Register("divineRealm.open", func(player iface.IPlayer, args ...string) bool {
		idx := utils.AtoUint32(args[0])
		status := utils.AtoUint32(args[1])
		err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CDivineRealmGmOpen, &pb3.G2CDivineRealmGmOpen{
			Idx:    idx,
			Status: status,
		})
		if nil != err {
			return false
		}
		return true
	}, 1)
}
