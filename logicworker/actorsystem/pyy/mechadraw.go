/**
 * @Author: LvYuMeng
 * @Date: 2025/7/2
 * @Desc: 机甲抽奖
**/

package pyy

import (
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
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/lotterylibs"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/net"
)

type MechaDraw struct {
	PlayerYYBase
	lottery *lotterylibs.LotteryBase
}

func (s *MechaDraw) OnInit() {
	s.lottery = &lotterylibs.LotteryBase{
		Player:                s.GetPlayer(),
		GetLuckTimes:          s.GetLuckTimes,
		GetLuckyValEx:         s.GetLuckyValEx,
		RawData:               s.RawData,
		GetSingleDiamondPrice: s.GetSingleDiamondPrice,
		AfterDraw:             s.AfterDraw,
	}
}

func (s *MechaDraw) ResetData() {
	state := s.GetYYData()
	if nil == state.MechaDraw {
		return
	}
	delete(state.MechaDraw, s.Id)
}

func (s *MechaDraw) OnOpen() {
	sysData := s.getSysData()
	sysData.TabId = 1
	s.s2cInfo()
}

func (s *MechaDraw) Login() {
	s.s2cInfo()
}

func (s *MechaDraw) OnReconnect() {
	s.s2cInfo()
}

func (s *MechaDraw) NewDay() {
	s.lottery.OnLotteryNewDay()
	s.s2cInfo()
}

func (s *MechaDraw) s2cInfo() {
	data := s.getSysData()

	rsp := &pb3.S2C_75_119{
		ActiveId: s.Id,
		TabId:    data.TabId,
		LibHit:   data.LibHit,
	}

	s.SendProto3(75, 119, rsp)
}

func (s *MechaDraw) getEvolutionData() *pb3.PYY_EvolutionMechaDraw {
	state := s.GetYYData()
	if nil == state.MechaDraw {
		state.MechaDraw = make(map[uint32]*pb3.PYY_EvolutionMechaDraw)
	}
	if state.MechaDraw[s.Id] == nil {
		state.MechaDraw[s.Id] = &pb3.PYY_EvolutionMechaDraw{}
	}
	return state.MechaDraw[s.Id]
}

func (s *MechaDraw) getSysData() *pb3.PYY_MechaDraw {
	data := s.getEvolutionData()
	if nil == data.SysData {
		data.SysData = &pb3.PYY_MechaDraw{}
	}
	if nil == data.SysData.LibHit {
		data.SysData.LibHit = map[uint32]*pb3.MechaDrawLibHit{}
	}
	return data.SysData
}

func (s *MechaDraw) RawData() *pb3.LotteryData {
	data := s.getEvolutionData()
	if nil == data.LotteryData {
		data.LotteryData = &pb3.LotteryData{}
	}

	s.lottery.InitData(data.LotteryData)

	return data.LotteryData
}

func (s *MechaDraw) GetSingleDiamondPrice() uint32 {
	conf := s.GetConf()
	if nil == conf {
		return 0
	}

	data := s.getSysData()

	tabConf, ok := conf.TabLibs[data.TabId]
	if !ok {
		return 0
	}

	singlePrice := jsondata.GetAutoBuyItemPrice(tabConf.Consume[0].Id, moneydef.BindDiamonds)
	if singlePrice <= 0 {
		singlePrice = jsondata.GetAutoBuyItemPrice(tabConf.Consume[0].Id, moneydef.Diamonds)
	}

	return uint32(singlePrice)
}

func (s *MechaDraw) GetLuckTimes() uint16 {
	conf := s.GetConf()
	if nil == conf {
		return 0
	}
	return uint16(conf.LuckTimes)
}

func (s *MechaDraw) GetLuckyValEx() *jsondata.LotteryLuckyValEx {
	conf := s.GetConf()
	if nil == conf {
		return nil
	}
	return conf.LuckyValEx
}

func (s *MechaDraw) GetConf() *jsondata.MechaDrawConfig {
	return jsondata.GetMechaDrawConf(s.ConfName, s.ConfIdx)
}

func (s *MechaDraw) AfterDraw(libId uint32, libConf *jsondata.LotteryLibConf, awardPoolConf *jsondata.LotteryLibAwardPool, oneAward jsondata.StdRewardVec) {
}

func (s *MechaDraw) GetGuaranteeLibId() uint32 {
	conf := s.GetConf()
	if nil == conf {
		return 0
	}

	data := s.getSysData()
	tabConf := conf.TabLibs[data.TabId]
	if nil == tabConf {
		return 0
	}

	return tabConf.ChanceLibId
}

