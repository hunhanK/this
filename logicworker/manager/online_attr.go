/**
 * @Author: LvYuMeng
 * @Date: 2023/12/22
 * @Desc:
**/

package manager

import (
	"jjyz/base/custom_id"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
)

func SetOnlineAttr(actorId uint64, attr, val uint32, add bool) {
	if custom_id.OnlineAttrBegin >= attr || custom_id.OnlineAttrEnd <= attr {
		return
	}
	staticVar := gshare.GetStaticVar()
	if nil == staticVar.OnlineAttr {
		staticVar.OnlineAttr = make(map[uint64]*pb3.SpAttr)
	}
	if nil == staticVar.OnlineAttr[actorId] {
		staticVar.OnlineAttr[actorId] = &pb3.SpAttr{}
	}
	onlineAttr := staticVar.OnlineAttr[actorId]
	if nil == onlineAttr.Attrs {
		onlineAttr.Attrs = make(map[uint32]uint32)
	}
	if add {
		onlineAttr.Attrs[attr] += val
	} else {
		onlineAttr.Attrs[attr] = val
	}

	return
}

func GetOnlineAttr(actorId uint64, attr uint32) uint32 {
	staticVar := gshare.GetStaticVar()
	if nil == staticVar.OnlineAttr {
		return 0
	}
	onlineAttr, ok := staticVar.OnlineAttr[actorId]
	if !ok || nil == onlineAttr.Attrs {
		return 0
	}
	return onlineAttr.Attrs[attr]
}

func DelOnlineAttr(actorId uint64, attr uint32) {
	staticVar := gshare.GetStaticVar()
	if nil == staticVar.OnlineAttr {
		return
	}
	onlineAttr, ok := staticVar.OnlineAttr[actorId]
	if !ok || nil == onlineAttr.Attrs {
		return
	}
	delete(onlineAttr.Attrs, attr)
}

func init() {
	event.RegSysEvent(custom_id.SeNewDayArrive, func(args ...interface{}) {
		staticVar := gshare.GetStaticVar()
		for _, v := range staticVar.OnlineAttr {
			if attrs := v.Attrs; nil != attrs {
				delete(v.Attrs, custom_id.OnlineAttrFriendDayAdd)
			}
		}
	})

}
