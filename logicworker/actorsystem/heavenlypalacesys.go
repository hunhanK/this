/**
 * @Author: zjj
 * @Date: 2025年7月24日
 * @Desc: 天宫秘境副本
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"sort"
)

type HeavenlyPalaceSys struct {
	Base
}

func (s *HeavenlyPalaceSys) s2cInfo() {
	s.SendProto3(9, 60, &pb3.S2C_9_60{
		Data: s.getData(),
	})
}

func (s *HeavenlyPalaceSys) getData() *pb3.HeavenlyPalaceData {
	data := s.GetBinaryData().HeavenlyPalaceData
	if data == nil {
		s.GetBinaryData().HeavenlyPalaceData = &pb3.HeavenlyPalaceData{}
		data = s.GetBinaryData().HeavenlyPalaceData
	}
	if data.DailyTimesStat == nil {
		data.DailyTimesStat = make(map[uint32]uint32)
	}
	if data.FollowBossMap == nil {
		data.FollowBossMap = make(map[uint32]bool)
	}
	return data
}

func (s *HeavenlyPalaceSys) OnReconnect() {
	s.s2cInfo()
}

func (s *HeavenlyPalaceSys) OnLogin() {
	s.s2cInfo()
}

func (s *HeavenlyPalaceSys) OnOpen() {
	s.s2cInfo()
}

func (s *HeavenlyPalaceSys) onNewDay() {
	data := s.getData()
	data.DailyTimesStat = make(map[uint32]uint32)
	s.s2cInfo()
}

// 召唤怪物
func (s *HeavenlyPalaceSys) c2sCallBoss(msg *base.Message) error {
	var req pb3.C2S_9_61
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return neterror.Wrap(err)
	}
	config := jsondata.GetHeavenlyPalaceFbConfig()
	if config == nil || config.BossList == nil {
		return neterror.ConfNotFoundError("conf not found")
	}

	monId := req.MonId
	bossConf := config.BossList[monId]
	if bossConf == nil {
		return neterror.ConfNotFoundError("%d not found conf", monId)
	}

	owner := s.GetOwner()
	err = owner.CallActorFunc(actorfuncid.G2FCallHeavenlyPalaceCreateBossCheckReq, &pb3.CommonSt{
		U32Param: monId,
	})
	if err != nil {
		return neterror.Wrap(err)
	}
	return nil
}

// 获取记录
func (s *HeavenlyPalaceSys) c2sGetRecords(msg *base.Message) error {
	var req pb3.C2S_9_62
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return neterror.Wrap(err)
	}
	s.SendProto3(9, 62, &pb3.S2C_9_62{
		MonId: req.MonId,
		Logs:  getHeavenlyPalaceRecordList(req.MonId),
	})
	return nil
}

// 获取场景boss列表
func (s *HeavenlyPalaceSys) c2sGetSceneBossList(_ *base.Message) error {
	return s.owner.CallActorMediumCrossFunc(actorfuncid.G2FCallHeavenlyPalaceSceneBossReq, &pb3.CommonSt{})
}

// 进入副本
func (s *HeavenlyPalaceSys) c2sEnterFb(_ *base.Message) error {
	owner := s.GetOwner()
	err := owner.EnterFightSrv(base.MediumCrossServer, fubendef.EnterHeavenlyPalaceFb, &pb3.CommonSt{})
	if err != nil {
		return neterror.Wrap(err)
	}
	return nil
}

// 关注boss
func (s *HeavenlyPalaceSys) c2sFollowBoss(msg *base.Message) error {
	var req pb3.C2S_9_66
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return neterror.ParamsInvalidError("unmarshal proto failed err: %s", err)
	}
	config := jsondata.GetHeavenlyPalaceFbConfig()
	monId := req.MonId
	if config == nil || config.BossList[monId] == nil {
		return neterror.ConfNotFoundError("config not found")
	}
	s.getData().FollowBossMap[monId] = !s.getData().FollowBossMap[monId]
	s.SendProto3(9, 66, &pb3.S2C_9_66{
		MonId:      monId,
		NeedFollow: s.getData().FollowBossMap[monId],
	})
	return nil
}

func getHeavenlyPalaceRecordList(monId uint32) []*pb3.HeavenlyPalaceRecord {
	staticVar := gshare.GetStaticVar()
	if staticVar == nil {
		return nil
	}
	if staticVar.HeavenlyPalaceRecords == nil {
		staticVar.HeavenlyPalaceRecords = make(map[uint32]*pb3.HeavenlyPalaceRecordList)
	}
	list, ok := staticVar.HeavenlyPalaceRecords[monId]
	if !ok {
		list = &pb3.HeavenlyPalaceRecordList{}
		staticVar.HeavenlyPalaceRecords[monId] = list
	}
	return list.List
}

func setHeavenlyPalaceRecordList(monId uint32, logs []*pb3.HeavenlyPalaceRecord) {
	staticVar := gshare.GetStaticVar()
	if staticVar == nil {
		return
	}
	if staticVar.HeavenlyPalaceRecords == nil {
		staticVar.HeavenlyPalaceRecords = make(map[uint32]*pb3.HeavenlyPalaceRecordList)
	}
	list, ok := staticVar.HeavenlyPalaceRecords[monId]
	if !ok {
		list = &pb3.HeavenlyPalaceRecordList{}
		staticVar.HeavenlyPalaceRecords[monId] = list
	}
	list.List = logs
}

// 添加击杀记录
func handleHeavenlyPalaceAppendRecord(buf []byte) {
	var req = &pb3.HeavenlyPalaceRecord{}
	err := pb3.Unmarshal(buf, req)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
	if req.MonId == 0 || req.ActorId == 0 {
		return
	}
	config := jsondata.GetHeavenlyPalaceFbConfig()
	if config == nil {
		return
	}
	limit := config.RecordLimit
	recordList := getHeavenlyPalaceRecordList(req.MonId)
	recordList = append(recordList, req)
	// 从大到小排序
	sort.Slice(recordList, func(i, j int) bool {
		return recordList[i].CreatedAt > recordList[j].CreatedAt
	})
	if uint32(len(recordList)) <= limit {
		setHeavenlyPalaceRecordList(req.MonId, recordList)
		return
	}
	var ret = make([]*pb3.HeavenlyPalaceRecord, limit)
	copy(ret, recordList)
	setHeavenlyPalaceRecordList(req.MonId, ret)
}

// 清除创建boss
func handleFGHeavenlyPalaceBossLeave(buf []byte) {
	var req = &pb3.HeavenlyPalaceBoss{}
	err := pb3.Unmarshal(buf, req)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
	if req.MonId == 0 || req.CallActorId == 0 {
		return
	}
	dataBase := manager.GetData(req.CallActorId, gshare.ActorDataBase)
	if dataBase == nil {
		return
	}
	engine.SendPlayerMessage(req.CallActorId, gshare.OfflineKillHeavenlyPalaceBoss, req)
}

// 返回材料
func handleFGReturnHeavenlyPalaceBossConsume(buf []byte) {
	var req = &pb3.HeavenlyPalaceBoss{}
	err := pb3.Unmarshal(buf, req)
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
	if req.MonId == 0 || req.CallActorId == 0 {
		return
	}
	dataBase := manager.GetData(req.CallActorId, gshare.ActorDataBase)
	if dataBase == nil {
		return
	}
	engine.SendPlayerMessage(req.CallActorId, gshare.OfflineReturnHeavenlyPalaceBossConsume, &pb3.CommonSt{U32Param: req.MonId})
}

// 创建Boss检测返回
func handleG2FCallHeavenlyPalaceCreateBossCheckRet(player iface.IPlayer, buf []byte) {
	obj := player.GetSysObj(sysdef.SiHeavenlyPalace)
	if obj == nil || !obj.IsOpen() {
		return
	}
	var req pb3.CommonSt
	err := pb3.Unmarshal(buf, &req)
	if err != nil {
		player.LogError("err:%v", err)
		return
	}
	config := jsondata.GetHeavenlyPalaceFbConfig()
	if config == nil || config.BossList == nil {
		player.LogError("conf not found")
		return
	}

	monId := req.U32Param
	bossConf := config.BossList[monId]
	if bossConf == nil {
		player.LogError("%d monster conf not found")
		return
	}
	if len(bossConf.Consume) == 0 || !player.ConsumeByConf(bossConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogHeavenlyPalaceCreateBoss}) {
		player.LogError("%d monster consume failed")
		return
	}

	err = player.CallActorFunc(actorfuncid.G2FCallHeavenlyPalaceCreateBossReq, &req)
	if err != nil {
		player.LogError("err:%v", err)
	}

	logworker.LogPlayerBehavior(player, pb3.LogId_LogHeavenlyPalaceCreateBoss, &pb3.LogPlayerCounter{
		NumArgs: uint64(monId),
		StrArgs: fmt.Sprintf("%d_%d", req.U32Param2, req.U32Param3),
	})
}

// 创建Boss返回
func handleG2FCallHeavenlyPalaceCreateBossRet(player iface.IPlayer, buf []byte) {
	var req pb3.G2FCallHeavenlyPalaceCreateBossRet
	err := pb3.Unmarshal(buf, &req)
	if err != nil {
		player.LogError("err:%v", err)
		return
	}
	obj := player.GetSysObj(sysdef.SiHeavenlyPalace)
	if obj == nil || !obj.IsOpen() {
		return
	}
	s, ok := obj.(*HeavenlyPalaceSys)
	if !ok {
		return
	}
	data := s.getData()
	data.CallBoss = req.Boss
	player.SendProto3(9, 61, &pb3.S2C_9_61{
		Boss: req.Boss,
		Ret:  req.Ret,
	})
	if req.Boss == nil {
		player.LogError("req boss is nil %+v", &req)
		return
	}
	if !req.Ret {
		engine.SendPlayerMessage(player.GetId(), gshare.OfflineReturnHeavenlyPalaceBossConsume, &pb3.CommonSt{
			U32Param: req.Boss.MonId,
		})
	}
}

// 处理奖励次数
func handleHeavenlyPalaceReduceTimes(player iface.IPlayer, buf []byte) {
	var req pb3.HeavenlyPalaceReduceTimes
	if err := pb3.Unmarshal(buf, &req); err != nil {
		return
	}
	obj := player.GetSysObj(sysdef.SiHeavenlyPalace)
	if obj == nil {
		return
	}
	s, ok := obj.(*HeavenlyPalaceSys)
	if !ok {
		return
	}
	data := s.getData()
	config := jsondata.GetHeavenlyPalaceFbConfig()
	if config == nil {
		player.LogError("conf not found")
		return
	}
	monId := req.MonId
	bossConf, exists := config.BossList[monId]
	if !exists {
		player.LogError("Boss %d Config not found", monId)
		return
	}
	curTimes := data.DailyTimesStat[monId]
	//检查奖励次数
	if curTimes >= uint32(bossConf.DailyTimes) {
		return
	}
	//更新奖励次数
	newTimes := curTimes + 1
	data.DailyTimesStat[monId] = newTimes
	// 发奖励
	if len(req.Awards) > 0 {
		stdRewardVec := jsondata.Pb3RewardVecToStdRewardVec(req.Awards)
		engine.GiveRewards(player, stdRewardVec, common.EngineGiveRewardParam{
			LogId:  pb3.LogId_LogHeavenlyPalaceGiveAwards,
			NoTips: true,
		})
		player.SendShowRewardsPop(stdRewardVec)
	}
	s.s2cInfo()
	logworker.LogPlayerBehavior(player, pb3.LogId_LogHeavenlyPalaceGiveAwards, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.MonId),
		StrArgs: fmt.Sprintf("%d", newTimes),
	})
}

// 离线消息-处理怪物被击杀
func handleOfflineKillHeavenlyPalaceBoss(player iface.IPlayer, msg pb3.Message) {
	heavenlyPalaceBoss, ok := msg.(*pb3.HeavenlyPalaceBoss)
	if !ok {
		return
	}
	obj := player.GetSysObj(sysdef.SiHeavenlyPalace)
	if obj == nil || !obj.IsOpen() {
		return
	}
	s, ok := obj.(*HeavenlyPalaceSys)
	if !ok {
		return
	}
	data := s.getData()
	if data.CallBoss != nil && data.CallBoss.Hdl == heavenlyPalaceBoss.Hdl {
		data.CallBoss = nil
		return
	}
}

// 离线消息-返回怪物创建消耗
func handleOfflineReturnHeavenlyPalaceBossConsume(player iface.IPlayer, _ pb3.Message) {
	obj := player.GetSysObj(sysdef.SiHeavenlyPalace)
	if obj == nil || !obj.IsOpen() {
		return
	}
	s, ok := obj.(*HeavenlyPalaceSys)
	if !ok {
		return
	}
	data := s.getData()
	if data.CallBoss == nil {
		return
	}
	config := jsondata.GetHeavenlyPalaceFbConfig()
	if config == nil || config.BossList == nil {
		player.LogError("conf not found")
		return
	}
	monId := data.CallBoss.MonId
	data.CallBoss = nil
	bossConf := config.BossList[monId]
	if bossConf == nil {
		player.LogError("%d monster conf not found")
		return
	}
	if len(bossConf.ReturnAwards) == 0 {
		return
	}
	mailmgr.SendMailToActor(player.GetId(), &mailargs.SendMailSt{
		ConfId:  config.MailId,
		Rewards: bossConf.ReturnAwards,
	})
	s.s2cInfo()
}

// 进入战斗服 同步今日奖励次数
func handleHeavenlyPalaceToFightSrv(player iface.IPlayer, _ ...interface{}) {
	obj := player.GetSysObj(sysdef.SiHeavenlyPalace)
	if obj == nil || !obj.IsOpen() {
		return
	}
	s, ok := obj.(*HeavenlyPalaceSys)
	if !ok {
		return
	}
	data := s.getData()
	err := player.CallActorFunc(actorfuncid.HeavenlyPalaceSaveAwardTimes, &pb3.HeavenlyPalaceData{DailyTimesStat: data.DailyTimesStat})
	if err != nil {
		player.LogError("err:%v", err)
		return
	}
}

// 注册跨天 重置奖励次数
func handleHeavenlyPalaceNewDay(player iface.IPlayer, _ ...interface{}) {
	obj := player.GetSysObj(sysdef.SiHeavenlyPalace)
	if obj == nil {
		return
	}
	sys, ok := obj.(*HeavenlyPalaceSys)
	if !ok {
		return
	}
	sys.onNewDay()
	handleHeavenlyPalaceToFightSrv(player)
}

func init() {
	RegisterSysClass(sysdef.SiHeavenlyPalace, func() iface.ISystem {
		return &HeavenlyPalaceSys{}
	})
	net.RegisterSysProtoV2(9, 61, sysdef.SiHeavenlyPalace, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*HeavenlyPalaceSys).c2sCallBoss
	})
	net.RegisterSysProtoV2(9, 62, sysdef.SiHeavenlyPalace, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*HeavenlyPalaceSys).c2sGetRecords
	})
	net.RegisterSysProtoV2(9, 63, sysdef.SiHeavenlyPalace, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*HeavenlyPalaceSys).c2sGetSceneBossList
	})
	net.RegisterSysProtoV2(9, 64, sysdef.SiHeavenlyPalace, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*HeavenlyPalaceSys).c2sEnterFb
	})
	net.RegisterSysProtoV2(9, 66, sysdef.SiHeavenlyPalace, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*HeavenlyPalaceSys).c2sFollowBoss
	})
	event.RegActorEventL(custom_id.AeLoginFight, handleHeavenlyPalaceToFightSrv)
	event.RegActorEvent(custom_id.AeNewDay, handleHeavenlyPalaceNewDay)
	engine.RegisterSysCall(sysfuncid.FGHeavenlyPalaceKillBossRecord, handleHeavenlyPalaceAppendRecord)
	engine.RegisterSysCall(sysfuncid.FGHeavenlyPalaceBossLeave, handleFGHeavenlyPalaceBossLeave)
	engine.RegisterSysCall(sysfuncid.FGReturnHeavenlyPalaceBossConsume, handleFGReturnHeavenlyPalaceBossConsume)
	engine.RegisterActorCallFunc(playerfuncid.F2GCallHeavenlyPalaceCreateBossCheckRet, handleG2FCallHeavenlyPalaceCreateBossCheckRet)
	engine.RegisterActorCallFunc(playerfuncid.F2GCallHeavenlyPalaceCreateBossRet, handleG2FCallHeavenlyPalaceCreateBossRet)
	engine.RegisterActorCallFunc(playerfuncid.F2GHeavenlyPalaceReduceTimes, handleHeavenlyPalaceReduceTimes)
	engine.RegisterMessage(gshare.OfflineKillHeavenlyPalaceBoss, func() pb3.Message { return &pb3.HeavenlyPalaceBoss{} }, handleOfflineKillHeavenlyPalaceBoss)
	engine.RegisterMessage(gshare.OfflineReturnHeavenlyPalaceBossConsume, func() pb3.Message { return &pb3.CommonSt{} }, handleOfflineReturnHeavenlyPalaceBossConsume)
}
