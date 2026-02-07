/**
 * @Author: zjj
 * @Date: 2024/12/24
 * @Desc:
**/

package sectgivegiftmgr

import (
	"jjyz/base/custom_id"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
)

func GetSectGiveGiftGlobalData() *pb3.SectGiveGiftGlobalData {
	staticVar := gshare.GetStaticVar()
	if staticVar.SectGiveGiftGlobalData == nil {
		staticVar.SectGiveGiftGlobalData = &pb3.SectGiveGiftGlobalData{}
	}
	data := staticVar.SectGiveGiftGlobalData
	if data.GiftTabData == nil {
		data.GiftTabData = make(map[uint32]*pb3.SectGiveGiftTabData)
	}
	if data.Chest == nil {
		data.Chest = new(pb3.SectGiveGiftChest)
	}
	return data
}

func GetSectGiveGiftChest() *pb3.SectGiveGiftChest {
	data := GetSectGiveGiftGlobalData()
	if data.Chest.Lv == 0 {
		data.Chest.Lv = 1
	}
	return data.Chest
}

func GetTabGiftData(tab uint32) *pb3.SectGiveGiftTabData {
	data := GetSectGiveGiftGlobalData()
	if data.GiftTabData[tab] == nil {
		data.GiftTabData[tab] = &pb3.SectGiveGiftTabData{}
	}
	return data.GiftTabData[tab]
}

func AddSectGiveGift(tab uint32, item *pb3.SectGiveGiftItem) {
	data := GetTabGiftData(tab)
	// 最新的往前面插入
	data.ItemList = append([]*pb3.SectGiveGiftItem{item}, data.ItemList...)
}

func AddChestExp(exp int64) {
	chest := GetSectGiveGiftChest()
	chest.Exp += exp
	for i := 0; i < 1000; i++ {
		chestConf := jsondata.GetSectGiveGiftChestConf(chest.Lv + 1)
		if chestConf == nil {
			return
		}
		if chestConf.Exp > chest.Exp {
			return
		}
		chest.Lv++
		chest.Exp -= chestConf.Exp
	}
}

func GetSectGiveGiftHdlSet() map[uint64]struct{} {
	var set = make(map[uint64]struct{})
	for _, tabData := range GetSectGiveGiftGlobalData().GiftTabData {
		for _, item := range tabData.ItemList {
			set[item.Hdl] = struct{}{}
		}
	}
	return set
}

func RunOne() {
	nowSec := time_util.NowSec()
	data := GetSectGiveGiftGlobalData()
	var expiredHdlSet = make(map[uint64]struct{})
	for _, tabData := range data.GiftTabData {
		if len(tabData.ItemList) == 0 {
			continue
		}
		var size int
		for size = len(tabData.ItemList); size > 0; size-- {
			item := tabData.ItemList[size-1]
			if item.ExpiredAt > nowSec {
				break
			}
			expiredHdlSet[item.Hdl] = struct{}{}
		}
		tabData.ItemList = tabData.ItemList[:size]
	}
	if len(expiredHdlSet) == 0 {
		return
	}
	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		player.TriggerEvent(custom_id.AeSectGiveGiftExpired, expiredHdlSet)
	})
}
