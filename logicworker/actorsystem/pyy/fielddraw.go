package pyy

import (
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/drawdef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/sysdef"
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
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
)

type FieldDraw struct {
	PlayerYYBase
	lottery lotterylibs.LotteryBase
}

func (s *FieldDraw) OnInit() {
	s.lottery = lotterylibs.LotteryBase{
		Player:                s.GetPlayer(),
		GetLuckTimes:          s.GetLuckTimes,
		GetLuckyValEx:         s.GetLuckyValEx,
		RawData:               s.RawData,
		GetSingleDiamondPrice: s.GetSingleDiamondPrice,
		AfterDraw:             s.AfterDraw,
	}
}

func (s *FieldDraw) OnOpen() {
	s.clearRecord(false)
	s.s2cInfo()
}

func (s *FieldDraw) OnEnd() {
	s.clearRecord(true)
	s.s2cInfo()
}

func (s *FieldDraw) Login() {
	s.s2cInfo()
}

func (s *FieldDraw) OnReconnect() {
	s.s2cInfo()
}

func (s *FieldDraw) GetLuckTimes() uint16 {
	conf, ok := jsondata.GetFieldDrawConf(s.ConfName, s.ConfIdx)
	if !ok {
		return 0
	}
	return uint16(conf.LuckTimes)
}

func (s *FieldDraw) GetLuckyValEx() *jsondata.LotteryLuckyValEx {
	conf, ok := jsondata.GetFieldDrawConf(s.ConfName, s.ConfIdx)
	if !ok {
		return nil
	}
	return conf.LuckyValEx
}

func (s *FieldDraw) RawData() *pb3.LotteryData {
	data := s.getData()
	return data.LotteryData
}

func (s *FieldDraw) GetSingleDiamondPrice() uint32 {
	conf, ok := jsondata.GetFieldDrawConf(s.ConfName, s.ConfIdx)
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

func (s *FieldDraw) AfterDraw(libId uint32, libConf *jsondata.LotteryLibConf, awardPoolConf *jsondata.LotteryLibAwardPool, oneAward jsondata.StdRewardVec) {
	data := s.getData().GetSysData()

	conf, ok := jsondata.GetFieldDrawConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	rewards := engine.FilterRewardByPlayer(s.GetPlayer(), oneAward)
	if len(rewards) <= 0 {
		return
	}

	record := &pb3.FieldDrawRecord{
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

func (s *FieldDraw) record(records *[]*pb3.FieldDrawRecord, record *pb3.FieldDrawRecord, recordLimit int) {
	*records = append(*records, record)
	if len(*records) > recordLimit {
		*records = (*records)[1:]
	}
}

func (s *FieldDraw) clearRecord(isEnd bool) {
	record := s.globalRecord()
	if record.StartTime < s.OpenTime {
		delete(gshare.GetStaticVar().PyyDatas.FieldDrawRecordList, s.Id)
	}

	if isEnd && record.StartTime == s.OpenTime {
		delete(gshare.GetStaticVar().PyyDatas.FieldDrawRecordList, s.Id)
	}

}

func (s *FieldDraw) globalRecord() *pb3.FieldDrawRecordList {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.PyyDatas {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}
	if globalVar.PyyDatas.FieldDrawRecordList == nil {
		globalVar.PyyDatas.FieldDrawRecordList = make(map[uint32]*pb3.FieldDrawRecordList)
	}
	if globalVar.PyyDatas.FieldDrawRecordList[s.Id] == nil {
		globalVar.PyyDatas.FieldDrawRecordList[s.Id] = &pb3.FieldDrawRecordList{}
	}
	if globalVar.PyyDatas.FieldDrawRecordList[s.Id].StartTime == 0 {
		globalVar.PyyDatas.FieldDrawRecordList[s.Id].StartTime = s.GetOpenTime()
	}
	return globalVar.PyyDatas.FieldDrawRecordList[s.Id]
}

func (s *FieldDraw) ResetData() {
	state := s.GetYYData()
	if nil == state.EvolutionFieldDraw {
		return
	}
	delete(state.EvolutionFieldDraw, s.Id)
}

func (s *FieldDraw) getData() *pb3.PYY_EvolutionFieldDraw {
	state := s.GetYYData()
	if nil == state.EvolutionFieldDraw {
		state.EvolutionFieldDraw = make(map[uint32]*pb3.PYY_EvolutionFieldDraw)
	}
	if state.EvolutionFieldDraw[s.Id] == nil {
		state.EvolutionFieldDraw[s.Id] = &pb3.PYY_EvolutionFieldDraw{}
	}

	sData := state.EvolutionFieldDraw[s.Id]
	if nil == sData.LotteryData {
		sData.LotteryData = &pb3.LotteryData{}
	}
	s.lottery.InitData(sData.LotteryData)

	if nil == sData.SysData {
		sData.SysData = &pb3.PYY_FieldDraw{}
	}
	return sData
}

func (s *FieldDraw) NewDay() {
	s.lottery.OnLotteryNewDay()
	s.s2cInfo()
}

func (s *FieldDraw) s2cInfo() {
	conf, ok := jsondata.GetFieldDrawConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	data := s.getData()
	s.SendProto3(75, 142, &pb3.S2C_75_142{
		ActiveId:       s.Id,
		TotalTimes:     data.SysData.TotalTimes,
		ChanceLibId:    s.lottery.GetLibId(conf.ChanceLibId),
		ChanceLibLucky: s.lottery.GetGuaranteeCount(conf.ChanceLibId),
	})
}
func (s *FieldDraw) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_75_143
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf, ok := jsondata.GetFieldDrawConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("FieldDraw conf is nil")
	}
	sData := s.getData().GetSysData()
	cos := conf.Cos[req.GetTimes()]
	if cos == nil {
		return neterror.ConfNotFoundError("FieldDraw cos conf is nil")
	}
	success, remove := s.player.ConsumeByConfWithRet(cos.Consume, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogFieldDrawConsume})
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
		engine.GiveRewards(s.GetPlayer(), bundledAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFieldDrawAward})
	}
	result := s.lottery.DoDraw(req.Times, useDiamondCount, conf.LibIds)
	engine.GiveRewards(s.GetPlayer(), result.Awards, common.EngineGiveRewardParam{
		LogId:        pb3.LogId_LogFieldDrawAward,
		NoTips:       true,
		BroadcastExt: []interface{}{s.Id},
	})
	sData.TotalTimes += req.Times

	rsp := &pb3.S2C_75_143{
		ActiveId:   s.Id,
		Times:      req.Times,
		TotalTimes: sData.TotalTimes,
	}

	for _, v := range result.LibResult {
		st := &pb3.FieldDrawSt{
			TreasureId:  v.LibId,
			AwardPoolId: v.AwardPoolConf.Id,
		}

		if oneAwards := engine.FilterRewardByPlayer(s.GetPlayer(), v.OneAwards); len(oneAwards) > 0 {
			st.ItemId = oneAwards[0].Id
			st.Count = uint32(oneAwards[0].Count)

		}
		rsp.Result = append(rsp.Result, st)
	}

	s.SendProto3(75, 143, rsp)

	s.GetPlayer().TriggerEvent(custom_id.AeActDrawTimes, &custom_id.ActDrawEvent{
		ActType: drawdef.ActFieldDraw,
		ActId:   s.Id,
		Times:   req.Times,
	})

	s.SendProto3(75, 145, &pb3.S2C_75_145{
		ActiveId:       s.Id,
		ChanceLibId:    s.lottery.GetLibId(conf.ChanceLibId),
		ChanceLibLucky: s.lottery.GetGuaranteeCount(conf.ChanceLibId),
	})

	return nil
}

