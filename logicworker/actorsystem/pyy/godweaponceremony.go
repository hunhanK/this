/**
 * @Author: LvYuMeng
 * @Date: 2024/5/21
 * @Desc: 神兵典礼
**/

package pyy

import (
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/guildmgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type GodWeaponCeremonySys struct {
	*YYQuestTargetBase
}

func createGodWeaponCeremonySys() iface.IPlayerYY {
	obj := &GodWeaponCeremonySys{}
	obj.YYQuestTargetBase = &YYQuestTargetBase{
		GetQuestIdSetFunc:        obj.getQuestIdSet,
		GetUnFinishQuestDataFunc: obj.getUnFinishQuestData,
		GetTargetConfFunc:        obj.getTargetConf,
		OnUpdateTargetDataFunc:   obj.onUpdateTargetData,
	}
	return obj
}

var godWeaponCeremonyRank = make(map[uint32]*base.Rank)

func (s *GodWeaponCeremonySys) getRank() *base.Rank {
	if !s.IsOpen() {
		return nil
	}
	conf, err := s.conf()
	if nil != err {
		return nil
	}
	if nil == godWeaponCeremonyRank[s.Id] {
		rank := new(base.Rank)
		rank.Init(conf.MaxRank)
		godWeaponCeremonyRank[s.Id] = rank
	}
	return godWeaponCeremonyRank[s.Id]
}

func (s *GodWeaponCeremonySys) data() *pb3.PYY_GodWeaponCeremony {
	yyData := s.GetYYData()
	if nil == yyData.GodWeaponCeremony {
		yyData.GodWeaponCeremony = make(map[uint32]*pb3.PYY_GodWeaponCeremony)
	}
	if nil == yyData.GodWeaponCeremony[s.Id] {
		yyData.GodWeaponCeremony[s.Id] = &pb3.PYY_GodWeaponCeremony{}
	}
	if nil == yyData.GodWeaponCeremony[s.Id].DailyScrificeCount {
		yyData.GodWeaponCeremony[s.Id].DailyScrificeCount = make(map[uint32]uint32)
	}
	if nil == yyData.GodWeaponCeremony[s.Id].DailyQuest {
		yyData.GodWeaponCeremony[s.Id].DailyQuest = make(map[uint32]*pb3.QuestData)
	}
	if nil == yyData.GodWeaponCeremony[s.Id].DailyQuestCount {
		yyData.GodWeaponCeremony[s.Id].DailyQuestCount = make(map[uint32]uint32)
	}
	return yyData.GodWeaponCeremony[s.Id]
}

func (s *GodWeaponCeremonySys) globalData() *pb3.GodWeaponCeremonyRecord {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.PyyDatas {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}
	if nil == globalVar.PyyDatas.GodWeaponCeremonyRecord {
		globalVar.PyyDatas.GodWeaponCeremonyRecord = make(map[uint32]*pb3.GodWeaponCeremonyRecord)
	}
	if nil == globalVar.PyyDatas.GodWeaponCeremonyRecord[s.Id] {
		globalVar.PyyDatas.GodWeaponCeremonyRecord[s.Id] = &pb3.GodWeaponCeremonyRecord{
			StartTime: s.GetOpenTime(),
			EndTime:   s.GetEndTime(),
			ConfIdx:   s.GetConfIdx(),
		}
	}
	if nil == globalVar.PyyDatas.GodWeaponCeremonyRecord[s.Id].DailyScoreMap {
		globalVar.PyyDatas.GodWeaponCeremonyRecord[s.Id].DailyScoreMap = make(map[uint64]uint32)
	}
	return globalVar.PyyDatas.GodWeaponCeremonyRecord[s.Id]
}

func (s *GodWeaponCeremonySys) OnOpen() {
	s.checkResetQuest()
	s.s2cInfo()
}

func (s *GodWeaponCeremonySys) OnEnd() {
	s.sendDailyAward()
	s.sendEndAward()
}

func (s *GodWeaponCeremonySys) ResetData() {
	yyData := s.GetYYData()
	if nil == yyData.GodWeaponCeremony {
		return
	}
	delete(yyData.GodWeaponCeremony, s.Id)
}

func (s *GodWeaponCeremonySys) conf() (*jsondata.YYGodWeaponCeremonyConf, error) {
	conf := jsondata.GetYYGodWeaponCeremonyConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return nil, neterror.ConfNotFoundError("GodWeaponCeremonySys conf is nil")
	}
	return conf, nil
}

