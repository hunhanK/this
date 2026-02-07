/**
 * @Author: LvYuMeng
 * @Date: 2024/4/26
 * @Desc:
**/

package pyy

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/jobchange"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
	"jjyz/gameserver/logicworker/internal/timer"
	"jjyz/gameserver/logicworker/mailmgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"time"
)

type ConfessionForLoveSys struct {
	PlayerYYBase
}

var confessForLoveRank = make(map[uint32]*ConfessForLoveRank)

type ConfessForLoveRank struct {
	r            map[uint32]*base.Rank
	deliverTimer *time_util.Timer
}

func (s *ConfessionForLoveSys) getConfessLoveRank() *ConfessForLoveRank {
	conf := jsondata.GetYYConfessionForLoveConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return nil
	}
	if nil == confessForLoveRank[s.Id] {
		confessForLoveRank[s.Id] = &ConfessForLoveRank{}
	}
	if nil == confessForLoveRank[s.Id].r {
		confessForLoveRank[s.Id].r = make(map[uint32]*base.Rank, 2)
	}

	var maxRank uint32
	for _, v := range conf.Rank {
		maxRank = utils.MaxUInt32(maxRank, v.RankMax)
	}

	for _, sex := range confessionForLoveRankSex {
		if nil != confessForLoveRank[s.Id].r[sex] {
			continue
		}
		r := initConfessionForLoveRank(conf)
		confessForLoveRank[s.Id].r[sex] = r
	}

	return confessForLoveRank[s.Id]
}

func (s *ConfessionForLoveSys) formatGlobalData() *pb3.ConfessionForLoveRecord {
	if !s.IsOpen() {
		s.LogError("confession for love params nil")
		return nil
	}
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.ConfessionForLoveRecord {
		globalVar.ConfessionForLoveRecord = make(map[uint32]*pb3.ConfessionForLoveRecord)
	}

	idx := s.Id
	if nil == globalVar.ConfessionForLoveRecord[idx] {
		globalVar.ConfessionForLoveRecord[idx] = &pb3.ConfessionForLoveRecord{}
	}

	g := globalVar.ConfessionForLoveRecord[idx]
	if nil == g.PData {
		g.PData = make(map[uint64]*pb3.ConfessionForLovePlayer)
	}
	if nil == g.RankInfo {
		g.RankInfo = make(map[uint32]*pb3.ConfessionForLoveRecordRank, 2)
	}

	if g.StartTime == 0 || g.StartTime <= s.OpenTime {
		g.StartTime = s.OpenTime
		g.EndTime = s.EndTime
		g.ConfIdx = s.ConfIdx
	}

	return g
}

func (s *ConfessionForLoveSys) Login() {
	s.s2cInfo()
}

func (s *ConfessionForLoveSys) OnReconnect() {
	s.s2cInfo()
}

func (s *ConfessionForLoveSys) OnOpen() {
	s.reset()
	s.s2cInfo()
}

func (s *ConfessionForLoveSys) reset() {
	s.formatGlobalData()
	s.setTimer(false)
}

func (s *ConfessionForLoveSys) setTimer(reload bool) {
	if s.formatGlobalData().IsDeliver {
		return
	}

	if nil == confessForLoveRank[s.Id] {
		confessForLoveRank[s.Id] = &ConfessForLoveRank{}
	}

	if nil != confessForLoveRank[s.Id].deliverTimer {
		if !reload {
			return
		}
		confessForLoveRank[s.Id].deliverTimer.Stop()
	}

	conf := jsondata.GetYYConfessionForLoveConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}

	nowSec := time_util.NowSec()
	edTime := s.EndTime - conf.BeforeEndHour*60*60
	var left uint32
	if edTime <= nowSec {
		left = 0
	} else {
		left = edTime - nowSec
	}

	confessForLoveRank[s.Id].deliverTimer = timer.SetTimeout(time.Duration(left)*time.Second, func() {
		checkConfessionForLoveRankDeliver(s.Id)
	})
}

