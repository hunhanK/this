/**
 * @Author: LvYuMeng
 * @Date: 2025/7/2
 * @Desc: 法身抽奖
**/

package pyy

import (
	"github.com/gzjjyz/srvlib/utils"
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
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
)

type DharmakayaDraw struct {
	PlayerYYBase
	lotteryMap map[uint32]*lotterylibs.LotteryBase
}

func (s *DharmakayaDraw) OnInit() {
	s.lotteryMap = map[uint32]*lotterylibs.LotteryBase{}

	conf := s.GetConf()
	if nil == conf {
		return
	}

	for _, v := range conf.TabLibs {
		s.lotteryMap[v.TabId] = s.creatLotteryBase(v.TabId)
	}
}

func (s *DharmakayaDraw) OnOpen() {
	s.clearRecord()
	s.s2cInfo()
}

func (s *DharmakayaDraw) Login() {
	s.s2cInfo()
}

func (s *DharmakayaDraw) OnReconnect() {
	s.s2cInfo()
}

func (s *DharmakayaDraw) clearRecord() {
	if record := s.getGlobalRecord(); record.GetStartTime() < s.GetOpenTime() {
		delete(gshare.GetStaticVar().PyyDatas.DharmakayaDrawRecordsList, s.GetId())
	}
}

func (s *DharmakayaDraw) NewDay() {
	for _, v := range s.lotteryMap {
		v.OnLotteryNewDay()
	}
	s.s2cInfo()
}

func (s *DharmakayaDraw) s2cInfo() {
	conf := s.GetConf()
	if nil == conf {
		return
	}

	rsp := &pb3.S2C_56_30{
		ActiveId: s.Id,
		Libs:     map[uint32]*pb3.DharmakayaDrawLib{},
	}

	for _, v := range conf.TabLibs {
		lottery, ok := s.lotteryMap[v.TabId]
		if !ok {
			continue
		}
		rsp.Libs[v.TabId] = &pb3.DharmakayaDrawLib{
			ChanceLibId:    lottery.GetLibId(v.ChanceLibId),
			ChanceLibLucky: lottery.GetGuaranteeCount(v.ChanceLibId),
		}
	}

	s.SendProto3(56, 30, rsp)
}

func (s *DharmakayaDraw) getEvolutionData() *pb3.PYY_EvolutionDharmakayaDraw {
	state := s.GetYYData()
	if nil == state.EvolutionDharmakayaDraw {
		state.EvolutionDharmakayaDraw = make(map[uint32]*pb3.PYY_EvolutionDharmakayaDraw)
	}
	if state.EvolutionDharmakayaDraw[s.Id] == nil {
		state.EvolutionDharmakayaDraw[s.Id] = &pb3.PYY_EvolutionDharmakayaDraw{}
	}
	return state.EvolutionDharmakayaDraw[s.Id]
}

func (s *DharmakayaDraw) getSysData() *pb3.PYY_DharmakayaDraw {
	data := s.getEvolutionData()
	if nil == data.SysData {
		data.SysData = &pb3.PYY_DharmakayaDraw{}
	}
	return data.SysData
}

func (s *DharmakayaDraw) getLotteryData(id uint32) *pb3.LotteryData {
	data := s.getEvolutionData()
	if nil == data.LotteryData {
		data.LotteryData = map[uint32]*pb3.LotteryData{}
	}

	lotteryData, ok := data.LotteryData[id]
	if !ok {
		lotteryData = &pb3.LotteryData{}
		data.LotteryData[id] = lotteryData
	}

	s.lotteryMap[id].InitData(lotteryData)

	return lotteryData
}

func (s *DharmakayaDraw) getGlobalRecord() *pb3.DharmakayaDrawRecordsList {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.PyyDatas {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}
	if globalVar.PyyDatas.DharmakayaDrawRecordsList == nil {
		globalVar.PyyDatas.DharmakayaDrawRecordsList = make(map[uint32]*pb3.DharmakayaDrawRecordsList)
	}
	if globalVar.PyyDatas.DharmakayaDrawRecordsList[s.Id] == nil {
		globalVar.PyyDatas.DharmakayaDrawRecordsList[s.Id] = &pb3.DharmakayaDrawRecordsList{}
	}
	if globalVar.PyyDatas.DharmakayaDrawRecordsList[s.Id].StartTime == 0 {
		globalVar.PyyDatas.DharmakayaDrawRecordsList[s.Id].StartTime = s.GetOpenTime()
	}
	return globalVar.PyyDatas.DharmakayaDrawRecordsList[s.Id]
}

