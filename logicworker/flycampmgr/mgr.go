/**
 * @Author: lzp
 * @Date: 2024/9/13
 * @Desc:
**/

package flycampmgr

import (
	"jjyz/gameserver/gshare"
)

func GetCampCount() map[uint32]uint32 {
	gVar := gshare.GetStaticVar()
	if gVar.CampCount == nil {
		gVar.CampCount = make(map[uint32]uint32)
	}

	return gVar.CampCount
}

func AddCampCount(camp uint32) {
	data := GetCampCount()
	data[camp] += 1
}

func OnChangeCamp(oldCamp, newCamp uint32) {
	data := GetCampCount()
	if data[oldCamp] >= 1 {
		data[oldCamp] -= 1
	} else {
		data[oldCamp] = 0
	}
	data[newCamp] += 1
}
