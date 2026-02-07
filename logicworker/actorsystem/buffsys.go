package actorsystem

import (
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/iface"
)

/**
 * @Author: ChenJunJi
 * @Desc: buff 逻辑服只负责保存数据, buff逻辑放在战斗服写
 * @Date: 2023/03/22 17:49
 */

func OnSaveBuffs(player iface.IPlayer, buf []byte) {
	msg := &pb3.BuffList{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		player.LogError("【保存buff】出错 %v", err)
		return
	}

	if binary := player.GetBinaryData(); nil != binary {
		binary.BuffList = msg.BuffList
	}
}

// 使用buff道具
func useBuffItem(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) < 1 {
		player.LogError("useItem:%d, Wrong number of parameters!", param.ItemId)
		return false, false, 0
	}
	buffId := conf.Param[0]

	buffConf := jsondata.GetBuffConfig(buffId)
	if nil == buffConf {
		return false, false, 0
	}

	player.AddBuff(buffId)

	return true, true, 1
}

func init() {
	miscitem.RegCommonUseItemHandle(itemdef.UseItemAddBuff, useBuffItem)
	engine.RegisterActorCallFunc(playerfuncid.SaveBuffs, OnSaveBuffs)
}