func (s *DharmakayaDraw) creatLotteryBase(id uint32) *lotterylibs.LotteryBase {
	return &lotterylibs.LotteryBase{
		Player:                s.GetPlayer(),
		GetLuckTimes:          s.GetLuckTimes,
		GetLuckyValEx:         s.GetLuckyValEx,
		RawData:               s.RawData(id),
		GetSingleDiamondPrice: s.GetSingleDiamondPrice,
		AfterDraw:             s.AfterDraw,
	}
}

func (s *DharmakayaDraw) RawData(id uint32) func() *pb3.LotteryData {
	return func() *pb3.LotteryData {
		return s.getLotteryData(id)
	}
}

func (s *DharmakayaDraw) GetSingleDiamondPrice() uint32 {
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

func (s *DharmakayaDraw) GetLuckTimes() uint16 {
	conf := s.GetConf()
	if nil == conf {
		return 0
	}
	return uint16(conf.LuckTimes)
}

func (s *DharmakayaDraw) GetLuckyValEx() *jsondata.LotteryLuckyValEx {
	conf := s.GetConf()
	if nil == conf {
		return nil
	}
	return conf.LuckyValEx
}

func (s *DharmakayaDraw) GetConf() *jsondata.DharmakayaDrawConfig {
	return jsondata.GetDharmakayaDrawConf(s.ConfName, s.ConfIdx)
}

func (s *DharmakayaDraw) AfterDraw(libId uint32, libConf *jsondata.LotteryLibConf, awardPoolConf *jsondata.LotteryLibAwardPool, oneAward jsondata.StdRewardVec) {
}

func (s *DharmakayaDraw) addRecord(tabId uint32, record *pb3.DharmakayaDrawRecord) {
	conf := s.GetConf()
	if nil == conf {
		return
	}

	tabConf := conf.TabLibs[tabId]
	if nil == tabConf {
		return
	}

	myData := s.getSysData()
	gData := s.getGlobalRecord()

	if pie.Uint32s(tabConf.RecordSuperLibs).Contains(record.TreasureId) {
		s.record(&gData.SuperRecords, record, int(conf.RecordNum))
		s.record(&myData.SuperRecords, record, int(conf.RecordNum))
	} else {
		s.record(&gData.Records, record, int(conf.RecordNum))
		s.record(&myData.Records, record, int(conf.RecordNum))
	}
}

func (s *DharmakayaDraw) record(records *[]*pb3.DharmakayaDrawRecord, record *pb3.DharmakayaDrawRecord, recordLimit int) {
	*records = append(*records, record)
	if len(*records) > recordLimit {
		*records = (*records)[1:]
	}
}

func (s *DharmakayaDraw) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_56_31
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := s.GetConf()
	if nil == conf {
		return neterror.ConfNotFoundError("DharmakayaDraw conf is nil")
	}

	tabConf := conf.TabLibs[req.TabId]
	if nil == tabConf {
		return neterror.ConfNotFoundError("tab %d conf is nil", req.TabId)
	}

	if tabConf.OpenSrvDay > gshare.GetOpenServerDay() {
		return neterror.ParamsInvalidError("open srv not reach")
	}
	if tabConf.MergeTimes > gshare.GetMergeTimes() {
		return neterror.ParamsInvalidError("mergeTimes not reach")
	}
	if tabConf.MergeDays > gshare.GetMergeSrvDayByTimes(tabConf.MergeTimes) {
		return neterror.ParamsInvalidError("mergeDays not reach")
	}

	cos := conf.Cos[req.GetTimes()]
	if cos == nil {
		return neterror.ConfNotFoundError("DharmakayaDraw cos conf is nil")
	}

	success, remove := s.player.ConsumeByConfWithRet(cos.Consume, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogPYYDharmakayaDrawConsume})
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
		engine.GiveRewards(s.GetPlayer(), bundledAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPYYDharmakayaDrawRewards})
	}

	lottery, ok := s.lotteryMap[req.TabId]
	if !ok {
		return neterror.SysNotExistError("")
	}

	result := lottery.DoDraw(req.Times, useDiamondCount, tabConf.LibIds)
	engine.GiveRewards(s.GetPlayer(), result.Awards, common.EngineGiveRewardParam{
		LogId:        pb3.LogId_LogPYYDharmakayaDrawRewards,
		NoTips:       true,
		BroadcastExt: []interface{}{s.Id},
	})

	sysData := s.getSysData()
	sysData.TotalTimes += req.Times

	rsp := &pb3.S2C_56_31{
		ActiveId:       s.Id,
		Times:          req.Times,
		TotalTimes:     sysData.TotalTimes,
		ChanceLibId:    lottery.GetLibId(tabConf.ChanceLibId),
		ChanceLibLucky: lottery.GetGuaranteeCount(tabConf.ChanceLibId),
		TabId:          req.TabId,
	}

	nowSec := time_util.NowSec()

	for _, v := range result.LibResult {
		st := &pb3.DharmakayaDrawSt{
			TreasureId:  v.LibId,
			AwardPoolId: v.AwardPoolConf.Id,
		}

		if oneAwards := engine.FilterRewardByPlayer(s.GetPlayer(), v.OneAwards); len(oneAwards) > 0 {
			st.ItemId = oneAwards[0].Id
			st.Count = uint32(oneAwards[0].Count)
		}

		rsp.Result = append(rsp.Result, st)

		s.addRecord(req.TabId, &pb3.DharmakayaDrawRecord{
			TreasureId:  st.TreasureId,
			AwardPoolId: st.AwardPoolId,
			ItemId:      st.ItemId,
			Count:       st.Count,
			TimeStamp:   nowSec,
			ActorName:   s.GetPlayer().GetName(),
			ActorId:     s.GetPlayer().GetId(),
		})
	}

	s.SendProto3(56, 31, rsp)

	s.GetPlayer().TriggerEvent(custom_id.AeActDrawTimes, &custom_id.ActDrawEvent{
		ActType: drawdef.ActDharmakaya,
		ActId:   s.Id,
		Times:   req.Times,
	})

	return nil
}

