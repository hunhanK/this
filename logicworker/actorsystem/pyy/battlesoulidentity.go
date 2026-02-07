/**
 * @Author: LvYuMeng
 * @Date: 2024/10/28
 * @Desc: 武魂鉴赏
**/

package pyy

import (
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/lotterylibs"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/net"
)

type BattleSoulIdentitySys struct {
	PlayerYYBase
	lottery lotterylibs.LotteryBase
}

func (s *BattleSoulIdentitySys) OnInit() {
	s.lottery = lotterylibs.LotteryBase{
		Player:                s.GetPlayer(),
		GetLuckTimes:          s.GetLuckTimes,
		GetLuckyValEx:         s.GetLuckyValEx,
		RawData:               s.RawData,
		GetSingleDiamondPrice: s.GetSingleDiamondPrice,
		AfterDraw:             s.AfterDraw,
	}
}

func (s *BattleSoulIdentitySys) Login() {
	s.s2cInfo()
}

func (s *BattleSoulIdentitySys) OnReconnect() {
	s.s2cInfo()
}

func (s *BattleSoulIdentitySys) OnOpen() {
	s.clearRecord()
	s.s2cInfo()
}

func (s *BattleSoulIdentitySys) OnEnd() {
	conf, ok := jsondata.GetYYBattleSoulIdentityConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	var rewardsVec []jsondata.StdRewardVec
	sData := s.data().GetBattleSoulIdentify()
	for _, v := range conf.SumAwards {
		if pie.Uint32s(sData.RevSumAwards).Contains(v.Id) {
			continue
		}
		if sData.TotalTimes < v.Times {
			continue
		}
		sData.RevSumAwards = append(sData.RevSumAwards, v.Id)
		rewardsVec = append(rewardsVec, v.Rewards)
	}
	rewards := jsondata.MergeStdReward(rewardsVec...)
	if len(rewards) > 0 {
		mailmgr.SendMailToActor(s.GetPlayer().GetId(), &mailargs.SendMailSt{
			ConfId:  conf.MailId,
			Rewards: rewards,
		})
	}
}

func (s *BattleSoulIdentitySys) GetGuaranteeLibId() uint32 {
	conf, ok := jsondata.GetYYBattleSoulIdentityConf(s.ConfName, s.ConfIdx)
	if !ok {
		return 0
	}
	return conf.GuaranteeLibId
}

func (s *BattleSoulIdentitySys) s2cInfo() {
	data := s.data()
	s.SendProto3(69, 215, &pb3.S2C_69_215{
		ActiveId: s.GetId(),
		Info:     data.GetBattleSoulIdentify(),
		Lucky:    s.lottery.GetGuaranteeCount(s.GetGuaranteeLibId()),
		IsFull:   s.IsDrawFull(),
	})
}

func (s *BattleSoulIdentitySys) clearRecord() {
	if record := s.globalRecord(); record.GetStartTime() < s.GetOpenTime() {
		delete(gshare.GetStaticVar().PyyDatas.BattleSoulIdentifyRecord, s.GetId())
	}
}

func (s *BattleSoulIdentitySys) ResetData() {
	state := s.GetYYData()
	if nil == state.EvolutionBattleSoulIdentify {
		return
	}
	delete(state.EvolutionBattleSoulIdentify, s.Id)
}

func (s *BattleSoulIdentitySys) data() *pb3.PYY_EvolutionBattleSoulIdentify {
	state := s.GetYYData()
	if nil == state.EvolutionBattleSoulIdentify {
		state.EvolutionBattleSoulIdentify = make(map[uint32]*pb3.PYY_EvolutionBattleSoulIdentify)
	}
	if state.EvolutionBattleSoulIdentify[s.Id] == nil {
		state.EvolutionBattleSoulIdentify[s.Id] = &pb3.PYY_EvolutionBattleSoulIdentify{}
	}
	sData := state.EvolutionBattleSoulIdentify[s.Id]
	if nil == sData.LotteryData {
		sData.LotteryData = &pb3.LotteryData{}
	}
	s.lottery.InitData(sData.LotteryData)
	if nil == sData.BattleSoulIdentify {
		sData.BattleSoulIdentify = &pb3.PYY_BattleSoulIdentify{}
	}
	return sData
}

