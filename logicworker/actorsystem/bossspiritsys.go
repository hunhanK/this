/**
 * @Author:
 * @Date:
 * @Desc:
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"time"
)

type BossSpiritSys struct {
	Base
}

func (s *BossSpiritSys) s2cInfo() {
	s.SendProto3(8, 90, &pb3.S2C_8_90{
		Data: s.getData(),
	})
}

func (s *BossSpiritSys) getData() *pb3.BossSpiritData {
	data := s.GetBinaryData().BossSpiritData
	if data == nil {
		s.GetBinaryData().BossSpiritData = &pb3.BossSpiritData{}
		data = s.GetBinaryData().BossSpiritData
	}
	if data.ItemPosMap == nil {
		data.ItemPosMap = make(map[uint32]uint32)
	}
	if data.BossFollowMap == nil {
		data.BossFollowMap = make(map[uint32]*pb3.BossSpiritBossFollow)
	}
	return data
}

func (s *BossSpiritSys) OnReconnect() {
	s.updateExtAttr()
	s.s2cInfo()
}

func (s *BossSpiritSys) OnLogin() {
	s.updateExtAttr()
	s.s2cInfo()
}

func (s *BossSpiritSys) OnOpen() {
	data := s.getData()
	data.Lv = 1
	s.updateExtAttr()
	s.s2cInfo()
}

func (s *BossSpiritSys) c2sUpLv(_ *base.Message) error {
	data := s.getData()
	owner := s.GetOwner()
	lv := data.Lv
	conf := jsondata.GetBossSpiritConfig(lv)
	if conf == nil {
		return neterror.ConfNotFoundError("not found %d conf", lv)
	}
	if len(conf.Pos) != len(data.ItemPosMap) {
		return neterror.ParamsInvalidError("not match %d pos num %d %d", lv, len(conf.Pos), len(data.ItemPosMap))
	}
	if conf.TipId > 0 {
		engine.BroadcastTipMsgById(conf.TipId, owner.GetId(), owner.GetName(), conf.Turn, conf.Star, conf.Name)
	}
	data.ItemPosMap = make(map[uint32]uint32)
	data.Lv += 1
	s.updateExtAttr()
	s.SendProto3(8, 91, &pb3.S2C_8_91{
		Data: data,
	})
	owner.TriggerQuestEventRange(custom_id.QttBossSpiritActive)
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogBossSpiritUpLv, &pb3.LogPlayerCounter{
		NumArgs: uint64(lv),
		StrArgs: fmt.Sprintf("%d", data.Lv),
	})
	return nil
}

func (s *BossSpiritSys) activeShard(itemId uint32) {
	data := s.getData()
	owner := s.GetOwner()
	lv := data.Lv
	conf := jsondata.GetBossSpiritConfig(lv)
	if conf == nil {
		owner.LogWarn("not found %d conf", lv)
		return
	}
	if conf.Pos == nil {
		owner.LogWarn("not found %d pos conf", lv)
		return
	}
	_, ok := data.ItemPosMap[itemId]
	if ok {
		owner.LogWarn("%d pos %d already active", lv, itemId)
		return
	}
	pos, ok := conf.Pos[itemId]
	if !ok {
		owner.LogWarn("not found %d pos %d conf", lv, itemId)
		return
	}
	data.ItemPosMap[itemId] = pos.Pos
	s.SendProto3(8, 92, &pb3.S2C_8_92{
		ItemId: itemId,
		Pos:    pos.Pos,
	})
	s.ResetSysAttr(attrdef.SaBossSpiritShard)
	s.updateExtAttr()
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogBossSpiritShardActive, &pb3.LogPlayerCounter{
		NumArgs: uint64(lv),
		StrArgs: fmt.Sprintf("%d_%d", itemId, pos),
	})
}

func (s *BossSpiritSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	owner := s.GetOwner()
	for i := uint32(1); i <= data.Lv; i++ {
		conf := jsondata.GetBossSpiritConfig(i)
		if conf == nil {
			continue
		}

		if i == data.Lv {
			for itemId := range data.ItemPosMap {
				conf := jsondata.GetItemConfig(itemId)
				if conf == nil {
					continue
				}
				engine.CheckAddAttrsToCalc(owner, calc, conf.StaticAttrs)
			}
			continue
		}

		for itemId := range conf.Pos {
			conf := jsondata.GetItemConfig(itemId)
			if conf == nil {
				continue
			}
			engine.CheckAddAttrsToCalc(owner, calc, conf.StaticAttrs)
		}
	}
}

// 被玩家击杀掉落碎片
func (s *BossSpiritSys) bossSpiritShardKillOtherActor(req *pb3.F2GLostBossSpirit) {
	data := s.getData()
	if data.Lv != req.Lv {
		return
	}
	config := jsondata.GetBossSpiritConfig(data.Lv)
	var posConf *jsondata.BossSpiritPos
	for _, pos := range config.Pos {
		if pos.Pos != req.Pos {
			continue
		}
		posConf = pos
		break
	}
	if posConf == nil {
		return
	}
	if !random.Hit(posConf.ExplosiveRate, 10000) {
		return
	}
	_, ok := data.ItemPosMap[posConf.ItemId]
	if !ok {
		return
	}
	delete(data.ItemPosMap, posConf.ItemId)
	owner := s.GetOwner()
	err := owner.CallActorMediumCrossFunc(playerfuncid.G2FLostBossSpirit, &pb3.G2FLostBossSpiritReq{
		AttackerId:    req.AttackerId,
		AttackerSrvId: req.AttackerSrvId,
		AttackerPfId:  req.AttackerPfId,
		FbId:          req.FbId,
		SceneId:       req.SceneId,
		DropId:        posConf.DropId,
		X:             req.X,
		Y:             req.Y,
	})
	if err != nil {
		owner.LogError("F2GLostBossSpiritReq err:%v", err)
	}
	s.ResetSysAttr(attrdef.SaBossSpiritShard)
	s.updateExtAttr()
	s.SendProto3(8, 93, &pb3.S2C_8_93{
		ItemId: posConf.ItemId,
		Pos:    posConf.Pos,
	})
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogBossSpiritShardDrop, &pb3.LogPlayerCounter{
		NumArgs: uint64(posConf.ItemId),
		StrArgs: fmt.Sprintf("%d_%d_%d_%d_%d", req.AttackerPfId, req.AttackerSrvId, req.AttackerId, req.FbId, req.SceneId),
	})
}

func (s *BossSpiritSys) c2sEnterFb(msg *base.Message) error {
	var req pb3.C2S_8_94
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return neterror.Wrap(err)
	}
	if req.SceneId == 0 {
		return neterror.ParamsInvalidError("sceneId is 0")
	}
	data := s.getData()
	fbConfig := jsondata.GetBossSpiritFbConfig()
	if fbConfig == nil {
		return neterror.ConfNotFoundError("not found bossSpiritFbConfig")
	}

	fbSceneConfig := jsondata.GetBossSpiritFbSceneConfig(req.SceneId)
	if fbSceneConfig == nil {
		return neterror.ConfNotFoundError("not found bossSpiritFbSceneConfig %d", req.SceneId)
	}
	if fbSceneConfig.EnterBossSpiritLv != 0 && data.Lv < fbSceneConfig.EnterBossSpiritLv {
		return neterror.ParamsInvalidError("not match %d %d %d", req.SceneId, fbSceneConfig.EnterBossSpiritLv, data.Lv)
	}
	if len(fbConfig.NotCanEnterRange) == 2 {
		hour := uint32(time.Now().Hour())
		if fbConfig.NotCanEnterRange[0] <= hour && hour <= fbConfig.NotCanEnterRange[1] {
			return neterror.ParamsInvalidError("not match %d %d %d", req.SceneId, fbSceneConfig.EnterBossSpiritLv, data.Lv)
		}
	}

	if !engine.FightClientExistPredicate(base.MediumCrossServer) {
		return neterror.InternalError("not medium cross server")
	}

	owner := s.GetOwner()
	freeTimes := fbConfig.FreeTimes
	if data.DailyTimes >= freeTimes {
		if len(fbConfig.EnterConsume) == 0 || !owner.ConsumeByConf(fbConfig.EnterConsume, false, common.ConsumeParams{LogId: pb3.LogId_LogEnterBossSpiritFbConsume}) {
			return neterror.ConsumeFailedError("consume not enough")
		}
	}
	err := owner.EnterFightSrv(base.MediumCrossServer, fubendef.EnterBossSpiritFb, &pb3.EnterBattleArenaFbReq{
		SceneId: req.SceneId,
	})
	if err != nil {
		return neterror.Wrap(err)
	}
	data.DailyTimes += 1
	s.SendProto3(8, 98, &pb3.S2C_8_98{
		DailyTimes: data.DailyTimes,
	})
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogEnterBossSpiritFbConsume, &pb3.LogPlayerCounter{
		NumArgs: uint64(data.DailyTimes),
		StrArgs: fmt.Sprintf("%d", req.SceneId),
	})
	return nil
}

func (s *BossSpiritSys) c2sFollowBoss(msg *base.Message) error {
	var req pb3.C2S_8_96
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return neterror.Wrap(err)
	}
	bossConfig := jsondata.GetBossSpiritFbSceneBossConfig(req.SceneId, req.MonId)
	if bossConfig == nil {
		bossConfig = jsondata.GetBossSpiritFbSceneBigBossConfig(req.SceneId, req.MonId)
	}
	if bossConfig == nil {
		return neterror.ConfNotFoundError("%d %d not found mon config", req.SceneId, req.MonId)
	}
	data := s.getData()
	follow := data.BossFollowMap[req.SceneId]
	if follow == nil {
		data.BossFollowMap[req.SceneId] = &pb3.BossSpiritBossFollow{}
		follow = data.BossFollowMap[req.SceneId]
	}
	if follow.BossFollowMap == nil {
		follow.BossFollowMap = make(map[uint32]bool)
	}
	follow.BossFollowMap[req.MonId] = req.NeedFollow
	s.SendProto3(8, 96, &pb3.S2C_8_96{
		SceneId:    req.SceneId,
		MonId:      req.MonId,
		NeedFollow: req.NeedFollow,
	})
	return nil
}

func (s *BossSpiritSys) updateExtAttr() {
	data := s.getData()
	var flag uint32
	for _, pos := range data.ItemPosMap {
		flag = utils.SetBit(flag, pos)
	}
	val := utils.Make64(flag, data.Lv)
	s.GetOwner().SetExtraAttr(attrdef.BossSpirits, attrdef.AttrValueAlias(val))
}

func (s *BossSpiritSys) CheckLootBossSpirit(itemId uint32) bool {
	config := jsondata.GetItemConfig(itemId)
	if config == nil || !itemdef.IsItemTypeBossSpiritShard(config.Type) {
		return true
	}
	data := s.getData()
	spiritConfig := jsondata.GetBossSpiritConfig(data.Lv)
	if spiritConfig == nil || spiritConfig.Pos[itemId] == nil {
		return false
	}
	pos, ok := data.ItemPosMap[itemId]
	if ok || pos > 0 {
		return false
	}
	return true
}

func (s *BossSpiritSys) callG2FBossSpiritBossPubList(sceneId uint32) {
	err := s.GetOwner().CallActorMediumCrossFunc(playerfuncid.G2FBossSpiritBossPubList, &pb3.CommonSt{
		U32Param: sceneId,
	})
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
	}
}

func (s *BossSpiritSys) c2sGetSceneInfo(msg *base.Message) error {
	var req pb3.C2S_8_99
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return neterror.Wrap(err)
	}
	s.callG2FBossSpiritBossPubList(req.SceneId)
	return nil
}

func useItemBossSpiritShard(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	success, del, cnt = true, true, param.Count
	obj := player.GetSysObj(sysdef.SiBossSpirit)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys := obj.(*BossSpiritSys)
	if sys == nil {
		return
	}
	sys.activeShard(param.ItemId)
	return
}

func calcBossSpiritAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	obj := player.GetSysObj(sysdef.SiBossSpirit)
	if obj == nil || !obj.IsOpen() {
		return
	}

	sys := obj.(*BossSpiritSys)
	if sys == nil {
		return
	}

	sys.calcAttr(calc)
}

// 被其他玩家击杀
func bossSpiritShardKillOtherActor(player iface.IPlayer, buf []byte) {
	var req pb3.F2GLostBossSpirit
	if err := pb3.Unmarshal(buf, &req); nil != err {
		player.LogError("err:%v", err)
		return
	}
	obj := player.GetSysObj(sysdef.SiBossSpirit)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys := obj.(*BossSpiritSys)
	if sys == nil {
		return
	}
	sys.bossSpiritShardKillOtherActor(&req)
}

func init() {
	RegisterSysClass(sysdef.SiBossSpirit, func() iface.ISystem {
		return &BossSpiritSys{}
	})
	net.RegisterSysProtoV2(8, 91, sysdef.SiBossSpirit, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*BossSpiritSys).c2sUpLv
	})
	net.RegisterSysProtoV2(8, 94, sysdef.SiBossSpirit, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*BossSpiritSys).c2sEnterFb
	})
	net.RegisterSysProtoV2(8, 96, sysdef.SiBossSpirit, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*BossSpiritSys).c2sFollowBoss
	})
	net.RegisterSysProtoV2(8, 99, sysdef.SiBossSpirit, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*BossSpiritSys).c2sGetSceneInfo
	})
	engine.RegAttrCalcFn(attrdef.SaBossSpiritShard, calcBossSpiritAttr)
	miscitem.RegCommonUseItemHandle(itemdef.UseItemBossSpiritShard, useItemBossSpiritShard)
	engine.RegisterActorCallFunc(playerfuncid.F2GLostBossSpiritReq, bossSpiritShardKillOtherActor)
	engine.RegQuestTargetProgress(custom_id.QttBossSpiritActive, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		obj := actor.GetSysObj(sysdef.SiBossSpirit)
		if obj == nil || !obj.IsOpen() {
			return 0
		}
		sys := obj.(*BossSpiritSys)
		if sys == nil {
			return 0
		}
		data := sys.getData()
		return data.Lv
	})
	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		obj := actor.GetSysObj(sysdef.SiBossSpirit)
		if obj == nil || !obj.IsOpen() {
			return
		}
		sys := obj.(*BossSpiritSys)
		sys.getData().DailyTimes = 0
		sys.s2cInfo()
	})
	gmevent.Register("enterbossspiritfb", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		sceneId := utils.AtoUint32(args[0])
		err := player.EnterFightSrv(base.MediumCrossServer, fubendef.EnterBossSpiritFb, &pb3.EnterBattleArenaFbReq{
			SceneId: sceneId,
		})
		if err != nil {
			player.LogError("enterbossspiritfb %d failed %v", sceneId, err)
			return false
		}
		return true
	}, 1)
}
