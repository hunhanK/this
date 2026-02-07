package robotmgr

import (
	"google.golang.org/protobuf/proto"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/guildmgr"
	"jjyz/gameserver/logicworker/internal/timer"
	"jjyz/gameserver/logicworker/manager"
	"time"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
)

var BattleArenaRobotMgrInstance = NewBattleArenaRobotMgr()

type BattleArenaActorData struct {
	ActorId    uint64
	CreateData *pb3.CreateActorData
	BaseData   *pb3.PlayerDataBase
}

type BattleAreaRobotMgr struct {
	robotActorDataMap          map[uint64]*BattleArenaActorData
	robotId2SysRobotActorIdMap map[uint64]uint64
	robotId2MirrorRobotActorId map[uint64]uint64
	hasInit                    bool
}

func (m *BattleAreaRobotMgr) Init() {
	if m.hasInit {
		return
	}
	m.refreshArenaDefaultRobots()
	m.refreshRealActorMirrorRobots()
	m.hasInit = true
}

func (m *BattleAreaRobotMgr) AddRealActorMirrorRobot(actorId uint64) error {
	robotActorId, err := RobotMgrInstance.AddRealActorMirrorRobot(actorId)
	if err != nil {
		logger.LogError(err.Error())
		return err
	}

	player := manager.GetPlayerPtrById(actorId)
	createActorData := player.PackCreateData()
	robotCreateActorData := proto.Clone(createActorData).(*pb3.CreateActorData)
	robotCreateActorData.IsRobot = true
	robotCreateActorData.RobotType = custom_id.ActorRobotTypeRealActorMirror
	robotCreateActorData.RobotConfigId = actorId

	baseData := manager.GetData(actorId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
	// 跟其他地方保持一致 进行拷贝
	robotBaseData := proto.Clone(baseData).(*pb3.PlayerDataBase)
	robotBaseData.Id = robotActorId

	m.robotActorDataMap[robotActorId] = &BattleArenaActorData{
		ActorId:    robotActorId,
		CreateData: robotCreateActorData,
		BaseData:   robotBaseData,
	}
	m.robotId2MirrorRobotActorId[actorId] = robotActorId

	return nil
}

func (m *BattleAreaRobotMgr) GetRobotFightVal(actorId uint64) (uint64, bool) {
	data, ok := m.GetRobotData(actorId)
	if !ok {
		return 0, false
	}
	return data.BaseData.Power, ok
}

func (m *BattleAreaRobotMgr) GetRobotCreateData(actorId uint64) (*pb3.CreateActorData, bool) {
	data, ok := m.GetRobotData(actorId)
	if !ok {
		return nil, false
	}
	return data.CreateData, ok
}

func (m *BattleAreaRobotMgr) GetRobotData(actorId uint64) (*BattleArenaActorData, bool) {
	data, ok := m.robotActorDataMap[actorId]
	return data, ok
}

func (m *BattleAreaRobotMgr) GetRealActorMirrorRobotByConfigId(confId uint64) (*ActorRobot, bool) {
	actorId, ok := m.robotId2MirrorRobotActorId[confId]
	if !ok {
		return nil, false
	}

	robot, ok := RobotMgrInstance.ActorRobots[actorId]
	return robot, ok
}

func (m *BattleAreaRobotMgr) RefreshRealActorMirrorRobot(confId, actorId uint64) {
	playerBaseData, ok := manager.GetData(confId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
	if !ok {
		return
	}

	player := manager.GetPlayerPtrById(confId)
	var robotCreateActorData = &pb3.CreateActorData{}

	if nil != player {
		createActorData := player.PackCreateData()
		robotCreateActorData = proto.Clone(createActorData).(*pb3.CreateActorData)
	} else {
		playerPropertyData, ok := manager.GetOfflineData(confId, gshare.ActorDataProperty).(*pb3.OfflineProperty)
		if !ok {
			return
		}
		robotCreateActorData.Job = playerBaseData.Job
		robotCreateActorData.Sex = playerBaseData.Sex
		robotCreateActorData.Name = playerBaseData.Name
		robotCreateActorData.Circle = playerBaseData.Circle
		robotCreateActorData.Level = playerBaseData.Lv
		robotCreateActorData.Hp = playerPropertyData.ExtraAttr[attrdef.Hp]
		robotCreateActorData.Skills = map[uint32]uint32{}
		for skillId, skillLv := range playerBaseData.Skills {
			if skillConf := jsondata.GetSkillConfig(skillId); nil != skillConf {
				robotCreateActorData.Skills[skillConf.Id] = skillLv
			}
		}
		if guild := guildmgr.GetGuildById(playerBaseData.GuildId); nil != guild {
			if member := guild.GetMember(playerBaseData.Id); nil != member {
				robotCreateActorData.GuildName = guild.GetName()
				robotCreateActorData.GuildPos = member.GetPosition()
			}
		}
		attrs := &pb3.SysAttr{
			Attrs: map[uint32]int64{},
		}
		for attrType, val := range playerPropertyData.FightAttr {
			attrs.Attrs[attrType] = val
		}

		extraAttrs := make(map[uint32]int64)
		for attrType, val := range playerPropertyData.ExtraAttr {
			extraAttrs[attrType] = val
		}

		robotCreateActorData.Attrs = map[uint32]*pb3.SysAttr{
			attrdef.SaBattleArena: attrs,
		}
		robotCreateActorData.ExtraAttrs = extraAttrs
	}
	robotCreateActorData.IsRobot = true
	robotCreateActorData.RobotType = custom_id.ActorRobotTypeRealActorMirror
	robotCreateActorData.RobotConfigId = confId

	robotBaseData := proto.Clone(playerBaseData).(*pb3.PlayerDataBase)
	robotBaseData.Id = actorId
	m.robotActorDataMap[actorId] = &BattleArenaActorData{
		ActorId:    actorId,
		CreateData: robotCreateActorData,
		BaseData:   robotBaseData,
	}
	m.robotId2MirrorRobotActorId[confId] = actorId
}

func (m *BattleAreaRobotMgr) refreshRealActorMirrorRobots() {
	robots, _ := RobotMgrInstance.Type2ActorRobotMap[custom_id.ActorRobotTypeRealActorMirror]
	for _, robot := range robots {
		m.RefreshRealActorMirrorRobot(robot.RobotConfigId, robot.ActorId)
	}
}

func (m *BattleAreaRobotMgr) refreshArenaDefaultRobots() {
	robots, _ := RobotMgrInstance.Type2ActorRobotMap[custom_id.ActorRobotTypeBattleArenaSysBuilt]
	conf := jsondata.GetBattleArenaConf()
	for _, robot := range robots {
		var robotConf *jsondata.ActorRobot
		for _, robotCfg := range conf.Robots {
			if robotCfg.RobotId == robot.RobotConfigId {
				robotConf = robotCfg
				break
			}
		}

		skills := make(map[uint32]uint32)
		for _, skill := range robotConf.Skills {
			if skillConf := jsondata.GetSkillConfig(skill.SkillId); nil != skillConf {
				skills[skillConf.Id] = skill.SkillLv
			}
		}

		attrs := &pb3.SysAttr{
			Attrs: map[uint32]int64{},
		}
		for _, attr := range robotConf.Attrs {
			attrs.Attrs[attr.Type] = int64(attr.Value)
		}

		extraAttrs := make(map[uint32]int64)
		for _, attr := range robotConf.ExtraAttrs {
			extraAttrs[attr.Type] = appeardef.ConvertRobotExtraAttrsVal(int64(attr.Type), int64(attr.Value))
		}
		extraAttrs[attrdef.FightValue] = int64(robotConf.FightVal)

		lv, _ := extraAttrs[attrdef.Level]
		vipLv, _ := extraAttrs[attrdef.VipLevel]
		hp, _ := attrs.Attrs[attrdef.MaxHp]

		m.robotActorDataMap[robot.ActorId] = &BattleArenaActorData{
			ActorId: robot.ActorId,
			CreateData: &pb3.CreateActorData{
				Sex:    robotConf.Sex,
				Job:    robotConf.Job,
				Hp:     hp,
				Name:   robotConf.Name,
				Level:  uint32(lv),
				Skills: skills,
				Attrs: map[uint32]*pb3.SysAttr{
					attrdef.SaBattleArena: attrs,
				},
				ExtraAttrs:    extraAttrs,
				IsRobot:       true,
				RobotConfigId: robotConf.RobotId,
				RobotType:     custom_id.ActorRobotTypeBattleArenaSysBuilt,
			},
			BaseData: &pb3.PlayerDataBase{
				Id:    robot.ActorId,
				Name:  robotConf.Name,
				Lv:    uint32(lv),
				VipLv: uint32(vipLv),
				Job:   robotConf.Job,
				Sex:   robotConf.Sex,
				Power: robotConf.FightVal,
			},
		}
		m.robotId2MirrorRobotActorId[robot.RobotConfigId] = robot.ActorId
	}
}

func NewBattleArenaRobotMgr() *BattleAreaRobotMgr {
	return &BattleAreaRobotMgr{
		robotActorDataMap:          map[uint64]*BattleArenaActorData{},
		robotId2MirrorRobotActorId: map[uint64]uint64{},
		robotId2SysRobotActorIdMap: map[uint64]uint64{},
	}
}

func init() {
	manager.GRankMgrIns.GetRankByType(gshare.RankTypeBattleArena).Init(10000)
	event.RegSysEvent(custom_id.SeNewDayArrive, func(args ...interface{}) {
		if !BattleArenaRobotMgrInstance.hasInit {
			BattleArenaRobotMgrInstance.Init()
			return
		}
		BattleArenaRobotMgrInstance.refreshRealActorMirrorRobots()
	})
	// 服务初始化完成后才开始定时检查真实玩家数据
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		timer.SetInterval(3*time.Minute, func() {
			if !BattleArenaRobotMgrInstance.hasInit {
				BattleArenaRobotMgrInstance.Init()
				return
			}
			BattleArenaRobotMgrInstance.refreshRealActorMirrorRobots()
		})
	})

	gmevent.Register("PerformanceFb", func(player iface.IPlayer, args ...string) bool {
		var nums int = 0
		if len(args) >= 1 {
			nums = utils.AtoInt(args[0])
		}

		robotsCreateDatas := make([]*pb3.EnterBattleArenaReq, 0, nums)

		if BattleArenaRobotMgrInstance == nil || len(BattleArenaRobotMgrInstance.robotActorDataMap) == 0 {
			logger.LogError("PerformanceFb failed for BattleArenaRobotMgrInstance is nil")
			return false
		}

		randKeys := make([]uint64, 0)

		for k := range BattleArenaRobotMgrInstance.robotActorDataMap {
			randKeys = append(randKeys, k)
		}

		for i := 0; i < nums; i++ {
			index := random.Interval(0, len(randKeys)-1)
			robotKey := randKeys[index]
			robot, ok := BattleArenaRobotMgrInstance.robotActorDataMap[robotKey]
			if !ok {
				logger.LogError("PerformanceFb error for robotKey invalid")
				continue
			}

			robotsCreateDatas = append(robotsCreateDatas, &pb3.EnterBattleArenaReq{
				OpponentId:         robot.ActorId,
				OpponentCreateData: robot.CreateData,
				PfId:               engine.GetPfId(),
				SrvId:              engine.GetServerId(),
			})
		}

		player.EnterFightSrv(base.LocalFightServer, fubendef.EnterPerformanceFb, &pb3.EnterPerformanceFbReq{
			Robots: robotsCreateDatas,
		})

		return true
	}, 1)
}
