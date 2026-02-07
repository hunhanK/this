/**
 * @Author: LvYuMeng
 * @Date: 2023/12/8
 * @Desc:
**/

package entity

import (
	"fmt"
	"github.com/gzjjyz/logger"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem"
	"jjyz/gameserver/logicworker/friendmgr"
	"jjyz/gameserver/logicworker/manager"
)

type GetPlayerShowStrAttr func(player iface.IPlayer) string

var playerShowStr = map[uint32]GetPlayerShowStrAttr{
	custom_id.ShowStrMarryName:             showStrMarryName,
	custom_id.ShowStrCustomTitle:           showStrCustomTitle,
	custom_id.ShowStrCustomCardDeclaration: showStrCustomCardDeclaration,
	custom_id.ShowStrCustomCardBackground:  showStrCustomCardBackground,
	custom_id.ShowStrCustomHonor:           showStrCustomHonor,
}

func (player *Player) PackageShowStr() map[uint32]string {
	attr := make(map[uint32]string)
	for i := custom_id.ShowStrStart; i <= custom_id.ShowStrEnd; i++ {
		fn := playerShowStr[uint32(i)]
		var str = ""
		if fn != nil {
			str = fn(player)
		}
		attr[uint32(i)] = str
	}
	return attr
}

func (player *Player) SyncShowStr(attr uint32) {
	if attr < custom_id.ShowStrStart || attr > custom_id.ShowStrEnd {
		return
	}
	if fn, ok := playerShowStr[attr]; ok {
		err := player.CallActorFunc(actorfuncid.SyncShowStr, &pb3.CommonSt{U32Param: attr, StrParam: fn(player)})
		if err != nil {
			logger.LogError("SyncShowStr err %v", err)
			return
		}
	}
}

func showStrMarryName(player iface.IPlayer) string {
	obj := player.GetSysObj(sysdef.SiMarry)
	if obj == nil || !obj.IsOpen() {
		return ""
	}
	sys, ok := obj.(*actorsystem.MarrySys)
	if !ok {
		return ""
	}
	data := sys.GetData()
	if !friendmgr.IsExistStatus(data.CommonId, custom_id.FsMarry) {
		return ""
	}
	if baseData, ok := manager.GetData(data.MarryId, gshare.ActorDataBase).(*pb3.PlayerDataBase); ok {
		return baseData.GetName()
	}
	return ""
}

func showStrCustomTitle(player iface.IPlayer) string {
	obj := player.GetSysObj(sysdef.SiTitle)
	if obj == nil || !obj.IsOpen() {
		return ""
	}
	sys, ok := obj.(*actorsystem.TitleSys)
	if !ok {
		return ""
	}
	titleId := player.GetExtraAttrU32(attrdef.TitleId)
	info, ok := sys.GetTitleInfo(titleId)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%d|%s", titleId, info.Name)
}

func showStrCustomCardDeclaration(player iface.IPlayer) string {
	obj := player.GetSysObj(sysdef.SiCustomCard)
	if obj == nil || !obj.IsOpen() {
		return ""
	}
	sys, ok := obj.(*actorsystem.CustomCardSys)
	if !ok {
		return ""
	}
	id := player.GetExtraAttrU32(attrdef.AppearPosCustomCardDeclaration)
	info, ok := sys.GetCustomCardInfo(id)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%d|%s", id, info.Content)
}

func showStrCustomCardBackground(player iface.IPlayer) string {
	obj := player.GetSysObj(sysdef.SiCustomCard)
	if obj == nil || !obj.IsOpen() {
		return ""
	}
	sys, ok := obj.(*actorsystem.CustomCardSys)
	if !ok {
		return ""
	}
	id := player.GetExtraAttrU32(attrdef.AppearPosCustomCardBackground)
	info, ok := sys.GetCustomCardInfo(id)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%d|%s", id, info.Content)
}

func showStrCustomHonor(player iface.IPlayer) string {
	obj := player.GetSysObj(sysdef.SiHonor)
	if obj == nil || !obj.IsOpen() {
		return ""
	}
	sys, ok := obj.(*actorsystem.HonorSys)
	if !ok {
		return ""
	}
	honorId := player.GetExtraAttrU32(attrdef.HonorId)
	info, ok := sys.GetHonorInfo(honorId)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%d|%s", honorId, info.Name)
}
