/**
 * @Author: LvYuMeng
 * @Date: 2025/4/15
 * @Desc: 五一祈福
**/

package pyy

import (
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/drawdef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/lotterylibs"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
)

type MayDayBlessSys struct {
	PlayerYYBase
	lottery lotterylibs.LotteryBase
}

func (s *MayDayBlessSys) Login() {
	s.s2cInfo()
}

func (s *MayDayBlessSys) OnReconnect() {
	s.s2cInfo()
}

func (s *MayDayBlessSys) OnOpen() {
	s.clearRecord()
	s.s2cInfo()
}

func (s *MayDayBlessSys) s2cInfo() {
	conf, ok := jsondata.GetYYMayDayBlessConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	data := s.getData()
	s.SendProto3(75, 40, &pb3.S2C_75_40{
		ActiveId:       s.GetId(),
		Info:           data.GetMayDayBless(),
		GuaranteeLibId: s.lottery.GetLibId(conf.GuaranteeLibId),
		GuaranteeLucky: s.lottery.GetGuaranteeCount(conf.GuaranteeLibId),
		ChanceLibId:    s.lottery.GetLibId(conf.ChanceLibId),
		ChanceLibLucky: s.lottery.GetGuaranteeCount(conf.ChanceLibId),
	})
}

func (s *MayDayBlessSys) clearRecord() {
	if record := s.globalRecord(); record.GetStartTime() < s.GetOpenTime() {
		delete(gshare.GetStaticVar().PyyDatas.MayDayBlessRecords, s.GetId())
	}
}

func (s *MayDayBlessSys) ResetData() {
	state := s.GetYYData()
	if nil == state.EvolutionMayDayBless {
		return
	}
	delete(state.EvolutionMayDayBless, s.Id)
}

func (s *MayDayBlessSys) OnInit() {
	s.lottery = lotterylibs.LotteryBase{
		Player:                s.GetPlayer(),
		GetLuckTimes:          s.GetLuckTimes,
		GetLuckyValEx:         s.GetLuckyValEx,
		RawData:               s.RawData,
		GetSingleDiamondPrice: s.GetSingleDiamondPrice,
		AfterDraw:             s.AfterDraw,
	}
}

func (s *MayDayBlessSys) GetLuckTimes() uint16 {
	conf, ok := jsondata.GetYYMayDayBlessConf(s.ConfName, s.ConfIdx)
	if !ok {
		return 0
	}
	return uint16(conf.LuckTimes)
}

func (s *MayDayBlessSys) GetLuckyValEx() *jsondata.LotteryLuckyValEx {
	conf, ok := jsondata.GetYYMayDayBlessConf(s.ConfName, s.ConfIdx)
	if !ok {
		return nil
	}
	return conf.LuckyValEx
}

func (s *MayDayBlessSys) RawData() *pb3.LotteryData {
	data := s.getData()
	return data.LotteryData
}