func (s *ConfessionForLoveSys) s2cInfo() {
	sendConfessForLoveInfo(s.Id, s.GetPlayer(), s.formatGlobalData())
}

func sendConfessForLoveInfo(id uint32, player iface.IPlayer, g *pb3.ConfessionForLoveRecord) {
	rsp := &pb3.S2C_53_80{ActiveId: id}
	if pData := g.PData[player.GetId()]; nil != pData {
		rsp.IsRev = pData.PraticipateRev
		rsp.MyScore = pData.Score
	}
	player.SendProto3(53, 80, rsp)
}

func (s *ConfessionForLoveSys) c2sRev(msg *base.Message) error {
	var req pb3.C2S_53_81
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf := jsondata.GetYYConfessionForLoveConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("confession for love conf is nil")
	}

	if s.isFinish() {
		return neterror.ParamsInvalidError("confession for love is over")
	}

	playerId := s.GetPlayer().GetId()
	g := s.formatGlobalData()

	pData := g.PData[playerId]

	if nil == pData || pData.Score < conf.ParticipateScore {
		s.GetPlayer().SendTipMsg(tipmsgid.AwardCondNotEnough)
		return nil
	}

	if pData.PraticipateRev {
		s.GetPlayer().SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	pData.PraticipateRev = true

	if len(conf.ParticipateAwards) > 0 {
		engine.GiveRewards(s.GetPlayer(), conf.ParticipateAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogConfessionForLoveParticipateAward})
	}

	s.s2cInfo()

	return nil
}

func GetConfessionForLoveRelRank(conf *jsondata.ConfessionForLoveConf, rank *pb3.ConfessionForLoveRank) {
	for _, v := range conf.Rank {
		if v.RankMax < rank.Rank || v.RankMin > rank.Rank {
			continue
		}
		if v.Score > rank.Score {
			rank.Rank = v.RankMax + 1
			continue
		}
		break
	}
}

func (s *ConfessionForLoveSys) c2sRankList(_ *base.Message) error {
	g := s.formatGlobalData()
	rsp := &pb3.S2C_53_82{
		ActiveId: s.Id,
		RankInfo: map[uint32]*pb3.ConfessionForLoveRankInfo{
			gshare.Male:   {},
			gshare.Female: {},
		},
	}

	playerId := s.GetPlayer().GetId()
	if nil != g.PData[playerId] {
		rsp.MyScore = g.PData[playerId].Score
	}

	conf := jsondata.GetYYConfessionForLoveConf(s.ConfName, s.ConfIdx)
	var maxRank uint32
	for _, v := range conf.Rank {
		maxRank = utils.MaxUInt32(maxRank, v.RankMax)
	}

	rankInfo := s.getConfessLoveRank()
	for sex, r := range rankInfo.r {
		rankIndex := uint32(0)
		rList := r.GetList(1, r.GetRankCount())
		for _, line := range rList {
			rankIndex++
			role, ok := manager.GetData(line.Id, gshare.ActorDataBase).(*pb3.PlayerDataBase)
			if !ok {
				continue
			}
			obj := &pb3.ConfessionForLoveRank{
				Rank:      rankIndex,
				ActorId:   line.Id,
				Name:      role.Name,
				Job:       role.Job,
				HeadFrame: role.HeadFrame,
				Head:      role.Head,
				Score:     uint32(line.Score),
			}
			GetConfessionForLoveRelRank(conf, obj)
			if obj.Rank == 1 {
				obj.Appear = manager.GetExtraAppearAttr(obj.ActorId)
			}
			rankIndex = obj.Rank
			rsp.RankInfo[sex].Rank = append(rsp.RankInfo[sex].Rank, obj)
			if rankIndex >= maxRank {
				break
			}
		}
	}
	s.SendProto3(53, 82, rsp)
	return nil
}

