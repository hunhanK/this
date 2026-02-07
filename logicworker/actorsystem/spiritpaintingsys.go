/**
 * @Author:
 * @Date:
 * @Desc:
**/

package actorsystem

import (
	"fmt"
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/random"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"time"
)

type SpiritPaintingSys struct {
	Base
}

func (s *SpiritPaintingSys) s2cInfo() {
	s.SendProto3(8, 230, &pb3.S2C_8_230{
		GlobalData: getSpiritPaintingGlobalData(),
		Data:       s.getData(),
	})
}

func (s *SpiritPaintingSys) getData() *pb3.SpiritPaintingData {
	data := s.GetBinaryData().SpiritPaintingData
	if data == nil {
		s.GetBinaryData().SpiritPaintingData = &pb3.SpiritPaintingData{}
		data = s.GetBinaryData().SpiritPaintingData
	}
	if data.SeasonMap == nil {
		data.SeasonMap = make(map[uint32]*pb3.SpiritPaintingPersonSeason)
	}
	return data
}

func getSpiritPaintingGlobalData() *pb3.SpiritPaintingGlobalData {
	data := gshare.GetStaticVar()
	if data.SpiritPaintingGlobalData == nil {
		data.SpiritPaintingGlobalData = &pb3.SpiritPaintingGlobalData{}
	}
	if data.SpiritPaintingGlobalData.SeasonMap == nil {
		data.SpiritPaintingGlobalData.SeasonMap = make(map[uint32]*pb3.SpiritPaintingSeason)
	}
	return data.SpiritPaintingGlobalData
}

func (s *SpiritPaintingSys) CanOpt(idx uint32) bool {
	data := getSpiritPaintingGlobalData()
	if data.CurIdx == 0 || data.CurIdx != idx {
		return false
	}
	return true
}

func (s *SpiritPaintingSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SpiritPaintingSys) OnLogin() {
	s.s2cInfo()
}

func (s *SpiritPaintingSys) OnOpen() {
	s.checkAndInitPersonSeason(0, true)
	s.s2cInfo()
}

func (s *SpiritPaintingSys) checkAndInitPersonSeason(idx uint32, forceReset bool) {
	data := getSpiritPaintingGlobalData()
	if idx == 0 {
		idx = data.CurIdx
	}
	if idx == 0 {
		return
	}
	spiritPaintingData := s.getData()
	if forceReset {
		delete(spiritPaintingData.SeasonMap, idx)
	}
	_, ok := spiritPaintingData.SeasonMap[idx]
	if !ok {
		spiritPaintingData.SeasonMap[idx] = &pb3.SpiritPaintingPersonSeason{
			Idx: idx,
		}
	}
}

func (s *SpiritPaintingSys) c2sUpLv(msg *base.Message) error {
	var req pb3.C2S_8_231
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	if !s.CanOpt(req.Idx) {
		return neterror.ParamsInvalidError("%d not opt", req.Idx)
	}
	data := s.getData()
	idx := req.Idx
	personSeason := data.SeasonMap[idx]
	if personSeason == nil {
		return neterror.ParamsInvalidError("%d not open season data", idx)
	}
	conf := jsondata.GetSpiritPaintingConf(idx)
	if conf == nil {
		return neterror.ConfNotFoundError("%d not found conf", idx)
	}
	lv := personSeason.Lv
	nextLv := lv + 1
	var nextLvConf *jsondata.SpiritPaintingLevelConf
	for _, levelConf := range conf.LevelConf {
		if levelConf.Lv != nextLv {
			continue
		}
		nextLvConf = levelConf
		break
	}
	if nextLvConf == nil {
		return neterror.ConfNotFoundError("%d not found %d level conf", idx, nextLv)
	}
	if len(nextLvConf.Consume) == 0 || !s.GetOwner().ConsumeByConf(nextLvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogSpiritPaintingUpLv}) {
		return neterror.ConsumeFailedError("spirit painting %d not enough consume", idx)
	}
	personSeason.Lv = nextLv
	s.SendProto3(8, 231, &pb3.S2C_8_231{
		Idx: idx,
		Lv:  nextLv,
	})
	s.ResetSysAttr(attrdef.SaSpiritPainting)

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogSpiritPaintingUpLv, &pb3.LogPlayerCounter{
		NumArgs: uint64(idx),
		StrArgs: fmt.Sprintf("%d", personSeason.Lv),
	})
	return nil
}
func (s *SpiritPaintingSys) c2sUpStar(msg *base.Message) error {
	var req pb3.C2S_8_232
	err := msg.UnpackagePbmsg(&req)
	if err != nil {
		return err
	}
	if !s.CanOpt(req.Idx) {
		return neterror.ParamsInvalidError("%d not opt", req.Idx)
	}
	data := s.getData()
	personSeason := data.SeasonMap[req.Idx]
	if personSeason == nil {
		return neterror.ParamsInvalidError("%d not open season data", req.Idx)
	}
	idx := req.Idx
	conf := jsondata.GetSpiritPaintingConf(idx)
	if conf == nil {
		return neterror.ConfNotFoundError("%d not found conf", idx)
	}
	star := personSeason.Star
	nextStar := star + 1
	var nextStarConf *jsondata.SpiritPaintingStarConf
	for _, starConf := range conf.StarConf {
		if starConf.Star != nextStar {
			continue
		}
		nextStarConf = starConf
		break
	}
	if nextStarConf == nil {
		return neterror.ConfNotFoundError("%d not found %d star conf", idx, nextStar)
	}
	if len(nextStarConf.Consume) == 0 || !s.GetOwner().ConsumeByConf(nextStarConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogSpiritPaintingUpStar}) {
		return neterror.ConsumeFailedError("spirit painting %d not enough consume", idx)
	}
	if !random.Hit(nextStarConf.Weight, 10000) {
		s.SendProto3(8, 232, &pb3.S2C_8_232{
			Idx:  idx,
			Star: personSeason.Star,
		})
		return nil
	}
	personSeason.Star = nextStar
	s.SendProto3(8, 232, &pb3.S2C_8_232{
		Idx:  idx,
		Star: personSeason.Star,
	})
	s.ResetSysAttr(attrdef.SaSpiritPainting)

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogSpiritPaintingUpStar, &pb3.LogPlayerCounter{
		NumArgs: uint64(idx),
		StrArgs: fmt.Sprintf("%d", personSeason.Star),
	})
	return nil
}

