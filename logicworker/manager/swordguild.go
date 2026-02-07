/**
 * @Author: zjj
 * @Date: 2024/12/2
 * @Desc: 剑宗主宰 (先临时放这里)
**/

package manager

import (
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
)

var swordGuildSchedule *pb3.SwordGuildSchedule
var competitionDayRange *pb3.SwordGuildCompetitionDayRange

func GetSwordGuildSchedule() (*pb3.SwordGuildSchedule, bool) {
	if nil == swordGuildSchedule {
		return nil, false
	}

	return swordGuildSchedule, true
}

func InSwordGuildCompetitionDay() bool {
	competitionRange := competitionDayRange
	if competitionRange == nil {
		return false
	}

	if competitionRange.IsEnd {
		return false
	}

	if !competitionRange.IsOpen {
		return false
	}

	return true
}

func handleF2GSyncSwordGuildSrvData(buf []byte) {
	var req pb3.SyncSwordGuildSrvDataReq
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogWarn("SyncSwordGuildSrvDataReq unmarshal failed")
		return
	}
	if len(req.Actors) == 0 {
		logger.LogWarn("actorIds is zero")
		return
	}
	var resp = &pb3.SyncSwordGuildSrvDataRet{
		PfId:          engine.GetPfId(),
		SrvId:         engine.GetServerId(),
		ActorFightMap: make(map[uint64]uint64),
	}
	for _, v := range req.Actors {
		dataBase, ok := GetData(v.Key, gshare.ActorDataBase).(*pb3.PlayerDataBase)
		if !ok {
			logger.LogWarn("not found %d actor base data", v.Key)
			continue
		}
		resp.ActorFightMap[v.Key] = dataBase.Power
	}
	if len(resp.ActorFightMap) == 0 {
		logger.LogWarn("actor power map is zero")
		return
	}
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FSyncSwordGuildSrvDataRet, resp)
	if err != nil {
		logger.LogWarn("handleF2GSyncSwordGuildSrvData CallFightSrvFunc %d failed, err:%v", sysfuncid.G2FSyncSwordGuildSrvDataRet, err)
		return
	}
}

func handleF2GSwordGuildCommandPlayerSetReq(buf []byte) {
	var req pb3.SwordGuildCommandPlayerSetReq
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogWarn("SwordGuildCommandPlayerSetReq unmarshal failed")
		return
	}
	if len(req.Actors) == 0 {
		logger.LogWarn("actorIds is zero")
		return
	}
	conf := jsondata.GetSwordGuildCompetitionConf()
	if nil == conf {
		return
	}
	var resp = &pb3.SwordGuildCommandPlayerSetRet{
		PfId:  engine.GetPfId(),
		SrvId: engine.GetServerId(),
	}
	for _, v := range req.Actors {
		if uint32(len(resp.Actors)) >= conf.CommandPlayerNum {
			break
		}
		if nil == GetPlayerPtrById(v.Key) {
			continue
		}
		resp.Actors = append(resp.Actors, v.Key)
	}
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FSwordGuildCommandPlayerSetRet, resp)
	if err != nil {
		logger.LogWarn("handleF2GSwordGuildCommandPlayerIdReq CallFightSrvFunc %d failed, err:%v", sysfuncid.G2FSwordGuildCommandPlayerSetRet, err)
		return
	}
}

func handleF2GSyncSwordGuildEnergy(buf []byte) {
	var req pb3.CommonSt
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogWarn("handleF2GSyncSwordGuildEnergy unmarshal failed")
		return
	}
	player := GetPlayerPtrById(req.U64Param)
	if nil == player {
		return
	}
	energy := req.U32Param
	player.SetExtraAttr(attrdef.SwordGuildEnergy, attrdef.AttrValueAlias(energy))
}

func handleF2GSyncSwordGuildSchedule(buf []byte) {
	var req pb3.SyncSwordGuildSchedule
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogWarn("handleF2GSyncSwordGuildSchedule unmarshal failed")
		return
	}
	swordGuildSchedule = req.Schedule
}