func (s *ConfessionForLoveSys) isFinish() bool {
	conf := jsondata.GetYYConfessionForLoveConf(s.ConfName, s.ConfIdx)
	if time_util.NowSec()+conf.BeforeEndHour*60*60 > s.EndTime {
		return true
	}
	g := s.formatGlobalData()
	return g.IsDeliver || g.IsOver
}

func (s *ConfessionForLoveSys) addScore(actorId uint64, score uint32) {
	g := s.formatGlobalData()
	if nil == g.PData[actorId] {
		g.PData[actorId] = &pb3.ConfessionForLovePlayer{}
	}
	g.PData[actorId].Score += score
}

func (s *ConfessionForLoveSys) onJobChange(job uint32) {
	if s.isFinish() {
		return
	}

	oldSex := s.GetPlayer().GetSex()
	newSex := custom_id.GetSexByJob(job)
	if oldSex == newSex {
		return
	}

	g := s.formatGlobalData()
	actorId := s.GetPlayer().GetId()

	data, ok := g.PData[actorId]
	if !ok {
		return
	}

	rankInfo := s.getConfessLoveRank()
	rankInfo.r[oldSex].Update(actorId, 0)
	rankInfo.r[newSex].Update(actorId, int64(data.Score))
}

func (s *ConfessionForLoveSys) onEvent(actorId uint64, score uint32) {
	if s.isFinish() {
		return
	}
	g := s.formatGlobalData()

	myId := s.GetPlayer().GetId()
	s.addScore(myId, score)
	s.s2cInfo()

	s.addScore(actorId, score)
	if actor := manager.GetPlayerPtrById(actorId); nil != actor {
		sendConfessForLoveInfo(s.Id, actor, g)
	}

	conf := jsondata.GetYYConfessionForLoveConf(s.ConfName, s.ConfIdx)
	if nil == conf || nil == conf.Rank {
		return
	}

	minScore := conf.Rank[0].Score
	for _, v := range conf.Rank {
		minScore = utils.MinUInt32(minScore, v.Score)
	}

	rankInfo := s.getConfessLoveRank()

	myScore := g.PData[myId].Score
	if myScore >= minScore {
		rankInfo.r[s.GetPlayer().GetSex()].Update(myId, int64(myScore))
	}

	target, ok := manager.GetData(actorId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
	if !ok {
		s.LogError("not found actorId(%d) info", actorId)
		return
	}

	targetScore := g.PData[actorId].Score
	if targetScore >= minScore {
		rankInfo.r[target.GetSex()].Update(actorId, int64(targetScore))
	}
}

func onConfessionForLoveEvent(player iface.IPlayer, args ...interface{}) {
	if len(args) < 3 {
		return
	}
	targetId := args[0].(uint64)
	score := args[2].(uint32)
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYConfessionForLove)
	for _, obj := range yyList {
		if s, ok := obj.(*ConfessionForLoveSys); ok && s.IsOpen() {
			s.onEvent(targetId, score)
		}
	}
}

func sendConfessionForLoveSeverMail(conf *jsondata.ConfessionForLoveConf, actorId uint64, sex uint32) {
	var roleName string
	if target, ok := manager.GetData(actorId, gshare.ActorDataBase).(*pb3.PlayerDataBase); ok {
		roleName = target.GetName()
	}
	awards := conf.MaleAwards
	mailId := common.Mail_ConfessionForLoveMaleAward

	if sex == gshare.Female {
		mailId = common.Mail_ConfessionForLoveFemaleAward
		awards = conf.FemaleAwards
	}

	argStr, _ := mailargs.MarshalMailArg(&mailargs.PlayerNameArgs{Name: roleName})

	mailmgr.AddSrvMailStr(uint16(mailId), argStr, awards)
}

