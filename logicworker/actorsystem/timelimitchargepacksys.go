/**
 * @Author: zjj
 * @Date: 2023年10月30日
 * @Desc: 限时直购礼包
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	neterror "jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
	"math"
	"time"
)

// 限时直购礼包系统

type TimeLimitChargePackSys struct {
	*QuestTargetBase
	timer *time_util.Timer
}

func (s *TimeLimitChargePackSys) resetTimer() {
	// 开启定时器
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}

	list := s.getData().List
	if len(list) == 0 {
		return
	}

	// firstTimeOutPacket
	var firstTimeOutPacket *pb3.TimeLimitSysPack
	nowSec := time_util.NowSec()
	diff := uint32(math.MaxUint32)
	for _, pack := range list {
		endAt := pack.LiveTime + pack.StartAt
		dissAt := nowSec - endAt
		if dissAt < diff {
			diff = dissAt
			firstTimeOutPacket = pack
		}
	}

	if diff == 0 || firstTimeOutPacket == nil {
		return
	}

	owner := s.GetOwner()
	s.timer = owner.SetTimeout(time.Duration(diff)*time.Second, s.handleTimeout)
}

func (s *TimeLimitChargePackSys) handleTimeout() {
	owner := s.GetOwner()
	ids := s.handleCheckTimeLimitPackOverTime()
	if len(ids) == 0 {
		return
	}
	for _, packId := range ids {
		owner.TriggerQuestEvent(custom_id.QttBuyOrOverTimeTimeLimitChargePack, packId, 1)
	}
	s.s2cInfo()
	s.resetTimer()
}

func (s *TimeLimitChargePackSys) stopTimer() {
	if s.timer != nil {
		s.timer.Stop()
	}
}

func (s *TimeLimitChargePackSys) OnOpen() {
	s.acceptAllQuest()
	s.handleTimeout()
	s.s2cInfo()
}

func (s *TimeLimitChargePackSys) OnLogin() {
	s.handleTimeout()
	s.s2cInfo()
}

func (s *TimeLimitChargePackSys) OnReconnect() {
	s.handleTimeout()
	s.s2cInfo()
}

func (s *TimeLimitChargePackSys) OnDestroy() {
	s.stopTimer()
}
func (s *TimeLimitChargePackSys) getData() *pb3.TimeLimitSysPackState {
	if s.GetBinaryData().TimeLimitChargePack == nil {
		s.GetBinaryData().TimeLimitChargePack = &pb3.TimeLimitSysPackState{}
	}
	return s.GetBinaryData().TimeLimitChargePack
}

func (s *TimeLimitChargePackSys) s2cInfo() {
	s.SendProto3(160, 20, &pb3.S2C_160_20{
		TimeLimitChargePack: s.getData(),
	})
}

func (s *TimeLimitChargePackSys) acceptAllQuest() {
	questConfMgr, ok := jsondata.GetTimeLimitChargeQuestConfMgr()
	if !ok {
		s.GetOwner().LogError("actor[%d] time limit charge quest list not found, accept all quest failed", s.GetOwner().GetId())
		return
	}
	data := s.getData()
	for _, questConf := range questConfMgr {
		data.Quests = append(data.Quests, &pb3.QuestData{
			Id: questConf.QuestId,
		})
	}
	for _, quest := range data.Quests {
		s.OnAcceptQuest(quest)
	}
}

// 触发直购礼包
func (s *TimeLimitChargePackSys) triggerPack(questId uint32) {
	owner := s.GetOwner()
	questConf, ok := jsondata.GetTimeLimitChargeQuestConf(questId)
	if !ok {
		owner.LogError("actor[%d] time limit charge quest[%d] not found, accept all quest failed", s.GetOwner().GetId(), questId)
		return
	}

	data := s.getData()
	packIds := questConf.PackIds
	if len(packIds) == 0 {
		owner.LogWarn("not trigger pack, questId %d", questId)
		return
	}

	if !jsondata.CheckTimeLimitChargePacketGroup(packIds) {
		owner.LogStack("trigger quest:%d , packIds:%v failed", questId, packIds)
		return
	}

	nowSec := time_util.NowSec()
	var newList []*pb3.TimeLimitSysPack
	for _, packId := range packIds {
		conf, ok := jsondata.GetTimeLimitChargePack(packId)
		if !ok {
			owner.LogError("not found pack %d", packId)
			continue
		}
		newList = append(newList, &pb3.TimeLimitSysPack{
			Id:            conf.PackId,
			StartAt:       nowSec,
			TotalBuyCount: 0,
			LiveTime:      conf.LiveTime,
			Hdl:           utils.Make64(nowSec, conf.PackId),
			Group:         conf.Group,
		})
	}

	newDataList := s.rmMinPackIdBySameGroup(newList, data.List)
	newDataList = append(newDataList, newList...)
	data.List = newDataList
	s.SendProto3(160, 22, &pb3.S2C_160_22{
		List: newList,
	})
	s.s2cInfo()
	s.handleTimeout()
}

func (s *TimeLimitChargePackSys) getQuestIdSet(qtt uint32) map[uint32]struct{} {
	var set = make(map[uint32]struct{})
	mgr, ok := jsondata.GetTimeLimitChargeQuestConfMgr()
	if !ok {
		return set
	}
	for _, conf := range mgr {
		var exist bool
		for _, target := range conf.Targets {
			if target.Type != qtt {
				continue
			}
			exist = true
			break
		}
		if exist {
			set[conf.QuestId] = struct{}{}
		}
	}
	return set
}

func (s *TimeLimitChargePackSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	data := s.getData()
	if pie.Uint32s(data.FinishQuestIds).Contains(id) {
		return nil
	}
	for _, questData := range data.Quests {
		if questData.Id != id {
			continue
		}
		return questData
	}
	return nil
}

func (s *TimeLimitChargePackSys) getQuestTargetConf(id uint32) []*jsondata.QuestTargetConf {
	conf, ok := jsondata.GetTimeLimitChargeQuestConf(id)
	if !ok {
		s.GetOwner().LogError("actor[%d] not found charge quest target conf , quest id %d", s.GetOwner().GetId(), id)
		return nil
	}
	return conf.Targets
}

func (s *TimeLimitChargePackSys) onUpdateTargetData(id uint32) {
	quest := s.getUnFinishQuestData(id)
	if nil == quest {
		return
	}

	if pie.Uint32s(s.getData().FinishQuestIds).Contains(quest.Id) {
		s.GetOwner().LogTrace("quest finished")
		return
	}

	if !s.CheckFinishQuest(quest) {
		s.GetOwner().LogTrace("quest not can finished")
		return
	}

	s.triggerPack(id)
	s.getData().FinishQuestIds = pie.Uint32s(s.getData().FinishQuestIds).Append(id).Unique()

	// 重新接取任务
	questConf, ok := jsondata.GetTimeLimitChargeQuestConf(id)
	if ok && questConf.IsRepeat {
		s.rmQuest(id)
		var newQuest = &pb3.QuestData{
			Id: questConf.QuestId,
		}
		s.OnAcceptQuest(newQuest)
		s.getData().Quests = append(s.getData().Quests, newQuest)
	}
}

func (s *TimeLimitChargePackSys) c2sChooseChargePack(msg *base.Message) error {
	var req pb3.C2S_160_21
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}
	hdl := req.Hdl
	packId := utils.High32(hdl)
	_, ok := jsondata.GetTimeLimitChargePack(packId)
	if !ok {
		err := neterror.ConfNotFoundError("not found time limit charge conf , packId is %d", packId)
		s.GetOwner().LogWarn("err:%v", err)
		return err
	}
	s.SendProto3(160, 21, &pb3.S2C_160_21{
		Hdl: hdl,
	})
	return nil
}

func (s *TimeLimitChargePackSys) handleCheckTimeLimitPackOverTime() []uint32 {
	if !s.IsOpen() {
		return nil
	}
	var timeSec = time_util.NowSec()
	list := s.getData().List
	if len(list) == 0 {
		return nil
	}
	var newList []*pb3.TimeLimitSysPack
	var timeOutPackIds []uint32
	for _, pack := range list {
		liveTime := pack.LiveTime
		if liveTime > 0 && pack.StartAt+liveTime < timeSec {
			timeOutPackIds = append(timeOutPackIds, pack.Id)
			continue
		}
		newList = append(newList, pack)
	}
	s.getData().List = newList
	return timeOutPackIds
}

func (s *TimeLimitChargePackSys) rmQuest(id uint32) {
	quests := s.getData().Quests
	var newQuests []*pb3.QuestData
	for _, quest := range quests {
		if quest.Id == id {
			continue
		}
		newQuests = append(newQuests, quest)
	}
	var newFinishIds []uint32
	for _, questId := range s.getData().FinishQuestIds {
		if questId == id {
			continue
		}
		newFinishIds = append(newFinishIds, questId)
	}
	s.getData().Quests = newQuests
	s.getData().FinishQuestIds = newFinishIds
}

func (s *TimeLimitChargePackSys) rmMinPackIdBySameGroup(newList, oldDataList []*pb3.TimeLimitSysPack) []*pb3.TimeLimitSysPack {
	var newDataList []*pb3.TimeLimitSysPack
	for _, pack := range newList {
		if pack.Group == 0 {
			continue
		}

		// 剔除同组且ID更小的礼包
		for _, sysPack := range oldDataList {
			if sysPack.Group != pack.Group {
				newDataList = append(newDataList, sysPack)
				continue
			}
			if sysPack.Id >= pack.Id {
				newDataList = append(newDataList, sysPack)
				continue
			}
		}
	}
	return newDataList
}

func (s *TimeLimitChargePackSys) getPacket(chargeId uint32) (*pb3.TimeLimitSysPack, error) {
	data := s.getData()
	var curPack *pb3.TimeLimitSysPack
	for _, pack := range data.List {
		chargePack, _ := jsondata.GetTimeLimitChargePack(pack.Id)
		if chargePack == nil {
			continue
		}
		if chargePack.ChargeId != chargeId {
			continue
		}
		curPack = pack
		break
	}
	if curPack == nil {
		return nil, neterror.ParamsInvalidError("actor[%d] charge pack not find, pack chargeId is %d", s.GetOwner().GetId(), chargeId)
	}
	return curPack, nil
}

func (s *TimeLimitChargePackSys) afterBuy(curPack *pb3.TimeLimitSysPack) {
	chargePack, _ := jsondata.GetTimeLimitChargePack(curPack.Id)
	list := s.getData().List
	// 是否需要移除
	if curPack.TotalBuyCount < chargePack.BuyNum {
		return
	}
	var newPackList []*pb3.TimeLimitSysPack
	for _, pack := range list {
		if pack.Hdl == curPack.Hdl {
			continue
		}
		newPackList = append(newPackList, pack)
	}
	s.getData().List = newPackList
	s.s2cInfo()
	s.GetOwner().TriggerQuestEvent(custom_id.QttBuyOrOverTimeTimeLimitChargePack, curPack.Id, 1)
}

func createTimeLimitChargePackSys() iface.ISystem {
	sys := &TimeLimitChargePackSys{}
	sys.QuestTargetBase = &QuestTargetBase{
		GetQuestIdSetFunc:        sys.getQuestIdSet,
		GetUnFinishQuestDataFunc: sys.getUnFinishQuestData,
		GetTargetConfFunc:        sys.getQuestTargetConf,
		OnUpdateTargetDataFunc:   sys.onUpdateTargetData,
	}
	return sys
}

func handleTimeLimitChargePackCharge(actor iface.IPlayer, conf *jsondata.ChargeConf, _ *pb3.ChargeCallBackParams) bool {
	if !handleCheckTimeLimitChargePackCharge(actor, conf) {
		return false
	}
	sys := actor.GetSysObj(sysdef.SiTimeLimitChargePack).(*TimeLimitChargePackSys)

	curPack, err := sys.getPacket(conf.ChargeId)
	if err != nil {
		sys.LogError("err:%v", err)
		return false
	}

	// GiveAwards
	chargePack, ok := jsondata.GetTimeLimitChargePack(curPack.Id)
	if !ok {
		actor.LogError("actor[%d] not found charge pack, cur pack id %d", actor.GetId(), curPack.Id)
		return false
	}
	if len(chargePack.Awards) > 0 {
		engine.GiveRewards(actor, chargePack.Awards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogTimeLimitChargePackGiveAwards,
		})
	}

	curPack.TotalBuyCount++
	actor.SendProto3(160, 23, &pb3.S2C_160_23{
		Hdl:  curPack.Hdl,
		Pack: curPack,
	})

	sys.afterBuy(curPack)
	return true
}

func handleCheckTimeLimitChargePackCharge(actor iface.IPlayer, conf *jsondata.ChargeConf) bool {
	sysObj := actor.GetSysObj(sysdef.SiTimeLimitChargePack)
	if sysObj == nil {
		return false
	}
	if !sysObj.IsOpen() {
		return false
	}
	sys, ok := sysObj.(*TimeLimitChargePackSys)
	if !ok {
		return false
	}

	curPack, err := sys.getPacket(conf.ChargeId)
	if err != nil {
		sys.LogError("err:%v", err)
		return false
	}

	liveTime := curPack.LiveTime
	if liveTime > 0 && liveTime+curPack.StartAt < time_util.NowSec() {
		sys.LogWarn("actor[%d] charge pack over time, packId is %d, start at is %d, live time is %d", sys.GetOwner().GetId(), curPack.Id, curPack.StartAt, liveTime)
		return false
	}

	pack, ok := jsondata.GetTimeLimitChargePack(curPack.Id)
	if !ok {
		return false
	}

	totalBuyCount := curPack.TotalBuyCount
	if totalBuyCount > pack.BuyNum {
		sys.LogWarn("actor[%d] buy charge pack over max num, cur num is %d , max is %d", sys.GetOwner().GetId(), totalBuyCount, pack.BuyNum)
		return false
	}

	if totalBuyCount+1 > pack.BuyNum {
		sys.LogWarn("actor[%d] buy charge pack over max num, cur num is %d ,want num is %d , max is %d", sys.GetOwner().GetId(), totalBuyCount, 1, pack.BuyNum)
		return false
	}

	return true
}

func init() {
	RegisterSysClass(sysdef.SiTimeLimitChargePack, func() iface.ISystem {
		return createTimeLimitChargePackSys()
	})

	engine.RegChargeEvent(chargedef.TimeLimitSysChargePack, handleCheckTimeLimitChargePackCharge, handleTimeLimitChargePackCharge)
	net.RegisterSysProtoV2(160, 21, sysdef.SiTimeLimitChargePack, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*TimeLimitChargePackSys).c2sChooseChargePack
	})
	gmevent.Register("TimeLimitChargePackSys.triggerPack", func(player iface.IPlayer, args ...string) bool {
		sys := player.GetSysObj(sysdef.SiTimeLimitChargePack)
		if sys == nil || !sys.IsOpen() {
			return false
		}
		sys.(*TimeLimitChargePackSys).triggerPack(utils.AtoUint32(args[0]))
		return true
	}, 1)
}