func handleF2GSyncSwordGuildScheduleState(buf []byte) {
	var req pb3.SyncSwordGuildSchedule
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogWarn("handleF2GSyncSwordGuildScheduleState unmarshal failed")
		return
	}
	swordGuildSchedule = req.Schedule
}

func handleF2GSyncCompetitionRange(buf []byte) {
	var req pb3.SwordGuildCompetitionDayRange
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogWarn("handleF2GSyncCompetitionRange unmarshal failed")
		return
	}
	competitionDayRange = &req
}

func handleF2GCompetitionDayStart(buf []byte) {
	var req pb3.SwordGuildCompetitionDayRange
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogWarn("handleF2GSyncCompetitionRange unmarshal failed")
		return
	}
	competitionDayRange = &req
}

func handleF2GCompetitionDayEnd(buf []byte) {
	var req pb3.SwordGuildCompetitionDayRange
	if err := pb3.Unmarshal(buf, &req); err != nil {
		logger.LogWarn("handleF2GSyncCompetitionRange unmarshal failed")
		return
	}
	competitionDayRange = &req
}

func handleFCDisconnectCross(_ []byte) {
	swordGuildSchedule = nil
	competitionDayRange = nil
}

func handleF2GSyncSwordGuildAuction(buf []byte) {
	var srvData pb3.SwordGuildBattlegroundSrvData
	if err := pb3.Unmarshal(buf, &srvData); err != nil {
		logger.LogWarn("handleF2GSyncSwordGuildAuction unmarshal failed")
		return
	}
	rank := GRankMgrIns.GetRankByType(gshare.RankTypeLevel)
	rankList := rank.GetList(1, 10)
	var average int64
	for _, v := range rankList {
		average += v.Score
	}
	averageLevel := uint32(average / int64(len(rankList)))
	var dropIds []uint32

	jsondata.EachSwordGuildAuctionDo(srvData.BattlegroundId, srvData.IsWinner, func(auctionConf *jsondata.SwordGuildAuctionAwardsConf) {
		if auctionConf.LevelMin > averageLevel || auctionConf.LevelMax < averageLevel {
			return
		}
		dropIds = append(dropIds, auctionConf.DropIds...)
	})

	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FSwordGuildAuctionRet, &pb3.G2FSwordGuildAuctionRet{
		DropIds: dropIds,
		PfId:    srvData.PfId,
		SrvId:   srvData.SrvId,
	})
	if err != nil {
		return
	}
}

func init() {
	engine.RegisterSysCall(sysfuncid.F2GSyncSwordGuildSrvData, handleF2GSyncSwordGuildSrvData)
	engine.RegisterSysCall(sysfuncid.F2GSwordGuildCommandPlayerSetReq, handleF2GSwordGuildCommandPlayerSetReq)
	engine.RegisterSysCall(sysfuncid.F2GSyncSwordGuildEnergy, handleF2GSyncSwordGuildEnergy)
	engine.RegisterSysCall(sysfuncid.F2GSyncSwordGuildSchedule, handleF2GSyncSwordGuildSchedule)
	engine.RegisterSysCall(sysfuncid.F2GSyncSwordGuildScheduleState, handleF2GSyncSwordGuildScheduleState)
	engine.RegisterSysCall(sysfuncid.F2GSyncCompetitionRange, handleF2GSyncCompetitionRange)
	engine.RegisterSysCall(sysfuncid.F2GCompetitionDayStart, handleF2GCompetitionDayStart)
	engine.RegisterSysCall(sysfuncid.F2GCompetitionDayEnd, handleF2GCompetitionDayEnd)
	engine.RegisterSysCall(sysfuncid.FCDisconnectCross, handleFCDisconnectCross)
	engine.RegisterSysCall(sysfuncid.F2GSwordGuildAuctionReq, handleF2GSyncSwordGuildAuction)
}
