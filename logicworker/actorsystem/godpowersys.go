/**
 * @Author: zjj
 * @Date: 2023年10月30日
 * @Desc: 神通
**/

package actorsystem

import (
	"encoding/json"
	"fmt"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type GodPowerSys struct {
	Base
}

func (s *GodPowerSys) GetData() map[uint32]*pb3.GodPowerLayer {
	if s.GetBinaryData().GodPowerState == nil {
		s.GetBinaryData().GodPowerState = make(map[uint32]*pb3.GodPowerLayer)
	}
	return s.GetBinaryData().GodPowerState
}

func (s *GodPowerSys) S2CInfo() {
	s.SendProto3(157, 0, &pb3.S2C_157_0{
		State: s.GetData(),
	})
}

func (s *GodPowerSys) OnLogin() {
	s.loadLayerOpen()
	s.S2CInfo()
}

func (s *GodPowerSys) OnReconnect() {
	s.loadLayerOpen()
	s.S2CInfo()
}

func (s *GodPowerSys) OnOpen() {
	s.loadLayerOpen()
	s.S2CInfo()
}

func (s *GodPowerSys) loadLayerOpen() {
	openServerDay := gshare.GetOpenServerDay()
	data := s.GetData()

	jsondata.EachGodPowerConfMgr(func(config *jsondata.GodPowerLayerConf) {
		// todo 跨服天数待定
		if config.OpenDay > openServerDay {
			return
		}

		// 初始化 界层
		realm, ok := data[config.Id]
		if !ok {
			data[config.Id] = &pb3.GodPowerLayer{
				Id:        config.Id,
				SeriesMap: make(map[uint32]*pb3.GodPowerSeries),
			}
			realm = data[config.Id]
		}

		// 初始化 系列
		for _, s := range config.SeriesMap {
			_, ok := realm.SeriesMap[s.Id]
			if !ok {
				realm.SeriesMap[s.Id] = &pb3.GodPowerSeries{}
			}
			if realm.SeriesMap[s.Id].GodPowerMap == nil {
				realm.SeriesMap[s.Id].GodPowerMap = make(map[uint32]*pb3.GodPower)
			}
		}

	})
}

func (s *GodPowerSys) checkOpenLayer(LayerId uint32) bool {
	_, ok := s.GetData()[LayerId]
	return ok
}

func (s *GodPowerSys) getGodPower(id uint32) (*pb3.GodPower, bool) {
	dataMap := s.GetData()
	for _, realm := range dataMap {
		for sk := range realm.SeriesMap {
			series := realm.SeriesMap[sk]
			godPower, ok := series.GodPowerMap[id]
			if !ok {
				continue
			}
			return godPower, true
		}
	}
	return nil, false
}

func (s *GodPowerSys) c2sUpLv(msg *base.Message) error {
	var req pb3.C2S_157_1
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	owner := s.GetOwner()
	if !s.checkOpenLayer(req.LayerId) {
		s.GetOwner().LogWarn("realm not open")
		owner.SendTipMsg(tipmsgid.TpUnlockNotMeet)
		return nil
	}

	godPower, ok := s.getGodPower(req.GodPowerId)
	if !ok {
		return neterror.ConfNotFoundError("god power not found")
	}

	godPowerConf, ok := jsondata.GetGodPowerConf(req.LayerId, req.SeriesId, req.GodPowerId)
	if !ok {
		return neterror.ConfNotFoundError("skill not found")
	}

	// 升级
	conf, ok := godPowerConf.LevelConf[fmt.Sprintf("%d", godPower.Lv)]
	if !ok {
		return neterror.ConfNotFoundError("lv is max , level is %d", godPower.Lv)
	}

	// 没有下一级
	levelConf := godPowerConf.LevelConf[fmt.Sprintf("%d", godPower.Lv+1)]
	if levelConf == nil {
		return neterror.ConfNotFoundError("lv is max ,next level is %d", godPower.Lv+1)
	}

	if len(conf.Consume) > 0 {
		if !owner.ConsumeByConf(conf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogGodPowerUpLv}) {
			s.GetOwner().LogWarn("consume failed")
			owner.SendTipMsg(tipmsgid.TpUseItemFailed)
			return nil
		}
	}

	godPower.Lv += 1
	s.ResetSysAttr(attrdef.SaGodPower)

	// 广播升级
	itemConf := jsondata.GetItemConfig(godPowerConf.ItemId)
	if itemConf != nil {
		engine.BroadcastTipMsgById(godPowerConf.UpLvTips, owner.GetId(), owner.GetName(), itemConf.Name, godPower.Lv)
	}

	owner.SendProto3(157, 1, &pb3.S2C_157_1{
		LayerId:    req.LayerId,
		SeriesId:   req.SeriesId,
		GodPowerId: req.GodPowerId,
		CurLv:      godPower.Lv,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogGodPowerToLv, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.LayerId),
		StrArgs: fmt.Sprintf("%d_%d_%d", req.SeriesId, req.GodPowerId, godPower.Lv),
	})
	return nil
}