func (s *GodWeaponCeremonySys) checkResetQuest() {
	if !s.IsOpen() {
		return
	}
	conf, err := s.conf()
	if nil != err {
		return
	}
	data := s.data()
	openDay := s.GetOpenDay()
	for _, questConf := range conf.DailyTask {
		if openDay < questConf.OpenDay || openDay > questConf.EndDay { //删除非运营期间任务
			delete(data.DailyQuest, questConf.Id)
			continue
		}
		if data.DailyQuestCount[questConf.Id] >= questConf.Count { //奖励次数限制
			continue
		}
		if nil != data.DailyQuest[questConf.Id] {
			continue
		}
		quest := &pb3.QuestData{
			Id: questConf.Id,
		}
		data.DailyQuest[questConf.Id] = quest
		s.OnAcceptQuest(quest)
	}
}

func (s *GodWeaponCeremonySys) checkTaskComplete(id uint32) {
	quest := s.getUnFinishQuestData(id)
	if nil == quest {
		return
	}
	conf, err := s.conf()
	if nil != err {
		return
	}
	data := s.data()
	if s.CheckFinishQuest(quest) {
		dailyTaskConf := conf.GetDailyTask(id)
		if nil == dailyTaskConf {
			return
		}
		if data.DailyQuestCount[id] >= dailyTaskConf.Count {
			return
		}
		data.DailyQuestCount[id]++
		if len(dailyTaskConf.Rewards) > 0 {
			mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
				ConfId:  common.Mail_GodWeaponCeremonyActAwards,
				Rewards: dailyTaskConf.Rewards,
				Content: &mailargs.PYYNameArgs{Name: dailyTaskConf.Name},
			})
		}
		delete(data.DailyQuest, id)
		s.checkResetQuest()
	}
	return
}

func (s *GodWeaponCeremonySys) onUpdateTargetData(id uint32) {
	if !s.IsOpen() {
		return
	}

	quest := s.getUnFinishQuestData(id)
	if nil == quest {
		return
	}

	data := s.data()
	s.checkTaskComplete(id)
	s.SendProto3(68, 8, &pb3.S2C_68_8{
		ActiveId: s.GetId(),
		Count:    data.DailyQuestCount[id],
		Quest:    quest,
	})
}

func (s *GodWeaponCeremonySys) getUnFinishQuestData(id uint32) *pb3.QuestData {
	return s.data().DailyQuest[id]
}

func (s *GodWeaponCeremonySys) getQuestIdSet(qt uint32) map[uint32]struct{} {
	if conf, _ := s.conf(); nil != conf {
		set := make(map[uint32]struct{})
		for _, v := range conf.DailyTask {
			for _, target := range v.Targets {
				if target.Type == qt {
					set[v.Id] = struct{}{}
				}
			}
		}
		return set
	}
	return nil
}

func (s *GodWeaponCeremonySys) getTargetConf(questId uint32) []*jsondata.QuestTargetConf {
	if conf, _ := s.conf(); nil != conf {
		for _, v := range conf.DailyTask {
			if v.Id == questId {
				return v.Targets
			}
		}
	}
	return nil
}

func (s *GodWeaponCeremonySys) OnReconnect() {
	s.s2cInfo()
}

func (s *GodWeaponCeremonySys) Login() {
	s.checkResetQuest()
	s.s2cInfo()
}

func (s *GodWeaponCeremonySys) s2cInfo() {
	s.SendProto3(68, 0, &pb3.S2C_68_0{
		ActiveId: s.GetId(),
		Info:     s.data(),
	})
	s.SendProto3(68, 7, &pb3.S2C_68_7{
		ActiveId:   s.GetId(),
		GuildScore: s.calcGuildDailyScore(s.GetPlayer().GetGuildId()),
	})
}

