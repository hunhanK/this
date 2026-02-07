package entity

import (
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"
	"math"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

var (
	appearTakeOnChange = map[uint32]func(player iface.IPlayer){
		appeardef.AppearPos_Fabao: func(player iface.IPlayer) {
			faBaoTakingChange(player)
		},
		appeardef.AppearPos_Head: func(player iface.IPlayer) {
			player.TriggerEvent(custom_id.AeHeadChange)
		},
		appeardef.AppearPos_HeadFrame: func(player iface.IPlayer) {
			player.TriggerEvent(custom_id.AeHeadFrameChange)
		},
	}
	appearTakeOffChange = map[uint32]func(player iface.IPlayer, oldId uint64){
		appeardef.AppearPos_Fabao: func(player iface.IPlayer, oldId uint64) {
			faBaoTakeOffChange(player, oldId)
		},
		appeardef.AppearPos_Head: func(player iface.IPlayer, oldId uint64) {
			player.TriggerEvent(custom_id.AeHeadChange)
		},
		appeardef.AppearPos_HeadFrame: func(player iface.IPlayer, oldId uint64) {
			player.TriggerEvent(custom_id.AeHeadFrameChange)
		},
	}
)

func (player *Player) checkTakeOnAppear(appear *pb3.SysAppearSt) bool {
	if appear.SysId >= math.MaxUint16 {
		logger.LogError("TakeOnAppear SysId too big %d", appear.SysId)
		return false
	}

	if appear.AppearId >= math.MaxUint32 {
		logger.LogError("TakeOnAppear AppearId too big %d", appear.AppearId)
		return false
	}

	return true
}

func (player *Player) TakeAppearChange(pos uint32, oldId uint64) {
	if fn, ok := appearTakeOffChange[pos]; ok && oldId > 0 {
		fn(player, oldId)
	}
	if fn, ok := appearTakeOnChange[pos]; ok {
		fn(player)
	}
	return
}

func (player *Player) TakeOffAppearById(pos uint32, appear *pb3.SysAppearSt) {
	if !player.checkTakeOnAppear(appear) {
		return
	}
	appearInfo := player.MainData.AppearInfo
	nowAppear, ok := appearInfo.Appear[pos]
	if !ok {
		return
	}
	if nowAppear.SysId == appear.SysId && nowAppear.AppearId == appear.AppearId {
		player.TakeOffAppear(pos)
	}
}

// todo 技能
func (player *Player) TakeOnAppear(pos uint32, appear *pb3.SysAppearSt, isTip bool) {
	if !player.checkTakeOnAppear(appear) {
		return
	}

	appearId := int64(appear.SysId)<<32 | int64(appear.AppearId)

	extraAttrId, ok := appeardef.AppearPosMapToExtraAttr[pos]
	if !ok {
		logger.LogError("TakeOnAppear pos %d not found", pos)
		return
	}

	player.SetExtraAttr(extraAttrId, appearId)

	appearInfo := player.MainData.AppearInfo
	old := appearInfo.Appear[pos]
	appearInfo.Appear[pos] = appear
	if nil != old {
		player.TakeAppearChange(pos, utils.Make64(old.AppearId, old.SysId))
	} else {
		player.TakeAppearChange(pos, 0)
	}
	if isTip {
		player.SendTipMsg(tipmsgid.TakeAppearChangeSuccess)
	}
	player.TriggerEvent(custom_id.AeAppearChange, pos)
}

func (player *Player) TakeOffAppear(pos uint32) {
	appearInfo := player.MainData.AppearInfo
	old := appearInfo.Appear[pos]
	if old == nil {
		return
	}
	delete(appearInfo.Appear, pos)
	extraAttrId := appeardef.AppearPosMapToExtraAttr[pos]
	player.SetExtraAttr(extraAttrId, 0)
	player.TakeAppearChange(pos, utils.Make64(old.AppearId, old.SysId))
}

func faBaoTakingChange(player iface.IPlayer) {
	extraAttrId, ok := appeardef.AppearPosMapToExtraAttr[appeardef.AppearPos_Fabao]
	if !ok {
		return
	}
	sysId := utils.High32(uint64(player.GetExtraAttr(extraAttrId)))
	appearId := utils.Low32(uint64(player.GetExtraAttr(extraAttrId)))
	binary := player.GetBinaryData()
	if sysId == appeardef.AppearSys_Fabao && appearId == appeardef.FaBaoInitAppear { //初始法宝
		if nil == binary.FabaoData || nil == binary.FabaoData.ExpLv {
			logger.LogError("fabaoData is nil")
			return
		}
		lv := binary.FabaoData.ExpLv.Lv
		lvConf := jsondata.GetFabaoLvConf(lv)
		if lvConf == nil {
			return
		}
		oldSkillLv := player.GetSkillLv(lvConf.AutoSkillId)
		if lvConf.AutoSkillId > 0 && (oldSkillLv == 0 || lvConf.AutoSkillLevel > oldSkillLv) {
			player.LearnSkill(lvConf.AutoSkillId, lvConf.AutoSkillLevel, true)
		}
	} else if sysId == appeardef.AppearSys_FabaoAppear { //其他幻形
		if nil == binary.FabaoFashionData || nil == binary.FabaoFashionData.Fashions {
			logger.LogError("FabaoFashionData is nil")
			return
		}
		fashion, ok := binary.FabaoFashionData.Fashions[appearId]
		if !ok {
			logger.LogError("FabaoFashionData(%d) is nil", appearId)
			return
		}
		conf := jsondata.GetFabaoFashionStarConf(appearId, fashion.Star)
		if nil == conf {
			logger.LogError("fabaoFashion conf(%d) star(%d )is nil", appearId, fashion.Star)
		}
		oldSkillLv := player.GetSkillLv(conf.SkillId)
		if conf.SkillId > 0 && (oldSkillLv == 0 || conf.SkillLevel > oldSkillLv) {
			player.LearnSkill(conf.SkillId, conf.SkillLevel, true)
		}
	}
}

func faBaoTakeOffChange(player iface.IPlayer, oldAttr uint64) {
	binary := player.GetBinaryData()
	sysId := utils.High32(oldAttr)
	appearId := utils.Low32(oldAttr)
	if sysId == appeardef.AppearSys_Fabao && appearId == appeardef.FaBaoInitAppear { //初始法宝
		if nil == binary.FabaoData || nil == binary.FabaoData.ExpLv {
			logger.LogError("fabaoData is nil")
			return
		}
		lvConf := jsondata.GetFabaoLvConf(binary.FabaoData.ExpLv.Lv)
		if lvConf == nil {
			return
		}
		player.ForgetSkill(lvConf.AutoSkillId, true, true, true)
	} else if sysId == appeardef.AppearSys_FabaoAppear { //其他幻形
		if nil == binary.FabaoFashionData || nil == binary.FabaoFashionData.Fashions {
			logger.LogError("FabaoFashionData is nil")
			return
		}
		fashion, ok := binary.FabaoFashionData.Fashions[appearId]
		if !ok {
			logger.LogError("FabaoFashionData(%d) is nil", appearId)
			return
		}
		conf := jsondata.GetFabaoFashionStarConf(appearId, fashion.Star)
		if nil == conf {
			logger.LogError("fabaoFashion conf(%d) star(%d )is nil", appearId, fashion.Star)
		}
		player.ForgetSkill(conf.SkillId, true, true, true)
	}
}
