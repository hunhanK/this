/**
 * @Author:
 * @Date:
 * @Desc:
**/

package yy

import (
	"fmt"
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"time"
)

type CampCheeringEventSys struct {
	YYBase
}

func (s *CampCheeringEventSys) getPlayerData(playerId uint64) *pb3.YYCampCheeringEventPlayerData {
	data := s.getData()
	for _, datum := range data.PlayerData {
		if datum.PlayerId == playerId {
			return datum
		}
	}
	var val = &pb3.YYCampCheeringEventPlayerData{
		PlayerId: playerId,
	}
	data.PlayerData = append(data.PlayerData, val)
	return val
}

func (s *CampCheeringEventSys) s2cInfo(player iface.IPlayer) {
	if player == nil {
		return
	}
	player.SendProto3(8, 220, &pb3.S2C_8_220{
		Data:     s.getPlayerData(player.GetId()),
		ActiveId: s.GetId(),
	})
}

func (s *CampCheeringEventSys) getData() *pb3.YYCampCheeringEventData {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}
	if nil == globalVar.YyDatas.CampCheeringEventData {
		globalVar.YyDatas.CampCheeringEventData = make(map[uint32]*pb3.YYCampCheeringEventData)
	}
	if globalVar.YyDatas.CampCheeringEventData[s.Id] == nil {
		globalVar.YyDatas.CampCheeringEventData[s.Id] = &pb3.YYCampCheeringEventData{}
	}
	return globalVar.YyDatas.CampCheeringEventData[s.Id]
}

func (s *CampCheeringEventSys) ResetData() {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}
	if nil == globalVar.YyDatas.CampCheeringEventData {
		return
	}
	delete(globalVar.YyDatas.CampCheeringEventData, s.Id)
}

func (s *CampCheeringEventSys) PlayerReconnect(player iface.IPlayer) {
	s.s2cInfo(player)
	err := s.c2sGetCrossData(player, nil)
	if err != nil {
		s.LogError("err:%v", err)
	}
}

func (s *CampCheeringEventSys) PlayerLogin(player iface.IPlayer) {
	s.s2cInfo(player)
	err := s.c2sGetCrossData(player, nil)
	if err != nil {
		s.LogError("err:%v", err)
	}
}

func (s *CampCheeringEventSys) OnOpen() {
	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		s.s2cInfo(player)
	})
}

func (s *CampCheeringEventSys) checkHour() bool {
	config := jsondata.GetYYCampCheeringEventConfig(s.ConfName, s.ConfIdx)
	if config == nil {
		return false
	}
	if uint32(time.Now().Hour()) >= config.CanNotJoinHour {
		return false
	}
	return true
}

func (s *CampCheeringEventSys) reissueAwards() {
	config := jsondata.GetYYCampCheeringEventConfig(s.ConfName, s.ConfIdx)
	if config == nil {
		return
	}
	for _, data := range s.getData().PlayerData {
		var totalAwards jsondata.StdRewardVec
		for _, processAward := range config.ProcessAwards {
			if data.Process < processAward.Process {
				continue
			}
			if utils.IsSetBit64(data.ReceiveFlag, processAward.Idx) {
				continue
			}
			data.ReceiveFlag = utils.SetBit64(data.ReceiveFlag, processAward.Idx)
			totalAwards = append(totalAwards, processAward.Rewards...)
		}
		totalAwards = jsondata.MergeStdReward(totalAwards)
		if len(totalAwards) > 0 {
			mailmgr.SendMailToActor(data.PlayerId, &mailargs.SendMailSt{
				ConfId:  config.ReissueMailId,
				Rewards: totalAwards,
			})
		}
	}
}

func (s *CampCheeringEventSys) NewDay() {
	s.reissueAwards()
	s.getData().PlayerData = []*pb3.YYCampCheeringEventPlayerData{}
	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		s.s2cInfo(player)
	})
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CYYCampCheeringEventCrossInit, &pb3.G2FYYCampCheeringEventCrossInitReq{
		ActiveId: s.GetId(),
		ConfIdx:  s.GetConfIdx(),
		ConfName: s.ConfName,
	})
	if err != nil {
		s.LogError("err:%v", err)
	}
	return
}

func (s *CampCheeringEventSys) OnEnd() {
	s.reissueAwards()
	return
}

func (s *CampCheeringEventSys) c2sGetCrossData(player iface.IPlayer, _ *base.Message) error {
	err := player.CallActorSmallCrossFunc(actorfuncid.G2CYYCampCheeringEventCrossData, &pb3.G2FYYCampCheeringEventCrossDataReq{
		ActiveId: s.GetId(),
		ConfIdx:  s.ConfIdx,
		ConfName: s.ConfName,
	})
	if err != nil {
		return neterror.Wrap(err)
	}
	return nil
}