func (s *GodWeaponCeremonySys) c2sProgressAward(msg *base.Message) error {
	var req pb3.C2S_68_1
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf, err := s.conf()
	if nil != err {
		return err
	}

	data := s.data()
	if pie.Uint32s(data.ProgressRev).Contains(req.GetId()) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}
	paConf := conf.GetProgressAward(req.GetId())
	if nil == paConf {
		return neterror.ConfNotFoundError("GodWeaponCeremonySys progress award conf(%d) is nil", req.GetId())
	}
	if paConf.Score > data.Score {
		return neterror.ParamsInvalidError("GodWeaponCeremonySys score not enough")
	}

	data.ProgressRev = append(data.ProgressRev, paConf.Id)
	engine.GiveRewards(s.GetPlayer(), paConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogGodWeaponCeremonyProgressAward})
	engine.BroadcastTipMsgById(tipmsgid.GodWeaponCeremonyGift, s.GetPlayer().GetId(), s.GetPlayer().GetName())
	s.SendProto3(68, 1, &pb3.S2C_68_1{
		ActiveId: s.GetId(),
		Id:       paConf.Id,
	})
	return nil
}

func (s *GodWeaponCeremonySys) calcGuildDailyScore(guildId uint64) uint32 {
	guild := guildmgr.GetGuildById(guildId)
	if nil == guild {
		return 0
	}
	gData := s.globalData()
	var totalScore uint32
	for memberId := range guild.Members {
		totalScore += gData.DailyScoreMap[memberId]
	}
	return totalScore
}

func (s *GodWeaponCeremonySys) broadcastGuildDailyScore(guildId uint64) {
	guild := guildmgr.GetGuildById(guildId)
	if nil == guild {
		return
	}

	guild.BroadcastProto(68, 7, &pb3.S2C_68_7{ActiveId: s.GetId(), GuildScore: s.calcGuildDailyScore(guildId)})

	return
}

const (
	GodWeaponCeremonyDailyReachPersonal = 1
	GodWeaponCeremonyDailyReachGuild    = 2
)

func (s *GodWeaponCeremonySys) getScoreByType(scoreType uint32) uint32 {
	var score uint32
	switch scoreType {
	case GodWeaponCeremonyDailyReachPersonal:
		score = s.data().DailyScore
	case GodWeaponCeremonyDailyReachGuild:
		score = s.calcGuildDailyScore(s.GetPlayer().GetGuildId())
	}
	return score
}

func (s *GodWeaponCeremonySys) c2sDailyReachAward(msg *base.Message) error {
	var req pb3.C2S_68_2
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf, err := s.conf()
	if nil != err {
		return err
	}

	data := s.data()
	draConf, ok := conf.DailyReachAward[req.GetType()]
	if !ok {
		return neterror.ConfNotFoundError("GodWeaponCeremonySys DailyReachAward conf(%d) is nil", req.GetType())
	}

	score := s.getScoreByType(req.GetType())
	if score < draConf.Score {
		return neterror.ParamsInvalidError("GodWeaponCeremonySys DailyReachAward score not enough")
	}
	if utils.IsSetBit(data.DailyReachRev, draConf.Type) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	data.DailyReachRev = utils.SetBit(data.DailyReachRev, draConf.Type)
	engine.GiveRewards(s.GetPlayer(), draConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogGodWeaponCeremonyDailyReach})

	s.SendProto3(68, 2, &pb3.S2C_68_2{
		ActiveId: s.GetId(),
		Type:     req.GetType(),
	})

	return nil
}

func (s *GodWeaponCeremonySys) c2sDailyChargeAward(msg *base.Message) error {
	var req pb3.C2S_68_3
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf, err := s.conf()
	if nil != err {
		return err
	}

	data := s.data()
	if pie.Uint32s(data.DailyChargeRev).Contains(req.GetId()) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	dailyChargeAwardConf := conf.GetDailyChargeAward(req.GetId())
	if nil == dailyChargeAwardConf {
		return neterror.ConfNotFoundError("GodWeaponCeremonySys daily charge conf(%d) is nil", req.GetId())
	}

	if s.GetDailyCharge() < dailyChargeAwardConf.Amount {
		return neterror.ParamsInvalidError("GodWeaponCeremonySys daily charge not enough")
	}

	data.DailyChargeRev = append(data.DailyChargeRev, req.GetId())
	engine.GiveRewards(s.GetPlayer(), dailyChargeAwardConf.Rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogGodWeaponCeremonyDailyCharge})

	s.SendProto3(68, 3, &pb3.S2C_68_3{
		ActiveId: s.GetId(),
		Id:       req.GetId(),
	})
	return nil
}

