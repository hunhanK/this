/**
 * @Author: LvYuMeng
 * @Date: 2025/1/15
 * @Desc: 春节红包
**/

package yy

import (
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/chargedef"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/internal/timer"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"time"
)

const SrvIdxStart = 100000

type YYRedPaper struct {
	YYBase
	timers []*time_util.Timer
}

func (yy *YYRedPaper) ClearTimers() {
	for _, v := range yy.timers {
		if nil != v {
			v.Stop()
		}
	}
	yy.timers = make([]*time_util.Timer, 0, 0)
}

func (yy *YYRedPaper) SetSrvRedPaperTimers() {
	yy.ClearTimers()
	conf := yy.GetConf()
	now := time_util.NowSec()
	todayZero := time_util.GetZeroTime(now)
	for _, v := range conf.SrvRedPapers {
		rsfSec := v.Hour*3600 + todayZero
		if now < rsfSec {
			interval := rsfSec - now
			redPaperConf := v
			timerSt := timer.SetTimeout(time.Second*time.Duration(interval), func() {
				yy.createSrvRedPaper(redPaperConf)
			})
			yy.timers = append(yy.timers, timerSt)
		}
	}
}

func (yy *YYRedPaper) OnInit() {
	yy.SetSrvRedPaperTimers()
}

func (yy *YYRedPaper) OnOpen() {
	yy.Broadcast(75, 14, &pb3.S2C_75_14{
		ActiveId: yy.Id,
		SrvIdx:   yy.getThisData().SrvIdx,
	})
	yy.Broadcast(75, 12, &pb3.S2C_75_12{
		ActiveId: yy.Id,
	})
}

func (yy *YYRedPaper) OnEnd() {
	yy.ClearTimers()
}

func (yy *YYRedPaper) NewDay() {
	yy.SetSrvRedPaperTimers()

	data := yy.getThisData()
	for _, line := range data.Info {
		line.FetchFlag = 0
	}

	manager.AllOnlinePlayerDo(func(actor iface.IPlayer) {
		yy.sendFetchFlag(actor)
	})
}

func (yy *YYRedPaper) PlayerLogin(player iface.IPlayer) {
	yy.sendFetchFlag(player)
	yy.sendRedPapers(player)
}

func (yy *YYRedPaper) PlayerReconnect(player iface.IPlayer) {
	yy.sendFetchFlag(player)
	yy.sendRedPapers(player)
}

func (yy *YYRedPaper) GetConf() *jsondata.YYRedPaperConf {
	return jsondata.GetYYRedPaperConf(yy.ConfName, yy.ConfIdx)
}

func (yy *YYRedPaper) ResetData() {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.YyDatas || nil == globalVar.YyDatas.RedPaperData {
		return
	}
	delete(globalVar.YyDatas.RedPaperData, yy.GetId())
}

func (yy *YYRedPaper) getThisData() *pb3.YYRedPaper {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}

	if globalVar.YyDatas.RedPaperData == nil {
		globalVar.YyDatas.RedPaperData = make(map[uint32]*pb3.YYRedPaper)
	}

	tmp := globalVar.YyDatas.RedPaperData[yy.Id]
	if nil == tmp {
		tmp = new(pb3.YYRedPaper)
	}
	if nil == tmp.Info {
		tmp.Info = make(map[uint64]*pb3.RedPaperPlayer)
	}
	if nil == tmp.RedPapers {
		tmp.RedPapers = make(map[uint32]*pb3.RedPaper)
	}
	if nil == tmp.SrvRecords {
		tmp.SrvRecords = make([]*pb3.RedPaperSrvRecord, 0, 0)
	}
	if tmp.SrvIdx == 0 {
		tmp.SrvIdx = SrvIdxStart
	}

	globalVar.YyDatas.RedPaperData[yy.Id] = tmp

	return tmp
}

func (yy *YYRedPaper) getPlayerData(player iface.IPlayer) *pb3.RedPaperPlayer {
	actData := yy.getThisData()
	playerId := player.GetId()
	if _, ok := actData.Info[playerId]; !ok {
		actData.Info[playerId] = new(pb3.RedPaperPlayer)
	}
	if nil == actData.Info[playerId].OpenIdx {
		actData.Info[playerId].OpenIdx = make(map[uint32]uint32)
	}
	return actData.Info[playerId]
}

