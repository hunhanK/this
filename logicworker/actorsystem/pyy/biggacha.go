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

type BigGacha struct {
	PlayerYYBase
	lottery lotterylibs.LotteryBase
}

func (s *BigGacha) OnInit() {
	s.lottery = lotterylibs.LotteryBase{
		Player:                s.GetPlayer(),
		GetLuckTimes:          s.GetLuckTimes,
		GetLuckyValEx:         s.GetLuckyValEx,
		RawData:               s.RawData,
		GetSingleDiamondPrice: s.GetSingleDiamondPrice,
		AfterDraw:             s.AfterDraw,
	}
}

func (s *BigGacha) getData() *pb3.PYY_ExBigGacha {
	state := s.GetYYData()
	if state.BigGacha == nil {
		state.BigGacha = make(map[uint32]*pb3.PYY_ExBigGacha)
	}
	if state.BigGacha[s.Id] == nil {
		state.BigGacha[s.Id] = &pb3.PYY_ExBigGacha{}
	}
	sData := state.BigGacha[s.Id]
	if sData.LotteryData == nil {
		sData.LotteryData = &pb3.LotteryData{}
	}
	s.lottery.InitData(sData.LotteryData)

	if sData.SysData == nil {
		sData.SysData = &pb3.PYY_BigGacha{}
	}

	return sData
}

func (s *BigGacha) OnOpen() {
	s.s2cInfo()
}

func (s *BigGacha) ResetData() {
	state := s.GetYYData()
	if state.BigGacha == nil {
		return
	}
	delete(state.BigGacha, s.Id)
}

func (s *BigGacha) s2cInfo() {
	s.SendProto3(143, 12, &pb3.S2C_143_12{
		ActiveId: s.Id,
		Data:     s.formatClient(),
	})
}

func (s *BigGacha) OnReconnect() {
	s.s2cInfo()
}

func (s *BigGacha) OnEnd() {
	s.clearRecord(true)
	s.s2cInfo()
}

func (s *BigGacha) Login() {
	s.s2cInfo()
}

func (s *BigGacha) clearRecord(isEnd bool) {
	record := s.globalRecord()
	if record.StartTime < s.OpenTime {
		delete(gshare.GetStaticVar().PyyDatas.BigGachaRecordList, s.Id)
	}

	if isEnd && record.StartTime == s.OpenTime {
		delete(gshare.GetStaticVar().PyyDatas.BigGachaRecordList, s.Id)
	}
}

func (s *BigGacha) GetLuckTimes() uint16 {
	conf := jsondata.GetBigGachaConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return 0
	}
	return uint16(conf.LuckTimes)
}

func (s *BigGacha) GetLuckyValEx() *jsondata.LotteryLuckyValEx {
	conf := jsondata.GetBigGachaConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return nil
	}
	return conf.LuckyValEx
}

func (s *BigGacha) RawData() *pb3.LotteryData {
	data := s.getData()
	return data.LotteryData
}

func (s *BigGacha) GetSingleDiamondPrice() uint32 {
	conf := jsondata.GetBigGachaConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return 0
	}
	consumeConf := conf.GetDrawConsume(1)
	singlePrice := jsondata.GetAutoBuyItemPrice(consumeConf[0].Id, moneydef.BindDiamonds)
	if singlePrice <= 0 {
		singlePrice = jsondata.GetAutoBuyItemPrice(consumeConf[0].Id, moneydef.Diamonds)
	}
	return uint32(singlePrice)
}

func (s *BigGacha) AfterDraw(libId uint32, libConf *jsondata.LotteryLibConf, awardPoolConf *jsondata.LotteryLibAwardPool, oneAward jsondata.StdRewardVec) {
	data := s.getData().GetSysData()

	conf := jsondata.GetBigGachaConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}
	rewards := engine.FilterRewardByPlayer(s.GetPlayer(), oneAward)
	if len(rewards) <= 0 {
		return
	}

	record := &pb3.BigGachaRecord{
		TreasureId:  libId,
		AwardPoolId: awardPoolConf.Id,
		ItemId:      rewards[0].Id,
		Count:       uint32(rewards[0].Count),
		TimeStamp:   time_util.NowSec(),
		ActorName:   s.GetPlayer().GetName(),
	}

	gData := s.globalRecord()

	if pie.Uint32s(conf.RecordSuperLibs).Contains(libId) {
		s.record(&gData.SuperRecords, record, int(conf.AllBigRecordNum))
		s.record(&data.SuperRecords, record, int(conf.PRecordNum))
	} else {
		s.record(&gData.Records, record, int(conf.AllSmallRecordNum))
		s.record(&data.Records, record, int(conf.PRecordNum))
	}
}
func (s *BigGacha) getGuarantee() []*pb3.ChanceGuarantee {
	conf := jsondata.GetBigGachaConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return nil
	}
	var guarantee []*pb3.ChanceGuarantee
	for _, chanceLibId := range conf.ChanceLibId {
		guarantee = append(guarantee, &pb3.ChanceGuarantee{
			LibId:     s.lottery.GetLibId(chanceLibId),
			Guarantee: s.lottery.GetGuaranteeCount(chanceLibId),
		})
	}
	return guarantee
}

