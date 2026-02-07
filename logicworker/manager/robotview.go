/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2022/5/26 14:23
 */

package manager

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
)

type ViewRobotFunc func(robot iface.IRobot, rsp *pb3.DetailedRoleInfo)

var (
	RobotViewFnMap = make(map[uint32]ViewRobotFunc)
)

func RegisterRobotViewFunc(viewType uint32, function func(robot iface.IRobot, rsp *pb3.DetailedRoleInfo)) {
	if _, ok := RobotViewFnMap[viewType]; ok {
		logger.LogStack("重复注册查看机器人信息函数. 类型 Type ：%d", viewType)
		return
	}

	RobotViewFnMap[viewType] = function
}

func viewRobot(actor iface.IPlayer, robotId uint64, viewType uint32, param string) {
	robot := engine.GetRobotById(robotId)
	if nil == robot {
		return
	}
	rsp := &pb3.S2C_2_27{}
	rsp.Id = robotId
	rsp.ViewType = viewType
	rsp.Param = param
	rsp.Info = &pb3.DetailedRoleInfo{
		Basic:       &pb3.PlayerDataBase{},
		FightProp:   make(map[uint32]int64),
		EquipDeatil: &pb3.EquipDetail{},
	}
	rsp.IsOnline = robot.IsFlagBit(custom_id.AfOnline)

	for idx := uint32(common.ViewTypeBegin); idx <= common.ViewTypeEnd; idx++ {
		if utils.IsSetBit(viewType, idx) {
			if fn, ok := RobotViewFnMap[idx]; ok {
				fn(robot, rsp.Info)
			}
		}
	}
	rsp.Info.EquipDeatil.AllGemData = actor.GetBinaryData().AllGemData

	actor.SendProto3(2, 27, rsp)
}

func init() {
	engine.RegisterRobotViewFunc = RegisterRobotViewFunc
}
