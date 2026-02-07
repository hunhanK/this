/**
 * @Author: LvYuMeng
 * @Date: 2024/7/29
 * @Desc:
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/actsweepmgr"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/engine/series"
	"jjyz/gameserver/fightworker"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/inffairyplacemgr"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
)

var ultimateBossEndTimes uint32

type UltimateBossSys struct {
	Base
}

func (s *UltimateBossSys) data() *pb3.UnltimateBossData {
	binary := s.GetBinaryData()
	if nil == binary.UnltimateBossData {
		binary.UnltimateBossData = &pb3.UnltimateBossData{}
	}
	return binary.UnltimateBossData
}

func (s *UltimateBossSys) OnLogin() {
	s.owner.SetExtraAttr(attrdef.UlitimateBossGatherTimes, int64(s.data().GatherTimes))
}

func (s *UltimateBossSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *UltimateBossSys) OnOpen() {
	data := s.data()
	conf := jsondata.GetUltimateBossConf()
	for bossId := range conf.Boss {
		data.FollowIds = append(data.FollowIds, bossId)
	}
	s.s2cInfo()
}

func (s *UltimateBossSys) OnReconnect() {
	s.s2cInfo()
}

func (s *UltimateBossSys) s2cInfo() {
	s.SendProto3(70, 0, &pb3.S2C_70_0{PlayerData: s.data()})
	s.SendProto3(70, 3, &pb3.S2C_70_3{Data: getUltimateBossInfo().BossInfo})
	s.reqFbInfo()
}

func (s *UltimateBossSys) reqFbInfo() {
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CReqUltimateBossFbInfo, &pb3.CommonSt{
		U64Param:  s.owner.GetId(),
		U32Param:  engine.GetPfId(),
		U32Param2: engine.GetServerId(),
	})
	if err != nil {
		s.LogError("err:%v", err)
		return
	}
}

func (s *UltimateBossSys) c2sEnter(msg *base.Message) error {
	conf := jsondata.GetUltimateBossConf()
	if nil == conf {
		return neterror.ConfNotFoundError("conf is nil")
	}
	if gshare.GetOpenServerDay() < conf.ServerDay {
		return neterror.ParamsInvalidError("open server day not ok")
	}
	req := &pb3.EnterFubenHdl{
		SceneId: conf.SceneId,
	}
	err := s.owner.EnterFightSrv(base.SmallCrossServer, fubendef.EnterFbHdl, req)
	if err != nil {
		return err
	}
	return nil
}

func (s *UltimateBossSys) c2sFollow(msg *base.Message) error {
	var req pb3.C2S_70_6
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	data := s.data()
	if req.GetIsFollow() {
		if !pie.Uint32s(data.FollowIds).Contains(req.GetBossId()) {
			data.FollowIds = append(data.FollowIds, req.GetBossId())
		}
	} else {
		data.FollowIds = pie.Uint32s(data.FollowIds).Filter(func(u uint32) bool {
			return u != req.GetBossId()
		})
	}
	s.SendProto3(70, 6, &pb3.S2C_70_6{
		BossId:   req.GetBossId(),
		IsFollow: req.GetIsFollow(),
	})
	return nil
}

func (s *UltimateBossSys) c2sReceiveFirstAward(msg *base.Message) error {
	var req pb3.C2S_70_7
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf := jsondata.GetUltimateBossConf()
	if nil == conf {
		return neterror.ConfNotFoundError("conf is nil")
	}
	bossConf, ok := conf.Boss[req.GetBossId()]
	if !ok {
		return neterror.ConfNotFoundError("no boss %d conf", req.GetBossId())
	}
	data := s.data()
	bossData := getUltimateBossInfo()
	bossInfo, ok := bossData.BossInfo[req.GetBossId()]
	if !ok {
		return neterror.ParamsInvalidError("no boss info")
	}
	if !bossInfo.CanRevFirstAward {
		s.owner.SendTipMsg(tipmsgid.AwardCondNotEnough)
		return nil
	}
	if pie.Uint32s(data.FirstKillAwardRev).Contains(req.GetBossId()) {
		s.owner.SendTipMsg(tipmsgid.AwardCondNotEnough)
		return nil
	}

	data.FirstKillAwardRev = append(data.FirstKillAwardRev, req.GetBossId())
	engine.GiveRewards(s.owner, bossConf.FirstKillAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogUltimateBossFirstKillAward})

	s.SendProto3(70, 7, &pb3.S2C_70_7{BossId: req.GetBossId()})
	return nil
}

func (s *UltimateBossSys) onPersonalGather(st *pb3.SyncUltimateBossGather) {
	data := s.data()
	data.GatherTimes++
	s.owner.SetExtraAttr(attrdef.UlitimateBossGatherTimes, int64(data.GatherTimes))
	event.TriggerSysEvent(custom_id.SeActSweepNewActSweepAdd, actsweepmgr.ActSweepByUltimateBoss, s.GetOwner().GetId())

	rewards := jsondata.FilterRewardByOption(jsondata.Pb3RewardVecToStdRewardVec(st.Awards),
		jsondata.WithFilterRewardOptionByJob(s.owner.GetJob()),
		jsondata.WithFilterRewardOptionBySex(s.owner.GetSex()),
	)

	if len(rewards) > 0 {
		engine.GiveRewards(s.owner, rewards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogUltimateBossGather,
		})
	}
	s.owner.SendShowRewardsPop(rewards)
}

func (s *UltimateBossSys) onNewDay() {
	data := s.data()
	data.GatherTimes = 0
	s.owner.SetExtraAttr(attrdef.UlitimateBossGatherTimes, int64(s.data().GatherTimes))
}

func onUltimateBossGather(player iface.IPlayer, buf []byte) {
	msg := &pb3.SyncUltimateBossGather{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		player.LogError("SyncUltimateBossGather unmarshal err %v", err)
		return
	}

	sys, ok := player.GetSysObj(sysdef.SiUltimateBoss).(*UltimateBossSys)
	if !ok {
		return
	}
	sys.onPersonalGather(msg)
}

func getUltimateBossInfo() *pb3.UltimateBossInfo {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.UltimateBossInfo {
		globalVar.UltimateBossInfo = &pb3.UltimateBossInfo{}
	}
	if nil == globalVar.UltimateBossInfo.BossInfo {
		globalVar.UltimateBossInfo.BossInfo = make(map[uint32]*pb3.UltimateBoss)
	}
	return globalVar.UltimateBossInfo
}

func syncUltimateBossKill(buf []byte) {
	var req pb3.CommonSt
	if err := pb3.Unmarshal(buf, &req); nil != err {
		return
	}
	campId := req.U32Param
	bossId := req.U32Param2
	timeStamp := req.U32Param3
	hostInfo := fightworker.GetHostInfo(base.SmallCrossServer)
	data := getUltimateBossInfo()
	if _, ok := data.BossInfo[bossId]; !ok {
		data.BossInfo[bossId] = &pb3.UltimateBoss{}
	}
	bossInfo := data.BossInfo[bossId]
	bossInfo.LastKillCampId = campId
	bossInfo.LastKillTime = timeStamp
	if campId == uint32(hostInfo.Camp) {
		bossInfo.CanRevFirstAward = true
	}
	rsp := &pb3.S2C_70_4{BossId: bossId, Boss: bossInfo}
	engine.Broadcast(chatdef.CIWorld, 0, 70, 4, rsp, 0)
}

func onUltimateBossNewDay(actor iface.IPlayer, args ...interface{}) {
	sys, ok := actor.GetSysObj(sysdef.SiUltimateBoss).(*UltimateBossSys)
	if !ok {
		return
	}

	if !sys.IsOpen() {
		return
	}

	sys.onNewDay()
}

func handleF2GSyncUltimateBossMgrEndTime(buf []byte) {
	var req pb3.CommonSt
	err := pb3.Unmarshal(buf, &req)
	if err != nil {
		logger.LogError("F2GSyncUltimateBossMgrEndTime err:%v", err)
		return
	}
	ultimateBossEndTimes = req.U32Param
	ultimateBoss := jsondata.GetActSweepConfByUltimateBoss()
	if ultimateBoss == nil {
		return
	}
	if ultimateBoss.OpenCond != nil && ultimateBossEndTimes < ultimateBoss.OpenCond.ActEndTimes {
		return
	}
	event.TriggerSysEvent(custom_id.SeActSweepNewActSweepAdd, actsweepmgr.ActSweepByUltimateBoss)
}

var singleUltimateBossController = &UltimateBossController{}

type UltimateBossController struct {
	actsweepmgr.Base
}

func (receiver *UltimateBossController) GetCanUseTimes(id uint32, playerId uint64) (useTimes uint32) {
	player := manager.GetPlayerPtrById(playerId)
	if player == nil {
		return
	}

	ultimateBoss := jsondata.GetActSweepConfByUltimateBoss()
	if ultimateBoss == nil {
		return
	}

	if ultimateBoss.OpenCond != nil && ultimateBoss.OpenCond.ActEndTimes > ultimateBossEndTimes {
		return
	}

	obj := player.GetSysObj(sysdef.SiUltimateBoss)
	if obj == nil || !obj.IsOpen() {
		return
	}
	_, ok := obj.(*UltimateBossSys)
	if !ok {
		return
	}
	return ultimateBoss.SweepTimes
}

func (receiver *UltimateBossController) GetUseTimes(id uint32, playerId uint64) (useTimes uint32, ret bool) {
	player := manager.GetPlayerPtrById(playerId)
	if player == nil {
		return
	}

	ultimateBoss := jsondata.GetActSweepConfByUltimateBoss()
	if ultimateBoss == nil {
		return
	}

	if ultimateBoss.OpenCond != nil && ultimateBoss.OpenCond.ActEndTimes > ultimateBossEndTimes {
		return
	}

	obj := player.GetSysObj(sysdef.SiUltimateBoss)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*UltimateBossSys)
	if !ok {
		return
	}
	return sys.data().GatherTimes, true
}

func (receiver *UltimateBossController) AddUseTimes(_ uint32, times uint32, playerId uint64) {
	player := manager.GetPlayerPtrById(playerId)
	if player == nil {
		return
	}
	obj := player.GetSysObj(sysdef.SiUltimateBoss)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*UltimateBossSys)
	if !ok {
		return
	}
	data := sys.data()
	data.GatherTimes += times
	player.SetExtraAttr(attrdef.UlitimateBossGatherTimes, int64(data.GatherTimes))
}

func handleUltimateBossAuctionReq(buf []byte) {
	var req pb3.C2GUltimateBossAuctionReq
	if err := pb3.Unmarshal(buf, &req); nil != err {
		return
	}

	conf := jsondata.GetUltimateBossConf()
	if nil == conf {
		return
	}

	var gatherConf *jsondata.UltimateBossGather
	for _, line := range conf.Gather {
		if line.Type == custom_id.UltimateBossTreasureTypeAuctionAward {
			gatherConf = line
			break
		}
	}

	if nil == gatherConf {
		return
	}

	averageLevel := manager.GRankMgrIns.GetAverageScoreByInterval(gshare.RankTypeLevel, 1, int(conf.TopLevelRange))
	averageCircle := manager.GRankMgrIns.GetAverageScoreByInterval(gshare.RankTypeBoundary, 1, int(conf.TopCircleRange))

	rewards := gatherConf.GetDropAward(uint32(averageLevel), uint32(averageCircle), req.IsFirstCamp)
	if len(rewards) == 0 {
		logger.LogError("no rewards, level:%d,circle:%d", averageLevel, averageCircle)
		return
	}

	rewards = jsondata.FilterRewardByOption(rewards, jsondata.WithFilterRewardOptionByOpenDayRange(gshare.GetOpenServerDay()))
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CUltimateBossAuctionRet, &pb3.G2CUltimateBossAuctionRet{
		Camp:        req.Camp,
		IsFirstCamp: req.IsFirstCamp,
		Awards:      jsondata.StdRewardVecToPb3RewardVec(rewards),
	})
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
}

// 开奖
func doOpenUltimateBossRedPaper(redPaperId, bossId uint32, rankList []*pb3.OneRankItem, player iface.IPlayer, openDay uint32) {
	conf := jsondata.GetUltimateBossConf()
	if nil == conf {
		return
	}

	rpConf, ok := conf.RedPaper[redPaperId]
	if !ok {
		logger.LogError("not found conf %d", redPaperId)
		return
	}

	var rankIdx uint32
	for i, line := range rankList {
		rankIdx = uint32(i + 1)
		for _, rankConf := range rpConf.RankList {
			if rankIdx >= rankConf.MinRank && rankIdx <= rankConf.MaxRank {
				rewards := jsondata.FilterRewardByOption(rankConf.Rewards, jsondata.WithFilterRewardOptionByOpenDayRange(openDay))
				mailmgr.SendMailToActor(line.Id, &mailargs.SendMailSt{
					ConfId:  common.Mail_UltimateBossRedPpaer,
					Content: &mailargs.RankArgs{Rank: rankIdx},
					Rewards: rewards,
				})
				break
			}
		}
	}

	hdl, err := series.AllocSeries()
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}

	data := getUltimateBossRedPaperInfo()
	record := &pb3.UltimateBossRedPaperInfo{
		Rank:       rankList,
		BossId:     bossId,
		TimeStamp:  time_util.NowSec(),
		RedPaperId: redPaperId,
		SrvOpenDay: openDay,
	}

	data[hdl] = record
	if nil != player {
		player.ChannelChat(&pb3.C2S_5_1{
			Channel:     chatdef.CIWorld,
			Params:      fmt.Sprintf("%d,%d", hdl, record.TimeStamp),
			ContentType: chatdef.ContentUltimateBossRedPaper,
		}, false)
	} else {
		sendSysChatWordMsgDirectly(&pb3.S2C_5_1{
			Channel:       chatdef.CIWorld,
			Params:        fmt.Sprintf("%d,%d", hdl, record.TimeStamp),
			ContentType:   chatdef.ContentUltimateBossRedPaper,
			RobotConfigId: chatdef.ChatVTuberRobotSys,
		}, true)
	}

	rsp, err := packetUltimateBossRedPaperInfo(hdl)
	if nil != err {
		logger.LogError("err:%v", err)
		return
	}

	engine.Broadcast(chatdef.CIWorld, 0, 70, 82, rsp, 0)
}

func packetUltimateBossRedPaperInfo(hdl uint64) (*pb3.S2C_70_82, error) {
	data := getUltimateBossRedPaperInfo()
	record, ok := data[hdl]
	if !ok {
		return nil, neterror.InternalError("record %d is nil", hdl)
	}

	rsp := &pb3.S2C_70_82{
		Handle:     hdl,
		BossId:     record.BossId,
		RedPaperId: record.RedPaperId,
		SrvOpenDay: record.SrvOpenDay,
	}

	for _, v := range record.Rank {
		if simply := manager.GetSimplyData(v.Id); nil != simply {
			rsp.Rank = append(rsp.Rank, &pb3.UltimateBossRedPaperRank{
				ActorId: v.Id,
				Damage:  v.Score,
				Name:    simply.Name,
			})
		}
	}

	return rsp, nil
}

func handleC2GUltimateBossSend(buf []byte) {
	var req pb3.C2GUltimateBossSend
	if err := pb3.Unmarshal(buf, &req); nil != err {
		return
	}

	conf := jsondata.GetUltimateBossConf()
	if nil == conf {
		return
	}

	var itemId uint32
	var rewards jsondata.StdRewardVec

	for _, v := range jsondata.Pb3RewardVecToStdRewardVec(req.Awards) {
		if itemdef.IsUltimateBossRedPaperItem(jsondata.GetItemType(v.Id)) {
			itemId = v.Id
		} else {
			rewards = append(rewards, v)
		}
	}

	logger.LogInfo("发放极品boss宗主红包：%d", itemId)

	if useConf := jsondata.GetUseItemConfById(itemId); nil != useConf && len(useConf.Param) > 0 {
		doOpenUltimateBossRedPaper(useConf.Param[0], req.BossId, req.ActorIds, nil, gshare.GetOpenServerDay())
	}
}

func getUltimateBossRedPaperInfo() map[uint64]*pb3.UltimateBossRedPaperInfo {
	globalVar := gshare.GetStaticVar()
	if globalVar.UltimateBossRedPaperInfo == nil {
		globalVar.UltimateBossRedPaperInfo = map[uint64]*pb3.UltimateBossRedPaperInfo{}
	}
	return globalVar.UltimateBossRedPaperInfo
}

func useItemUltimateBossRedPaper(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	itemSt := player.GetItemByHandle(param.Handle)
	if nil == itemSt.Ext || nil == itemSt.Ext.UltimateBossRedPaper {
		return true, true, param.Count
	}

	if len(conf.Param) < 1 {
		return false, false, 0
	}

	ext := itemSt.Ext.UltimateBossRedPaper

	doOpenUltimateBossRedPaper(conf.Param[0], ext.BossId, ext.Ranks, player, ext.SrvOpenDay)

	return true, true, param.Count
}

func init() {
	RegisterSysClass(sysdef.SiUltimateBoss, func() iface.ISystem {
		return &UltimateBossSys{}
	})

	net.RegisterSysProtoV2(70, 1, sysdef.SiUltimateBoss, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*UltimateBossSys).c2sEnter
	})

	net.RegisterSysProtoV2(70, 6, sysdef.SiUltimateBoss, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*UltimateBossSys).c2sFollow
	})

	net.RegisterSysProtoV2(70, 7, sysdef.SiUltimateBoss, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*UltimateBossSys).c2sReceiveFirstAward
	})

	net.RegisterProto(70, 82, func(player iface.IPlayer, msg *base.Message) error {
		var req pb3.C2S_70_82
		if err := msg.UnPackPb3Msg(&req); nil != err {
			return err
		}
		rsp, err := packetUltimateBossRedPaperInfo(req.Handle)
		if nil != err {
			return err
		}
		player.SendProto3(70, 82, rsp)
		return nil
	})

	event.RegActorEvent(custom_id.AeNewDay, onUltimateBossNewDay)
	event.RegSysEvent(custom_id.SeNewDayArrive, onUltimateBossSeNewDay)

	engine.RegisterActorCallFunc(playerfuncid.UltimateBossGather, onUltimateBossGather)
	engine.RegisterSysCall(sysfuncid.F2GSyncUltimateBossMgrEndTime, handleF2GSyncUltimateBossMgrEndTime)
	engine.RegisterSysCall(sysfuncid.C2GSyncUltimateBossKill, syncUltimateBossKill)
	engine.RegisterSysCall(sysfuncid.C2GUltimateBossAuctionReq, handleUltimateBossAuctionReq)
	engine.RegisterSysCall(sysfuncid.C2GUltimateBossSend, handleC2GUltimateBossSend)

	actsweepmgr.Reg(actsweepmgr.ActSweepByUltimateBoss, singleUltimateBossController)

	miscitem.RegCommonUseItemHandle(itemdef.UseItemUltimateBossRedPaper, useItemUltimateBossRedPaper)

	gmevent.Register("showCampMaster", func(player iface.IPlayer, args ...string) bool {
		leaderId := inffairyplacemgr.GetLocalInfFairyPlaceMgr().GetMasterLeaderId()
		var name string
		if s := manager.GetSimplyData(leaderId); nil != s {
			name = s.Name
		}
		player.SendTipMsg(tipmsgid.TpStr, fmt.Sprintf("%s,%d", name, leaderId))
		return true
	}, 1)
}

func onUltimateBossSeNewDay(args ...interface{}) {
	ultimateBossEndTimes = 0

	conf := jsondata.GetUltimateBossConf()
	if nil == conf {
		return
	}
	data := getUltimateBossRedPaperInfo()
	newData := map[uint64]*pb3.UltimateBossRedPaperInfo{}
	nowSec := time_util.NowSec()
	for hdl, v := range data {
		record := v
		if record.TimeStamp+conf.MsgTimeOut <= nowSec {
			continue
		}
		newData[hdl] = record
	}
	return
}