func (s *BigGacha) record(records *[]*pb3.BigGachaRecord, record *pb3.BigGachaRecord, recordLimit int) {
	*records = append(*records, record)
	if len(*records) > recordLimit {
		*records = (*records)[1:]
	}
}

func (s *BigGacha) c2sFreePick(msg *base.Message) error {
	var req pb3.C2S_143_13
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf := jsondata.GetBigGachaConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("BigGachaConf is nil")
	}
	isNoLimit := false
	isAwardPoolInConf := false
	drawConf := s.GetPlayer().GetDrawLibConf(conf.DefLibId)
	if drawConf != nil {
		for _, pool := range drawConf.AwardPool {
			if pool.Id == req.AwardPoolId {
				if pool.Count == 0 {
					isNoLimit = true
				}
				isAwardPoolInConf = true
				break
			}
		}
	}

	if !isAwardPoolInConf {
		return neterror.ConfNotFoundError("The reward is not in the prize pool")
	}
	res := s.lottery.GetNotGetAwards(conf.DefLibId)
	var isPick bool
	for _, v := range res.LibResult {
		if v.AwardPoolConf == nil {
			continue
		}
		if v.AwardPoolConf.Id != req.AwardPoolId {
			continue
		}
		isPick = true
		break
	}
	if !isNoLimit && !isPick {
		return neterror.ParamsInvalidError("The reward has reached its maximum limit")
	}
	data := s.getData()
	data.SysData.UpAwardPoolId = req.AwardPoolId
	s.SendProto3(143, 13, &pb3.S2C_143_13{
		ActiveId:    s.Id,
		AwardPoolId: req.AwardPoolId,
	})
	return nil
}

func (s *BigGacha) formatClient() *pb3.BigGachaClient {
	conf := jsondata.GetBigGachaConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return nil
	}

	drawConf := s.GetPlayer().GetDrawLibConf(conf.DefLibId)
	if drawConf == nil {
		return nil
	}
	data := s.getData()
	sData := s.getData().GetSysData()
	upGetIds := make(map[uint32]uint32)
	for _, v := range drawConf.AwardPool {
		upGetIds[v.Id] = data.LotteryData.AwardCount[v.Id]
	}

	client := &pb3.BigGachaClient{
		TotalTimes:  sData.TotalTimes,
		Guarantee:   s.getGuarantee(),
		UpGetIds:    upGetIds,
		AwardPoolId: sData.UpAwardPoolId,
	}
	return client
}

