/**
 * @Author: zjj
 * @Date: 2024/5/29
 * @Desc:
**/

package gshare

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
)

var TeamFbSysMap = map[int]int{
	sysdef.SiEquipFuBen:   custom_id.TeamEquipFuBenFlag,
	sysdef.SiExpFuben:     custom_id.TeamExpFuBenFlag,
	sysdef.SiAncientTower: custom_id.TeamAncientTowerFlag,
	sysdef.SiBeastRampant: custom_id.TeamBeastRampantFlag,
}

func SetTeamFbSysOpenFlag(bit int64, sysId uint32) int64 {
	if val, ok := TeamFbSysMap[int(sysId)]; ok {
		bit = int64(utils.SetBit64(uint64(bit), uint32(val)))
	}
	return bit
}
