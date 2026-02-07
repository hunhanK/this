/**
 * @Author: LvYuMeng
 * @Date: 2025/6/17
 * @Desc:
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

type SummerSurfDraw struct {
	PlayerYYBase
	lottery lotterylibs.LotteryBase
}

func (s *SummerSurfDraw) OnInit() {
	s.lottery = lotterylibs.LotteryBase{
		Player:                s.GetPlayer(),
		GetLuckTimes:          s.GetLuckTimes,
		GetLuckyValEx:         s.GetLuckyValEx,
		RawData:               s.RawData,
		GetSingleDiamondPrice: s.GetSingleDiamondPrice,
		AfterDraw:             s.AfterDraw,
	}
}

func (s *SummerSurfDraw) OnOpen() {
	s.clearRecord(false)
	s.s2cInfo()
}

func (s *SummerSurfDraw) OnEnd() {
	s.clearRecord(true)
	s.s2cInfo()
}

func (s *SummerSurfDraw) Login() {
	s.s2cInfo()
}

func (s *SummerSurfDraw) OnReconnect() {
	s.s2cInfo()
}

func (s *SummerSurfDraw) GetLuckTimes() uint16 {
	conf, ok := jsondata.GetSummerSurfConf(s.ConfName, s.ConfIdx)
	if !ok {
		return 0
	}
	return uint16(conf.LuckTimes)
}

func (s *SummerSurfDraw) GetLuckyValEx() *jsondata.LotteryLuckyValEx {
	conf, ok := jsondata.GetSummerSurfConf(s.ConfName, s.ConfIdx)
	if !ok {
		return nil
	}
	return conf.LuckyValEx
}

func (s *SummerSurfDraw) RawData() *pb3.LotteryData {
	data := s.getData()
	return data.LotteryData
}

func (s *SummerSurfDraw) GetSingleDiamondPrice() uint32 {
	conf, ok := jsondata.GetSummerSurfConf(s.ConfName, s.ConfIdx)
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

func (s *SummerSurfDraw) AfterDraw(libId uint32, libConf *jsondata.LotteryLibConf, awardPoolConf *jsondata.LotteryLibAwardPool, oneAward jsondata.StdRewardVec) {
	data := s.getData().GetSysData()

	conf, ok := jsondata.GetSummerSurfConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	rewards := engine.FilterRewardByPlayer(s.GetPlayer(), oneAward)
	if len(rewards) <= 0 {
		return
	}

	record := &pb3.SummerSurfRecord{
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

func (s *SummerSurfDraw) record(records *[]*pb3.SummerSurfRecord, record *pb3.SummerSurfRecord, recordLimit int) {
	*records = append(*records, record)
	if len(*records) > recordLimit {
		*records = (*records)[1:]
	}
}

func (s *SummerSurfDraw) clearRecord(isEnd bool) {
	record := s.globalRecord()
	if record.StartTime < s.OpenTime {
		delete(gshare.GetStaticVar().PyyDatas.SummerSurfRecordList, s.Id)
	}

	if isEnd && record.StartTime == s.OpenTime {
		delete(gshare.GetStaticVar().PyyDatas.SummerSurfRecordList, s.Id)
	}
}

func (s *SummerSurfDraw) globalRecord() *pb3.SummerSurfRecordList {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.PyyDatas {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}
	if globalVar.PyyDatas.SummerSurfRecordList == nil {
		globalVar.PyyDatas.SummerSurfRecordList = make(map[uint32]*pb3.SummerSurfRecordList)
	}
	if globalVar.PyyDatas.SummerSurfRecordList[s.Id] == nil {
		globalVar.PyyDatas.SummerSurfRecordList[s.Id] = &pb3.SummerSurfRecordList{}
	}
	if globalVar.PyyDatas.SummerSurfRecordList[s.Id].StartTime == 0 {
		globalVar.PyyDatas.SummerSurfRecordList[s.Id].StartTime = s.GetOpenTime()
	}
	return globalVar.PyyDatas.SummerSurfRecordList[s.Id]
}

func (s *SummerSurfDraw) ResetData() {
	state := s.GetYYData()
	if nil == state.EvolutionSummerSurf {
		return
	}
	delete(state.EvolutionSummerSurf, s.Id)
}

func (s *SummerSurfDraw) GetHitItemNums() uint32 {
	data := s.getData()
	return uint32(len(data.SysData.HitItems))
}

func (s *SummerSurfDraw) getData() *pb3.PYY_EvolutionSummerSurf {
	state := s.GetYYData()
	if nil == state.EvolutionSummerSurf {
		state.EvolutionSummerSurf = make(map[uint32]*pb3.PYY_EvolutionSummerSurf)
	}
	if state.EvolutionSummerSurf[s.Id] == nil {
		state.EvolutionSummerSurf[s.Id] = &pb3.PYY_EvolutionSummerSurf{}
	}

	sData := state.EvolutionSummerSurf[s.Id]
	if nil == sData.LotteryData {
		sData.LotteryData = &pb3.LotteryData{}
	}
	s.lottery.InitData(sData.LotteryData)

	if nil == sData.SysData {
		sData.SysData = &pb3.PYY_SummerSurf{}
	}
	if nil == sData.SysData.HitItems {
		sData.SysData.HitItems = map[uint32]uint32{}
	}
	if nil == sData.SysData.LibHit {
		sData.SysData.LibHit = map[uint32]*pb3.SummerSurfLibHit{}
	}
	return sData
}

func (s *SummerSurfDraw) NewDay() {
	s.lottery.OnLotteryNewDay()
	data := s.getData()
	data.SysData.DailyTimes = 0
	s.s2cInfo()
}

func (s *SummerSurfDraw) s2cInfo() {
	conf, ok := jsondata.GetSummerSurfConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	data := s.getData()
	s.SendProto3(75, 90, &pb3.S2C_75_90{
		ActiveId:       s.Id,
		TotalTimes:     data.SysData.TotalTimes,
		DailyTimes:     data.SysData.DailyTimes,
		ChanceLibId:    s.lottery.GetLibId(conf.ChanceLibId),
		ChanceLibLucky: s.lottery.GetGuaranteeCount(conf.ChanceLibId),
		HitItemNum:     uint32(len(data.SysData.HitItems)),
		LibHit:         data.SysData.LibHit,
	})
}

func (s *SummerSurfDraw) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_75_91
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf, ok := jsondata.GetSummerSurfConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("SummerSurfDraw conf is nil")
	}
	sData := s.getData().GetSysData()

	if sData.DailyTimes+req.Times > conf.DailyDrawTimes {
		return neterror.ConfNotFoundError("daily times limit")
	}

	cos := conf.Cos[req.GetTimes()]
	if cos == nil {
		return neterror.ConfNotFoundError("SummerSurfDraw cos conf is nil")
	}

	success, remove := s.player.ConsumeByConfWithRet(cos.Consume, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogSummerSurfDrawConsume})
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
		engine.GiveRewards(s.GetPlayer(), bundledAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSummerSurfDrawAward})
	}

	result := s.lottery.DoDraw(req.Times, useDiamondCount, conf.LibIds)
	engine.GiveRewards(s.GetPlayer(), result.Awards, common.EngineGiveRewardParam{
		LogId:        pb3.LogId_LogSummerSurfDrawAward,
		NoTips:       true,
		BroadcastExt: []interface{}{s.Id},
	})

	sData.TotalTimes += req.Times
	sData.DailyTimes += req.Times

	rsp := &pb3.S2C_75_91{
		ActiveId:   s.Id,
		Times:      req.Times,
		TotalTimes: sData.TotalTimes,
		DailyTimes: sData.DailyTimes,
	}

	/////
	for _, v := range result.LibResult {
		st := &pb3.SummerSurfSt{
			TreasureId:  v.LibId,
			AwardPoolId: v.AwardPoolConf.Id,
		}

		if oneAwards := engine.FilterRewardByPlayer(s.GetPlayer(), v.OneAwards); len(oneAwards) > 0 {
			st.ItemId = oneAwards[0].Id
			st.Count = uint32(oneAwards[0].Count)

		}
		rsp.Result = append(rsp.Result, st)

		if pie.Uint32s(conf.HitItems).Contains(st.ItemId) {
			sData.HitItems[st.ItemId]++
		}

		if _, exist := sData.LibHit[v.LibId]; !exist {
			sData.LibHit[v.LibId] = &pb3.SummerSurfLibHit{}
		}
		if nil == sData.LibHit[v.LibId].AwardPoolId {
			sData.LibHit[v.LibId].AwardPoolId = make(map[uint32]uint32)
		}
		sData.LibHit[v.LibId].AwardPoolId[v.AwardPoolConf.Id]++
	}

	rsp.HitItemNum = uint32(len(sData.HitItems))
	rsp.LibHit = sData.LibHit

	s.SendProto3(75, 91, rsp)

	s.GetPlayer().TriggerEvent(custom_id.AeActDrawTimes, &custom_id.ActDrawEvent{
		ActType: drawdef.ActSummerSurf,
		ActId:   s.Id,
		Times:   req.Times,
	})

	s.SendProto3(75, 93, &pb3.S2C_75_93{
		ActiveId:       s.Id,
		ChanceLibId:    s.lottery.GetLibId(conf.ChanceLibId),
		ChanceLibLucky: s.lottery.GetGuaranteeCount(conf.ChanceLibId),
	})

	return nil
}

const (
	SummerSurfDrawRecordGType = 1
	SummerSurfDrawRecordPType = 2
)

func (s *SummerSurfDraw) c2sRecord(msg *base.Message) error {
	var req pb3.C2S_75_92
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	rsp := &pb3.S2C_75_92{ActiveId: s.Id, Type: req.GetType()}

	switch req.Type {
	case SummerSurfDrawRecordGType:
		gData := s.globalRecord()
		rsp.Record = gData.Records
		rsp.SuperRecord = gData.SuperRecords
	case SummerSurfDrawRecordPType:
		data := s.getData().GetSysData()
		rsp.Record = data.Records
		rsp.SuperRecord = data.SuperRecords
	}

	s.SendProto3(75, 92, rsp)
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYSummerSurf, func() iface.IPlayerYY {
		return &SummerSurfDraw{}
	})

	net.RegisterYYSysProtoV2(75, 91, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SummerSurfDraw).c2sDraw
	})
	net.RegisterYYSysProtoV2(75, 92, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*SummerSurfDraw).c2sRecord
	})
	engine.RegRewardsBroadcastHandler(tipmsgid.SummerDrawTips, func(actorId uint64, actorName string, id uint32, count int64, serverId uint32, param common.EngineGiveRewardParam) ([]interface{}, bool) {
		if len(param.BroadcastExt) < 1 {
			return nil, false
		}
		return []interface{}{actorId, actorName, param.BroadcastExt[0], id}, true //玩家id，玩家名，道具
	})

}