func (s *MechaDraw) IsDrawFull() bool {
	libId := s.GetGuaranteeLibId()
	if libId == 0 {
		return false
	}

	if s.lottery.CanLibUse(libId, false) {
		return false
	}

	return true
}

func (s *MechaDraw) pkgResult(v *lotterylibs.LibResult) *pb3.MechaDrawSt {
	st := &pb3.MechaDrawSt{
		TreasureId:  v.LibId,
		AwardPoolId: v.AwardPoolConf.Id,
	}

	if oneAwards := engine.FilterRewardByPlayer(s.GetPlayer(), v.OneAwards); len(oneAwards) > 0 {
		st.ItemId = oneAwards[0].Id
		st.Count = uint32(oneAwards[0].Count)
	}

	return st
}

func (s *MechaDraw) c2sDraw(msg *base.Message) error {
	var req pb3.C2S_75_120
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := s.GetConf()
	if nil == conf {
		return neterror.ConfNotFoundError("MechaDraw conf is nil")
	}

	if s.IsDrawFull() {
		return neterror.ParamsInvalidError("is over")
	}

	data := s.getSysData()
	tabConf := conf.TabLibs[data.TabId]
	if nil == tabConf {
		return neterror.ConfNotFoundError("tab %d conf is nil", data.TabId)
	}

	success, remove := s.player.ConsumeByConfWithRet(tabConf.Consume, req.AutoBuy, common.ConsumeParams{LogId: pb3.LogId_LogMechaDrawConsume})
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

	if len(tabConf.DrawScore) > 0 {
		engine.GiveRewards(s.GetPlayer(), tabConf.DrawScore, common.EngineGiveRewardParam{LogId: pb3.LogId_LogMechaDrawAwards})
	}

	result := s.lottery.DoDraw(1, useDiamondCount, tabConf.LibIds)
	engine.GiveRewards(s.GetPlayer(), result.Awards, common.EngineGiveRewardParam{
		LogId:        pb3.LogId_LogMechaDrawAwards,
		NoTips:       true,
		BroadcastExt: []interface{}{s.Id},
	})

	sysData := s.getSysData()

	rsp := &pb3.S2C_75_120{
		ActiveId: s.Id,
		TabId:    sysData.TabId,
	}

	isHit := len(result.LibResult) > 0 && result.LibResult[0].LibId == tabConf.ChanceLibId
	for _, v := range result.LibResult {
		rsp.Result = append(rsp.Result, s.pkgResult(v))
	}

	if isHit {
		var leftRewardsVec []jsondata.StdRewardVec
		for _, libId := range tabConf.LibIds {
			left := s.lottery.GetNotGetAwards(libId)
			if left.Awards != nil {
				leftRewardsVec = append(leftRewardsVec, left.Awards)
			}
		}

		rewards := jsondata.AppendStdReward(leftRewardsVec...)
		if len(rewards) > 0 {
			engine.GiveRewards(s.GetPlayer(), rewards, common.EngineGiveRewardParam{
				LogId:  pb3.LogId_LogMechaDrawAwards,
				NoTips: true,
			})
		}
	}

	s.SendProto3(75, 120, rsp)

	s.GetPlayer().TriggerEvent(custom_id.AeActDrawTimes, &custom_id.ActDrawEvent{
		ActType:    drawdef.ActMechaDraw,
		ActId:      s.Id,
		Times:      1,
		ReachScore: tabConf.ReachScore,
	})

	for _, v := range rsp.Result {
		libHit, exist := sysData.LibHit[v.TreasureId]
		if !exist {
			libHit = &pb3.MechaDrawLibHit{}
			sysData.LibHit[v.TreasureId] = libHit
		}
		if nil == libHit.AwardPoolId {
			libHit.AwardPoolId = map[uint32]uint32{}
		}
		libHit.AwardPoolId[v.AwardPoolId]++
	}

	if isHit {
		if _, ok := conf.TabLibs[sysData.TabId+1]; ok {
			sysData.TabId++
			sysData.LibHit = nil
			s.getEvolutionData().LotteryData = nil
		}
	}

	s.s2cInfo()

	return nil
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYMecha, func() iface.IPlayerYY {
		return &MechaDraw{}
	})

	net.RegisterYYSysProtoV2(75, 120, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*MechaDraw).c2sDraw
	})

	engine.RegRewardsBroadcastHandler(tipmsgid.MechaDrawTips, engine.CommonYYDrawBroadcast)
}