const (
	DharmakayaDrawRecordGType = 1
	DharmakayaDrawRecordPType = 2
)

func (s *DharmakayaDraw) c2sRecord(msg *base.Message) error {
	var req pb3.C2S_56_32
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	rsp := &pb3.S2C_56_32{ActiveId: s.Id, Type: req.GetType()}

	switch req.Type {
	case DharmakayaDrawRecordGType:
		gData := s.getGlobalRecord()
		rsp.Record = gData.Records
		rsp.SuperRecord = gData.SuperRecords
	case DharmakayaDrawRecordPType:
		data := s.getSysData()
		rsp.Record = data.Records
		rsp.SuperRecord = data.SuperRecords
	}

	s.SendProto3(56, 32, rsp)
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYDharmakayaDraw, func() iface.IPlayerYY {
		return &DharmakayaDraw{}
	})

	net.RegisterYYSysProtoV2(56, 31, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*DharmakayaDraw).c2sDraw
	})

	net.RegisterYYSysProtoV2(56, 32, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*DharmakayaDraw).c2sRecord
	})

	engine.RegRewardsBroadcastHandler(tipmsgid.DharmakayaDrawTips1, func(actorId uint64, actorName string, id uint32, count int64, serverId uint32, param common.EngineGiveRewardParam) ([]interface{}, bool) {
		if len(param.BroadcastExt) < 1 {
			return nil, false
		}
		return []interface{}{actorId, actorName, param.BroadcastExt[0], id}, true
	})

	gmevent.Register("fashen.draw", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 2 {
			return false
		}
		count := utils.AtoUint32(args[0])
		tabId := utils.AtoUint32(args[1])
		yys := pyymgr.GetPlayerAllYYObj(player, yydefine.PYYDharmakayaDraw)
		if len(yys) == 0 {
			return false
		}
		yy := yys[0]
		if nil == yy || !yy.IsOpen() {
			return false
		}
		myYY, ok := yy.(*DharmakayaDraw)
		if !ok {
			return false
		}
		conf := myYY.GetConf()
		if nil == conf {
			return false
		}

		tabConf := conf.TabLibs[tabId]
		if nil == tabConf {
			return false
		}
		lo, ok := myYY.lotteryMap[tabId]
		if !ok {
			return false
		}
		lo.GmDraw(count, 0, tabConf.LibIds)
		myYY.s2cInfo()
		return true
	}, 1)
}