func (s *GodWeaponCeremonySys) checkSacrificeCond(conf *jsondata.GodWeaponCeremonyDailySacrifice, itemList []uint64) error {
	if conf.OpenDay != s.GetOpenDay() {
		return neterror.ConfNotFoundError("GodWeaponCeremonySys daily sacrifice not this open day")
	}
	if !pie.Uint64s(itemList).AreUnique() || len(itemList) == 0 {
		return neterror.ParamsInvalidError("GodWeaponCeremonySys daily sacrifice item params err")
	}
	num := uint32(len(itemList))
	if s.data().DailyScrificeCount[conf.Id]+num > conf.Count {
		return neterror.ParamsInvalidError("GodWeaponCeremonySys daily sacrifice receive limit")
	}
	checkItemCond := func(id uint64) bool {
		if conf.ItemType == 0 {
			return pie.Uint32s(conf.Params).Contains(uint32(id))
		}
		bagSys, ok := s.GetPlayer().GetSysObj(sysdef.SiBag).(*actorsystem.BagSystem)
		if !ok {
			return false
		}
		item := bagSys.FindItemByHandle(id)
		if nil == item || item.Count > 1 {
			return false
		}
		itemConf := jsondata.GetItemConfig(item.GetItemId())
		if nil == itemConf || itemConf.Type != conf.ItemType {
			return false
		}
		if len(conf.Params) < 5 {
			return false
		}
		job, sex, quality, stage, star := conf.Params[0], conf.Params[1], conf.Params[2], conf.Params[3], conf.Params[4]
		if job > 0 && itemConf.Job > 0 && uint32(itemConf.Job) != s.GetPlayer().GetJob() {
			return false
		}
		if sex > 0 && itemConf.Sex > 0 && uint32(itemConf.Sex) != s.GetPlayer().GetSex() {
			return false
		}
		if quality > 0 && itemConf.Quality < quality {
			return false
		}
		if stage > 0 && itemConf.Stage < stage {
			return false
		}
		if star > 0 && itemConf.Star < star {
			return false
		}
		return true
	}

	for _, itemId := range itemList {
		if !checkItemCond(itemId) {
			return neterror.ParamsInvalidError("GodWeaponCeremonySys daily sacrifice not suit")
		}
	}
	return nil
}

func (s *GodWeaponCeremonySys) sacrificeItem(cosType uint32, itemList []uint64) bool {
	if cosType == 0 {
		var consumes jsondata.ConsumeVec
		for _, itemId := range itemList {
			consumes = append(consumes, &jsondata.Consume{Id: uint32(itemId), Count: 1})
		}
		return s.GetPlayer().ConsumeByConf(consumes, false, common.ConsumeParams{LogId: pb3.LogId_LogGodWeaponCeremonyDailySacrifice})
	} else {
		if bagSys, ok := s.GetPlayer().GetSysObj(sysdef.SiBag).(*actorsystem.BagSystem); ok {
			for _, hdl := range itemList {
				if !bagSys.RemoveItemByHandle(hdl, pb3.LogId_LogGodWeaponCeremonyDailySacrifice) {
					s.GetPlayer().LogError("remove items(%v) hdl(%v) err", itemList, hdl)
					return false
				}
			}
			return true
		}
	}
	return false
}

func (s *GodWeaponCeremonySys) c2sDailySacrifice(msg *base.Message) error {
	var req pb3.C2S_68_5
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf, err := s.conf()
	if nil != err {
		return err
	}

	sacrificeConf := conf.GetDailySacrifice(req.GetId())
	if nil == sacrificeConf {
		return neterror.ConfNotFoundError("GodWeaponCeremonySys daily sacrifice conf(%d) is nil", req.GetId())
	}

	if err := s.checkSacrificeCond(sacrificeConf, req.GetExt()); nil != err {
		return err
	}

	if !s.sacrificeItem(sacrificeConf.ItemType, req.GetExt()) {
		s.GetPlayer().SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	data := s.data()
	count := uint32(len(req.GetExt()))
	data.DailyScrificeCount[req.GetId()] += count
	s.addSacrificeScore(sacrificeConf.Sacrifice*count, req.GetId())
	rewards := jsondata.StdRewardMulti(sacrificeConf.Rewards, int64(count))
	engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogGodWeaponCeremonyDailySacrifice})

	s.SendProto3(68, 5, &pb3.S2C_68_5{
		ActiveId: s.GetId(),
		Id:       req.GetId(),
		Progress: data.DailyScrificeCount[req.GetId()],
	})
	return nil
}