const (
	FieldDrawRecordGType = 1
	FieldDrawRecordPType = 2
)

func (s *FieldDraw) c2sRecord(msg *base.Message) error {
	var req pb3.C2S_75_144
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	rsp := &pb3.S2C_75_144{ActiveId: s.Id, Type: req.GetType()}

	switch req.Type {
	case FieldDrawRecordGType:
		gData := s.globalRecord()
		rsp.Record = gData.Records
		rsp.SuperRecord = gData.SuperRecords
	case FieldDrawRecordPType:
		data := s.getData().GetSysData()
		rsp.Record = data.Records
		rsp.SuperRecord = data.SuperRecords
	}

	s.SendProto3(75, 144, rsp)
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYFieldDraw, func() iface.IPlayerYY {
		return &FieldDraw{}
	})
	net.RegisterYYSysProtoV2(75, 143, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*FieldDraw).c2sDraw
	})
	net.RegisterYYSysProtoV2(75, 144, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*FieldDraw).c2sRecord
	})
	engine.RegRewardsBroadcastHandler(tipmsgid.DomainDrawTip, func(actorId uint64, actorName string, id uint32, count int64, serverId uint32, param common.EngineGiveRewardParam) ([]interface{}, bool) {
		if len(param.BroadcastExt) < 1 {
			return nil, false
		}
		return []interface{}{actorId, actorName, param.BroadcastExt[0], id}, true
	})
	gmevent.Register("FieldDraw", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 2 {
			return false
		}
		yyId := utils.AtoUint32(args[0])
		totalTimes := utils.AtoInt(args[1])
		sys, ok := player.GetSysObj(sysdef.SiPlayerYY).(*pyymgr.YYMgr)
		if !ok {
			return false
		}
		obj := sys.GetObjById(yyId)
		if nil == obj || obj.GetClass() != yydefine.PYYFieldDraw {
			return false
		}
		s := obj.(*FieldDraw)
		conf, exist := jsondata.GetFieldDrawConf(s.ConfName, s.ConfIdx)
		if !exist {
			return false
		}
		s.lottery.GmDraw(uint32(totalTimes), uint32(totalTimes), conf.LibIds)

		return true
	}, 1)

}
