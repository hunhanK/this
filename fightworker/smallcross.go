/**
 * @Author: zjj
 * @Date: 2024/12/19
 * @Desc:
**/

package fightworker

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/gameserver/engine"
)

func GetSmallCrossCamp() uint32 {
	hostInfo := GetHostInfo(base.SmallCrossServer)
	return uint32(hostInfo.Camp)
}

func getMediumCrossCamp() uint64 {
	if !engine.FightClientExistPredicate(base.MediumCrossServer) {
		return 0
	}
	sHostInfo := GetHostInfo(base.MediumCrossServer)
	return utils.Make64(sHostInfo.CrossId, sHostInfo.ZoneId)
}