func (s *GodWeaponCeremonySys) addSacrificeScore(addScore uint32, srcId uint32) {
	data := s.data()
	data.Score += addScore
	data.DailyScore += addScore

	gData := s.globalData()
	gData.DailyScoreMap[s.GetPlayer().GetId()] = data.DailyScore

	s.SendProto3(68, 6, &pb3.S2C_68_6{ActiveId: s.GetId(), Score: data.GetScore(), DailyScore: data.GetDailyScore()})
	s.calcGuildDailyScore(s.GetPlayer().GetGuildId())
	s.broadcastGuildDailyScore(s.GetPlayer().GetGuildId())
	logworker.LogPlayerBehavior(s.GetPlayer(), pb3.LogId_LogGodWeaponCeremonySacrificeScoreChange, &pb3.LogPlayerCounter{
		NumArgs: uint64(s.GetId()),
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"srcId":    srcId,
			"addScore": addScore,
			"score":    data.Score,
		}),
	})

	rank := s.getRank()
	rank.Update(s.GetPlayer().GetId(), int64(data.Score))
	return
}

func (s *GodWeaponCeremonySys) c2sRank(msg *base.Message) error {
	conf, err := s.conf()
	if nil != err {
		return err
	}
	rsp := &pb3.S2C_68_9{ActiveId: s.GetId()}
	if rank := s.getRank(); nil != rank {
		rList := rank.GetList(1, int(conf.MaxRank))
		for _, v := range rList {
			if baseData, ok := manager.GetData(v.GetId(), gshare.ActorDataBase).(*pb3.PlayerDataBase); ok {
				rsp.Rank = append(rsp.Rank, &pb3.GodWeaponCeremonyRank{
					ActorId: v.GetId(),
					Name:    baseData.GetName(),
					Score:   uint32(v.GetScore()),
				})
			}
		}
	}
	s.SendProto3(68, 9, rsp)
	return nil
}

func (s *GodWeaponCeremonySys) sendDailyAward() {
	data := s.data()
	conf, err := s.conf()
	if nil != err {
		return
	}
	var rewardVec []jsondata.StdRewardVec
	if reachConf := conf.DailyReachAward[GodWeaponCeremonyDailyReachPersonal]; nil != reachConf {
		if reachConf.Score <= data.DailyScore && !utils.IsSetBit(data.DailyReachRev, reachConf.Type) {
			data.DailyReachRev = utils.SetBit(data.DailyReachRev, reachConf.Type)
			rewardVec = append(rewardVec, reachConf.Rewards)
		}
	}
	dailyCharge := s.GetDailyCharge()
	for _, v := range conf.DailyChargeAward {
		if v.Amount <= dailyCharge && !pie.Uint32s(data.DailyChargeRev).Contains(v.Id) {
			data.DailyChargeRev = append(data.DailyChargeRev, v.Id)
			rewardVec = append(rewardVec, v.Rewards)
		}
	}
	rewards := jsondata.MergeStdReward(rewardVec...)
	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_GodWeaponCeremonyDailyAwards,
			Rewards: rewards,
		})
	}
}

func (s *GodWeaponCeremonySys) sendEndAward() {
	conf, err := s.conf()
	if nil != err {
		return
	}
	data := s.data()
	var rewardVec []jsondata.StdRewardVec
	for _, v := range conf.ProgressAward {
		if v.Score <= data.Score && !pie.Uint32s(data.ProgressRev).Contains(v.Id) {
			data.ProgressRev = append(data.ProgressRev, v.Id)
			rewardVec = append(rewardVec, v.Rewards)
		}
	}
	rewards := jsondata.MergeStdReward(rewardVec...)
	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  common.Mail_GodWeaponCeremonyEndAwards,
			Rewards: rewards,
		})
	}
}

