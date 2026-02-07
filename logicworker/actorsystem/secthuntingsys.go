/**
 * @Author:
 * @Date:
 * @Desc:
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logicworker/robotmgr"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"time"
)

type SectHuntingSys struct {
	Base
}

func (s *SectHuntingSys) s2cInfo() {
	s.SendProto3(8, 200, &pb3.S2C_8_200{
		GlobalData: manager.GetSectHuntingGlobalData(),
		Data:       s.getData(),
	})
}

func (s *SectHuntingSys) getData() *pb3.SectHuntingData {
	data := s.GetBinaryData().SectHuntingData
	if data == nil {
		s.GetBinaryData().SectHuntingData = &pb3.SectHuntingData{}
		data = s.GetBinaryData().SectHuntingData
	}
	return data
}

func (s *SectHuntingSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SectHuntingSys) OnLogin() {
	s.s2cInfo()
}

func (s *SectHuntingSys) OnOpen() {
	s.s2cInfo()
}

func (s *SectHuntingSys) c2sAttackFb(msg *base.Message) error {
	var req pb3.C2S_8_201
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	globalData := manager.GetSectHuntingGlobalData()
	if globalData == nil || globalData.Boss == nil || globalData.Boss.BossId == 0 {
		return neterror.ParamsInvalidError("boss info not found")
	}

	config := jsondata.GetSectHuntingConfig()
	if config == nil {
		return neterror.ConfNotFoundError("SectHuntingConfig is nil")
	}

	// 11:30:00,23:59:59
	hourRange := config.ChallengeHourRange
	inRange := isTimeInRange(hourRange)
	owner := s.GetOwner()
	if !inRange {
		owner.SendTipMsg(tipmsgid.TpXianZongHuntingTips)
		return neterror.ParamsInvalidError("not in time range %v", hourRange)
	}

	bossConf := jsondata.GetSectHuntingBossConf(globalData.Boss.BossId)
	if bossConf == nil {
		return neterror.ConfNotFoundError("boss %d conf not found", globalData.Boss.BossId)
	}

	// 同副本同场景 拦住
	if owner.GetFbId() == config.FbId && owner.GetSceneId() == bossConf.SceneId {
		return neterror.ParamsInvalidError("already in fbId %d", config.FbId)
	}
	data := s.getData()
	if data.LiveTime != 0 && data.LiveTime > config.LiveTime {
		return neterror.ParamsInvalidError("today not can enter %d %d", data.LiveTime, config.LiveTime)
	}
	err = owner.EnterFightSrv(base.LocalFightServer, fubendef.EnterSectHuntingBossFb, &pb3.EnterSectHuntingFbReq{
		SceneId:  bossConf.SceneId,
		LiveTime: data.LiveTime,
	})
	if err != nil {
		return neterror.InternalError("enter fight srv failed err: %s", err)
	}
	s.SendProto3(8, 201, &pb3.S2C_8_201{
		LiveTime: data.LiveTime,
	})

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogSectHuntingAttackBoss, &pb3.LogPlayerCounter{
		NumArgs: uint64(globalData.Boss.BossId),
		StrArgs: fmt.Sprintf("%d", data.LiveTime),
	})
	return nil
}
func (s *SectHuntingSys) c2sRecChapter(msg *base.Message) error {
	var req pb3.C2S_8_202
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	globalData := manager.GetSectHuntingGlobalData()
	if globalData == nil || globalData.Boss == nil || globalData.Boss.BossId == 0 {
		return neterror.ParamsInvalidError("boss info not found")
	}
	config := jsondata.GetSectHuntingConfig()
	if config == nil {
		return neterror.ConfNotFoundError("SectHuntingConfig is nil")
	}
	bossConf := jsondata.GetSectHuntingBossConf(globalData.Boss.BossId)
	if bossConf == nil {
		return neterror.ConfNotFoundError("boss %d conf not found", globalData.Boss.BossId)
	}
	if bossConf.ChapterId <= 0 || bossConf.ChapterId-1 == 0 {
		return neterror.ParamsInvalidError("boss:%d not pass chapter id %d", bossConf.BossId, bossConf.ChapterId)
	}

	passChapterId := bossConf.ChapterId - 1
	data := s.getData()
	if data.RecChapterId >= passChapterId {
		return neterror.ParamsInvalidError("already rec chapter id %d", data.RecChapterId)
	}

	var recChapterIds pie.Uint32s
	var totalAwards jsondata.StdRewardVec
	for _, chapter := range config.Chapter {
		// 玩家已经领过
		if data.RecChapterId >= chapter.Id {
			continue
		}
		// 还没得领
		if passChapterId < chapter.Id {
			continue
		}
		recChapterIds = recChapterIds.Append(chapter.Id)
		totalAwards = append(totalAwards, chapter.Awards...)
		totalAwards = jsondata.MergeStdReward(totalAwards)
	}
	if len(recChapterIds) == 0 {
		return neterror.ParamsInvalidError("no rec chapter")
	}

	data.RecChapterId = recChapterIds.Max()
	owner := s.GetOwner()
	if len(totalAwards) > 0 {
		engine.GiveRewards(owner, totalAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSectHuntingRecChapter})
		owner.SendShowRewardsPop(totalAwards)
	}
	s.SendProto3(8, 202, &pb3.S2C_8_202{
		RecChapterId: data.RecChapterId,
	})

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogSectHuntingRecChapter, &pb3.LogPlayerCounter{
		NumArgs: uint64(data.RecChapterId),
		StrArgs: fmt.Sprintf("%v", recChapterIds),
	})
	return nil
}
func (s *SectHuntingSys) c2sRecActorAwards(msg *base.Message) error {
	var req pb3.C2S_8_203
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}

	config := jsondata.GetSectHuntingConfig()
	if config == nil {
		return neterror.ConfNotFoundError("SectHuntingConfig is nil")
	}

	data := s.getData()
	damage := data.Damage
	recActorAwardsId := data.RecActorAwardsId

	var recDamageIds pie.Uint32s
	var totalAwards jsondata.StdRewardVec
	for _, aAwards := range config.ActorAwards {
		// 玩家已经领过
		if aAwards.Id <= recActorAwardsId {
			continue
		}
		// 还没得领
		if aAwards.Damage > damage {
			continue
		}
		recDamageIds = recDamageIds.Append(aAwards.Id)
		totalAwards = append(totalAwards, aAwards.Awards...)
		totalAwards = jsondata.MergeStdReward(totalAwards)
	}
	if len(recDamageIds) == 0 {
		return neterror.ParamsInvalidError("no rec chapter")
	}

	data.RecActorAwardsId = recDamageIds.Max()
	owner := s.GetOwner()
	if len(totalAwards) > 0 {
		engine.GiveRewards(owner, totalAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSectHuntingRecActorAwards})
		owner.SendShowRewardsPop(totalAwards)
	}
	s.SendProto3(8, 203, &pb3.S2C_8_203{
		RecActorAwardsId: data.RecActorAwardsId,
	})

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogSectHuntingRecActorAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(data.RecChapterId),
		StrArgs: fmt.Sprintf("%v", recDamageIds),
	})
	return nil
}

func (s *SectHuntingSys) c2sAskHelp(msg *base.Message) error {
	var req pb3.C2S_8_209
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	owner := s.GetOwner()
	owner.ChannelChat(&pb3.C2S_5_1{
		Msg:     req.Msg,
		Channel: req.Channel,
		Params:  req.Params,
	}, true)
	return nil
}

// 请求轮换到下一个怪物
func handleF2GRotateTheNextBoss(_ []byte) {
	manager.RotateTheNextSectHuntingBoss()
	data := manager.GetSectHuntingGlobalData()
	if data.NotNextBoss {
		logger.LogError("没有找到下一轮怪物")
		return
	}
	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FRotateTheNextBoss, &pb3.G2FRotateTheNextBossReq{
		BossId: data.Boss.BossId,
	})
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
}

// 保存怪物血量信息
func handleF2GSaveSectHuntingBoss(buf []byte) {
	var req pb3.F2GSaveSectHuntingBossReq
	err := pb3.Unmarshal(buf, &req)
	if err != nil {
		return
	}
	boss := manager.GetSectHuntingGlobalData().Boss
	if boss == nil {
		logger.LogError("boss not data")
		return
	}
	if boss.BossId != req.BossId {
		logger.LogError("bossId not equal %d %d", boss.BossId, req.BossId)
		return
	}
	boss.BuckleHp = req.BuckleHp
}

// 保存玩家伤害
func handleF2GSaveActorDamage(player iface.IPlayer, buf []byte) {
	config := jsondata.GetSectHuntingConfig()
	if config == nil || len(config.ActorAwards) == 0 {
		player.LogError("not found SectHuntingConfig")
		return
	}

	var req pb3.F2GSectHuntingSaveActorDamageReq
	err := pb3.Unmarshal(buf, &req)
	if err != nil {
		player.LogError("err:%v", err)
		return
	}
	obj := player.GetSysObj(sysdef.SiSectHunting)
	if obj == nil || !obj.IsOpen() {
		return
	}
	s, ok := obj.(*SectHuntingSys)
	if !ok {
		return
	}

	// 修正最大伤害 不存储过大的伤害
	data := s.getData()
	data.Damage += req.Damage
	maxDamage := config.ActorAwards[len(config.ActorAwards)-1].Damage
	if s.getData().Damage > maxDamage {
		s.getData().Damage = maxDamage
	}
	s.SendProto3(8, 205, &pb3.S2C_8_205{
		Damage: s.getData().Damage,
	})

	// 排行榜更新伤害
	rank := manager.GRankMgrIns.GetRankByType(gshare.RankSectHuntingDamage)
	score, _ := rank.GetScoreById(int64(player.GetId()))
	manager.GRankMgrIns.UpdateRank(gshare.RankSectHuntingDamage, player.GetId(), score+int64(req.Damage))

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogSectHuntingSaveActorDamage, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.BossId),
		StrArgs: fmt.Sprintf("%d", req.Damage),
	})
}

func handleF2GSectHuntingSaveLiveTime(player iface.IPlayer, buf []byte) {
	var req pb3.F2GSectHuntingSaveLiveTimeReq
	err := pb3.Unmarshal(buf, &req)
	if err != nil {
		player.LogError("err:%v", err)
		return
	}
	obj := player.GetSysObj(sysdef.SiSectHunting)
	if obj == nil || !obj.IsOpen() {
		return
	}
	s, ok := obj.(*SectHuntingSys)
	if !ok {
		return
	}
	data := s.getData()
	data.LiveTime = req.LiveTime
	s.SendProto3(8, 211, &pb3.S2C_8_211{
		LiveTime: data.LiveTime,
	})
}

func handleF2GSectHuntingCreateRobot(player iface.IPlayer, buf []byte) {
	var req pb3.F2GSectHuntingCreateRobotReq
	err := pb3.Unmarshal(buf, &req)
	if err != nil {
		player.LogError("err:%v", err)
		return
	}
	var ids []uint32
	for i := uint32(0); i < req.RobotNum; i++ {
		conf := jsondata.RandomOneAssistFightRobot(req.FbId, ids)
		if conf == nil {
			return
		}
		createData := robotmgr.CopyRealActorMirrorRobotData(player.GetId(), &custom_id.MirrorRobotParam{
			RobotType: custom_id.ActorRobotTypeAssistFight,
		})
		ids = append(ids, uint32(conf.Id))
		createData.RobotConfigId = conf.Id
		createData.Level = player.GetLevel()
		createData.Circle = player.GetCircle()
		err = engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FTeamFbCreateAssistFightRobotReq, &pb3.CreateAssistFightRobotReq{
			SceneId:        req.SceneId,
			SmallCrossCamp: uint32(player.GetSmallCrossCamp()),
			FbHdl:          req.FbHdl,
			FbId:           req.FbId,
			MirrorData:     createData,
		})
		if err != nil {
			logger.LogError("handleF2GSectHuntingCreateRobot [%d] err:%v", createData.RobotConfigId, err)
		}
	}
}

func isTimeInRange(hourRange []string) bool {
	if len(hourRange) != 2 {
		return false
	}

	// 解析开始时间和结束时间
	pStartTime, err := time.Parse("15:04:05", hourRange[0])
	if err != nil {
		return false
	}

	pEndTime, err := time.Parse("15:04:05", hourRange[1])
	if err != nil {
		return false
	}

	// 获取当前时间
	currentTime := time.Now()
	startTime := time.Date(currentTime.Year(), currentTime.Month(), currentTime.Day(), pStartTime.Hour(), pStartTime.Minute(), pStartTime.Second(), 0, currentTime.Location())
	endTime := time.Date(currentTime.Year(), currentTime.Month(), currentTime.Day(), pEndTime.Hour(), pEndTime.Minute(), pEndTime.Second(), 0, currentTime.Location())

	// 判断当前时间是否在范围内
	return (currentTime.After(startTime) && currentTime.Before(endTime)) || currentTime.Equal(startTime) || currentTime.Equal(endTime)
}

func handleOfflineSectHuntingSysResetLiveTime(player iface.IPlayer, msg pb3.Message) {
	obj := player.GetSysObj(sysdef.SiSectHunting)
	if obj == nil || !obj.IsOpen() {
		return
	}
	s, ok := obj.(*SectHuntingSys)
	if !ok {
		return
	}
	s.getData().LiveTime = 0
	s.s2cInfo()
}
func handleOfflineSectHuntingSysResetChallengeBoss(player iface.IPlayer, msg pb3.Message) {
	commonSt := msg.(*pb3.CommonSt)
	if commonSt == nil {
		return
	}
	data := manager.GetSectHuntingGlobalData()
	bossId := commonSt.U32Param
	if bossId == 0 {
		return
	}
	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FCreateSectHuntingMon, &pb3.G2FCreateSectHuntingMonReq{
		BossId:   data.Boss.BossId,
		BuckleHp: 0,
	})
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
	data.Boss.BossId = bossId
	data.Boss.BuckleHp = 0
	obj := player.GetSysObj(sysdef.SiSectHunting)
	if obj == nil || !obj.IsOpen() {
		return
	}
	s, ok := obj.(*SectHuntingSys)
	if !ok {
		return
	}
	s.s2cInfo()
}

func init() {
	RegisterSysClass(sysdef.SiSectHunting, func() iface.ISystem {
		return &SectHuntingSys{}
	})

	net.RegisterSysProtoV2(8, 201, sysdef.SiSectHunting, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SectHuntingSys).c2sAttackFb
	})
	net.RegisterSysProtoV2(8, 202, sysdef.SiSectHunting, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SectHuntingSys).c2sRecChapter
	})
	net.RegisterSysProtoV2(8, 203, sysdef.SiSectHunting, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SectHuntingSys).c2sRecActorAwards
	})
	net.RegisterSysProtoV2(8, 209, sysdef.SiSectHunting, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SectHuntingSys).c2sAskHelp
	})
	engine.RegisterSysCall(sysfuncid.F2GRotateTheNextBoss, handleF2GRotateTheNextBoss)
	engine.RegisterSysCall(sysfuncid.F2GSaveSectHuntingBoss, handleF2GSaveSectHuntingBoss)
	engine.RegisterActorCallFunc(playerfuncid.F2GSectHuntingSaveActorDamage, handleF2GSaveActorDamage)
	engine.RegisterActorCallFunc(playerfuncid.F2GSectHuntingSaveLiveTime, handleF2GSectHuntingSaveLiveTime)
	engine.RegisterActorCallFunc(playerfuncid.F2GSectHuntingCreateRobot, handleF2GSectHuntingCreateRobot)
	engine.RegisterMessage(gshare.OfflineSectHuntingSysResetLiveTime, func() pb3.Message {
		return &pb3.CommonSt{}
	}, handleOfflineSectHuntingSysResetLiveTime)
	engine.RegisterMessage(gshare.OfflineSectHuntingSysResetChallengeBoss, func() pb3.Message {
		return &pb3.CommonSt{}
	}, handleOfflineSectHuntingSysResetChallengeBoss)
	initSectHuntingGm()
}

func initSectHuntingGm() {
	gmevent.Register("SectHuntingSys.addDamage", func(player iface.IPlayer, args ...string) bool {
		obj := player.GetSysObj(sysdef.SiSectHunting)
		if obj == nil || !obj.IsOpen() {
			return false
		}
		s, ok := obj.(*SectHuntingSys)
		if !ok {
			return false
		}
		s.getData().Damage += utils.AtoUint64(args[0])
		s.s2cInfo()
		return true
	}, 1)
	gmevent.Register("SectHuntingSys.setChallengeBoss", func(player iface.IPlayer, args ...string) bool {
		engine.SendPlayerMessage(player.GetId(), gshare.OfflineSectHuntingSysResetChallengeBoss, &pb3.CommonSt{
			U32Param: utils.AtoUint32(args[0]),
		})
		return true
	}, 1)
	gmevent.Register("SectHuntingSys.resetDailyEnter", func(player iface.IPlayer, args ...string) bool {
		engine.SendPlayerMessage(player.GetId(), gshare.OfflineSectHuntingSysResetLiveTime, &pb3.CommonSt{})
		return true
	}, 1)
}