func (yy *YYRedPaper) getRedPaperData(idx uint32) *pb3.RedPaper {
	actData := yy.getThisData()
	if _, ok := actData.RedPapers[idx]; !ok {
		actData.RedPapers[idx] = new(pb3.RedPaper)
	}
	return actData.RedPapers[idx]
}

// sendRedPapers 下发红包信息
func (yy *YYRedPaper) sendRedPapers(player iface.IPlayer) {
	data := yy.getThisData()
	playerData := yy.getPlayerData(player)
	now := time_util.NowSec()

	msg := &pb3.S2C_75_14{
		ActiveId:  yy.Id,
		RedPapers: make([]*pb3.RedPaperCover, 0, 0),
		SrvIdx:    data.SrvIdx,
		OpenIdxs:  playerData.OpenIdx,
	}
	for k, v := range data.RedPapers {
		if v.DelTime < now {
			continue
		}
		msg.RedPapers = append(msg.RedPapers, &pb3.RedPaperCover{
			Name:    v.Name,
			Idx:     k,
			Type:    v.Type,
			DelTime: v.DelTime,
			ActorId: v.ActorId,
			Job:     v.Job,
			Head:    v.Head,
		})
	}
	player.SendProto3(75, 14, msg)
}

// sendFetchFlag 下发领取信息
func (yy *YYRedPaper) sendFetchFlag(player iface.IPlayer) {
	playerData := yy.getPlayerData(player)
	msg := &pb3.S2C_75_12{
		ActiveId:  yy.Id,
		FetchFlag: playerData.FetchFlag,
	}
	player.SendProto3(75, 12, msg)
}

// sendSrvRecord 发送全服记录
func (yy *YYRedPaper) sendSrvRecord(player iface.IPlayer) {
	data := yy.getThisData()
	msg := &pb3.S2C_75_11{
		ActiveId: yy.Id,
		Records:  data.SrvRecords,
	}
	player.SendProto3(75, 11, msg)
}

// sendPaperRecord 发送单个红包领取记录
func (yy *YYRedPaper) sendPaperRecord(player iface.IPlayer, idx uint32) {
	data := yy.getThisData()
	redPaper, ok := data.RedPapers[idx]
	if !ok {
		return
	}
	playerData := yy.getPlayerData(player)

	msg := &pb3.S2C_75_10{
		ActiveId: yy.Id,
		Idx:      idx,
		Records:  redPaper.Records,
		MyGet:    playerData.OpenIdx[idx],
	}

	player.SendProto3(75, 10, msg)
}

// openRedPaper 开红包
func (yy *YYRedPaper) openRedPaper(player iface.IPlayer, idx uint32) error {
	conf := yy.GetConf()
	if player.GetLevel() < conf.LvLimit {
		player.SendTipMsg(tipmsgid.TpLevelNotReach)
		return nil
	}

	data := yy.getThisData()
	redPaper, ok := data.RedPapers[idx]
	if !ok {
		return neterror.ParamsInvalidError("red paper not exist")
	}

	if redPaper.DelTime < time_util.NowSec() {
		return neterror.ParamsInvalidError("time out")
	}

	playerData := yy.getPlayerData(player)
	// 该玩家开过这个红包
	if _, ok := playerData.OpenIdx[idx]; ok {
		return neterror.ParamsInvalidError("has received")
	}

	money := redPaper.Min
	if redPaper.Step > 0 {
		money += random.IntervalUU(0, (redPaper.Max-redPaper.Min)/redPaper.Step) * redPaper.Step
	}

	playerData.OpenIdx[idx] = money
	player.AddMoney(conf.MoneyType, int64(money), true, pb3.LogId_LogYYRedPaperOpenAward)

	// 领取记录
	redPaper.Records = append(redPaper.Records, &pb3.RedPaperRecord{
		Id:      player.GetId(),
		Name:    player.GetName(),
		Diamond: money,
	})

	if len(redPaper.Records) >= RedPaperRecordRev {
		redPaper.Records = redPaper.Records[1:]
	}

	yy.sendPaperRecord(player, idx)

	return nil
}