func sendConfessionForLoveRankMail(conf *jsondata.ConfessionForLoveConf, actorId uint64, rank uint32) {
	for _, v := range conf.Rank {
		if rank >= v.RankMin && rank <= v.RankMax {
			data, ok := manager.GetData(actorId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
			if !ok {
				logger.LogError("not found actorId(%d) info", actorId)
				continue
			}
			mailmgr.SendMailToActor(actorId, &mailargs.SendMailSt{
				ConfId:  uint16(common.Mail_ConfessionForLoveRankAward),
				Content: &mailargs.RankArgs{Rank: rank},
				Rewards: jsondata.StdRewardFilterSex(data.Sex, v.Rewards),
			})
			return
		}
	}
}

func checkConfessionForLoveRankDeliver(id uint32) {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.ConfessionForLoveRecord || nil == globalVar.ConfessionForLoveRecord[id] {
		return
	}

	g := globalVar.ConfessionForLoveRecord[id]
	if g.IsDeliver || g.IsOver {
		return
	}

	if nil == confessForLoveRank[id] || nil == confessForLoveRank[id].r {
		return
	}

	pyyConf := jsondata.GetPlayerYYConf(id)
	if nil == pyyConf {
		return
	}

	yyConf := jsondata.GetYYConfessionForLoveConf(pyyConf.ConfName, g.ConfIdx)
	if nil == yyConf {
		return
	}

	var maxRank uint32
	for _, v := range yyConf.Rank {
		maxRank = utils.MaxUInt32(maxRank, v.RankMax)
	}

	g.IsDeliver = true

	logger.LogDebug("confession for love rank deliver start")

	for sex, r := range confessForLoveRank[id].r {
		rankIndex := uint32(0)
		rList := r.GetList(1, r.GetRankCount())

		logger.LogDebug("confession for love sex rank sex:%d rank list %v", sex, rList)

		for _, line := range rList {
			rankIndex++
			obj := &pb3.ConfessionForLoveRank{
				Rank:    rankIndex,
				ActorId: line.Id,
				Score:   uint32(line.Score),
			}
			GetConfessionForLoveRelRank(yyConf, obj)
			rankIndex = obj.Rank
			if obj.Rank == 1 {
				sendConfessionForLoveSeverMail(yyConf, obj.ActorId, sex)
			}
			sendConfessionForLoveRankMail(yyConf, obj.ActorId, obj.Rank)
			if rankIndex >= maxRank {
				break
			}
		}
	}

	logger.LogDebug("confession for love rank deliver end")
}

func initConfessionForLoveRank(conf *jsondata.ConfessionForLoveConf) *base.Rank {
	r := new(base.Rank)
	var maxRank uint32
	for _, v := range conf.Rank {
		maxRank = utils.MaxUInt32(maxRank, v.RankMax)
	}
	r.Init(maxRank)
	return r
}

var confessionForLoveRankSex = []uint32{gshare.Male, gshare.Female}

func forRangeConfessForLoveRecord(fn func(id uint32, record *pb3.ConfessionForLoveRecord, conf *jsondata.ConfessionForLoveConf)) {
	globalVar := gshare.GetStaticVar()
	for id, record := range globalVar.ConfessionForLoveRecord {
		pyyConf := jsondata.GetPlayerYYConf(id)
		if nil == pyyConf {
			continue
		}
		yyConf := jsondata.GetYYConfessionForLoveConf(pyyConf.ConfName, record.ConfIdx)
		if nil == yyConf {
			continue
		}
		fn(id, record, yyConf)
	}
}

func clearRangeConfessForLoveRecord() {
	globalVar := gshare.GetStaticVar()
	var delIds []uint32
	for id, record := range globalVar.ConfessionForLoveRecord {
		if record.IsOver && record.IsDeliver {
			delIds = append(delIds, id)
		}
	}
	for _, delId := range delIds {
		delete(globalVar.ConfessionForLoveRecord, delId)
		delete(confessForLoveRank, delId)
	}
}

func loadConfessionForLoveRank(args ...interface{}) {
	forRangeConfessForLoveRecord(func(id uint32, record *pb3.ConfessionForLoveRecord, yyConf *jsondata.ConfessionForLoveConf) {
		confessForLoveRank[id] = &ConfessForLoveRank{
			r: make(map[uint32]*base.Rank, 2),
		}
		for _, sex := range confessionForLoveRankSex {
			r := initConfessionForLoveRank(yyConf)
			if nil != record.RankInfo && nil != record.RankInfo[sex] {
				for _, v := range record.RankInfo[sex].Rank {
					r.Update(v.Id, v.Score)
				}
			}
			confessForLoveRank[id].r[sex] = r
		}

		if record.IsDeliver {
			return
		}
		nowSec := time_util.NowSec()
		edTime := record.EndTime - yyConf.BeforeEndHour*60*60
		var left uint32
		if edTime <= nowSec {
			left = 0
		} else {
			left = edTime - nowSec
		}
		confessForLoveRank[id].deliverTimer = timer.SetTimeout(time.Duration(left)*time.Second, func() {
			checkConfessionForLoveRankDeliver(id)
		})
	})
}

func checkConfessionForLoveRankOver(args ...interface{}) {
	clearRangeConfessForLoveRecord()
	forRangeConfessForLoveRecord(func(id uint32, record *pb3.ConfessionForLoveRecord, yyConf *jsondata.ConfessionForLoveConf) {
		if record.IsOver {
			return
		}
		nowSec := time_util.NowSec()
		if nowSec < record.EndTime {
			return
		}
		record.IsOver = true
		for actorId, v := range record.PData {
			if v.PraticipateRev {
				continue
			}
			if v.Score >= yyConf.ParticipateScore {
				v.PraticipateRev = true
				mailmgr.SendMailToActor(actorId, &mailargs.SendMailSt{
					ConfId:  common.Mail_ConfessionForLoveParticipateAward,
					Rewards: yyConf.ParticipateAwards,
				})
			}
		}
	})
}

func saveConfessionForLoveRank(args ...interface{}) {
	globalVar := gshare.GetStaticVar()
	for id, record := range globalVar.ConfessionForLoveRecord {
		rank := confessForLoveRank[id]
		if nil == rank {
			continue
		}
		if nil == record.RankInfo {
			record.RankInfo = make(map[uint32]*pb3.ConfessionForLoveRecordRank, 2)
		}
		for sex, r := range rank.r {
			record.RankInfo[sex] = &pb3.ConfessionForLoveRecordRank{Rank: r.GetList(1, r.GetRankCount())}
		}
	}
}

func handleConfessionForLove(player iface.IPlayer, job uint32) bool {
	yyList := pyymgr.GetPlayerAllYYObj(player, yydefine.YYConfessionForLove)
	for _, obj := range yyList {
		if s, ok := obj.(*ConfessionForLoveSys); ok && s.IsOpen() {
			s.onJobChange(job)
		}
	}
	return true
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYConfessionForLove, func() iface.IPlayerYY {
		return &ConfessionForLoveSys{}
	})

	net.RegisterYYSysProtoV2(53, 81, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*ConfessionForLoveSys).c2sRev
	})

	net.RegisterYYSysProtoV2(53, 82, func(sys iface.IPlayerYY) func(msg *base.Message) error {
		return sys.(*ConfessionForLoveSys).c2sRankList
	})

	event.RegSysEvent(custom_id.SeServerInit, loadConfessionForLoveRank)

	event.RegSysEvent(custom_id.SeServerInit, checkConfessionForLoveRankOver)
	event.RegSysEvent(custom_id.SeNewDayArrive, checkConfessionForLoveRankOver)

	event.RegSysEvent(custom_id.SeBeforeSaveGlobalVar, saveConfessionForLoveRank)

	event.RegActorEvent(custom_id.AeSendConfession, onConfessionForLoveEvent)

	jobchange.RegJobChangeFunc(jobchange.YYConfessionForLove, &jobchange.Fn{Fn: handleConfessionForLove})

}
