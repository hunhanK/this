/**
 * @Author: LvYuMeng
 * @Date: 2025/8/4
 * @Desc: 机甲召唤
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

type MechaSummon struct {
	PlayerYYBase
	lottery *lotterylibs.LotteryBase
}

func (s *MechaSummon) OnInit() {
	s.lottery = &lotterylibs.LotteryBase{
		Player:                s.GetPlayer(),
		GetLuckTimes:          s.GetLuckTimes,
		GetLuckyValEx:         s.GetLuckyValEx,
		RawData:               s.RawData,
		GetSingleDiamondPrice: s.GetSingleDiamondPrice,
		AfterDraw:             s.AfterDraw,
	}
}

func (s *MechaSummon) ResetData() {
	state := s.GetYYData()
	if nil == state.MechaSummon {
		return
	}
	delete(state.MechaSummon, s.Id)
}

func (s *MechaSummon) OnOpen() {
	s.clearRecord(false)
	sysData := s.getSysData()
	sysData.TabId = 1
	s.s2cInfo()
}

func (s *MechaSummon) OnEnd() {
	s.clearRecord(true)
}

func (s *MechaSummon) clearRecord(isEnd bool) {
	record := s.globalRecord()
	if record.StartTime < s.OpenTime {
		delete(gshare.GetStaticVar().PyyDatas.MechaSummonRecordsList, s.Id)
	}

	if isEnd && record.StartTime == s.OpenTime {
		delete(gshare.GetStaticVar().PyyDatas.MechaSummonRecordsList, s.Id)
	}
}

func (s *MechaSummon) Login() {
	s.s2cInfo()
}

func (s *MechaSummon) OnReconnect() {
	s.s2cInfo()
}

func (s *MechaSummon) NewDay() {
	s.lottery.OnLotteryNewDay()
	s.s2cInfo()
}

func (s *MechaSummon) s2cInfo() {
	data := s.getSysData()

	rsp := &pb3.S2C_75_127{
		ActiveId:   s.Id,
		TabId:      data.TabId,
		TotalTimes: data.TotalTimes,
	}

	s.SendProto3(75, 127, rsp)
}

func (s *MechaSummon) getEvolutionData() *pb3.PYY_EvolutionMechaSummon {
	state := s.GetYYData()
	if nil == state.MechaSummon {
		state.MechaSummon = make(map[uint32]*pb3.PYY_EvolutionMechaSummon)
	}
	if state.MechaSummon[s.Id] == nil {
		state.MechaSummon[s.Id] = &pb3.PYY_EvolutionMechaSummon{}
	}
	return state.MechaSummon[s.Id]
}

func (s *MechaSummon) getSysData() *pb3.PYY_MechaSummon {
	data := s.getEvolutionData()
	if nil == data.SysData {
		data.SysData = &pb3.PYY_MechaSummon{}
	}
	return data.SysData
}

func (s *MechaSummon) RawData() *pb3.LotteryData {
	data := s.getEvolutionData()
	if nil == data.LotteryData {
		data.LotteryData = &pb3.LotteryData{}
	}

	s.lottery.InitData(data.LotteryData)

	return data.LotteryData
}

func (s *MechaSummon) GetSingleDiamondPrice() uint32 {
	conf := s.GetConf()
	if nil == conf {
		return 0
	}

	consumeConf := conf.GetDrawConsume(1)
	singlePrice := jsondata.GetAutoBuyItemPrice(consumeConf[0].Id, moneydef.BindDiamonds)
	if singlePrice <= 0 {
		singlePrice = jsondata.GetAutoBuyItemPrice(consumeConf[0].Id, moneydef.Diamonds)
	}

	return uint32(singlePrice)
}

func (s *MechaSummon) GetLuckTimes() uint16 {
	conf := s.GetConf()
	if nil == conf {
		return 0
	}
	return uint16(conf.LuckTimes)
}

func (s *MechaSummon) GetLuckyValEx() *jsondata.LotteryLuckyValEx {
	conf := s.GetConf()
	if nil == conf {
		return nil
	}
	return conf.LuckyValEx
}

func (s *MechaSummon) GetConf() *jsondata.MechaSummonConfig {
	return jsondata.GetMechaSummonConf(s.ConfName, s.ConfIdx)
}

func (s *MechaSummon) globalRecord() *pb3.MechaSummonRecordsList {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.PyyDatas {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}
	if globalVar.PyyDatas.MechaSummonRecordsList == nil {
		globalVar.PyyDatas.MechaSummonRecordsList = make(map[uint32]*pb3.MechaSummonRecordsList)
	}
	if globalVar.PyyDatas.MechaSummonRecordsList[s.Id] == nil {
		globalVar.PyyDatas.MechaSummonRecordsList[s.Id] = &pb3.MechaSummonRecordsList{}
	}
	if globalVar.PyyDatas.MechaSummonRecordsList[s.Id].StartTime == 0 {
		globalVar.PyyDatas.MechaSummonRecordsList[s.Id].StartTime = s.GetOpenTime()
	}
	return globalVar.PyyDatas.MechaSummonRecordsList[s.Id]
}

func (s *MechaSummon) AfterDraw(libId uint32, libConf *jsondata.LotteryLibConf, awardPoolConf *jsondata.LotteryLibAwardPool, oneAward jsondata.StdRewardVec) {
	data := s.getSysData()

	conf := s.GetConf()
	if nil == conf {
		return
	}

	rewards := engine.FilterRewardByPlayer(s.GetPlayer(), oneAward)
	if len(rewards) <= 0 {
		return
	}

	record := &pb3.MechaSummonRecord{
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

func (s *MechaSummon) record(records *[]*pb3.MechaSummonRecord, record *pb3.MechaSummonRecord, recordLimit int) {
	*records = append(*records, record)
	if len(*records) > recordLimit {
		*records = (*records)[1:]
	}
}

func (s *MechaSummon) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_75_128
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := s.GetConf()
	if nil == conf {
		return neterror.ConfNotFoundError("MechaSummon conf is nil")
	}

	times := req.Times
	sysData := s.getSysData()
	consume := conf.GetDrawConsume(times)
	if len(consume) == 0 {
		return neterror.ConfNotFoundError("MechaSummon consume conf is nil")
	}

	tabLimit := uint32(len(conf.TabLibs))
	for i := sysData.TabId; i <= tabLimit; i++ {
		if conf.TabLibs[i].TotalTimes <= sysData.TotalTimes {
			return neterror.ParamsInvalidError("is full")
		}
	}

	success, remove := s.player.ConsumeByConfWithRet(consume, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogMechaSummonConsume})
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

	remainTimes := times
	result := &lotterylibs.LotteryResult{}

	start := sysData.TabId

	var bundledAwards jsondata.StdRewardVec
	for i := start; i <= tabLimit; i++ {
		if remainTimes == 0 {
			break
		}

		drawTimes := remainTimes
		remainCount := conf.TabLibs[i].TotalTimes - sysData.TotalTimes

		var needChange bool
		if i != tabLimit && remainCount <= remainTimes {
			needChange = true
			drawTimes = remainCount
		}

		remainTimes -= drawTimes

		diamondCount := useDiamondCount

		if drawTimes < useDiamondCount {
			diamondCount = drawTimes
			useDiamondCount -= drawTimes
		} else {
			useDiamondCount = 0
		}

		if len(conf.TabLibs[i].DrawScore) > 0 {
			bundledAwards = append(bundledAwards, jsondata.StdRewardMulti(conf.TabLibs[i].DrawScore, int64(drawTimes))...)
		}

		res := s.lottery.DoDraw(drawTimes, diamondCount, conf.TabLibs[i].LibIds)

		result.LibResult = append(result.LibResult, res.LibResult...)
		result.Awards = append(result.Awards, res.Awards...)
		sysData.TotalTimes += drawTimes

		if needChange {
			sysData.TabId = i + 1
			s.getEvolutionData().LotteryData = nil
		}
	}

	if len(bundledAwards) > 0 {
		engine.GiveRewards(s.GetPlayer(), bundledAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogMechaSummonAwards})
	}

	engine.GiveRewards(s.GetPlayer(), result.Awards, common.EngineGiveRewardParam{
		LogId:        pb3.LogId_LogMechaSummonAwards,
		NoTips:       true,
		BroadcastExt: []interface{}{s.Id},
	})

	rsp := &pb3.S2C_75_128{
		ActiveId: s.Id,
		Times:    times,
	}

	for _, v := range result.LibResult {
		st := &pb3.MechaSummonSt{
			TreasureId:  v.LibId,
			AwardPoolId: v.AwardPoolConf.Id,
			ItemId:      v.OneAwards[0].Id,
			Count:       uint32(v.OneAwards[0].Count),
		}
		rsp.Result = append(rsp.Result, st)
	}

	s.SendProto3(75, 128, rsp)
	s.GetPlayer().TriggerEvent(custom_id.AeActDrawTimes, &custom_id.ActDrawEvent{
		ActType: drawdef.ActMechaSummon,
		ActId:   s.Id,
		Times:   req.Times,
	})

	s.s2cInfo()

	return nil
}

const (
	MechaSummonRecordGType = 1
	MechaSummonRecordPType = 2
)

func (s *MechaSummon) c2sRecord(msg *base.Message) error {
	var req pb3.C2S_75_129
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	rsp := &pb3.S2C_75_129{ActiveId: s.Id, Type: req.GetType()}

	switch req.Type {
	case MechaSummonRecordGType:
		gData := s.globalRecord()
		rsp.Record = gData.Records
		rsp.SuperRecord = gData.SuperRecords
	case MechaSummonRecordPType:
		data := s.getSysData()
		rsp.Record = data.Records
		rsp.SuperRecord = data.SuperRecords
	}

	s.SendProto3(75, 129, rsp)
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYMechaSummon, func() iface.IPlayerYY {
		return &MechaSummon{}
	})

	net.RegisterYYSysProtoV2(75, 128, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*MechaSummon).c2sDraw
	})

	net.RegisterYYSysProtoV2(75, 129, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*MechaSummon).c2sRecord
	})

	engine.RegRewardsBroadcastHandler(tipmsgid.MechaSummonTips, engine.CommonYYDrawBroadcast)
}