func (s *CampCheeringEventSys) c2sRandomCamp(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_8_222
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}
	actorId := player.GetId()
	data := s.getPlayerData(actorId)
	if data.Camp != 0 {
		return neterror.ParamsInvalidError("already get camp %d", data.Camp)
	}
	if !s.checkHour() {
		return neterror.ParamsInvalidError("cur time not can join")
	}
	err = player.CallActorSmallCrossFunc(actorfuncid.G2CYYCampCheeringEventCrossRandomCamp, &pb3.G2FYYCampCheeringEventCrossRandomCampReq{
		ActiveId: s.GetId(),
		ConfIdx:  s.GetConfIdx(),
		ConfName: s.ConfName,
	})
	if err != nil {
		return neterror.Wrap(err)
	}
	return nil
}

func (s *CampCheeringEventSys) c2sAddProcess(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_8_223
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	config := jsondata.GetYYCampCheeringEventConfig(s.ConfName, s.ConfIdx)
	if config == nil {
		return neterror.ConfNotFoundError("not found conf %s", s.GetPrefix())
	}
	if !s.checkHour() {
		return neterror.ParamsInvalidError("cur time not can join")
	}
	data := s.getPlayerData(player.GetId())
	if data.Camp == 0 {
		return neterror.ParamsInvalidError("not belong camp")
	}
	times := req.Times
	var addProcessConf *jsondata.YYCampCheeringEventAddProcess
	for _, addProcess := range config.AddProcess {
		if addProcess.Times != times {
			continue
		}
		addProcessConf = addProcess
		break
	}
	if addProcessConf == nil {
		return neterror.ConfNotFoundError("%s not found %d add process conf", s.GetPrefix(), req.Times)
	}
	if len(addProcessConf.Consume) == 0 || !player.ConsumeByConf(addProcessConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogYYCampCheeringEventAddProcess}) {
		return neterror.ConsumeFailedError("%s %d add process failed", s.GetPrefix(), req.Times)
	}
	if len(addProcessConf.Rewards) > 0 {
		engine.GiveRewards(player, addProcessConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogYYCampCheeringEventAddProcess})
	}
	data.Process += addProcessConf.Process
	player.SendProto3(8, 223, &pb3.S2C_8_223{
		ActiveId: s.GetId(),
		Times:    times,
		Process:  data.Process,
	})
	err = player.CallActorSmallCrossFunc(actorfuncid.G2CYYCampCheeringEventCrossAddProcess, &pb3.G2FYYCampCheeringEventCrossAddProcessReq{
		ActiveId: s.GetId(),
		ConfIdx:  s.GetConfIdx(),
		ConfName: s.ConfName,
		Process:  addProcessConf.Process,
		Camp:     data.Camp,
	})
	if err != nil {
		s.LogError("err:%v", err)
	}
	logworker.LogPlayerBehavior(player, pb3.LogId_LogYYCampCheeringEventAddProcess, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d_%d", times, data.Process),
	})
	return nil
}

func (s *CampCheeringEventSys) c2sRecAwards(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_8_224
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	config := jsondata.GetYYCampCheeringEventConfig(s.ConfName, s.ConfIdx)
	if config == nil {
		return neterror.ConfNotFoundError("%s not found conf", s.GetPrefix())
	}

	playerId := player.GetId()
	data := s.getPlayerData(playerId)
	reqIdxList := req.IdxList
	var canRecIdx pie.Uint32s
	for _, idx := range reqIdxList {
		if utils.IsSetBit64(data.ReceiveFlag, idx) {
			continue
		}
		canRecIdx = canRecIdx.Append(idx)
	}

	canRecIdx = canRecIdx.Unique()
	if len(canRecIdx) == 0 {
		return neterror.ParamsInvalidError("%s not can rec %d %v", s.GetPrefix(), data.ReceiveFlag, reqIdxList)
	}

	var totalAwards jsondata.StdRewardVec
	for _, processAward := range config.ProcessAwards {
		if data.Process < processAward.Process {
			continue
		}
		if utils.IsSetBit64(data.ReceiveFlag, processAward.Idx) {
			continue
		}
		if !canRecIdx.Contains(processAward.Idx) {
			continue
		}
		data.ReceiveFlag = utils.SetBit64(data.ReceiveFlag, processAward.Idx)
		totalAwards = append(totalAwards, processAward.Rewards...)
	}

	totalAwards = jsondata.MergeStdReward(totalAwards)
	if len(totalAwards) > 0 {
		engine.GiveRewards(player, totalAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogYYCampCheeringEventRecAwards})
	}

	player.SendProto3(8, 224, &pb3.S2C_8_224{
		ActiveId:    s.GetId(),
		ReceiveFlag: data.ReceiveFlag,
	})
	logworker.LogPlayerBehavior(player, pb3.LogId_LogYYCampCheeringEventRecAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: fmt.Sprintf("%d_%s", data.ReceiveFlag, canRecIdx.JSONString()),
	})
	return nil
}

func handleC2GYYCampCheeringEventCrossRandomCamp(buf []byte) {
	var req pb3.G2FYYCampCheeringEventCrossRandomCampRet
	err := pb3.Unmarshal(buf, &req)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
	yy := yymgr.GetYYByActId(req.ActiveId)
	if yy == nil || !yy.IsOpen() {
		return
	}
	sys := yy.(*CampCheeringEventSys)
	data := sys.getPlayerData(req.ActorId)
	data.Camp = req.Camp
	player := manager.GetPlayerPtrById(req.ActorId)
	if player == nil {
		return
	}
	player.SendProto3(8, 222, &pb3.S2C_8_222{
		ActiveId: req.ActiveId,
		Camp:     req.Camp,
	})
}