func (s *GodWeaponCeremonySys) BeforeNewDay() {
	s.sendDailyAward()
}

func (s *GodWeaponCeremonySys) NewDay() {
	data := s.data()
	data.DailyScore = 0
	data.DailyReachRev = 0
	data.DailyChargeRev = nil
	data.DailyScrificeCount = nil
	s.checkResetQuest()
	s.s2cInfo()
}

func onGodWeaponCeremonyMerge(args ...interface{}) {
	godWeaponCeremonyRank = make(map[uint32]*base.Rank)
}

func calcDailyGodWeaponCeremonyAwards(args ...interface{}) {
	pyyData := gshare.GetStaticVar().PyyDatas
	if nil == pyyData {
		return
	}
	curTime := time_util.NowSec()
	calcGuildDailyAward := func(id uint32) {
		gData := pyyData.GodWeaponCeremonyRecord[id]
		if time_util.IsSameDay(curTime, gData.LastCalcTime) {
			return
		}
		for _, guild := range guildmgr.GuildMap {
			pyyConf := jsondata.GetPlayerYYConf(id)
			if nil == pyyConf {
				continue
			}
			conf := jsondata.GetYYGodWeaponCeremonyConf(pyyConf.ConfName, gData.ConfIdx)
			if nil == conf {
				continue
			}
			var totalScore uint32
			for memberId := range guild.Members {
				totalScore += gData.DailyScoreMap[memberId]
			}
			reachConf := conf.DailyReachAward[GodWeaponCeremonyDailyReachGuild]
			if nil == reachConf {
				continue
			}
			if totalScore < reachConf.Score {
				continue
			}
			members := make([]uint64, 0, len(guild.Members))
			for memberId := range guild.Members {
				members = append(members, memberId)
			}
			if len(reachConf.Rewards) > 0 {
				mailmgr.SendMailToActors(members, &mailargs.SendMailSt{
					ConfId:  common.Mail_GodWeaponCeremonyDailyAwards,
					Rewards: reachConf.Rewards,
				})
			}
		}
		gData.LastCalcTime = curTime
		gData.DailyScoreMap = nil
	}

	var delIds []uint32
	for id, gData := range pyyData.GodWeaponCeremonyRecord {
		calcGuildDailyAward(id)
		if gData.EndTime <= curTime {
			delIds = append(delIds, id)
		}
	}
	for _, id := range delIds {
		delete(pyyData.GodWeaponCeremonyRecord, id)
		delete(godWeaponCeremonyRank, id)
	}
}

func saveGodWeaponCeremonyRank(args ...interface{}) {
	if pyyData := gshare.GetStaticVar().PyyDatas; nil != pyyData {
		for id, v := range pyyData.GodWeaponCeremonyRecord {
			if r, ok := godWeaponCeremonyRank[id]; ok {
				v.Rank = r.GetList(1, r.GetRankCount())
			}
		}
	}
}

func loadGodWeaponCeremonyRank(args ...interface{}) {
	calcDailyGodWeaponCeremonyAwards()
	if pyyData := gshare.GetStaticVar().PyyDatas; nil != pyyData {
		for id, v := range pyyData.GodWeaponCeremonyRecord {
			if nil == v.Rank {
				continue
			}
			rank := new(base.Rank)
			rank.Init(30)
			for _, rankItem := range v.Rank {
				rank.Update(rankItem.Id, rankItem.Score)
			}
			godWeaponCeremonyRank[id] = rank
		}
	}
}

func onGodWeaponCeremonyRangeDo(playerId uint64, fn func(s *GodWeaponCeremonySys)) {
	player := manager.GetPlayerPtrById(playerId)
	if nil == player {
		return
	}
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYGodWeaponCeremony)
	for _, obj := range yyList {
		if s, ok := obj.(*GodWeaponCeremonySys); ok && s.IsOpen() {
			fn(s)
			return
		}
	}
	return
}