func (s *BattleSoulIdentitySys) globalRecord() *pb3.BattleSoulIdentifyRecordsList {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.PyyDatas {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}
	if globalVar.PyyDatas.BattleSoulIdentifyRecord == nil {
		globalVar.PyyDatas.BattleSoulIdentifyRecord = make(map[uint32]*pb3.BattleSoulIdentifyRecordsList)
	}
	if globalVar.PyyDatas.BattleSoulIdentifyRecord[s.Id] == nil {
		globalVar.PyyDatas.BattleSoulIdentifyRecord[s.Id] = &pb3.BattleSoulIdentifyRecordsList{}
	}
	if globalVar.PyyDatas.BattleSoulIdentifyRecord[s.Id].StartTime == 0 {
		globalVar.PyyDatas.BattleSoulIdentifyRecord[s.Id].StartTime = s.GetOpenTime()
	}
	return globalVar.PyyDatas.BattleSoulIdentifyRecord[s.Id]
}

func (s *BattleSoulIdentitySys) GetLuckTimes() uint16 {
	conf, ok := jsondata.GetYYBattleSoulIdentityConf(s.ConfName, s.ConfIdx)
	if !ok {
		return 0
	}
	return conf.LuckTimes
}

func (s *BattleSoulIdentitySys) GetLuckyValEx() *jsondata.LotteryLuckyValEx {
	conf, ok := jsondata.GetYYBattleSoulIdentityConf(s.ConfName, s.ConfIdx)
	if !ok {
		return nil
	}
	return conf.LuckyValEx
}

func (s *BattleSoulIdentitySys) RawData() *pb3.LotteryData {
	data := s.data()
	return data.LotteryData
}

func (s *BattleSoulIdentitySys) GetSingleDiamondPrice() uint32 {
	conf, ok := jsondata.GetYYBattleSoulIdentityConf(s.ConfName, s.ConfIdx)
	if !ok {
		return 0
	}
	consumeConf := conf.GetDrawConsume(1)
	singlePrice := jsondata.GetAutoBuyItemPrice(consumeConf[0].Id, moneydef.BindDiamonds)
	if singlePrice <= 0 {
		singlePrice = jsondata.GetAutoBuyItemPrice(consumeConf[0].Id, moneydef.Diamonds)
	}
	return uint32(singlePrice)
}

func (s *BattleSoulIdentitySys) NewDay() {
	s.lottery.OnLotteryNewDay()
}

const (
	BattleSoulIdentityRecordGType = 1
	BattleSoulIdentityRecordPType = 2
)

func (s *BattleSoulIdentitySys) AfterDraw(libId uint32, libConf *jsondata.LotteryLibConf, awardPoolConf *jsondata.LotteryLibAwardPool, oneAward jsondata.StdRewardVec) {
	sData := s.data().GetBattleSoulIdentify()

	conf, ok := jsondata.GetYYBattleSoulIdentityConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	rewards := engine.FilterRewardByPlayer(s.GetPlayer(), awardPoolConf.Awards)
	if len(rewards) <= 0 {
		return
	}

	record := &pb3.BattleSoulIdentifyRecord{
		TreasureId:  libId,
		AwardPoolId: awardPoolConf.Id,
		ItemId:      rewards[0].Id,
		Count:       uint32(rewards[0].Count),
		TimeStamp:   time_util.NowSec(),
		ActorName:   s.GetPlayer().GetName(),
	}

	gData := s.globalRecord()

	if pie.Uint32s(conf.SpRecordIDs).Contains(libId) {
		s.record(&gData.SuperRecords, record, int(conf.GlobalRecordCount))
		s.record(&sData.SuperRecords, record, int(conf.PersonalRecordCount))
	} else {
		s.record(&gData.Records, record, int(conf.GlobalRecordCount))
		s.record(&sData.Records, record, int(conf.PersonalRecordCount))
	}
}

func (s *BattleSoulIdentitySys) record(records *[]*pb3.BattleSoulIdentifyRecord, record *pb3.BattleSoulIdentifyRecord, recordLimit int) {
	*records = append(*records, record)
	if len(*records) > recordLimit {
		*records = (*records)[1:]
	}
}

func (s *BattleSoulIdentitySys) c2sSumAward(msg *base.Message) error {
	var req pb3.C2S_69_218
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf, ok := jsondata.GetYYBattleSoulIdentityConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("BattleSoulIdentitySys conf is nil")
	}

	sData := s.data().GetBattleSoulIdentify()

	var rewardVec []jsondata.StdRewardVec
	var ids []uint32

	for _, v := range conf.SumAwards {
		if req.GetId() > 0 && v.Id != req.GetId() {
			continue
		}
		if pie.Uint32s(sData.RevSumAwards).Contains(v.Id) {
			continue
		}
		if sData.TotalTimes < v.Times {
			continue
		}
		ids = append(ids, v.Id)
		rewardVec = append(rewardVec, v.Rewards)
	}

	if len(ids) == 0 {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	for _, id := range ids {
		sData.RevSumAwards = append(sData.RevSumAwards, id)
	}

	engine.GiveRewards(s.GetPlayer(), jsondata.MergeStdReward(rewardVec...), common.EngineGiveRewardParam{LogId: pb3.LogId_LogBattleSoulIdentifySumAwards})

	s.SendProto3(69, 218, &pb3.S2C_69_218{
		ActiveId: s.GetId(),
		Id:       ids,
	})
	return nil
}