func (s *SpiritPaintingSys) calcAttr(calc *attrcalc.FightAttrCalc) {
	owner := s.GetOwner()
	data := s.getData()
	for _, season := range data.SeasonMap {
		conf := jsondata.GetSpiritPaintingConf(season.Idx)
		if conf == nil {
			continue
		}
		for _, levelConf := range conf.LevelConf {
			if levelConf.Lv != season.Lv {
				continue
			}
			engine.CheckAddAttrsToCalc(owner, calc, levelConf.Attrs)
			break
		}
		for _, starConf := range conf.StarConf {
			if starConf.Star != season.Star {
				continue
			}
			engine.CheckAddAttrsToCalc(owner, calc, starConf.Attrs)
			break
		}
		for _, starSkill := range conf.StarSkill {
			if starSkill.Star > season.Star {
				continue
			}
			engine.CheckAddAttrsToCalc(owner, calc, starSkill.Attrs)
		}
	}
}

func (s *SpiritPaintingSys) calcAttrAddRate(totalCalc *attrcalc.FightAttrCalc, calc *attrcalc.FightAttrCalc) {
	owner := s.GetOwner()
	data := s.getData()
	for _, season := range data.SeasonMap {
		conf := jsondata.GetSpiritPaintingConf(season.Idx)
		if conf == nil {
			continue
		}
		for _, levelConf := range conf.LevelConf {
			if levelConf.Lv != season.Lv {
				continue
			}
			if levelConf.AddRate != 0 {
				engine.CheckAddAttrsRateRoundingUp(owner, calc, levelConf.Attrs, levelConf.AddRate)
			}
			break
		}
	}
}

