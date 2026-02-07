/**
 * @Author: LvYuMeng
 * @Date: 2024/7/1
 * @Desc:
**/

package pyy

import (
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/drawdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/lotterylibs"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
)

type FairyGachaSys struct {
	PlayerYYBase
	lottery lotterylibs.LotteryBase
}

func (s *FairyGachaSys) OnInit() {
	s.lottery = lotterylibs.LotteryBase{
		Player:                s.GetPlayer(),
		GetLuckTimes:          s.GetLuckTimes,
		GetLuckyValEx:         s.GetLuckyValEx,
		RawData:               s.RawData,
		GetSingleDiamondPrice: s.GetSingleDiamondPrice,
		AfterDraw:             s.AfterDraw,
	}
}

func (s *FairyGachaSys) ResetData() {
	state := s.GetYYData()
	if nil == state.EvolutionFairyGacha {
		return
	}
	delete(state.EvolutionFairyGacha, s.Id)
}

func (s *FairyGachaSys) GetLuckTimes() uint16 {
	conf := jsondata.GetYYFairyGachaConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return 0
	}
	return conf.LuckTimes
}

func (s *FairyGachaSys) GetLuckyValEx() *jsondata.LotteryLuckyValEx {
	conf := jsondata.GetYYFairyGachaConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return nil
	}
	return conf.LuckyValEx
}

func (s *FairyGachaSys) RawData() *pb3.LotteryData {
	data := s.GetData()
	return data.LotteryData
}

func (s *FairyGachaSys) GetSingleDiamondPrice() uint32 {
	conf := jsondata.GetYYFairyGachaConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return 0
	}
	gachaId := s.GetGachaId()
	if gachaId == 0 {
		return 0
	}

	libConf := conf.Libs[gachaId]
	consumeConf := libConf.GetGachaConsume(1)
	itemId := consumeConf[0].Id
	singlePrice := jsondata.GetAutoBuyItemPrice(itemId, moneydef.BindDiamonds)
	if singlePrice <= 0 {
		singlePrice = jsondata.GetAutoBuyItemPrice(itemId, moneydef.Diamonds)
	}
	singlePrice *= int64(consumeConf[0].Count)

	return uint32(singlePrice)
}

const (
	fairyGachaRecordLimit = 100
)

func (s *FairyGachaSys) AfterDraw(libId uint32, libConf *jsondata.LotteryLibConf, awardPoolConf *jsondata.LotteryLibAwardPool, oneAward jsondata.StdRewardVec) {
	rewards := engine.FilterRewardByPlayer(s.GetPlayer(), awardPoolConf.Awards)
	if len(rewards) <= 0 {
		return
	}

	record := &pb3.FairyGachaRecord{
		TreasureId:  libId,
		AwardPoolId: awardPoolConf.Id,
		ItemId:      rewards[0].Id,
		Count:       uint32(rewards[0].Count),
		TimeStamp:   time_util.NowSec(),
	}

	data := s.GetData().GetFairyGacha()
	data.Record = append(data.Record, record)

	if len(data.Record) > fairyGachaRecordLimit {
		data.Record = data.Record[1:]
	}

	s.SendProto3(27, 153, &pb3.S2C_27_153{
		ActiveId: s.GetId(),
		Records:  []*pb3.FairyGachaRecord{record},
	})

	conf := jsondata.GetYYFairyGachaConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}
	if itemConf := jsondata.GetItemConfig(rewards[0].Id); nil != itemConf && itemConf.Quality >= conf.BroadcastQuality && itemdef.IsFairy(itemConf.Type) {
		engine.BroadcastTipMsgById(tipmsgid.HuanLingDrawGiftTip, s.GetPlayer().GetId(), s.GetPlayer().GetName(), engine.StdRewardToBroadcast(s.GetPlayer(), rewards))
	}
}

func (s *FairyGachaSys) GetData() *pb3.PYY_EvolutionFairyGacha {
	state := s.GetYYData()
	if nil == state.EvolutionFairyGacha {
		state.EvolutionFairyGacha = make(map[uint32]*pb3.PYY_EvolutionFairyGacha)
	}
	if state.EvolutionFairyGacha[s.Id] == nil {
		state.EvolutionFairyGacha[s.Id] = &pb3.PYY_EvolutionFairyGacha{}
	}
	sData := state.EvolutionFairyGacha[s.Id]
	if nil == sData.LotteryData {
		sData.LotteryData = &pb3.LotteryData{}
	}
	s.lottery.InitData(sData.LotteryData)
	if nil == sData.FairyGacha {
		sData.FairyGacha = &pb3.PYY_FairyGacha{}
	}
	return sData
}

func (s *FairyGachaSys) GetGachaId() uint32 {
	conf := jsondata.GetYYFairyGachaConf(s.ConfName, s.ConfIdx)
	if len(conf.GachaIds) <= 0 {
		return 0
	}

	var gachaId uint32
	if len(conf.GachaIds) > 1 {
		gachaId = s.GetData().GetFairyGacha().GetLibId()
	} else {
		gachaId = conf.GachaIds[0]
	}

	return gachaId
}

func (s *FairyGachaSys) Login() {
	s.s2cInfo()
}

func (s *FairyGachaSys) OnReconnect() {
	s.s2cInfo()
}

func (s *FairyGachaSys) s2cInfo() {
	s.SendProto3(27, 150, &pb3.S2C_27_150{
		ActiveId:  s.GetId(),
		Data:      s.GetData().GetFairyGacha(),
		Guarantee: s.lottery.GetGuaranteeCount(s.GetGuaranteeLibId()),
	})
}