func (s *GodPowerSys) c2sUpStage(msg *base.Message) error {
	var req pb3.C2S_157_3
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	owner := s.GetOwner()
	if !s.checkOpenLayer(req.LayerId) {
		s.GetOwner().LogWarn("realm not open")
		owner.SendTipMsg(tipmsgid.TpUnlockNotMeet)
		return nil
	}

	godPower, ok := s.getGodPower(req.GodPowerId)
	if !ok {
		return neterror.ConfNotFoundError("god power not found")
	}

	godPowerConf, ok := jsondata.GetGodPowerConf(req.LayerId, req.SeriesId, req.GodPowerId)
	if !ok {
		return neterror.ConfNotFoundError("skill not found")
	}

	nextStage := godPower.Stage + 1
	// 满阶 没有下一阶
	nextConf, ok := godPowerConf.StageConf[fmt.Sprintf("%d", nextStage)]
	if !ok {
		s.GetOwner().LogWarn("stage is max , stage is %d", nextStage)
		owner.SendTipMsg(tipmsgid.TpUnlockNotMeet)
		return nil
	}

	if len(nextConf.Consume) > 0 {
		if !owner.ConsumeByConf(nextConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogGodPowerUpStage}) {
			owner.SendTipMsg(tipmsgid.TpUseItemFailed)
			return nil
		}
	}

	godPower.Stage = nextStage
	// 学习技能
	if nextConf.SkillId > 0 && nextConf.SkillLv > 0 {
		owner.LearnSkill(nextConf.SkillId, nextConf.SkillLv, true)
	}

	s.ResetSysAttr(attrdef.SaGodPower)
	owner.SendProto3(157, 3, &pb3.S2C_157_3{
		LayerId:    req.LayerId,
		SeriesId:   req.SeriesId,
		GodPowerId: req.GodPowerId,
		Stage:      godPower.Stage,
	})
	logworker.LogPlayerBehavior(owner, pb3.LogId_LogGodPowerUpStage, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.LayerId),
		StrArgs: fmt.Sprintf("%d_%d_%d", req.SeriesId, req.GodPowerId, godPower.Stage),
	})
	return nil
}

func (s *GodPowerSys) c2sActive(msg *base.Message) error {
	var req pb3.C2S_157_2
	if err := msg.UnPackPb3Msg(&req); nil != err {
		return err
	}

	err := s.active(req.LayerId, req.SeriesId, req.GodPowerId, false)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return err
	}

	skill, ok := s.getGodPower(req.GodPowerId)
	if !ok {
		return neterror.InternalError("not found active god power , req is %d %d %d", req.LayerId, req.SeriesId, req.GodPowerId)
	}

	s.SendProto3(157, 2, &pb3.S2C_157_2{
		IsAutoActive: false,
		LayerId:      req.LayerId,
		SeriesId:     req.SeriesId,
		GodPowerId:   req.GodPowerId,
		CurLv:        skill.Lv,
	})

	bytes, _ := json.Marshal(map[string]interface{}{
		"IsAutoActive": false,
		"LayerId":      req.LayerId,
		"SeriesId":     req.SeriesId,
		"GodPowerId":   req.GodPowerId,
		"CurLv":        skill.Lv,
	})
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogGodPowerActorActive, &pb3.LogPlayerCounter{
		StrArgs: string(bytes),
	})
	return nil
}

func (s *GodPowerSys) active(layerId, seriesId, godPowerId uint32, isAutoActive bool) error {
	_, ok := s.getGodPower(godPowerId)
	// 已激活
	if ok {
		s.GetOwner().LogWarn("skill active")
		return nil
	}
	data := s.GetData()

	realm, ok := data[layerId]
	if !ok {
		return neterror.ConfNotFoundError("realm not found , LayerId is %d", layerId)
	}

	series, ok := realm.SeriesMap[seriesId]
	if !ok {
		return neterror.ConfNotFoundError("series not found , seriesId is %d", seriesId)
	}

	// 未激活
	godPowerConf, ok := jsondata.GetGodPowerConf(layerId, seriesId, godPowerId)
	if !ok {
		return neterror.ConfNotFoundError("skill conf not found , skillId is %d", godPowerId)
	}

	// 使用碎片激活
	owner := s.GetOwner()
	if !isAutoActive && godPowerConf.Cost > 0 {
		var cs jsondata.ConsumeVec
		cs = append(cs, &jsondata.Consume{
			Id:    godPowerConf.SpiritsDebris,
			Count: godPowerConf.Cost,
		})

		if !owner.ConsumeByConf(cs, false, common.ConsumeParams{LogId: pb3.LogId_LogGodPowerActive}) {
			owner.SendTipMsg(tipmsgid.TpUseItemFailed)
			return neterror.ConsumeFailedError("consume failed")
		}
	}

	series.GodPowerMap[godPowerId] = &pb3.GodPower{
		Id:      godPowerConf.ItemId,
		SkillId: godPowerConf.SkillId,
		Lv:      1,
	}

	owner.LearnSkill(godPowerConf.SkillId, 1, true)

	s.ResetSysAttr(attrdef.SaGodPower)

	// 广播激活
	itemConf := jsondata.GetItemConfig(godPowerConf.ItemId)
	if itemConf != nil {
		engine.BroadcastTipMsgById(godPowerConf.ActiveTips, owner.GetId(), owner.GetName(), itemConf.Name)
	}

	return nil
}