func handleTopFightStatus(args ...interface{}) {
	if len(args) < 1 {
		return
	}
	msg, ok := args[0].(*pb3.SyncTopFightSchedule)
	if !ok {
		return
	}
	if msg.Group == nil {
		return
	}
	var zeroSchedule *pb3.TopFightSchedule
	for _, schedule := range msg.Group.PrepareList {
		if schedule.Type != 0 {
			continue
		}
		zeroSchedule = schedule
		break
	}
	if zeroSchedule == nil || zeroSchedule.StartTime == 0 {
		logger.LogError("没有巅峰竞技预告")
		return
	}
	globalData := getSpiritPaintingGlobalData()
	if globalData.CurIdx != 0 {
		sec := time_util.NowSec()
		spiritPaintingSeason, ok := globalData.SeasonMap[globalData.CurIdx]
		if ok {
			if spiritPaintingSeason.StartAt <= sec && sec <= spiritPaintingSeason.EndAt {
				logger.LogError("当期赛季还没结束 %d %d %d", spiritPaintingSeason.StartAt, sec, spiritPaintingSeason.EndAt)
				return
			}
		}
	}
	// 计算时间
	startTime := time.Unix(int64(zeroSchedule.StartTime), 0)
	weekday := startTime.Weekday()
	var durationTime uint32
	var curScheduleDuration uint32 = 7
	if weekday != 1 {
		// 不是周一开
		if weekday == 0 {
			weekday = 7
		}
		curScheduleDuration = uint32(7-weekday+7) + 1
	}
	durationTime = (curScheduleDuration+7)*86400 - 1
	zeroAt := time_util.GetZeroTime(zeroSchedule.StartTime)
	var seasonStartAt = zeroAt
	var seasonEndAt = zeroAt + durationTime
	globalData.CurIdx += 1
	globalData.SeasonMap[globalData.CurIdx] = &pb3.SpiritPaintingSeason{
		Idx:     globalData.CurIdx,
		StartAt: seasonStartAt,
		EndAt:   seasonEndAt,
	}
	logger.LogInfo("灵画新赛季开启。idx：%d, startAt：%d,endAt：%d", globalData.CurIdx, seasonStartAt, seasonEndAt)
	engine.Broadcast(chatdef.CIWorld, 0, 8, 233, &pb3.S2C_8_233{
		GlobalData: globalData,
	}, 0)
	manager.AllOfflineDataBaseDo(func(p *pb3.PlayerDataBase) bool {
		engine.SendPlayerMessage(p.Id, gshare.OfflineOpenSpiritPaintingSeason, &pb3.CommonSt{
			U32Param: globalData.CurIdx,
			BParam:   false,
		})
		return true
	})
}

func handleOfflineOpenSpiritPaintingSeason(player iface.IPlayer, msg pb3.Message) {
	obj := player.GetSysObj(sysdef.SiSpiritPainting)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*SpiritPaintingSys)
	if !ok {
		return
	}
	st, ok := msg.(*pb3.CommonSt)
	if !ok {
		return
	}
	sys.checkAndInitPersonSeason(st.U32Param, st.BParam)
	sys.s2cInfo()
}

func calcSpiritPaintingAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	obj := player.GetSysObj(sysdef.SiSpiritPainting)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*SpiritPaintingSys)
	if !ok {
		return
	}
	sys.calcAttr(calc)
}

func calcSpiritPaintingAttrAddRate(player iface.IPlayer, totalCalc *attrcalc.FightAttrCalc, calc *attrcalc.FightAttrCalc) {
	obj := player.GetSysObj(sysdef.SiSpiritPainting)
	if obj == nil || !obj.IsOpen() {
		return
	}
	sys, ok := obj.(*SpiritPaintingSys)
	if !ok {
		return
	}
	sys.calcAttrAddRate(totalCalc, calc)
}

func handleSpiritPaintingMerge(_ ...interface{}) {
	globalData := getSpiritPaintingGlobalData()
	curIdx := globalData.CurIdx
	manager.AllOfflineDataBaseDo(func(p *pb3.PlayerDataBase) bool {
		engine.SendPlayerMessage(p.Id, gshare.OfflineOpenSpiritPaintingSeason, &pb3.CommonSt{
			U32Param: curIdx,
		})
		return true
	})
}

func init() {
	RegisterSysClass(sysdef.SiSpiritPainting, func() iface.ISystem {
		return &SpiritPaintingSys{}
	})

	net.RegisterSysProtoV2(8, 231, sysdef.SiSpiritPainting, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SpiritPaintingSys).c2sUpLv
	})
	net.RegisterSysProtoV2(8, 232, sysdef.SiSpiritPainting, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SpiritPaintingSys).c2sUpStar
	})
	event.RegSysEvent(custom_id.SeTopFightStatus, handleTopFightStatus)
	engine.RegAttrCalcFn(attrdef.SaSpiritPainting, calcSpiritPaintingAttr)
	engine.RegAttrAddRateCalcFn(attrdef.SaSpiritPainting, calcSpiritPaintingAttrAddRate)
	engine.RegisterMessage(gshare.OfflineOpenSpiritPaintingSeason, func() pb3.Message {
		return &pb3.CommonSt{}
	}, handleOfflineOpenSpiritPaintingSeason)
	event.RegSysEvent(custom_id.SeMerge, handleSpiritPaintingMerge)
	gmevent.Register("SpiritPaintingSys.reset", func(player iface.IPlayer, args ...string) bool {
		data := getSpiritPaintingGlobalData()
		data.CurIdx = 0
		data.SeasonMap = make(map[uint32]*pb3.SpiritPaintingSeason)
		return true
	}, 1)
}