// createRedPaper 创建红包
func (yy *YYRedPaper) createRedPaper(player iface.IPlayer, redPaperConf *jsondata.YYRedPaperCharge) {
	data := yy.getThisData()
	data.IncIdx++

	// 创建红包数据
	name, now := player.GetName(), time_util.NowSec()
	data.RedPapers[data.IncIdx] = &pb3.RedPaper{
		ActorId: player.GetId(),
		Name:    player.GetName(),
		Job:     player.GetJob(),
		Head:    player.GetHead(),
		Type:    redPaperConf.Type,
		DelTime: now + redPaperConf.Duration,
		Records: make([]*pb3.RedPaperRecord, 0, 0),
		Min:     redPaperConf.Min,
		Max:     redPaperConf.Max,
		Step:    redPaperConf.Step,
	}
	// 添加到全服记录
	data.SrvRecords = append(data.SrvRecords, &pb3.RedPaperSrvRecord{
		Name: name,
		Time: now,
		Type: redPaperConf.Type,
	})

	if len(data.SrvRecords) >= RedPaperRecordSend {
		data.SrvRecords = data.SrvRecords[1:]
	}

	yy.SendNewAddRedPaper(data.IncIdx)

	conf := yy.GetConf()

	if conf.ChargeBroadcast > 0 {
		engine.BroadcastTipMsgById(conf.ChargeBroadcast, player.GetId(), player.GetName(), conf.RedPaperName, redPaperConf.BlessingWord)
	}
}

// createSrvRedPaper 创建全服红包
func (yy *YYRedPaper) createSrvRedPaper(redPaperConf *jsondata.YYSrvRedPaper) {
	data := yy.getThisData()
	data.SrvIdx++

	// 创建红包数据
	data.RedPapers[data.SrvIdx] = &pb3.RedPaper{
		DelTime: time_util.NowSec() + redPaperConf.Duration,
		Records: make([]*pb3.RedPaperRecord, 0, 0),
		Min:     redPaperConf.Min,
		Max:     redPaperConf.Max,
		Step:    redPaperConf.Step,
	}

	conf := yy.GetConf()
	if conf.GmBroadcast > 0 {
		engine.BroadcastTipMsgById(conf.GmBroadcast, conf.RedPaperName, redPaperConf.BlessingWord)
	}

	yy.SendNewAddRedPaper(data.SrvIdx)
}

func (yy *YYRedPaper) SendNewAddRedPaper(idx uint32) {
	data := yy.getThisData()
	redPaper := data.RedPapers[idx]
	if nil == redPaper {
		return
	}

	// 广播新增红包
	msg := &pb3.S2C_75_15{
		ActiveId: yy.Id,
		RedPaper: &pb3.RedPaperCover{
			Name:    redPaper.Name,
			Idx:     idx,
			Type:    redPaper.Type,
			DelTime: redPaper.DelTime,
			ActorId: redPaper.ActorId,
			Job:     redPaper.Job,
			Head:    redPaper.Head,
		},
	}

	engine.Broadcast(chatdef.CIWorld, 0, 75, 15, msg, 0)
}

func (yy *YYRedPaper) redPaperChargeCheck(player iface.IPlayer, chargeId uint32) bool {
	conf := yy.GetConf()
	playerData := yy.getPlayerData(player)
	for k, v := range conf.ChargeList {
		idx := uint32(k)
		if !utils.IsSetBit(playerData.FetchFlag, idx) {
			return v.ChargeId == chargeId
		}
	}

	return false
}

// redPaperCharge 充值
func (yy *YYRedPaper) redPaperCharge(player iface.IPlayer, chargeId uint32) bool {
	conf := yy.GetConf()
	for k, v := range conf.ChargeList {
		idx := uint32(k)
		if v.ChargeId == chargeId {
			playerData := yy.getPlayerData(player)
			if utils.IsSetBit(playerData.FetchFlag, idx) {
				continue
			}
			redPaperConf := v
			playerData.FetchFlag = utils.SetBit(playerData.FetchFlag, idx)
			engine.GiveRewards(player, conf.ChargeList[idx].Awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogYYRedPaperChargeAward})

			yy.sendFetchFlag(player)
			yy.createRedPaper(player, redPaperConf)
			return true
		}
	}

	return false
}