func (s *FairyGachaSys) GetGuaranteeLibId() uint32 {
	conf := jsondata.GetYYFairyGachaConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return 0
	}

	gachaId := s.GetGachaId()
	if gachaId == 0 {
		return 0
	}

	libConf := conf.Libs[gachaId]
	if nil == libConf {
		return 0
	}

	return libConf.SuperLibId
}

func (s *FairyGachaSys) NewDay() {
	s.lottery.OnLotteryNewDay()
}

func (s *FairyGachaSys) dropActItem(times uint32) {
	conf := jsondata.GetYYFairyGachaConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}

	var rewardVecs []jsondata.StdRewardVec
	for i := times; i > 0; i-- {
		for _, v := range conf.ActDrop {
			yy := pyymgr.GetPlayerYYObj(s.GetPlayer(), v.ActId)
			if nil == yy || !yy.IsOpen() {
				continue
			}
			if !random.Hit(v.Rate, 10000) {
				continue
			}
			rewardVecs = append(rewardVecs, v.Rewards)
		}
	}

	rewards := jsondata.MergeStdReward(rewardVecs...)
	if len(rewards) > 0 {
		engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogFairyGachaActDrop,
		})
	}
}

func (s *FairyGachaSys) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_27_151
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYFairyGachaConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("FairyGachaSys conf is nil")
	}

	gachaId := s.GetGachaId()
	if gachaId == 0 {
		return neterror.ParamsInvalidError("FairyGachaSys gachaId is zero")
	}

	libConf := conf.Libs[gachaId]
	if nil == libConf {
		return neterror.ConfNotFoundError("FairyGachaSys libs conf is zero")
	}

	consumes := libConf.GetGachaConsume(req.GetTimes())
	if nil == consumes {
		return neterror.ConfNotFoundError("FairyGachaSys consumes conf is nik")
	}
	success, remove := s.player.ConsumeByConfWithRet(consumes, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogFairyGacha})
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
		engine.GiveRewards(s.GetPlayer(), bundledAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogFairyGacha})
	}

	result := s.lottery.DoDraw(req.Times, useDiamondCount, libConf.LibIds)
	if len(result.Awards) > 0 {
		engine.GiveRewards(s.GetPlayer(), result.Awards, common.EngineGiveRewardParam{
			LogId:  pb3.LogId_LogFairyGacha,
			NoTips: true,
		})
	}

	data := s.GetData().GetFairyGacha()
	data.TotalTimes += req.GetTimes()

	rsp := &pb3.S2C_27_151{
		ActiveId:   s.GetId(),
		Times:      req.GetTimes(),
		TotalTimes: data.GetTotalTimes(),
		Guarantee:  s.lottery.GetGuaranteeCount(s.GetGuaranteeLibId()),
	}
	for _, v := range result.LibResult {
		rsp.Result = append(rsp.Result, &pb3.FairyGachaSt{
			TreasureId:  v.LibId,
			AwardPoolId: v.AwardPoolConf.Id,
		})
	}

	s.SendProto3(27, 151, rsp)
	s.dropActItem(req.Times)
	s.GetPlayer().TriggerQuestEvent(custom_id.QttFairyGachaDrawTime, 0, int64(req.Times))
	s.GetPlayer().TriggerQuestEvent(custom_id.QttAchievementsFairyGachaDrawTime, 0, int64(req.Times))
	s.GetPlayer().TriggerEvent(custom_id.AeActDrawTimes, &custom_id.ActDrawEvent{
		ActType: drawdef.ActDrawTypeFairy,
		ActId:   s.Id,
		Times:   req.Times,
	})

	return nil
}

func (s *FairyGachaSys) c2sAnchor(msg *base.Message) error {
	var req pb3.C2S_27_152
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYFairyGachaConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("FairyGachaSys conf is nil")
	}

	if len(conf.GachaIds) <= 1 {
		return neterror.ParamsInvalidError("FairyGachaSys need not anchor")
	}

	if nil == conf.Libs[req.GetId()] {
		return neterror.ConfNotFoundError("FairyGachaSys lib %d conf is nil", req.GetId())
	}

	data := s.GetData().GetFairyGacha()
	data.LibId = req.GetId()

	s.SendProto3(27, 152, &pb3.S2C_27_152{
		ActiveId: s.GetId(),
		Id:       req.GetId(),
	})
	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYFairyGacha, func() iface.IPlayerYY {
		return &FairyGachaSys{}
	})

	net.RegisterYYSysProtoV2(27, 151, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*FairyGachaSys).c2sDraw
	})
	net.RegisterYYSysProtoV2(27, 152, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*FairyGachaSys).c2sAnchor
	})

	gmevent.Register("gacha", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		sys, ok := player.GetSysObj(sysdef.SiPlayerYY).(*pyymgr.YYMgr)
		if !ok {
			return false
		}

		objs := sys.GetAllObj(yydefine.YYFairyGacha)
		if len(objs) == 0 {
			return false
		}
		if objs[0].GetClass() != yydefine.YYFairyGacha {
			return false
		}

		obj := objs[0]
		msg := base.NewMessage()
		msg.SetCmd(27<<8 | 151)
		err := msg.PackPb3Msg(&pb3.C2S_27_151{
			Base: &pb3.YYBase{
				ActiveId: obj.GetId(),
			},
			Times: utils.AtoUint32(args[0]),
		})
		if err != nil {
			return false
		}

		s := obj.(*FairyGachaSys)
		err = s.c2sDraw(msg)
		if err != nil {
			s.LogError("gm c2sDraw err:%v", err)
			return false
		}
		return true
	}, 1)
}
