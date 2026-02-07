/**
 * @Author:
 * @Date: 2024/6/19
 * @Desc: 全服运营活动-兽潮来袭
**/

package yy

import (
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"
)

const (
	DemonSubduingBTEffectTypeBossRefresh = 1 // 指定子类型boss刷新
	DemonSubduingBTEffectTypeItemAwards  = 2 // 全服奖励
)

const (
	DemonSubduingEffectStatus1 = 1 // 正生效
	DemonSubduingEffectStatus2 = 2 // 已生效
)

type DemonSubduing struct {
	YYBase
}

func (s *DemonSubduing) OnInit() {
	s.sendAllPlayers()
}

func (s *DemonSubduing) NewDay() {
	data := s.GetData()
	data.Value = 0
	data.Effects = make(map[uint32]*pb3.YYDemonSubduingEffect)
	s.sendAllPlayers()
}

func (s *DemonSubduing) OnEnd() {
	// 发送玩家未领取的个人奖励
	conf := jsondata.GetYYDemonSubduingConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}
	data := s.GetData()

	getUnFetchRewards := func(playerId uint64, value uint32) jsondata.StdRewardVec {
		var rewards jsondata.StdRewardVec
		for id, rConf := range conf.PDonate {
			rewardsBit := data.PlayerRewards[playerId]
			if utils.IsSetBit(rewardsBit, id) {
				continue
			}
			if value >= rConf.Count {
				rewards = append(rewards, rConf.Rewards...)
			}
		}
		return rewards
	}

	for k, v := range data.PlayerMap {
		rewards := getUnFetchRewards(k, v)
		if len(rewards) > 0 {
			mailmgr.SendMailToActor(k, &mailargs.SendMailSt{
				ConfId:  conf.MailId,
				Rewards: rewards,
			})
		}
	}
}

func (s *DemonSubduing) OnOpen() {
	s.sendAllPlayers()
}

func (s *DemonSubduing) PlayerLogin(player iface.IPlayer) {
	s.sendPlayerData(player)
}

func (s *DemonSubduing) PlayerReconnect(player iface.IPlayer) {
	s.sendPlayerData(player)
}

func (s *DemonSubduing) ResetData() {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}
	if globalVar.YyDatas.DemonSubduingData == nil {
		return
	}
	delete(globalVar.YyDatas.DemonSubduingData, s.Id)
}

func (s *DemonSubduing) GetData() *pb3.YYDemonSubduing {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}
	if globalVar.YyDatas.DemonSubduingData == nil {
		globalVar.YyDatas.DemonSubduingData = make(map[uint32]*pb3.YYDemonSubduing)
	}
	if globalVar.YyDatas.DemonSubduingData[s.Id] == nil {
		globalVar.YyDatas.DemonSubduingData[s.Id] = &pb3.YYDemonSubduing{}
	}

	bTData := globalVar.YyDatas.DemonSubduingData[s.Id]
	if bTData.Effects == nil {
		bTData.Effects = make(map[uint32]*pb3.YYDemonSubduingEffect)
	}
	if bTData.PlayerMap == nil {
		bTData.PlayerMap = make(map[uint64]uint32)
	}
	if bTData.PlayerRewards == nil {
		bTData.PlayerRewards = make(map[uint64]uint32)
	}

	return bTData
}

func (s *DemonSubduing) AddDonate(player iface.IPlayer, num uint32) {
	data := s.GetData()
	preVal := data.Value

	data.Value += num
	data.PlayerMap[player.GetId()] += num

	s.CheckUpServerValue(preVal, data.Value)
	manager.UpdatePlayScoreRank(ranktype.PlayScoreRankTypeDemonSubduing, player, int64(data.PlayerMap[player.GetId()]), false, 0)
}

func (s *DemonSubduing) AutoDonateDemonSubduing() {
	conf := jsondata.GetYYDemonSubduingConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}
	hour := time_util.Now().Hour()
	hConf, ok := conf.DonateGrow[uint32(hour)]
	if !ok || len(hConf.DonateValues) < 2 {
		return
	}
	min := hConf.DonateValues[0]
	max := hConf.DonateValues[1]
	count := random.IntervalUU(min, max)

	data := s.GetData()
	preVal := data.Value
	data.Value += count

	s.CheckUpServerValue(preVal, data.Value)
}

func (s *DemonSubduing) CheckUpServerValue(preVal, newVal uint32) {
	conf := jsondata.GetYYDemonSubduingConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}
	for _, v := range conf.SDonate {
		// 保证执行一次
		if preVal < v.Count && newVal >= v.Count {
			s.DoSeverEffect(v)
		}
	}
}

func (s *DemonSubduing) DoSeverEffect(conf *jsondata.YYDemonSubduingDonate) {
	rConf := conf.ServerReward
	if rConf == nil {
		return
	}

	// 之前的同类型buff直接置为已生效
	effData := s.getDemonSubduingByEffType(rConf.RType)
	if effData != nil {
		effData.Status = DemonSubduingEffectStatus2
	}

	data := s.GetData()
	if _, ok := data.Effects[conf.Id]; !ok {
		data.Effects[conf.Id] = &pb3.YYDemonSubduingEffect{Id: conf.Id, EffectType: rConf.RType}
	}
	effect, _ := data.Effects[conf.Id]

	switch rConf.RType {
	case DemonSubduingBTEffectTypeBossRefresh:
		if effect.Status == DemonSubduingEffectStatus2 {
			return
		}
		effect.Status = DemonSubduingEffectStatus2
		s.callFightSrv(rConf.Params)
	case DemonSubduingBTEffectTypeItemAwards:
		if effect.Status == DemonSubduingEffectStatus2 {
			return
		}
		effect.Status = DemonSubduingEffectStatus2
		mailmgr.AddSrvMailStr(rConf.MailId, "", rConf.Rewards)
	}
	if rConf.TipId > 0 {
		engine.BroadcastTipMsgById(rConf.TipId, rConf.Name)
	}
	// 全服推送
	s.sendAllPlayers()
}

