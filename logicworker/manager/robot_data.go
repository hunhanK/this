/**
 * @Author: LvYuMeng
 * @Date: 2024/4/19
 * @Desc:
**/

package manager

import (
	"jjyz/base"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
)

func GetRobotSimplyData(robotId uint64) *pb3.SimplyPlayerData {
	robot := engine.GetRobotById(robotId)
	if nil == robot {
		return nil
	}
	simplyRole := &pb3.SimplyPlayerData{
		Id:             robot.GetRobotId(),
		Name:           robot.GetName(),
		Circle:         uint32(robot.GetAttr(attrdef.Circle)),
		Lv:             robot.GetLevel(),
		VipLv:          uint32(robot.GetAttr(attrdef.VipLevel)),
		Job:            uint32(robot.GetAttr(attrdef.Job)) >> base.SexBit,
		Sex:            uint32(robot.GetAttr(attrdef.Job)) & (1 << base.SexBit),
		GuildId:        robot.GetGuildId(),
		LastLogoutTime: robot.GetLastLogoutTime(),
		BubbleFrame:    robot.GetBubbleFrame(),
		HeadFrame:      robot.GetHeadFrame(),
		LoginTime:      robot.GetLoginTime(),
		Head:           robot.GetHead(),
		Power:          uint64(robot.GetAttr(attrdef.FightValue)),
	}
	//标签没加
	return simplyRole
}
