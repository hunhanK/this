/**
 * @Author: LvYuMeng
 * @Date: 2024/4/19
 * @Desc:
**/

package engine

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"
)

var GetRobotById func(id uint64) iface.IRobot
var RegisterRobotViewFunc func(viewType uint32, function func(robot iface.IRobot, rsp *pb3.DetailedRoleInfo))

func IsRobot(actorId uint64) bool {
	// 检查是否设置了机器人标志位
	if !utils.IsSetBit64(actorId, base.ActorIdRobotFlag) {
		return false
	}

	actorId = utils.ClearBit64(actorId, base.ActorIdRobotFlag)
	// 提取 pfId
	pfId := uint32((actorId >> 40) & base.MaxPfId)
	// 检查 pfId 是否等于 1（机器人的 pfId 固定为 1）
	return pfId == 1
}
