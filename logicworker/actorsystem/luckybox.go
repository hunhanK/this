/**
 * @Author: zjj
 * @Date: 2023年10月30日
 * @Desc: 幸运箱
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils/pie"
)

type LuckyBoxSys struct {
	Base
	RecFreeAwards bool
	DrawRewardMap map[uint32]bool
}

func (s *LuckyBoxSys) GetData() *pb3.LuckyBoxState {
	binary := s.GetBinaryData()
	if nil == binary.LuckyBoxState {
		binary.LuckyBoxState = &pb3.LuckyBoxState{}
	}
	if s.DrawRewardMap == nil {
		s.DrawRewardMap = make(map[uint32]bool)
	}
	return binary.LuckyBoxState
}

func (s *LuckyBoxSys) reset() {
	binary := s.GetBinaryData()
	binary.LuckyBoxState = &pb3.LuckyBoxState{}
	binary.LuckyBoxState.WaitRecAwardsIdx = -1
	binary.LuckyBoxState.AwardList = nil
	s.RecFreeAwards = false
	s.DrawRewardMap = make(map[uint32]bool)
}

func (s *LuckyBoxSys) S2CInfo() {
	s.SendProto3(146, 1, &pb3.S2C_146_1{
		State: s.GetData(),
	})
}

func (s *LuckyBoxSys) getOpenTimeConfIdx(itemId uint32) int {
	var ret = -1
	conf, ok := jsondata.GetLuckyBoxConf(itemId)
	if !ok {
		s.GetOwner().LogWarn("not found benefit pray conf ")
		return ret
	}

	// 看下当前开服时间符合哪个区间
	openServerDay := gshare.GetOpenServerDay()
	for idx := range conf.TimeConf {
		if int64(openServerDay) < conf.TimeConf[idx].StartTime ||
			int64(openServerDay) > conf.TimeConf[idx].EndTime {
			continue
		}
		ret = int(uint32(idx))
		break
	}

	return ret
}

func (s *LuckyBoxSys) OnReconnect() {
	defer s.reset()

	// 是否有等待领取的
	if s.GetData().WaitRecAwardsIdx < 0 {
		return
	}

	var award *jsondata.LuckyBoxReward
	awards, isFreeGrid := s.getAwards(s.GetData().CurItemId, s.GetData().CurTurn+1)

	// 只处理付费奖励
	if isFreeGrid {
		return
	}

	if len(awards) == 0 {
		return
	}

	if uint32(len(awards)) < uint32(s.GetData().WaitRecAwardsIdx) {
		return
	}

	// 已经领过付费奖励了
	if pie.Uint32s(s.GetData().RecAwardsIdxs).Contains(uint32(s.GetData().WaitRecAwardsIdx)) {
		return
	}

	award = awards[uint32(s.GetData().WaitRecAwardsIdx)] // 领付费奖励
	mailmgr.SendMailToActor(s.GetOwner().GetId(), &mailargs.SendMailSt{
		ConfId:  common.Mail_LuckyBox,
		Rewards: []*jsondata.StdReward{award.StdReward},
	})
}

func (s *LuckyBoxSys) OnLogout() {
	s.reset()
}

func (s *LuckyBoxSys) GetTimeConfIdx(itemId uint32) uint32 {
	boxConf, ok := jsondata.GetLuckyBoxConf(itemId)
	if !ok {
		s.GetOwner().LogWarn("not found benefit pray conf ")
		return 0
	}
	var timeConfIdx uint32
	openServerDay := gshare.GetOpenServerDay()
	for i := range boxConf.TimeConf {
		if int64(openServerDay) < boxConf.TimeConf[i].StartTime ||
			int64(openServerDay) > boxConf.TimeConf[i].EndTime {
			continue
		}
		timeConfIdx = uint32(i)
		break
	}
	return timeConfIdx
}

func (s *LuckyBoxSys) c2sAwardUnSkipTurntable(msg *base.Message) error {
	var req pb3.C2S_146_3
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	data := s.GetData()

	// 道具使用不一致
	if data.CurItemId != req.ItemId {
		s.GetOwner().LogInfo("use lucky box unequal,data cur item id %d", data.CurItemId)
		return nil
	}

	// 首轮是免费奖励
	if req.Turn == 1 {
		s.SendProto3(146, 2, &pb3.S2C_146_2{
			RecAwardsIdx: data.RecFreeAwardIdx,
			IsAdvance:    false,
			IsFreeGrid:   true,
		})
		return nil
	}

	// 是否有等待领取的
	if data.WaitRecAwardsIdx > 0 {
		s.GetOwner().LogWarn("waiting rec awards , waiting idx is %d", data.WaitRecAwardsIdx)
		return nil
	}

	// 付费奖励
	idx, _, ok := s.drawReward(req.ItemId, req.Turn)
	if !ok {
		s.GetOwner().LogWarn("not found reward")
		return nil
	}

	awards, _ := s.getAwards(req.ItemId, req.Turn)
	if awards == nil {
		s.GetOwner().LogWarn("not found award")
		return nil
	}

	data.WaitRecAwardsIdx = int32(idx)

	s.SendProto3(146, 2, &pb3.S2C_146_2{
		RecAwardsIdx:  idx,
		IsAdvance:     awards[idx].IsAdvance,
		IsFreeGrid:    false,
		ShowTurntable: true,
	})
	return nil
}

func (s *LuckyBoxSys) c2sClose(msg *base.Message) error {
	var req pb3.C2S_146_2
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	s.GetOwner().LogInfo("close item is %d", req.ItemId)
	s.reset()
	s.SendProto3(146, 3, &pb3.S2C_146_3{
		ItemId:      req.ItemId,
		ClientParam: req.ClientParam,
	})
	return nil
}

// 获取奖励
func (s *LuckyBoxSys) getAwards(itemId uint32, turn uint32) (jsondata.StdLuckyBoxRewardVec, bool) {
	boxConf, ok := jsondata.GetLuckyBoxConf(itemId)
	if !ok {
		s.LogWarn("not found benefit pray conf ")
		return nil, false
	}
	if turn == 0 {
		s.LogWarn("turn is zero")
		return nil, false
	}

	// 看下当前开服时间符合哪个区间
	data := s.GetData()
	curTimeConfIdx := data.CurTimeConfIdx
	if uint32(len(boxConf.TimeConf)) <= curTimeConfIdx {
		s.LogWarn("boxConf:%d,idx:%d", len(boxConf.TimeConf), curTimeConfIdx)
		return nil, false
	}
	var timeConf = boxConf.TimeConf[curTimeConfIdx]

	if uint32(len(timeConf.Times)) < turn {
		s.LogWarn("boxConf Times:%d,turn:%d", len(timeConf.Times), turn)
		return nil, false
	}

	// 免费格子
	times := timeConf.Times[turn-1]
	if times == nil {
		s.GetOwner().LogWarn("not found times conf , req turn is %d", turn)
		return nil, false
	}

	// 是否免费格子
	var isFreeGrid = len(times.Consume) == 0
	var rewards jsondata.StdLuckyBoxRewardVec
	if isFreeGrid {
		rewards = append(rewards, &jsondata.LuckyBoxReward{
			StdReward: jsondata.Pb3RewardToStdReward(data.FreeAward.Award),
			IsAdvance: data.FreeAward.IsAdvance,
		})
	} else {
		rewards = jsondata.Pb3LuckyBoxRewardVecToStdLuckyBoxRewardVec(data.AwardList)
	}

	return rewards, isFreeGrid
}

// 随机奖励初始化
func (s *LuckyBoxSys) randomConfigAwards(itemId uint32) {
	boxConf, ok := jsondata.GetLuckyBoxConf(itemId)
	if !ok {
		s.LogWarn("not found benefit pray conf ")
		return
	}

	data := s.GetData()
	curTimeConfIdx := data.CurTimeConfIdx
	if uint32(len(boxConf.TimeConf)) <= curTimeConfIdx {
		s.LogWarn("boxConf:%d,idx:%d", len(boxConf.TimeConf), curTimeConfIdx)
		return
	}

	var timeConf = boxConf.TimeConf[curTimeConfIdx]
	if len(timeConf.FreeRewards) == 0 || len(timeConf.RewardPos) == 0 {
		s.LogWarn("not found rewards")
		return
	}

	// 免费奖励
	{
		pool := new(random.Pool)
		for idx, reward := range timeConf.FreeRewards {
			pool.AddItem(idx, reward.Weight)
		}
		idx := pool.RandomOne().(int)
		s.GetData().RecFreeAwardIdx = uint32(idx)
		vec := jsondata.StdLuckyBoxRewardVecToPb3LuckyBoxRewardVec(jsondata.StdLuckyBoxRewardVec{timeConf.FreeRewards[idx]})
		s.GetData().FreeAward = vec[0]
	}

	// 付费奖励
	{
		for _, reward := range timeConf.RewardPos {
			pool := new(random.Pool)
			for _, boxReward := range reward.RewardPool {
				pool.AddItem(boxReward, boxReward.Weight)
			}
			one := pool.RandomOne().(*jsondata.LuckyBoxReward)
			pb3Reward := jsondata.StdRewardToPb3Reward(one.StdReward)
			pb3Reward.Weight = uint32(reward.Weight)
			s.GetData().AwardList = append(s.GetData().AwardList, &pb3.LuckyBoxAward{
				Award:     pb3Reward,
				IsAdvance: one.IsAdvance,
			})
		}
	}

}

// 转盘抽奖
func (s *LuckyBoxSys) drawReward(itemId uint32, turn uint32) (uint32, bool, bool) {
	boxConf, ok := jsondata.GetLuckyBoxConf(itemId)
	if !ok {
		s.GetOwner().LogWarn("not found benefit pray conf ")
		return 0, false, false
	}

	// 抽奖
	if s.DrawRewardMap[turn] {
		s.GetOwner().LogWarn("already drawReward , turn is %d", turn)
		return 0, false, false
	}

	data := s.GetData()
	curTimeConfIdx := data.CurTimeConfIdx
	if uint32(len(boxConf.TimeConf)) <= curTimeConfIdx {
		s.LogWarn("boxConf:%d,idx:%d", len(boxConf.TimeConf), curTimeConfIdx)
		return 0, false, false
	}
	var timeConf = boxConf.TimeConf[curTimeConfIdx]

	// 轮次
	if 1 > turn || turn > uint32(len(timeConf.Times)) {
		s.GetOwner().LogWarn("req turn is %d , max box conf time is %d", turn, len(timeConf.Times))
		return 0, false, false
	}

	// 轮次
	if data.CurTurn+1 > turn {
		s.GetOwner().LogWarn("req turn is %d , req current round is incorrect %d", data.CurTurn, turn)
		return 0, false, false
	}

	// 免费格子
	times := timeConf.Times[turn-1]
	if times == nil {
		s.GetOwner().LogWarn("not found times conf , req turn is %d", turn)
		return 0, false, false
	}

	// 是否免费格子
	var isFreeGrid = len(times.Consume) == 0

	if len(times.Consume) > 0 {
		if !s.GetOwner().ConsumeByConf(times.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogLuckyBox}) {
			s.GetOwner().LogWarn("consume failed")
			s.owner.SendTipMsg(tipmsgid.TpUseItemFailed)
			return 0, false, false
		}
	}

	var rewards = jsondata.Pb3LuckyBoxRewardVecToStdLuckyBoxRewardVec(data.AwardList)
	if isFreeGrid {
		rewards = jsondata.Pb3LuckyBoxRewardVecToStdLuckyBoxRewardVec([]*pb3.LuckyBoxAward{data.FreeAward})
	}

	// 权重池处理一下
	pool := new(random.Pool)
	for idx := range rewards {
		// 非免费的需要跳过已经抽到的
		if !isFreeGrid && pie.Uint32s(data.RecAwardsIdxs).Contains(uint32(idx)) {
			s.GetOwner().LogInfo("skip,idx is %d ...", idx)
			continue
		}
		pool.AddItem(uint32(idx), rewards[idx].Weight)
	}
	idx := pool.RandomOne().(uint32)
	s.DrawRewardMap[turn] = true
	return idx, isFreeGrid, true
}

func (s *LuckyBoxSys) c2sAward(msg *base.Message) error {
	var req pb3.C2S_146_1
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}
	data := s.GetData()
	// 道具使用不一致
	if data.CurItemId != req.ItemId {
		s.GetOwner().LogInfo("use lucky box unequal")
		return nil
	}

	var idx = req.RecAwardsIdx
	var award *jsondata.LuckyBoxReward
	var isFreeGrid bool

	if req.Turn == 0 || req.Turn != data.CurTurn+1 {
		return neterror.ParamsInvalidError("turn is err")
	}

	// 只领奖励
	if req.OnlyGiveAwards || req.Turn == 1 {
		var awards jsondata.StdLuckyBoxRewardVec
		awards, isFreeGrid = s.getAwards(req.ItemId, req.Turn)
		if !isFreeGrid {
			if pie.Uint32s(data.RecAwardsIdxs).Contains(req.RecAwardsIdx) {
				// 已经领过付费奖励了
				s.GetOwner().LogWarn("already rec free award")
				return nil
			}
			if uint32(len(awards)) < req.RecAwardsIdx {
				s.GetOwner().LogWarn("award has err , req RecAwardsIdx is %d", req.RecAwardsIdx)
				return nil
			}
			award = awards[req.RecAwardsIdx] // 领付费奖励
		} else {
			if s.RecFreeAwards {
				s.GetOwner().LogWarn("already rec free awards")
				return nil
			}
			if len(awards) > 0 {
				award = awards[0]
			}
		}
	} else {
		// 是否有等待领取的
		if data.WaitRecAwardsIdx > 0 {
			s.GetOwner().LogWarn("waiting rec awards , waiting idx is %d", data.WaitRecAwardsIdx)
			return nil
		}

		// 不是只领奖励 需要抽下奖
		var ok bool
		idx, isFreeGrid, ok = s.drawReward(req.ItemId, req.Turn)
		if !ok {
			s.GetOwner().LogWarn("not found reward")
			return nil
		}
		awards, _ := s.getAwards(req.ItemId, req.Turn)
		if awards == nil {
			s.GetOwner().LogWarn("not found award")
			return nil
		}
		award = awards[idx] // 领付费奖励
	}

	// 下发奖励
	if award != nil {
		engine.GiveRewards(s.GetOwner(), []*jsondata.StdReward{award.StdReward}, common.EngineGiveRewardParam{LogId: pb3.LogId_LogLuckyBox})
		// 广播
		if !isFreeGrid {
			itemConf := jsondata.GetItemConfig(award.Id)
			boxItemConf := jsondata.GetItemConfig(req.ItemId)
			if itemConf != nil && boxItemConf != nil {
				if itemConf.Stage == 0 {
					engine.BroadcastTipMsgById(tipmsgid.XingYunXiang, s.GetOwner().GetName(), boxItemConf.Name, itemConf.Id)
				} else {
					var msgId = tipmsgid.XingYunXiang2
					if itemConf.Stage >= 3 {
						msgId = tipmsgid.XingYunXiang3
					}
					engine.BroadcastTipMsgById(uint32(msgId), s.GetOwner().GetName(), boxItemConf.Name, itemConf.Stage, itemConf.Id)
				}
			}
		}
	}

	data.CurTurn = req.Turn
	if isFreeGrid {
		data.RecFreeAwardIdx = idx
	} else {
		data.RecAwardsIdxs = append(data.RecAwardsIdxs, idx)
	}

	data.WaitRecAwardsIdx = -1
	s.RecFreeAwards = true

	s.SendProto3(146, 2, &pb3.S2C_146_2{
		RecAwardsIdx:   idx,
		IsFreeGrid:     isFreeGrid,
		OnlyGiveAwards: true,
	})
	return nil
}

func useLuckyBox(player iface.IPlayer, param *miscitem.UseItemParamSt, _ *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	player.LogInfo("useItem param :%v,user id is %d", param, player.GetId())
	luckyBoxSys := player.GetSysObj(sysdef.SiLuckyBox).(*LuckyBoxSys)
	luckyBoxSys.reset()

	// 看下当前开服时间符合哪个区间
	var timeConfIdx = luckyBoxSys.getOpenTimeConfIdx(param.ItemId)
	if timeConfIdx < 0 {
		player.LogWarn("not found time conf , open server day is %d", gshare.GetOpenServerDay())
		return
	}

	data := luckyBoxSys.GetData()
	data.CurItemId = param.ItemId
	data.CurTimeConfIdx = luckyBoxSys.GetTimeConfIdx(param.ItemId)

	// 初始化奖励
	luckyBoxSys.randomConfigAwards(param.ItemId)
	data.WaitRecAwardsIdx = int32(data.RecFreeAwardIdx)
	data.StartIng = true

	luckyBoxSys.S2CInfo()
	// 一次只能使用一个
	return true, true, 1
}

func init() {
	RegisterSysClass(sysdef.SiLuckyBox, func() iface.ISystem {
		return &LuckyBoxSys{}
	})

	miscitem.RegCommonUseItemHandle(itemdef.UseItemLuckyBox, useLuckyBox)

	net.RegisterSysProtoV2(146, 1, sysdef.SiLuckyBox, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*LuckyBoxSys).c2sAward
	})
	net.RegisterSysProtoV2(146, 2, sysdef.SiLuckyBox, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*LuckyBoxSys).c2sClose
	})
	net.RegisterSysProtoV2(146, 3, sysdef.SiLuckyBox, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*LuckyBoxSys).c2sAwardUnSkipTurntable
	})
}