func onGodWeaponCeremonyGuildAddMember(args ...interface{}) {
	if len(args) < 2 {
		return
	}
	guildId, ok := args[0].(uint64)
	if !ok {
		return
	}

	sendGodWeaponCeremonyGuildScore(guildId)
}

func onGodWeaponCeremonyGuildRemoveMember(args ...interface{}) {
	if len(args) < 2 {
		return
	}
	guildId, ok1 := args[0].(uint64)
	actorId, ok2 := args[1].(uint64)
	if !ok1 || !ok2 {
		return
	}

	onGodWeaponCeremonyRangeDo(actorId, func(s *GodWeaponCeremonySys) {
		s.SendProto3(68, 7, &pb3.S2C_68_7{
			ActiveId:   s.GetId(),
			GuildScore: 0,
		})
	})
	sendGodWeaponCeremonyGuildScore(guildId)
}

func sendGodWeaponCeremonyGuildScore(guildId uint64) {
	guild := guildmgr.GetGuildById(guildId)
	if nil == guild {
		return
	}
	curTime := time_util.NowSec()
	if pyyData := gshare.GetStaticVar().PyyDatas; nil != pyyData {
		for id, v := range pyyData.GodWeaponCeremonyRecord {
			if curTime < v.StartTime || curTime >= v.EndTime {
				continue
			}
			var totalScore uint32
			for memberId := range guild.Members {
				totalScore += v.DailyScoreMap[memberId]
			}
			guild.BroadcastProto(68, 7, &pb3.S2C_68_7{ActiveId: id, GuildScore: totalScore})
		}
	}
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYGodWeaponCeremony, createGodWeaponCeremonySys)

	net.RegisterYYSysProtoV2(68, 1, func(s iface.IPlayerYY) func(msg *base.Message) error {
		return s.(*GodWeaponCeremonySys).c2sProgressAward
	})
	net.RegisterYYSysProtoV2(68, 2, func(s iface.IPlayerYY) func(msg *base.Message) error {
		return s.(*GodWeaponCeremonySys).c2sDailyReachAward
	})
	net.RegisterYYSysProtoV2(68, 3, func(s iface.IPlayerYY) func(msg *base.Message) error {
		return s.(*GodWeaponCeremonySys).c2sDailyChargeAward
	})
	net.RegisterYYSysProtoV2(68, 5, func(s iface.IPlayerYY) func(msg *base.Message) error {
		return s.(*GodWeaponCeremonySys).c2sDailySacrifice
	})
	net.RegisterYYSysProtoV2(68, 9, func(s iface.IPlayerYY) func(msg *base.Message) error {
		return s.(*GodWeaponCeremonySys).c2sRank
	})

	event.RegSysEvent(custom_id.SeNewDayArrive, calcDailyGodWeaponCeremonyAwards)
	event.RegSysEvent(custom_id.SeBeforeSaveGlobalVar, saveGodWeaponCeremonyRank)
	event.RegSysEvent(custom_id.SeServerInit, loadGodWeaponCeremonyRank)
	event.RegSysEvent(custom_id.SeMerge, onGodWeaponCeremonyMerge)
	event.RegSysEvent(custom_id.SeGuildAddMember, onGodWeaponCeremonyGuildAddMember)
	event.RegSysEvent(custom_id.SeGuildRemoveMember, onGodWeaponCeremonyGuildRemoveMember)

	gmevent.Register("ceremony.quest", func(player iface.IPlayer, args ...string) bool {
		questId := utils.AtoUint32(args[0])
		yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYGodWeaponCeremony)
		for _, obj := range yyList {
			if s, ok := obj.(*GodWeaponCeremonySys); ok && s.IsOpen() {
				quest := s.data().DailyQuest[questId]
				if nil == quest {
					return false
				}
				s.GmFinishQuest(s.data().DailyQuest[questId])
				return true
			}
		}
		return false
	}, 1)

	gmevent.Register("ceremony.score", func(player iface.IPlayer, args ...string) bool {
		addScore := utils.AtoUint32(args[0])
		yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYGodWeaponCeremony)
		for _, obj := range yyList {
			if s, ok := obj.(*GodWeaponCeremonySys); ok && s.IsOpen() {
				s.addSacrificeScore(addScore, 0)
				return true
			}
		}
		return false
	}, 1)
}
