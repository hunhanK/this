package robotmgr

import (
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/logicworker/manager"
)

func init() {
	manager.RobotRankPlayerDataSetter = func(playerId uint64, info *pb3.RankInfo) bool {
		robot, ok := RobotMgrInstance.ActorRobots[playerId]
		if !ok {
			return false
		}

		switch robot.RobotType {
		case custom_id.ActorRobotTypeBattleArenaSysBuilt:
			data, ok := BattleArenaRobotMgrInstance.GetRobotData(playerId)
			if !ok {
				return false
			}
			info.Name = data.BaseData.Name
			info.Job = data.BaseData.Job
			info.Head = data.BaseData.Head
			info.HeadFrame = data.BaseData.HeadFrame
			info.VipLv = data.BaseData.VipLv
			info.Appear = map[uint32]int64{}
			info.Value = int64(data.BaseData.Power)
			for i := attrdef.AppearCloth; i <= attrdef.AppearFabao; i++ {
				info.Appear[uint32(i)] = data.CreateData.ExtraAttrs[uint32(i)]
			}
			return true
		default:
			if player := manager.GetPlayerPtrById(robot.RobotConfigId); nil != player {
				info.Head = player.GetHead()
				info.HeadFrame = player.GetHeadFrame()
				info.VipLv = player.GetBinaryData().Vip
				info.Name = player.GetName()
				info.Job = player.GetJob()
				info.Appear = manager.GetExtraAppearAttr(robot.RobotConfigId)
			} else {
				if data := manager.GetData(robot.RobotConfigId, gshare.ActorDataBase); data != nil {
					if baseData, ok := data.(*pb3.PlayerDataBase); ok {
						info.Name = baseData.Name
						info.Job = baseData.Job
						info.Head = baseData.Head
						info.HeadFrame = baseData.HeadFrame
						info.VipLv = baseData.VipLv
						info.Appear = manager.GetExtraAppearAttr(robot.RobotConfigId)
					}
				}
			}
		}
		return false
	}
}