func (s *DemonSubduing) getDemonSubduingByEffType(effType uint32) *pb3.YYDemonSubduingEffect {
	data := s.GetData()
	now := time_util.NowSec()
	for _, effect := range data.Effects {
		if effect.EffectType != effType {
			continue
		}
		if effect.Status != DemonSubduingEffectStatus1 {
			continue
		}
		if now >= effect.StartTime && now < effect.EndTime {
			return effect
		}
	}
	return nil
}

func (s *DemonSubduing) sendPlayerData(player iface.IPlayer) {
	if player == nil {
		return
	}
	player.SendProto3(70, 60, &pb3.S2C_70_60{
		ActiveId: s.GetId(),
		Data:     s.packPlayerData(player),
	})
}

func (s *DemonSubduing) packPlayerData(player iface.IPlayer) *pb3.YYDemonSubduingPlayerInfo {
	data := s.GetData()
	return &pb3.YYDemonSubduingPlayerInfo{
		TotalVal:      data.GetValue(),
		PerVal:        data.PlayerMap[player.GetId()],
		PerGetRewards: data.PlayerRewards[player.GetId()],
		Effects:       data.Effects,
	}
}

func (s *DemonSubduing) sendAllPlayers() {
	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		s.sendPlayerData(player)
	})
}

func (s *DemonSubduing) callFightSrv(subTypeList []uint32) {
	if len(subTypeList) == 0 {
		return
	}
	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FRelieveSceneMonstersBySubType, &pb3.CommonSt{
		U32Param: subTypeList[0],
	})
	if err != nil {
		s.LogError("err: %v", err)
	}

	err = engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FRelieveSceneMonstersBySubType, &pb3.CommonSt{
		U32Param: subTypeList[0],
	})
	if err != nil {
		s.LogError("err: %v", err)
	}
}

func (s *DemonSubduing) c2sFetchRewards(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_70_61
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf := jsondata.GetYYDemonSubduingConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ParamsInvalidError("conf not found")
	}

	pConf, ok := conf.PDonate[req.Idx]
	if !ok {
		return neterror.ParamsInvalidError("conf pDonate(%d) not found", req.Idx)
	}

	data := s.GetData()
	playerId := player.GetId()
	if data.PlayerMap[playerId] < pConf.Count {
		return neterror.ParamsInvalidError("conf pDonate(%d) donation limit", req.Idx)
	}

	rewardsBit := data.PlayerRewards[playerId]
	if utils.IsSetBit(rewardsBit, req.Idx) {
		return neterror.ParamsInvalidError("conf pDonate(%d) rewards has fetch", req.Idx)
	}

	// 设置标志位
	rewardsBit = utils.SetBit(rewardsBit, req.Idx)
	data.PlayerRewards[playerId] = rewardsBit

	// 发送奖励
	engine.GiveRewards(player, pConf.Rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogYYDemonSubduingPlayerDonateAwards,
	})

	player.SendProto3(70, 61, &pb3.S2C_70_61{ActId: s.Id, Idx: req.Idx})
	return nil
}

func (s *DemonSubduing) handleKillMon(player iface.IPlayer, monId uint32, count uint32) {
	conf := jsondata.GetYYDemonSubduingConf(s.ConfName, s.ConfIdx)
	if conf == nil || conf.MonsterDonate == nil {
		player.LogError("%s conf not found", s.GetPrefix())
		return
	}
	monsterConf := jsondata.GetMonsterConf(monId)
	if monsterConf == nil {
		player.LogError("mon conf not found %s %d", monId)
		return
	}
	monsterDonate := conf.MonsterDonate[monsterConf.SubType]
	if monsterDonate == nil {
		return
	}
	if pie.Uint32s(monsterDonate.SkipMonIds).Contains(monsterConf.Id) {
		player.LogInfo("skip %d monster", monsterConf.Id)
		return
	}
	s.AddDonate(player, monsterDonate.Donate*count)
	s.sendPlayerData(player)
}

func handleDemonSubduingAeKillMon(player iface.IPlayer, args ...interface{}) {
	if len(args) < 4 {
		return
	}
	monId, ok := args[0].(uint32)
	if !ok {
		return
	}
	count, ok := args[2].(uint32)
	if !ok {
		return
	}
	allYY := yymgr.GetAllYY(yydefine.YYDemonSubduing)
	for _, iYunYing := range allYY {
		iYunYing.(*DemonSubduing).handleKillMon(player, monId, count)
	}
}

func init() {
	yymgr.RegisterYYType(yydefine.YYDemonSubduing, func() iface.IYunYing {
		return &DemonSubduing{}
	})
	event.RegActorEvent(custom_id.AeKillMon, handleDemonSubduingAeKillMon)
	event.RegActorEvent(custom_id.AeQuickAttackKillMon, handleDemonSubduingAeKillMon)
	net.RegisterGlobalYYSysProto(70, 61, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*DemonSubduing).c2sFetchRewards
	})
	gmevent.Register("YYDemonSubduing.AddDonate", func(player iface.IPlayer, args ...string) bool {
		allYY := yymgr.GetAllYY(yydefine.YYDemonSubduing)
		for _, iYunYing := range allYY {
			iYunYing.(*DemonSubduing).AddDonate(player, utils.AtoUint32(args[0]))
			iYunYing.(*DemonSubduing).sendPlayerData(player)
		}
		return true
	}, 1)
}