func handleC2GYYCampCheeringEventCrossSettlement(buf []byte) {
	var req pb3.C2GYYCampCheeringEventCrossSettlementReq
	err := pb3.Unmarshal(buf, &req)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}

	yy := yymgr.GetYYByActId(req.ActiveId)
	if yy == nil || !yy.IsOpen() {
		return
	}

	sys := yy.(*CampCheeringEventSys)
	config := jsondata.GetYYCampCheeringEventConfig(sys.ConfName, sys.ConfIdx)
	if config == nil {
		sys.LogError("SuccAndFailedAwards not found conf")
		return
	}

	if len(config.SuccAndFailedAwards) != 2 {
		sys.LogError("SuccAndFailedAwards size not equal 2")
		return
	}

	var failedConf *jsondata.YYCampCheeringEventSuccAndFailedAwards
	var succConf *jsondata.YYCampCheeringEventSuccAndFailedAwards
	for _, conf := range config.SuccAndFailedAwards {
		if conf.Win > 0 {
			succConf = conf
		} else {
			failedConf = conf
		}
	}

	if failedConf == nil || succConf == nil {
		sys.LogError("not failed or succ conf")
		return
	}

	data := sys.getData()
	openDay := sys.GetOpenDay()
	for _, datum := range data.PlayerData {
		if datum.Camp == 0 {
			continue
		}
		var c *jsondata.YYCampCheeringEventSuccAndFailedAwards
		if datum.Camp == req.WinnerCamp {
			c = succConf
		} else {
			c = failedConf
		}
		dayAwards := c.DayRewards[openDay]
		if dayAwards == nil {
			continue
		}
		mailmgr.SendMailToActor(datum.PlayerId, &mailargs.SendMailSt{
			ConfId:  c.MailId,
			Rewards: dayAwards.Rewards,
		})
	}
}

func init() {
	yymgr.RegisterYYType(yydefine.YYCampCheeringEvent, func() iface.IYunYing {
		return &CampCheeringEventSys{}
	})

	net.RegisterGlobalYYSysProto(8, 221, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*CampCheeringEventSys).c2sGetCrossData
	})
	net.RegisterGlobalYYSysProto(8, 222, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*CampCheeringEventSys).c2sRandomCamp
	})
	net.RegisterGlobalYYSysProto(8, 223, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*CampCheeringEventSys).c2sAddProcess
	})
	net.RegisterGlobalYYSysProto(8, 224, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*CampCheeringEventSys).c2sRecAwards
	})
	engine.RegisterSysCall(sysfuncid.C2GYYCampCheeringEventCrossRandomCamp, handleC2GYYCampCheeringEventCrossRandomCamp)
	engine.RegisterSysCall(sysfuncid.C2GYYCampCheeringEventCrossSettlement, handleC2GYYCampCheeringEventCrossSettlement)
	initYYCampCheeringEventGM()
}

func initYYCampCheeringEventGM() {
	gmevent.Register("YYCampCheeringEvent.AddProcess", func(player iface.IPlayer, args ...string) bool {
		yymgr.EachAllYYObj(yydefine.YYCampCheeringEvent, func(obj iface.IYunYing) {
			s, ok := obj.(*CampCheeringEventSys)
			if !ok {
				return
			}
			data := s.getPlayerData(player.GetId())
			if data.Camp == 0 {
				return
			}
			data.Process += utils.AtoUint32(args[0])
			s.s2cInfo(player)
			err := player.CallActorSmallCrossFunc(actorfuncid.G2CYYCampCheeringEventCrossAddProcess, &pb3.G2FYYCampCheeringEventCrossAddProcessReq{
				ActiveId: s.GetId(),
				ConfIdx:  s.GetConfIdx(),
				ConfName: s.ConfName,
				Process:  utils.AtoUint32(args[0]),
				Camp:     data.Camp,
			})
			if err != nil {
				s.LogError("err:%v", err)
				return
			}
		})
		return true
	}, 1)
	gmevent.Register("YYCampCheeringEvent.RandomCamp", func(player iface.IPlayer, args ...string) bool {
		yymgr.EachAllYYObj(yydefine.YYCampCheeringEvent, func(obj iface.IYunYing) {
			s, ok := obj.(*CampCheeringEventSys)
			if !ok {
				return
			}
			actorId := player.GetId()
			data := s.getPlayerData(actorId)
			if data.Camp != 0 {
				return
			}
			err := player.CallActorSmallCrossFunc(actorfuncid.G2CYYCampCheeringEventCrossRandomCamp, &pb3.G2FYYCampCheeringEventCrossRandomCampReq{
				ActiveId: s.GetId(),
				ConfIdx:  s.GetConfIdx(),
				ConfName: s.ConfName,
			})
			if err != nil {
				s.LogError("err:%v", err)
				return
			}
		})
		return true
	}, 1)
}
