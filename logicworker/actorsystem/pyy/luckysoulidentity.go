/**
 * @Author: LvYuMeng
 * @Date: 2024/10/29
 * @Desc: 幸运鉴定
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

type LuckyIdentitySys struct {
	PlayerYYBase
	lottery lotterylibs.LotteryBase
}

func (s *LuckyIdentitySys) OnInit() {
	s.lottery = lotterylibs.LotteryBase{
		Player:                s.GetPlayer(),
		GetLuckTimes:          s.GetLuckTimes,
		GetLuckyValEx:         s.GetLuckyValEx,
		RawData:               s.RawData,
		GetSingleDiamondPrice: s.GetSingleDiamondPrice,
		AfterDraw:             s.AfterDraw,
	}
}

func (s *LuckyIdentitySys) Login() {
	s.s2cInfo()
}

func (s *LuckyIdentitySys) OnReconnect() {
	s.s2cInfo()
}

func (s *LuckyIdentitySys) OnOpen() {
	s.clearRecord()
	s.setLibId(0)
	s.s2cInfo()
}

func (s *LuckyIdentitySys) s2cInfo() {
	data := s.data()
	s.SendProto3(69, 230, &pb3.S2C_69_230{
		ActiveId: s.GetId(),
		Info:     data.GetLuckyIdentify(),
		Lucky:    s.lottery.GetGuaranteeCount(s.GetGuaranteeLibId()),
	})
}

func (s *LuckyIdentitySys) GetGuaranteeLibId() uint32 {
	return s.data().GetLuckyIdentify().ChooseId
}

func (s *LuckyIdentitySys) clearRecord() {
	if record := s.globalRecord(); record.GetStartTime() < s.GetOpenTime() {
		delete(gshare.GetStaticVar().PyyDatas.LuckyIdentifyRecord, s.GetId())
	}
}

func (s *LuckyIdentitySys) ResetData() {
	state := s.GetYYData()
	if nil == state.EvolutionLuckyIdentify {
		return
	}
	delete(state.EvolutionLuckyIdentify, s.GetId())
}

func (s *LuckyIdentitySys) data() *pb3.PYY_EvolutionLuckyIdentify {
	state := s.GetYYData()
	if nil == state.EvolutionLuckyIdentify {
		state.EvolutionLuckyIdentify = make(map[uint32]*pb3.PYY_EvolutionLuckyIdentify)
	}
	if state.EvolutionLuckyIdentify[s.Id] == nil {
		state.EvolutionLuckyIdentify[s.Id] = &pb3.PYY_EvolutionLuckyIdentify{}
	}
	sData := state.EvolutionLuckyIdentify[s.Id]
	if nil == sData.LotteryData {
		sData.LotteryData = &pb3.LotteryData{}
	}
	s.lottery.InitData(sData.LotteryData)
	if nil == sData.LuckyIdentify {
		sData.LuckyIdentify = &pb3.PYY_LuckyIdentify{}
	}
	if nil == sData.LuckyIdentify.LibHit {
		sData.LuckyIdentify.LibHit = make(map[uint32]*pb3.LuckyIdentifyLibHit)
	}
	return sData
}

func (s *LuckyIdentitySys) globalRecord() *pb3.LuckyIdentifyRecordsList {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.PyyDatas {
		globalVar.PyyDatas = &pb3.PYYDatas{}
	}
	if globalVar.PyyDatas.LuckyIdentifyRecord == nil {
		globalVar.PyyDatas.LuckyIdentifyRecord = make(map[uint32]*pb3.LuckyIdentifyRecordsList)
	}
	if globalVar.PyyDatas.LuckyIdentifyRecord[s.Id] == nil {
		globalVar.PyyDatas.LuckyIdentifyRecord[s.Id] = &pb3.LuckyIdentifyRecordsList{}
	}
	if globalVar.PyyDatas.LuckyIdentifyRecord[s.Id].StartTime == 0 {
		globalVar.PyyDatas.LuckyIdentifyRecord[s.Id].StartTime = s.GetOpenTime()
	}
	return globalVar.PyyDatas.LuckyIdentifyRecord[s.Id]
}

func (s *LuckyIdentitySys) GetLuckTimes() uint16 {
	conf, ok := jsondata.GetYYLuckyIdentityConf(s.ConfName, s.ConfIdx)
	if !ok {
		return 0
	}
	return conf.LuckTimes
}

func (s *LuckyIdentitySys) GetLuckyValEx() *jsondata.LotteryLuckyValEx {
	conf, ok := jsondata.GetYYLuckyIdentityConf(s.ConfName, s.ConfIdx)
	if !ok {
		return nil
	}
	return conf.LuckyValEx
}

func (s *LuckyIdentitySys) RawData() *pb3.LotteryData {
	data := s.data()
	return data.LotteryData
}

func (s *LuckyIdentitySys) GetSingleDiamondPrice() uint32 {
	conf, ok := jsondata.GetYYLuckyIdentityConf(s.ConfName, s.ConfIdx)
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

func (s *LuckyIdentitySys) NewDay() {
	s.lottery.OnLotteryNewDay()
}

const (
	LuckyIdentityRecordGType = 1
	LuckyIdentityRecordPType = 2
)

func (s *LuckyIdentitySys) AfterDraw(libId uint32, libConf *jsondata.LotteryLibConf, awardPoolConf *jsondata.LotteryLibAwardPool, oneAward jsondata.StdRewardVec) {
	sData := s.data().GetLuckyIdentify()
	if sData.GetChooseId() == libId {
		sData.BigHitLibIds = append(sData.BigHitLibIds, libId)
	}
	if nil == sData.LibHit[libId] {
		sData.LibHit[libId] = &pb3.LuckyIdentifyLibHit{}
	}
	if nil == sData.LibHit[libId].AwardPoolId {
		sData.LibHit[libId].AwardPoolId = make(map[uint32]uint32)
	}
	sData.LibHit[libId].AwardPoolId[awardPoolConf.Id]++

	conf, ok := jsondata.GetYYLuckyIdentityConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	rewards := engine.FilterRewardByPlayer(s.GetPlayer(), awardPoolConf.Awards)
	if len(rewards) <= 0 {
		return
	}

	record := &pb3.LuckyIdentifyRecord{
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

func (s *LuckyIdentitySys) record(records *[]*pb3.LuckyIdentifyRecord, record *pb3.LuckyIdentifyRecord, recordLimit int) {
	*records = append(*records, record)
	if len(*records) > recordLimit {
		*records = (*records)[1:]
	}
}

func (s *LuckyIdentitySys) c2sRecord(msg *base.Message) error {
	var req pb3.C2S_69_232
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	rsp := &pb3.S2C_69_232{ActiveId: s.Id, Type: req.GetType()}

	switch req.Type {
	case LuckyIdentityRecordGType:
		gData := s.globalRecord()
		rsp.Record = gData.Records
		rsp.SuperRecord = gData.SuperRecords
	case LuckyIdentityRecordPType:
		data := s.data().GetLuckyIdentify()
		rsp.Record = data.Records
		rsp.SuperRecord = data.SuperRecords
	}

	s.SendProto3(69, 232, rsp)
	return nil
}

func (s *LuckyIdentitySys) setLibId(libId uint32) (bool, error) {
	conf, ok := jsondata.GetYYLuckyIdentityConf(s.ConfName, s.ConfIdx)
	if !ok {
		return false, neterror.ConfNotFoundError("LuckyIdentitySys conf is nil")
	}

	if libId == 0 {
		libId = conf.DefaultLib
	}

	data := s.data().GetLuckyIdentify()

	if len(data.BigHitLibIds) == 0 && libId != conf.DefaultLib {
		return false, neterror.ParamsInvalidError("first lib cant set")
	}

	if pie.Uint32s(data.BigHitLibIds).Contains(libId) {
		return false, neterror.ParamsInvalidError("already draw")
	}

	if data.ChooseId > 0 && data.ChooseId == libId {
		return false, neterror.ParamsInvalidError("already choose")
	}

	if data.ChooseId > 0 && !pie.Uint32s(data.BigHitLibIds).Contains(data.ChooseId) {
		return false, neterror.ParamsInvalidError("this loop not over")
	}

	if uint32(len(data.BigHitLibIds)) >= conf.OptionalNum+1 {
		return false, neterror.ParamsInvalidError("exceed choose times")
	}

	data.ChooseId = libId

	return true, nil
}

func (s *LuckyIdentitySys) c2sLibSet(msg *base.Message) error {
	var req pb3.C2S_69_231
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	ok, err := s.setLibId(req.GetLibId())
	if !ok {
		return err
	}
	s.SendProto3(69, 231, &pb3.S2C_69_231{
		ActiveId: s.GetId(),
		LibId:    s.data().GetLuckyIdentify().ChooseId,
	})
	return nil
}

func (s *LuckyIdentitySys) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_69_233
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf, ok := jsondata.GetYYLuckyIdentityConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("LuckyIdentitySys conf is nil")
	}

	sData := s.data().GetLuckyIdentify()

	if sData.GetChooseId() == 0 || pie.Uint32s(sData.BigHitLibIds).Contains(sData.GetChooseId()) {
		return neterror.ParamsInvalidError("choose %d cant draw", sData.GetChooseId())
	}

	libConf, ok := conf.Libs[sData.GetChooseId()]
	if !ok {
		return neterror.ConfNotFoundError("LuckyIdentitySys lib conf %d is nil", sData.GetChooseId())
	}

	consumes := conf.GetDrawConsume(req.GetTimes())
	if nil == consumes {
		return neterror.ConfNotFoundError("LuckyIdentitySys consumes conf is ni;")
	}
	success, remove := s.player.ConsumeByConfWithRet(consumes, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogLuckyIdentifyConsume})
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
		engine.GiveRewards(s.GetPlayer(), bundledAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogLuckyIdentifyDrawAwards})
	}

	result := s.lottery.DoDraw(req.Times, useDiamondCount, libConf.LibIds)
	engine.GiveRewards(s.GetPlayer(), result.Awards, common.EngineGiveRewardParam{
		LogId:        pb3.LogId_LogLuckyIdentifyDrawAwards,
		NoTips:       true,
		BroadcastExt: []interface{}{s.Id},
	})

	sData.TotalTimes += req.GetTimes()

	rsp := &pb3.S2C_69_233{
		ActiveId:   s.GetId(),
		Times:      req.GetTimes(),
		TotalTimes: sData.GetTotalTimes(),
		Lucky:      s.lottery.GetGuaranteeCount(s.GetGuaranteeLibId()),
		LibHit:     sData.GetLibHit(),
	}

	for _, v := range result.LibResult {
		rsp.Result = append(rsp.Result, &pb3.LuckyIdentifySt{
			TreasureId:  v.LibId,
			AwardPoolId: v.AwardPoolConf.Id,
		})
	}

	s.SendProto3(69, 233, rsp)
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYLuckyIdentity, func() iface.IPlayerYY {
		return &LuckyIdentitySys{}
	})

	net.RegisterYYSysProtoV2(69, 231, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*LuckyIdentitySys).c2sLibSet
	})
	net.RegisterYYSysProtoV2(69, 232, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*LuckyIdentitySys).c2sRecord
	})
	net.RegisterYYSysProtoV2(69, 233, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*LuckyIdentitySys).c2sDraw
	})

	engine.RegRewardsBroadcastHandler(tipmsgid.LuckyIdentityDraw, func(actorId uint64, actorName string, id uint32, count int64, serverId uint32, param common.EngineGiveRewardParam) ([]interface{}, bool) {
		if len(param.BroadcastExt) < 1 {
			return nil, false
		}
		return []interface{}{actorId, actorName, param.BroadcastExt[0], id}, true //玩家id，玩家名，活动id，道具
	})

}
