/**
 * @Author: zjj
 * @Date: 2023年10月30日
 * @Desc: 限时货币礼包
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
	"math"
	"time"
)

// 限时货币礼包系统

type TimeLimitMoneyPackSys struct {
	*QuestTargetBase
	timer *time_util.Timer
}

func (s *TimeLimitMoneyPackSys) resetTimer() {
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

func (s *TimeLimitMoneyPackSys) handleTimeout() {
	owner := s.GetOwner()
	ids := s.handleCheckTimeLimitPackOverTime()
	if len(ids) == 0 {
		return
	}
	for _, packId := range ids {
		owner.TriggerQuestEvent(custom_id.QttBuyOrOverTimeTimeLimitMoneyPack, packId, 1)
	}
	s.s2cInfo()
	s.resetTimer()
}

func (s *TimeLimitMoneyPackSys) stopTimer() {
	if s.timer != nil {
		s.timer.Stop()
	}
}

func (s *TimeLimitMoneyPackSys) OnOpen() {
	s.acceptAllQuest()
	s.handleTimeout()
	s.s2cInfo()
}

func (s *TimeLimitMoneyPackSys) OnLogin() {
	s.handleTimeout()
	s.s2cInfo()
}

func (s *TimeLimitMoneyPackSys) OnReconnect() {
	s.handleTimeout()
	s.s2cInfo()
}

func (s *TimeLimitMoneyPackSys) OnDestroy() {
	s.stopTimer()
}

func (s *TimeLimitMoneyPackSys) getData() *pb3.TimeLimitSysPackState {
	if s.GetBinaryData().TimeLimitMoneyPack == nil {
		s.GetBinaryData().TimeLimitMoneyPack = &pb3.TimeLimitSysPackState{}
	}
	return s.GetBinaryData().TimeLimitMoneyPack
}

func (s *TimeLimitMoneyPackSys) s2cInfo() {
	s.SendProto3(160, 0, &pb3.S2C_160_0{
		TimeLimitMoneyPack: s.getData(),
	})
}

func (s *TimeLimitMoneyPackSys) acceptAllQuest() {
	questConfMgr, ok := jsondata.GetTimeLimitMoneyQuestConfMgr()
	if !ok {
		s.GetOwner().LogError("actor[%d] time limit money quest list not found, accept all quest failed", s.GetOwner().GetId())
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

// 触发货币礼包
func (s *TimeLimitMoneyPackSys) triggerPack(questId uint32) {
	owner := s.GetOwner()
	questConf, ok := jsondata.GetTimeLimitMoneyQuestConf(questId)
	if !ok {
		owner.LogError("actor[%d] time limit money quest[%d] not found, accept all quest failed", s.GetOwner().GetId(), questId)
		return
	}

	data := s.getData()
	packIds := questConf.PackIds
	if len(packIds) == 0 {
		owner.LogWarn("not trigger pack, questId %d", questId)
		return
	}

	if !jsondata.CheckTimeLimitMoneyPacketGroup(packIds) {
		owner.LogStack("trigger quest:%d , packIds:%v failed", questId, packIds)
		return
	}

	nowSec := time_util.NowSec()
	var newList []*pb3.TimeLimitSysPack
	for _, packId := range packIds {
		conf, ok := jsondata.GetTimeLimitMoneyPack(packId)
		if !ok {
			owner.LogWarn("not found pack %d", packId)
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
	s.SendProto3(160, 2, &pb3.S2C_160_2{
		List: newList,
	})
	s.s2cInfo()
	s.handleTimeout()
}

func (s *TimeLimitMoneyPackSys) getQuestIdSet(qtt uint32) map[uint32]struct{} {
	var set = make(map[uint32]struct{})
	mgr, ok := jsondata.GetTimeLimitMoneyQuestConfMgr()
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

func (s *TimeLimitMoneyPackSys) getUnFinishQuestData(id uint32) *pb3.QuestData {
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

func (s *TimeLimitMoneyPackSys) getQuestTargetConf(id uint32) []*jsondata.QuestTargetConf {
	conf, ok := jsondata.GetTimeLimitMoneyQuestConf(id)
	if !ok {
		s.GetOwner().LogError("actor[%d] not found money quest target conf , quest id %d", s.GetOwner().GetId(), id)
		return nil
	}
	return conf.Targets
}

func (s *TimeLimitMoneyPackSys) onUpdateTargetData(id uint32) {
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
	questConf, ok := jsondata.GetTimeLimitMoneyQuestConf(id)
	if ok && questConf.IsRepeat {
		s.rmQuest(id)
		var newQuest = &pb3.QuestData{
			Id: questConf.QuestId,
		}
		s.OnAcceptQuest(newQuest)
		s.getData().Quests = append(s.getData().Quests, newQuest)
	}
}

func (s *TimeLimitMoneyPackSys) getPacket(hdl uint64) (*pb3.TimeLimitSysPack, error) {
	data := s.getData()
	var curPack *pb3.TimeLimitSysPack
	for _, pack := range data.List {
		if pack.Hdl != hdl {
			continue
		}
		curPack = pack
		break
	}

	if curPack == nil {
		return nil, neterror.ParamsInvalidError("actor[%d] money pack not open, packId is %d", s.GetOwner().GetId(), hdl)
	}

	return curPack, nil
}

func (s *TimeLimitMoneyPackSys) c2sBuyMoneyPack(msg *base.Message) error {
	var req pb3.C2S_160_1
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}

	hdl := req.Hdl
	packId := utils.High32(hdl)
	moneyPack, ok := jsondata.GetTimeLimitMoneyPack(packId)
	if !ok {
		err := neterror.ConfNotFoundError("actor[%d] not found money pack conf , id is %d", s.GetOwner().GetId(), packId)
		s.GetOwner().LogWarn("warn: %v", err)
		return err
	}

	curPack, err := s.getPacket(hdl)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}

	liveTime := curPack.LiveTime
	if liveTime > 0 && liveTime+curPack.StartAt < time_util.NowSec() {
		return neterror.ParamsInvalidError("actor[%d] money pack over time, packId is %d, start at is %d, live time is %d", s.GetOwner().GetId(), packId, curPack.StartAt, liveTime)
	}

	totalBuyCount := curPack.TotalBuyCount
	if totalBuyCount > moneyPack.BuyNum {
		return neterror.ParamsInvalidError("actor[%d] buy money pack over max num, cur num is %d , max is %d", s.GetOwner().GetId(), totalBuyCount, moneyPack.BuyNum)
	}

	if totalBuyCount+req.Count > moneyPack.BuyNum {
		return neterror.ParamsInvalidError("actor[%d] buy money pack over max num, cur num is %d ,want num is %d , max is %d", s.GetOwner().GetId(), totalBuyCount, req.Count, moneyPack.BuyNum)
	}

	var needConsumeVec jsondata.ConsumeVec
	for _, consume := range moneyPack.Consume {
		needConsumeVec = append(needConsumeVec, &jsondata.Consume{
			Type:       consume.Type,
			Id:         consume.Id,
			Count:      consume.Count * req.Count,
			CanAutoBuy: consume.CanAutoBuy,
			Job:        consume.Job,
		})
	}

	if len(needConsumeVec) > 0 && !s.GetOwner().ConsumeByConf(needConsumeVec, false, common.ConsumeParams{LogId: pb3.LogId_LogTimeLimitMoneyPackBuyPack}) {
		return neterror.ConsumeFailedError("buy %d failed", req.Hdl)
	}

	if len(moneyPack.Awards) > 0 {
		var needGiveAwards jsondata.StdRewardVec
		for i := uint32(0); i < req.Count; i++ {
			needGiveAwards = append(needGiveAwards, moneyPack.Awards...)
		}
		engine.GiveRewards(s.GetOwner(), needGiveAwards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogTimeLimitMoneyPackGiveAwards,
		})
	}

	curPack.TotalBuyCount += req.Count
	s.SendProto3(160, 1, &pb3.S2C_160_1{
		Pack: curPack,
	})
	s.handleCheckTimeLimitPackOverTime()
	s.afterBuy(curPack)
	return nil
}

func (s *TimeLimitMoneyPackSys) afterBuy(curPack *pb3.TimeLimitSysPack) {
	moneyPack, _ := jsondata.GetTimeLimitMoneyPack(curPack.Id)
	list := s.getData().List
	// 是否需要移除
	if curPack.TotalBuyCount < moneyPack.BuyNum {
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
	s.GetOwner().TriggerQuestEvent(custom_id.QttBuyOrOverTimeTimeLimitMoneyPack, curPack.Id, 1)
}

func (s *TimeLimitMoneyPackSys) rmMinPackIdBySameGroup(newList, oldDataList []*pb3.TimeLimitSysPack) []*pb3.TimeLimitSysPack {
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

func (s *TimeLimitMoneyPackSys) handleCheckTimeLimitPackOverTime() []uint32 {
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

func (s *TimeLimitMoneyPackSys) rmQuest(id uint32) {
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

func createTimeLimitMoneyPackSys() iface.ISystem {
	sys := &TimeLimitMoneyPackSys{}
	sys.QuestTargetBase = &QuestTargetBase{
		GetQuestIdSetFunc:        sys.getQuestIdSet,
		GetUnFinishQuestDataFunc: sys.getUnFinishQuestData,
		GetTargetConfFunc:        sys.getQuestTargetConf,
		OnUpdateTargetDataFunc:   sys.onUpdateTargetData,
	}
	return sys
}

func init() {
	RegisterSysClass(sysdef.SiTimeLimitMoneyPack, func() iface.ISystem {
		return createTimeLimitMoneyPackSys()
	})
	net.RegisterSysProtoV2(160, 1, sysdef.SiTimeLimitMoneyPack, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*TimeLimitMoneyPackSys).c2sBuyMoneyPack
	})
	gmevent.Register("TimeLimitMoneyPackSys.triggerPack", func(player iface.IPlayer, args ...string) bool {
		sys := player.GetSysObj(sysdef.SiTimeLimitMoneyPack)
		if sys == nil || !sys.IsOpen() {
			return false
		}
		sys.(*TimeLimitMoneyPackSys).triggerPack(utils.AtoUint32(args[0]))
		return true
	}, 1)
}
