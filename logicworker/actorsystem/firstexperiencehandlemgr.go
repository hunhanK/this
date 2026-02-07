/**
 * @Author: zjj
 * @Date: 2024/1/9
 * @Desc: 首次体验管理器具体体验类型 handle
**/

package actorsystem

import (
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"
)

// 开始
var beforeStartFirstExperienceMgr = map[uint32]func(player iface.IPlayer, experienceData *pb3.FirstExperience, experienceConf *jsondata.FirstExperienceConf) (canStart bool, timeOut uint32){
	uint32(pb3.ExperienceType_ExperienceTypeSpirity): beforeStartExperienceBySpirit, // 前置检查精灵体验
}
var startFirstExperienceMgr = map[uint32]func(player iface.IPlayer, experienceData *pb3.FirstExperience, experienceConf *jsondata.FirstExperienceConf){
	uint32(pb3.ExperienceType_ExperienceTypeSpirity): startExperienceBySpirit, // 主动使用精灵体验
}

// 结束
var endExperienceMgr = map[uint32]func(player iface.IPlayer, st *pb3.CommonSt) bool{
	uint32(pb3.ExperienceType_ExperienceTypeSpirity): endExperienceBySpirit, // 精灵体验时间到期
}

func beforeStartExperienceBySpirit(player iface.IPlayer, experienceData *pb3.FirstExperience, experienceConf *jsondata.FirstExperienceConf) (canStart bool, timeOut uint32) {
	if len(experienceConf.Nums) < 2 {
		player.LogWarn("参数不足")
		return false, 0
	}

	timeOut = experienceConf.Nums[0]
	skinId := experienceConf.Nums[1]

	spiritSys, ok := player.GetSysObj(sysdef.SiSpiritPet).(*SpiritySys)
	if !ok || !spiritSys.IsOpen() {
		return false, 0
	}

	// 已经激活
	ok = spiritSys.CheckSpiritActive(skinId)
	if ok {
		return false, 0
	}
	experienceData.ExtU32Param = skinId

	return true, timeOut
}

func startExperienceBySpirit(player iface.IPlayer, experienceData *pb3.FirstExperience, experienceConf *jsondata.FirstExperienceConf) {
	spiritSys, ok := player.GetSysObj(sysdef.SiSpiritPet).(*SpiritySys)
	if !ok || !spiritSys.IsOpen() {
		return
	}

	// 激活
	err := spiritSys.ActiveSkin(experienceData.ExtU32Param)
	if err != nil {
		player.LogError("err:%v", err)
		return
	}
}

func endExperienceBySpirit(player iface.IPlayer, st *pb3.CommonSt) (ret bool) {
	ret = true
	typ := uint32(pb3.ExperienceType_ExperienceTypeSpirity)
	experienceConf, err := jsondata.GetFirstExperienceConf(typ)
	if err != nil {
		player.LogError("err:%v", err)
		return
	}

	if len(experienceConf.Nums) < 2 {
		return
	}

	skinId := experienceConf.Nums[1]

	// 要结束的不是当前体验的 不能结束
	if st != nil && skinId != st.U32Param {
		ret = false
		return
	}

	obj := player.GetSysObj(sysdef.SiSpiritPet)
	if obj == nil || !obj.IsOpen() {
		player.LogWarn("sys not find")
		return
	}

	spiritSys, ok := player.GetSysObj(sysdef.SiSpiritPet).(*SpiritySys)
	if !ok || !spiritSys.IsOpen() {
		return
	}

	err = spiritSys.DisactiveSkin(skinId)
	if err != nil {
		player.LogError("err:%v", err)
		return
	}

	return true
}