func (s *BigGacha) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_143_16
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetBigGachaConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("BigGachaConf is nil")
	}

	sData := s.getData().GetSysData()
	if sData.UpAwardPoolId == 0 {
		return neterror.ParamsInvalidError("Not select prize")
	}

	consume := conf.GetDrawConsume(req.Times)
	if len(consume) == 0 {
		return neterror.ConfNotFoundError("consume is nil")
	}

	singleCos := conf.GetDrawConsume(1)
	if len(singleCos) == 0 {
		return neterror.ConfNotFoundError("BigGacha cos conf is nil")
	}

	canUseCount := uint32(s.player.GetItemCount(singleCos[0].Id, -1)) / singleCos[0].Count

	var (
		actTimes    uint32
		isHit       bool
		totalAwards jsondata.StdRewardVec
		resList     []*lotterylibs.LotteryResult
	)

	singlePrice := s.GetSingleDiamondPrice()
	canConsume := s.player.CheckConsumeByConf(conf.Cos[req.Times].Consume, req.AutoBuy, pb3.LogId_LogBigGachaConsume)
	if !canConsume {
		s.player.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	hitDef := func(r *lotterylibs.LotteryResult) bool {
		for _, v := range r.LibResult {
			if v.AwardPoolConf == nil {
				continue
			}
			if v.AwardPoolConf.Id != sData.UpAwardPoolId {
				continue
			}
			return true
		}
		return false
	}

	for i := uint32(1); i <= req.Times; i++ {
		actTimes++
		var useDiamondCount uint32
		if actTimes > canUseCount && singlePrice > 0 {
			useDiamondCount = 1
		}
		result := s.lottery.DoDraw(1, useDiamondCount, conf.LibIds)
		resList = append(resList, result)
		totalAwards = append(totalAwards, result.Awards...)
		isHit = hitDef(result)

		if isHit {
			break
		}
	}

	realConsume := jsondata.ConsumeMulti(singleCos, actTimes)
	if len(realConsume) == 0 {
		return neterror.ParamsInvalidError("consume nil")
	}

	success := s.player.ConsumeByConf(realConsume, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogBigGachaConsume})
	if !success {
		s.player.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	bundledAwards := jsondata.StdRewardMulti(conf.DrawScore, int64(actTimes))
	if len(bundledAwards) > 0 {
		engine.GiveRewards(s.GetPlayer(), bundledAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogBigGachaAwards})
	}
	jumpId := conf.TipsJump
	engine.GiveRewards(s.GetPlayer(), totalAwards, common.EngineGiveRewardParam{
		LogId:        pb3.LogId_LogBigGachaAwards,
		NoTips:       true,
		BroadcastExt: []interface{}{s.Id, jumpId},
	})

	if isHit {
		sData.UpAwardPoolId = 0
	}

	sData.TotalTimes += actTimes
	rsp := &pb3.S2C_143_16{
		ActiveId: s.Id,
		Times:    actTimes,
	}

	for _, lotteryRes := range resList {
		for _, libRes := range lotteryRes.LibResult {
			if libRes.AwardPoolConf == nil || len(libRes.OneAwards) == 0 {
				continue
			}
			firstReward := libRes.OneAwards[0]
			pbSt := &pb3.BigGachaSt{
				TreasureId:  libRes.LibId,
				AwardPoolId: libRes.AwardPoolConf.Id,
				ItemId:      firstReward.Id,
				Count:       uint32(firstReward.Count),
			}
			rsp.Result = append(rsp.Result, pbSt)
		}
	}

	s.GetPlayer().TriggerEvent(custom_id.AeActDrawTimes, &custom_id.ActDrawEvent{
		ActType: drawdef.ActBigGacha,
		ActId:   s.Id,
		Times:   actTimes,
	})
	rsp.Data = s.formatClient()
	s.SendProto3(143, 16, rsp)
	return nil
}

func (s *BigGacha) globalRecord() *pb3.BigGachaRecordList {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.PyyDatas {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}
	if globalVar.PyyDatas.BigGachaRecordList == nil {
		globalVar.PyyDatas.BigGachaRecordList = make(map[uint32]*pb3.BigGachaRecordList)
	}
	if globalVar.PyyDatas.BigGachaRecordList[s.Id] == nil {
		globalVar.PyyDatas.BigGachaRecordList[s.Id] = &pb3.BigGachaRecordList{}
	}
	if globalVar.PyyDatas.BigGachaRecordList[s.Id].StartTime == 0 {
		globalVar.PyyDatas.BigGachaRecordList[s.Id].StartTime = s.GetOpenTime()
	}
	return globalVar.PyyDatas.BigGachaRecordList[s.Id]
}

const (
	BigGachaRecordGType     = 1
	BigGachaDrawRecordPType = 2
)

func (s *BigGacha) c2sRecord(msg *base.Message) error {
	var req pb3.C2S_143_14
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	rsp := &pb3.S2C_143_14{
		ActiveId: s.Id,
		Type:     req.Type,
	}
	switch req.Type {
	case BigGachaRecordGType:
		gData := s.globalRecord()
		rsp.Record = gData.Records
		rsp.SuperRecord = gData.SuperRecords
	case BigGachaDrawRecordPType:
		data := s.getData().GetSysData()
		rsp.Record = data.Records
		rsp.SuperRecord = data.SuperRecords
	}
	s.SendProto3(143, 14, rsp)
	return nil
}

func (s *BigGacha) GetChangeLibConf(libId uint32) *jsondata.LotteryLibConf {
	sysData := s.getData().SysData
	myUp := sysData.UpAwardPoolId
	if myUp == 0 {
		return nil
	}

	libConf := jsondata.ShallowCopyLotteryLibConf(libId)
	if nil == libConf {
		return nil
	}

	for _, v := range libConf.AwardPool {
		awardConf := v
		if v.Id == myUp {
			awardConf.Weight = 100
		}

	}

	return libConf
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYBigGacha, func() iface.IPlayerYY {
		return &BigGacha{}
	})

	net.RegisterYYSysProtoV2(143, 13, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*BigGacha).c2sFreePick
	})
	net.RegisterYYSysProtoV2(143, 16, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*BigGacha).c2sDraw
	})
	net.RegisterYYSysProtoV2(143, 14, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*BigGacha).c2sRecord
	})
}
