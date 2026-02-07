/**
 * @Author: LvYuMeng
 * @Date: 2024/10/9
 * @Desc:
**/

package robotmgr

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/series"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/logicworker/guildmgr"
	"jjyz/gameserver/logicworker/manager"
)

func GetMirrorRobotActorId() (uint64, error) {
	s, errno := series.GetActorIdSeries(engine.GetServerId())
	if 0 != errno {
		return 0, fmt.Errorf("alloc actor series error:%d", errno)
	}

	robotActorId, err := base.MakePlayerId(engine.GetPfId(), engine.GetServerId(), s)
	if nil != err {
		return 0, err
	}
	return robotActorId, nil
}

func CopyRealActorMirrorRobotData(playerId uint64, params *custom_id.MirrorRobotParam) *pb3.CreateActorData {
	playerData, ok := manager.GetData(playerId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
	if !ok {
		return nil
	}

	playerPropertyData, ok := manager.GetData(playerId, gshare.ActorDataProperty).(*pb3.OfflineProperty)
	if !ok {
		return nil
	}

	createData := &pb3.CreateActorData{
		IsRobot:       true,
		RobotType:     params.RobotType,
		RobotConfigId: params.RobotConfigId,
		Job:           playerData.GetJob(),
		Sex:           playerData.GetSex(),
		Name:          playerData.GetName(),
		Level:         playerData.GetLv(),
		Circle:        playerData.GetCircle(),
	}

	if guild := guildmgr.GetGuildById(playerData.GetGuildId()); nil != guild {
		createData.GuildName = guild.GetName()
		member := guild.GetMember(playerData.GetId())
		createData.GuildPos = member.GetPosition()
	}

	createData.Skills = playerData.Skills

	createData.Attrs = map[uint32]*pb3.SysAttr{
		attrdef.SaMirror: {
			Attrs: playerPropertyData.FightAttr,
		},
	}

	createData.ExtraAttrs = playerPropertyData.ExtraAttr

	return createData
}