func (s *BattleSoulIdentitySys) c2sRecord(msg *base.Message) error {
	var req pb3.C2S_69_217
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	rsp := &pb3.S2C_69_217{ActiveId: s.Id, Type: req.GetType()}

	switch req.Type {
	case BattleSoulIdentityRecordGType:
		gData := s.globalRecord()
		rsp.Record = gData.Records
		rsp.SuperRecord = gData.SuperRecords
	case BattleSoulIdentityRecordPType:
		data := s.data().GetBattleSoulIdentify()
		rsp.Record = data.Records
		rsp.SuperRecord = data.SuperRecords
	}

	s.SendProto3(69, 217, rsp)
	return nil
}

func (s *BattleSoulIdentitySys) IsDrawFull() bool {
	libId := s.GetGuaranteeLibId()
	if libId == 0 {
		return false
	}

	if s.lottery.CanLibUse(libId, false) {
		return false
	}

	return true
}

func (s *BattleSoulIdentitySys) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_69_219
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf, ok := jsondata.GetYYBattleSoulIdentityConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("BattleSoulIdentitySys conf is nil")
	}

	sData := s.data().GetBattleSoulIdentify()

	consumes := conf.GetDrawConsume(req.GetTimes())
	if nil == consumes {
		return neterror.ConfNotFoundError("BattleSoulIdentitySys consumes conf is ni;")
	}
	success, remove := s.player.ConsumeByConfWithRet(consumes, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogBattleSoulIdentifyConsume})
	if !success {
		s.player.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	diamond := uint32(remove.MoneyMap[moneydef.Diamonds] + remove.MoneyMap[moneydef.BindDiamonds])
	singlePrice := s.GetSingleDiamondPrice()
	var useDiamondCount uint32
	if singlePrice > 0 {
		useDiamondCount = diamond / singlePrice
	}

	bundledAwards := jsondata.StdRewardMulti(conf.GiveRewards, int64(req.Times))
	if len(bundledAwards) > 0 {
		engine.GiveRewards(s.GetPlayer(), bundledAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogBattleSoulIdentifyDrawAwards})
	}

	result := s.lottery.DoDraw(req.Times, useDiamondCount, conf.LibsId)
	engine.GiveRewards(s.GetPlayer(), result.Awards, common.EngineGiveRewardParam{
		LogId:        pb3.LogId_LogBattleSoulIdentifyDrawAwards,
		NoTips:       true,
		BroadcastExt: []interface{}{s.Id},
	})

	sData.TotalTimes += req.GetTimes()

	rsp := &pb3.S2C_69_219{
		ActiveId:   s.GetId(),
		Times:      req.GetTimes(),
		TotalTimes: sData.GetTotalTimes(),
		Lucky:      s.lottery.GetGuaranteeCount(s.GetGuaranteeLibId()),
		IsFull:     s.IsDrawFull(),
	}

	for _, v := range result.LibResult {
		rsp.Result = append(rsp.Result, &pb3.BattleSoulIdentifySt{
			TreasureId:  v.LibId,
			AwardPoolId: v.AwardPoolConf.Id,
		})
	}

	s.SendProto3(69, 219, rsp)
	return nil

}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYBattleSoulIdentity, func() iface.IPlayerYY {
		return &BattleSoulIdentitySys{}
	})

	net.RegisterYYSysProtoV2(69, 217, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*BattleSoulIdentitySys).c2sRecord
	})
	net.RegisterYYSysProtoV2(69, 218, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*BattleSoulIdentitySys).c2sSumAward
	})
	net.RegisterYYSysProtoV2(69, 219, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*BattleSoulIdentitySys).c2sDraw
	})

	engine.RegRewardsBroadcastHandler(tipmsgid.BattleSoulIdentityDraw, func(actorId uint64, actorName string, id uint32, count int64, serverId uint32, param common.EngineGiveRewardParam) ([]interface{}, bool) {
		if len(param.BroadcastExt) < 1 {
			return nil, false
		}
		return []interface{}{actorId, actorName, param.BroadcastExt[0], id}, true //玩家id，玩家名，活动id，道具
	})

}
