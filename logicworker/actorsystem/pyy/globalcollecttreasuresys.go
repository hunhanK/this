/**
 * @Author: zjj
 * @Date: 2024/8/5
 * @Desc: 全民集卡 - 卡池
**/

package pyy

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/lotterylibs"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
)

type GlobalCollectTreasureSys struct {
	PlayerYYBase
	lottery lotterylibs.LotteryBase
}

func (s *GlobalCollectTreasureSys) OnInit() {
	s.lottery = lotterylibs.LotteryBase{
		Player:                s.GetPlayer(),
		GetLuckTimes:          s.GetLuckTimes,
		GetLuckyValEx:         s.GetLuckyValEx,
		RawData:               s.RawData,
		GetSingleDiamondPrice: s.GetSingleDiamondPrice,
		AfterDraw:             s.AfterDraw,
	}
}

func (s *GlobalCollectTreasureSys) GetLuckTimes() uint16 {
	conf, ok := jsondata.GetGlobalCollectTreasureConf(s.ConfName, s.ConfIdx)
	if !ok {
		return uint16(0)
	}
	return conf.LuckTimes
}

func (s *GlobalCollectTreasureSys) GetLuckyValEx() *jsondata.LotteryLuckyValEx {
	conf, ok := jsondata.GetGlobalCollectTreasureConf(s.ConfName, s.ConfIdx)
	if !ok {
		return nil
	}
	return conf.LuckyValEx
}

func (s *GlobalCollectTreasureSys) getData() *pb3.PYYGlobalCollectCardsTreasure {
	state := s.GetYYData()
	if nil == state.GlobalCollectCardsTreasureMap {
		state.GlobalCollectCardsTreasureMap = make(map[uint32]*pb3.PYYGlobalCollectCardsTreasure)
	}
	if state.GlobalCollectCardsTreasureMap[s.Id] == nil {
		state.GlobalCollectCardsTreasureMap[s.Id] = &pb3.PYYGlobalCollectCardsTreasure{}
	}
	sData := state.GlobalCollectCardsTreasureMap[s.Id]
	if nil == sData.LotteryData {
		sData.LotteryData = &pb3.LotteryData{}
	}
	s.lottery.InitData(sData.LotteryData)
	if nil == sData.GlobalCollectCardsTreasure {
		sData.GlobalCollectCardsTreasure = &pb3.GlobalCollectCardsTreasureData{}
	}
	return sData
}

func (s *GlobalCollectTreasureSys) ResetData() {
	state := s.GetYYData()
	if nil == state.GlobalCollectCardsTreasureMap {
		return
	}
	delete(state.GlobalCollectCardsTreasureMap, s.Id)
}

func (s *GlobalCollectTreasureSys) RawData() *pb3.LotteryData {
	data := s.getData()
	return data.LotteryData
}

func (s *GlobalCollectTreasureSys) GetSingleDiamondPrice() uint32 {
	conf, ok := jsondata.GetGlobalCollectTreasureConf(s.ConfName, s.ConfIdx)
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

func (s *GlobalCollectTreasureSys) AfterDraw(id uint32, conf *jsondata.LotteryLibConf, conf2 *jsondata.LotteryLibAwardPool, oneAward jsondata.StdRewardVec) {
	return
}

func (s *GlobalCollectTreasureSys) Login() {
	s.s2cInfo()
}

func (s *GlobalCollectTreasureSys) OnReconnect() {
	s.s2cInfo()
}

func (s *GlobalCollectTreasureSys) s2cInfo() {
	s.SendProto3(61, 80, &pb3.S2C_61_80{
		ActiveId: s.GetId(),
		Data:     s.getData().GetGlobalCollectCardsTreasure(),
	})
}

func (s *GlobalCollectTreasureSys) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_61_81
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf, ok := jsondata.GetGlobalCollectTreasureConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("GlobalCollectTreasureSys conf is nil")
	}

	consumes := conf.GetDrawConsume(req.GetTimes())
	if nil == consumes {
		return neterror.ConfNotFoundError("GlobalCollectTreasureSys consumes conf is nik")
	}
	success, remove := s.player.ConsumeByConfWithRet(consumes, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogGlobalCollectTreasureDoDraw})
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

	result := s.lottery.DoDraw(req.Times, useDiamondCount, conf.LibIds)
	engine.GiveRewards(s.GetPlayer(), result.Awards, common.EngineGiveRewardParam{
		LogId:  pb3.LogId_LogGlobalCollectTreasureDoDraw,
		NoTips: true,
	})

	sData := s.getData().GetGlobalCollectCardsTreasure()
	sData.TotalTimes += req.GetTimes()

	rsp := &pb3.S2C_61_81{
		ActiveId:   s.GetId(),
		Times:      req.GetTimes(),
		TotalTimes: sData.GetTotalTimes(),
	}
	for _, v := range result.LibResult {
		rsp.Result = append(rsp.Result, &pb3.PYYGlobalCollectCardsTreasureRet{
			TreasureId:  v.LibId,
			AwardPoolId: v.AwardPoolConf.Id,
		})
	}
	s.SendProto3(61, 81, rsp)
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYGlobalCollectTreasure, func() iface.IPlayerYY {
		return &GlobalCollectTreasureSys{}
	})
	net.RegisterYYSysProtoV2(61, 81, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*GlobalCollectTreasureSys).c2sDraw
	})
}