func (s *MayDayBlessSys) GetSingleDiamondPrice() uint32 {
	conf, ok := jsondata.GetYYMayDayBlessConf(s.ConfName, s.ConfIdx)
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

const (
	MayDayBlessRecordGType = 1
	MayDayBlessRecordPType = 2
)

func (s *MayDayBlessSys) AfterDraw(libId uint32, libConf *jsondata.LotteryLibConf, awardPoolConf *jsondata.LotteryLibAwardPool, oneAward jsondata.StdRewardVec) {
	data := s.getData().GetMayDayBless()
	if _, ok := data.LibHit[libId]; !ok {
		data.LibHit[libId] = &pb3.MayDayBlessLibHit{}
	}
	if nil == data.LibHit[libId].AwardPoolId {
		data.LibHit[libId].AwardPoolId = make(map[uint32]uint32)
	}
	data.LibHit[libId].AwardPoolId[awardPoolConf.Id]++

	conf, ok := jsondata.GetYYMayDayBlessConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	rewards := engine.FilterRewardByPlayer(s.GetPlayer(), awardPoolConf.Awards)
	if len(rewards) <= 0 {
		return
	}

	record := &pb3.MayDayBlessRecord{
		TreasureId:  libId,
		AwardPoolId: awardPoolConf.Id,
		ItemId:      rewards[0].Id,
		Count:       uint32(rewards[0].Count),
		TimeStamp:   time_util.NowSec(),
		ActorName:   s.GetPlayer().GetName(),
	}

	gData := s.globalRecord()

	if pie.Uint32s(conf.RecordSuperLibs).Contains(libId) {
		s.record(&gData.SuperRecords, record, int(conf.RecordNum))
		s.record(&data.SuperRecords, record, int(conf.RecordNum))
	} else {
		s.record(&gData.Records, record, int(conf.RecordNum))
		s.record(&data.Records, record, int(conf.RecordNum))
	}
}

func (s *MayDayBlessSys) record(records *[]*pb3.MayDayBlessRecord, record *pb3.MayDayBlessRecord, recordLimit int) {
	*records = append(*records, record)
	if len(*records) > recordLimit {
		*records = (*records)[1:]
	}
}

func (s *MayDayBlessSys) globalRecord() *pb3.MayDayBlessRecordList {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.PyyDatas {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}
	if globalVar.PyyDatas.MayDayBlessRecords == nil {
		globalVar.PyyDatas.MayDayBlessRecords = make(map[uint32]*pb3.MayDayBlessRecordList)
	}
	if globalVar.PyyDatas.MayDayBlessRecords[s.Id] == nil {
		globalVar.PyyDatas.MayDayBlessRecords[s.Id] = &pb3.MayDayBlessRecordList{}
	}
	if globalVar.PyyDatas.MayDayBlessRecords[s.Id].StartTime == 0 {
		globalVar.PyyDatas.MayDayBlessRecords[s.Id].StartTime = s.GetOpenTime()
	}
	return globalVar.PyyDatas.MayDayBlessRecords[s.Id]
}

func (s *MayDayBlessSys) getData() *pb3.PYY_EvolutionMayDayBless {
	state := s.GetYYData()
	if nil == state.EvolutionMayDayBless {
		state.EvolutionMayDayBless = make(map[uint32]*pb3.PYY_EvolutionMayDayBless)
	}
	if state.EvolutionMayDayBless[s.Id] == nil {
		state.EvolutionMayDayBless[s.Id] = &pb3.PYY_EvolutionMayDayBless{}
	}

	sData := state.EvolutionMayDayBless[s.Id]
	if nil == sData.LotteryData {
		sData.LotteryData = &pb3.LotteryData{}
	}
	s.lottery.InitData(sData.LotteryData)

	if nil == sData.MayDayBless {
		sData.MayDayBless = &pb3.PYY_MayDayBless{}
	}
	if nil == sData.MayDayBless.LibHit {
		sData.MayDayBless.LibHit = make(map[uint32]*pb3.MayDayBlessLibHit)
	}
	return sData
}

func (s *MayDayBlessSys) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_75_41
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf, ok := jsondata.GetYYMayDayBlessConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("MayDayBlessSys conf is nil")
	}

	cos := conf.Cos[req.GetTimes()]
	if cos == nil {
		return neterror.ConfNotFoundError("MayDayBlessSys cos conf is nil")
	}

	success, remove := s.player.ConsumeByConfWithRet(cos.Consume, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogMayDayBlessDrawAward})
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

	bundledAwards := jsondata.StdRewardMulti(conf.DrawScore, int64(req.Times))
	if len(bundledAwards) > 0 {
		engine.GiveRewards(s.GetPlayer(), bundledAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogMayDayBlessDrawAward})
	}

	result := s.lottery.DoDraw(req.Times, useDiamondCount, conf.LibIds)
	engine.GiveRewards(s.GetPlayer(), result.Awards, common.EngineGiveRewardParam{
		LogId:        pb3.LogId_LogMayDayBlessDrawAward,
		NoTips:       true,
		BroadcastExt: []interface{}{s.Id},
	})

	sData := s.getData().GetMayDayBless()
	sData.TotalTimes += req.GetTimes()

	rsp := &pb3.S2C_75_41{
		ActiveId:       s.GetId(),
		Times:          req.GetTimes(),
		TotalTimes:     sData.GetTotalTimes(),
		Lucky:          s.lottery.GetGuaranteeCount(conf.GuaranteeLibId),
		LibHit:         sData.GetLibHit(),
		GuaranteeLibId: s.lottery.GetLibId(conf.GuaranteeLibId),
		GuaranteeLucky: s.lottery.GetGuaranteeCount(conf.GuaranteeLibId),
		ChanceLibId:    s.lottery.GetLibId(conf.ChanceLibId),
		ChanceLibLucky: s.lottery.GetGuaranteeCount(conf.ChanceLibId),
	}
	for _, v := range result.LibResult {
		rewards := engine.FilterRewardByPlayer(s.GetPlayer(), v.AwardPoolConf.Awards)
		rsp.Result = append(rsp.Result, &pb3.MayDayBlessSt{
			TreasureId:  v.LibId,
			AwardPoolId: v.AwardPoolConf.Id,
			ItemId:      rewards[0].Id,
			Count:       uint32(rewards[0].Count),
		})
	}

	s.SendProto3(75, 41, rsp)

	s.GetPlayer().TriggerEvent(custom_id.AeActDrawTimes, &custom_id.ActDrawEvent{
		ActType: drawdef.ActMayDayBless,
		ActId:   s.Id,
		Times:   req.Times,
	})

	return nil
}

func (s *MayDayBlessSys) c2sRecord(msg *base.Message) error {
	var req pb3.C2S_75_42
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	rsp := &pb3.S2C_75_42{ActiveId: s.Id, Type: req.GetType()}

	switch req.Type {
	case MayDayBlessRecordGType:
		gData := s.globalRecord()
		rsp.Record = gData.Records
		rsp.SuperRecord = gData.SuperRecords
	case MayDayBlessRecordPType:
		data := s.getData().GetMayDayBless()
		rsp.Record = data.Records
		rsp.SuperRecord = data.SuperRecords
	}

	s.SendProto3(75, 42, rsp)
	return nil
}

func (s *MayDayBlessSys) NewDay() {
	s.lottery.OnLotteryNewDay()
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYMayDayBless, func() iface.IPlayerYY {
		return &MayDayBlessSys{}
	})

	net.RegisterYYSysProtoV2(75, 41, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*MayDayBlessSys).c2sDraw
	})
	net.RegisterYYSysProtoV2(75, 42, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*MayDayBlessSys).c2sRecord
	})

	engine.RegRewardsBroadcastHandler(tipmsgid.MayDayBlessTips, func(actorId uint64, actorName string, id uint32, count int64, serverId uint32, param common.EngineGiveRewardParam) ([]interface{}, bool) {
		if len(param.BroadcastExt) < 1 {
			return nil, false
		}
		return []interface{}{actorId, actorName, param.BroadcastExt[0], id}, true //玩家id，玩家名，道具
	})

}