// 转换碎片
func (s *GodPowerSys) convertDebris(itemId uint32, count int64) error {
	_, _, conf, ok := jsondata.GetGodPowerSkillByItemId(itemId)
	if !ok {
		return neterror.ConfNotFoundError("item[%d] not found", itemId)
	}

	var award jsondata.StdRewardVec
	for i := int64(0); i < count; i++ {
		award = append(award, &jsondata.StdReward{Id: conf.SpiritsDebris, Count: int64(conf.Num)})
	}

	engine.GiveRewards(s.GetOwner(), award, common.EngineGiveRewardParam{LogId: pb3.LogId_LogGodPowerConvertDebris})
	return nil
}

func useItemGodPowerSkillHandle(player iface.IPlayer, param *miscitem.UseItemParamSt, _ *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	s, ok := player.GetSysObj(sysdef.SiGodPower).(*GodPowerSys)
	if !ok {
		return
	}

	layerId, seriesId, conf, ok := jsondata.GetGodPowerSkillByItemId(param.ItemId)
	if !ok {
		s.GetOwner().LogWarn("not found skill conf")
		return
	}

	_, ok = s.getGodPower(conf.ItemId)
	if ok {
		err := s.convertDebris(param.ItemId, param.Count)
		if err != nil {
			s.GetOwner().LogError("err:%v", err)
			return
		}
		return true, true, param.Count
	}

	// 没激活
	err := s.active(layerId, seriesId, conf.ItemId, true)
	if err != nil {
		s.GetOwner().LogError("err:%v", err)
		return
	}

	// 多的道具变碎片
	if param.Count > 1 {
		err := s.convertDebris(param.ItemId, param.Count-1)
		if err != nil {
			s.GetOwner().LogError("err:%v", err)
			return
		}
	}

	s.GetOwner().SendProto3(157, 2, &pb3.S2C_157_2{
		IsAutoActive: true,
		LayerId:      layerId,
		SeriesId:     seriesId,
		GodPowerId:   conf.ItemId,
		CurLv:        1,
	})

	return true, true, param.Count
}

// 重算属性
func calcGodPowerSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s := player.GetSysObj(sysdef.SiGodPower).(*GodPowerSys)
	if !s.IsOpen() {
		return
	}
	data := s.GetData()
	var attrs jsondata.AttrVec

	// 计算等级带来的提升
	for layerId, realm := range data {
		for seriesId, series := range realm.SeriesMap {
			for _, godPower := range series.GodPowerMap {
				godPowerConf, ok := jsondata.GetGodPowerConf(layerId, seriesId, godPower.Id)
				if !ok {
					s.GetOwner().LogWarn("not found skill , LayerId %d, seriesId %d, godPower %d", layerId, seriesId, godPower.Id)
					continue
				}

				if levelConf := godPowerConf.LevelConf[fmt.Sprintf("%d", godPower.Lv)]; levelConf != nil {
					if len(levelConf.Attrs) > 0 {
						attrs = append(attrs, levelConf.Attrs...)
					}
					if len(levelConf.ExtAttrs) > 0 {
						attrs = append(attrs, levelConf.ExtAttrs...)
					}
				}

				if stageConf := godPowerConf.StageConf[fmt.Sprintf("%d", godPower.Stage)]; stageConf != nil {
					if len(stageConf.Attrs) > 0 {
						attrs = append(attrs, stageConf.Attrs...)
					}
				}

			}
		}
	}

	// 加属性
	if len(attrs) > 0 {
		engine.CheckAddAttrsToCalc(player, calc, attrs)
	}
}

func init() {
	RegisterSysClass(sysdef.SiGodPower, func() iface.ISystem {
		return &GodPowerSys{}
	})

	net.RegisterSysProtoV2(157, 1, sysdef.SiGodPower, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*GodPowerSys).c2sUpLv
	})

	net.RegisterSysProtoV2(157, 2, sysdef.SiGodPower, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*GodPowerSys).c2sActive
	})
	net.RegisterSysProtoV2(157, 3, sysdef.SiGodPower, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*GodPowerSys).c2sUpStage
	})

	// 注册道具使用
	miscitem.RegCommonUseItemHandle(itemdef.UseItemGodPowerSkill, useItemGodPowerSkillHandle)

	// 计算属性
	engine.RegAttrCalcFn(attrdef.SaGodPower, calcGodPowerSysAttr)
	gmevent.Register("setGodPowerLv", func(player iface.IPlayer, args ...string) bool {
		s, ok := player.GetSysObj(sysdef.SiGodPower).(*GodPowerSys)
		if !ok {
			return false
		}
		gowPowerId := utils.AtoUint32(args[0])
		gowPowerLv := utils.AtoUint32(args[1])
		godPower, ok := s.getGodPower(gowPowerId)
		if !ok {
			return false
		}
		godPower.Lv = gowPowerLv
		s.S2CInfo()
		return true
	}, 1)
}
