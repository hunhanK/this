/**
 * @Author: LvYuMeng
 * @Date: 2025/6/3
 * @Desc:
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

type CrazyThousandsDraw struct {
	PlayerYYBase
	lottery lotterylibs.LotteryBase
}

func (s *CrazyThousandsDraw) OnInit() {
	s.lottery = lotterylibs.LotteryBase{
		Player:                s.GetPlayer(),
		GetLuckTimes:          s.GetLuckTimes,
		GetLuckyValEx:         s.GetLuckyValEx,
		RawData:               s.RawData,
		GetSingleDiamondPrice: s.GetSingleDiamondPrice,
		AfterDraw:             s.AfterDraw,
	}
}

func (s *CrazyThousandsDraw) Login() {
	s.s2cInfo()
}

func (s *CrazyThousandsDraw) OnReconnect() {
	s.s2cInfo()
}

func (s *CrazyThousandsDraw) OnOpen() {
	s.clearRecord()
	s.s2cInfo()
}

func (s *CrazyThousandsDraw) GetLuckTimes() uint16 {
	conf, ok := jsondata.GetYYCrazyThousandsDrawConf(s.ConfName, s.ConfIdx)
	if !ok {
		return 0
	}
	return uint16(conf.LuckTimes)
}

func (s *CrazyThousandsDraw) GetLuckyValEx() *jsondata.LotteryLuckyValEx {
	conf, ok := jsondata.GetYYCrazyThousandsDrawConf(s.ConfName, s.ConfIdx)
	if !ok {
		return nil
	}
	return conf.LuckyValEx
}

func (s *CrazyThousandsDraw) RawData() *pb3.LotteryData {
	data := s.getData()
	return data.LotteryData
}

func (s *CrazyThousandsDraw) GetSingleDiamondPrice() uint32 {
	conf, ok := jsondata.GetYYCrazyThousandsDrawConf(s.ConfName, s.ConfIdx)
	if !ok {
		return 0
	}
	singlePrice := jsondata.GetAutoBuyItemPrice(conf.DrawConsume[0].Id, moneydef.BindDiamonds)
	if singlePrice <= 0 {
		singlePrice = jsondata.GetAutoBuyItemPrice(conf.DrawConsume[0].Id, moneydef.Diamonds)
	}
	return uint32(singlePrice)
}

func (s *CrazyThousandsDraw) AfterDraw(libId uint32, libConf *jsondata.LotteryLibConf, awardPoolConf *jsondata.LotteryLibAwardPool, oneAward jsondata.StdRewardVec) {
	data := s.getData().SysData

	conf, ok := jsondata.GetYYCrazyThousandsDrawConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	rewards := engine.FilterRewardByPlayer(s.GetPlayer(), awardPoolConf.Awards)
	if len(rewards) <= 0 {
		return
	}

	record := &pb3.CrazyThousandsDrawRecord{
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

	for _, awardConf := range awardPoolConf.Awards {
		if awardConf.Broadcast > 0 { //玩家id，玩家名，活动id，道具
			engine.BroadcastTipMsgById(tipmsgid.CrazyThousandsTips, s.GetPlayer().GetId(), s.GetPlayer().GetName(), s.GetId(), awardConf.Id)
		}
	}
}

func (s *CrazyThousandsDraw) record(records *[]*pb3.CrazyThousandsDrawRecord, record *pb3.CrazyThousandsDrawRecord, recordLimit int) {
	*records = append(*records, record)
	if len(*records) > recordLimit {
		*records = (*records)[1:]
	}
}

func (s *CrazyThousandsDraw) clearRecord() {
	if record := s.globalRecord(); record.GetStartTime() < s.GetOpenTime() {
		delete(gshare.GetStaticVar().PyyDatas.CrazyThousandsDrawRecordList, s.GetId())
	}
}

func (s *CrazyThousandsDraw) globalRecord() *pb3.CrazyThousandsDrawRecordList {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.PyyDatas {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}
	if globalVar.PyyDatas.CrazyThousandsDrawRecordList == nil {
		globalVar.PyyDatas.CrazyThousandsDrawRecordList = make(map[uint32]*pb3.CrazyThousandsDrawRecordList)
	}
	if globalVar.PyyDatas.CrazyThousandsDrawRecordList[s.Id] == nil {
		globalVar.PyyDatas.CrazyThousandsDrawRecordList[s.Id] = &pb3.CrazyThousandsDrawRecordList{}
	}
	if globalVar.PyyDatas.CrazyThousandsDrawRecordList[s.Id].StartTime == 0 {
		globalVar.PyyDatas.CrazyThousandsDrawRecordList[s.Id].StartTime = s.GetOpenTime()
	}
	return globalVar.PyyDatas.CrazyThousandsDrawRecordList[s.Id]
}

func (s *CrazyThousandsDraw) ResetData() {
	state := s.GetYYData()
	if nil == state.EvolutionCrazyThousandsDraw {
		return
	}
	delete(state.EvolutionCrazyThousandsDraw, s.Id)
}

func (s *CrazyThousandsDraw) getData() *pb3.PYY_EvolutionCrazyThousandsDraw {
	state := s.GetYYData()
	if nil == state.EvolutionCrazyThousandsDraw {
		state.EvolutionCrazyThousandsDraw = make(map[uint32]*pb3.PYY_EvolutionCrazyThousandsDraw)
	}
	if state.EvolutionCrazyThousandsDraw[s.Id] == nil {
		state.EvolutionCrazyThousandsDraw[s.Id] = &pb3.PYY_EvolutionCrazyThousandsDraw{}
	}

	sData := state.EvolutionCrazyThousandsDraw[s.Id]
	if nil == sData.LotteryData {
		sData.LotteryData = &pb3.LotteryData{}
	}
	s.lottery.InitData(sData.LotteryData)

	if nil == sData.SysData {
		sData.SysData = &pb3.PYY_CrazyThousandsDraw{}
	}
	if nil == sData.SysData.Boxes {
		sData.SysData.Boxes = make(map[uint32]*pb3.CrazyThousandsDrawTemporaryBox)
	}
	return sData
}

func (s *CrazyThousandsDraw) NewDay() {
	s.lottery.OnLotteryNewDay()
	data := s.getData()
	data.SysData.UseTimes = 0
	s.s2cInfo()
}

func (s *CrazyThousandsDraw) s2cInfo() {
	data := s.getData()
	s.SendProto3(75, 80, &pb3.S2C_75_80{
		ActiveId:   s.Id,
		TotalTimes: data.SysData.TotalTimes,
		Boxes:      data.SysData.Boxes,
		UseTimes:   data.SysData.UseTimes,
	})
}

func (s *CrazyThousandsDraw) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_75_81
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf, ok := jsondata.GetYYCrazyThousandsDrawConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("CrazyThousandsDraw conf is nil")
	}

	if len(conf.DrawConsume) == 0 {
		return neterror.ConfNotFoundError("CrazyThousandsDraw cos conf is nil")
	}

	success, remove := s.player.ConsumeByConfWithRet(conf.DrawConsume, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogCrazyThousandsDrawConsume})
	if !success {
		s.player.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	if req.Times != conf.DrawTimes {
		return neterror.ParamsInvalidError("time not equal")
	}

	diamond := uint32(remove.MoneyMap[moneydef.Diamonds] + remove.MoneyMap[moneydef.BindDiamonds])
	singlePrice := s.GetSingleDiamondPrice()
	var useDiamondCount uint32
	if singlePrice > 0 {
		useDiamondCount = diamond / singlePrice
	}

	result := s.lottery.DoDraw(req.Times, useDiamondCount, conf.LibIds)

	sData := s.getData().SysData
	sData.TotalTimes += req.GetTimes()

	rsp := &pb3.S2C_75_81{
		ActiveId:   s.Id,
		Times:      req.Times,
		TotalTimes: sData.TotalTimes,
	}

	newIdx := uint32(len(sData.Boxes) + 1)
	newBox := &pb3.CrazyThousandsDrawTemporaryBox{
		Id: newIdx,
	}

	for _, v := range result.LibResult {
		rewards := engine.FilterRewardByPlayer(s.GetPlayer(), v.AwardPoolConf.Awards)
		itemId, count := rewards[0].Id, uint32(rewards[0].Count)
		rsp.Result = append(rsp.Result, &pb3.CrazyThousandsDrawSt{
			TreasureId:  v.LibId,
			AwardPoolId: v.AwardPoolConf.Id,
			ItemId:      itemId,
			Count:       count,
		})
		newBox.Items = append(newBox.Items, &pb3.CrazyThousandsDrawItem{
			ItemId: itemId,
			Count:  count,
		})
	}

	sData.Boxes[newIdx] = newBox

	s.SendProto3(75, 81, rsp)
	s.SendProto3(75, 84, &pb3.S2C_75_84{
		ActiveId: s.Id,
		Box:      newBox,
	})

	return nil
}

const (
	CrazyThousandsDrawRecordGType = 1
	CrazyThousandsDrawRecordPType = 2
)

func (s *CrazyThousandsDraw) c2sRecord(msg *base.Message) error {
	var req pb3.C2S_75_82
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	rsp := &pb3.S2C_75_82{ActiveId: s.Id, Type: req.GetType()}

	switch req.Type {
	case CrazyThousandsDrawRecordGType:
		gData := s.globalRecord()
		rsp.Record = gData.Records
		rsp.SuperRecord = gData.SuperRecords
	case CrazyThousandsDrawRecordPType:
		data := s.getData().SysData
		rsp.Record = data.Records
		rsp.SuperRecord = data.SuperRecords
	}

	s.SendProto3(75, 82, rsp)
	return nil
}

func (s *CrazyThousandsDraw) c2sOpenBox(msg *base.Message) error {
	var req pb3.C2S_75_83
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf, ok := jsondata.GetYYCrazyThousandsDrawConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("CrazyThousandsDraw conf is nil")
	}

	sData := s.getData().SysData
	if sData.UseTimes >= conf.DailyRevTimes {
		return neterror.ParamsInvalidError("times not enough")
	}

	box, ok := sData.Boxes[req.BoxId]
	if !ok {
		return neterror.ParamsInvalidError("not exist box %d", req.BoxId)
	}

	if box.IsRev {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	sData.UseTimes++
	box.IsRev = true

	var rewards jsondata.StdRewardVec
	for _, v := range box.Items {
		rewards = append(rewards, &jsondata.StdReward{Id: v.ItemId, Count: int64(v.Count)})
	}

	engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogCrazyThousandsDrawOpenBox,
	})

	s.SendProto3(75, 83, &pb3.S2C_75_83{
		ActiveId: s.Id,
		BoxId:    box.Id,
		UseTimes: sData.UseTimes,
	})

	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYCrazyThousandsDraw, func() iface.IPlayerYY {
		return &CrazyThousandsDraw{}
	})

	net.RegisterYYSysProtoV2(75, 81, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*CrazyThousandsDraw).c2sDraw
	})
	net.RegisterYYSysProtoV2(75, 82, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*CrazyThousandsDraw).c2sRecord
	})
	net.RegisterYYSysProtoV2(75, 83, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*CrazyThousandsDraw).c2sOpenBox
	})
}