const (
	RedPaperOptionOpen       = 1
	RedPaperOptionShowDetail = 2

	RedPaperRecordSend = 100
	RedPaperRecordRev  = 50
)

func (yy *YYRedPaper) c2sOpenRedPaper(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_75_10
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	opt, idx := req.GetOpt(), req.GetIdx()
	if opt == RedPaperOptionOpen {
		return yy.openRedPaper(player, idx)
	} else if opt == RedPaperOptionShowDetail {
		yy.sendPaperRecord(player, idx)
	}

	return nil
}

func (yy *YYRedPaper) c2sServerRecord(player iface.IPlayer, msg *base.Message) error {
	yy.sendSrvRecord(player)
	return nil
}

func RedPaperRechargeCheck(player iface.IPlayer, conf *jsondata.ChargeConf) bool {
	objList := yymgr.GetAllYY(yydefine.YYRedPaper)
	for _, v := range objList {
		if obj, ok := v.(*YYRedPaper); ok && obj.IsOpen() {
			if obj.redPaperChargeCheck(player, conf.ChargeId) {
				return true
			}
		}
	}

	return false
}

func RedPaperRecharge(player iface.IPlayer, conf *jsondata.ChargeConf, params *pb3.ChargeCallBackParams) bool {
	objList := yymgr.GetAllYY(yydefine.YYRedPaper)
	for _, v := range objList {
		if obj, ok := v.(*YYRedPaper); ok && obj.IsOpen() {
			if obj.redPaperCharge(player, conf.ChargeId) {
				return true
			}
		}
	}

	return false
}

func init() {
	yymgr.RegisterYYType(yydefine.YYRedPaper, func() iface.IYunYing {
		return &YYRedPaper{}
	})

	engine.RegChargeEvent(chargedef.YYRedPaper, RedPaperRechargeCheck, RedPaperRecharge)

	net.RegisterGlobalYYSysProto(75, 10, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYRedPaper).c2sOpenRedPaper
	})
	net.RegisterGlobalYYSysProto(75, 11, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYRedPaper).c2sServerRecord
	})

	gmevent.Register("openRedPaper", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		yyList := yymgr.GetAllYY(yydefine.YYRedPaper)
		if nil == yyList || len(yyList) == 0 {
			return false
		}
		for _, yy := range yyList {
			s, ok := yy.(*YYRedPaper)
			if !ok || !s.IsOpen() {
				return false
			}
			err := s.openRedPaper(player, utils.AtoUint32(args[0]))
			if err != nil {
				return false
			}
		}
		return true
	}, 1)
	gmevent.Register("createSrvRedPaper", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		yyList := yymgr.GetAllYY(yydefine.YYRedPaper)
		if nil == yyList || len(yyList) == 0 {
			return false
		}
		idx := utils.AtoUint32(args[0])
		for _, yy := range yyList {
			s, ok := yy.(*YYRedPaper)
			if !ok || !s.IsOpen() {
				return false
			}
			conf := s.GetConf()
			if idx <= 0 || idx >= uint32(len(conf.SrvRedPapers)) {
				return false
			}
			s.createSrvRedPaper(conf.SrvRedPapers[idx-1])
		}
		return true
	}, 1)
	gmevent.Register("createPlayerRedPaper", func(player iface.IPlayer, args ...string) bool {
		if len(args) < 1 {
			return false
		}
		yyList := yymgr.GetAllYY(yydefine.YYRedPaper)
		if nil == yyList || len(yyList) == 0 {
			return false
		}
		idx := utils.AtoUint32(args[0])
		for _, yy := range yyList {
			s, ok := yy.(*YYRedPaper)
			if !ok || !s.IsOpen() {
				return false
			}
			conf := s.GetConf()
			if idx <= 0 || idx >= uint32(len(conf.ChargeList)) {
				return false
			}
			s.createRedPaper(player, conf.ChargeList[idx-1])
		}
		return true
	}, 1)
}
