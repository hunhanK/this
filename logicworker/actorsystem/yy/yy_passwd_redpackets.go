/**
 * @Author: LvYuMeng
 * @Date: 2024/6/24
 * @Desc:
**/

package yy

import (
	"fmt"
	"github.com/gzjjyz/random"
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/attrdef"
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
	"jjyz/gameserver/logicworker/guildmgr"
	"jjyz/gameserver/logicworker/internal/timer"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"strings"
	"time"
)

type YYPasswdRedPackets struct {
	YYBase
	nextOpen []*time_util.Timer
}

func (s *YYPasswdRedPackets) data() *pb3.YYPasswdRedPackets {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}
	if globalVar.YyDatas.PasswdRedPackets == nil {
		globalVar.YyDatas.PasswdRedPackets = make(map[uint32]*pb3.YYPasswdRedPackets)
	}
	if globalVar.YyDatas.PasswdRedPackets[s.Id] == nil {
		globalVar.YyDatas.PasswdRedPackets[s.Id] = &pb3.YYPasswdRedPackets{}
	}
	if globalVar.YyDatas.PasswdRedPackets[s.Id].PlayerDatas == nil {
		globalVar.YyDatas.PasswdRedPackets[s.Id].PlayerDatas = make(map[uint64]*pb3.PasswdRedPackets)
	}
	if globalVar.YyDatas.PasswdRedPackets[s.Id].HistoryPwd == nil {
		globalVar.YyDatas.PasswdRedPackets[s.Id].HistoryPwd = make(map[uint32]bool)
	}
	return globalVar.YyDatas.PasswdRedPackets[s.Id]
}

func (s *YYPasswdRedPackets) OnOpen() {
	conf, ok := jsondata.GetYYPasswdRedPacketsConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	curTime := time_util.NowSec()

	for _, openTimer := range s.nextOpen {
		openTimer.Stop()
	}
	s.nextOpen = nil

	var nearTime uint32
	for _, str := range conf.ResetTime {
		openTime := time_util.ToTodayTime(str)
		if openTime <= curTime {
			nearTime = utils.MaxUInt32(nearTime, openTime)
			continue
		}
		nextTimer := timer.SetTimeout(time.Duration(openTime-time_util.NowSec())*time.Second, func() {
			s.changeLoop(openTime)
		})
		s.nextOpen = append(s.nextOpen, nextTimer)
	}

	s.changeLoop(nearTime)
}

func (s *YYPasswdRedPackets) ResetData() {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.YyDatas || nil == globalVar.YyDatas.PasswdRedPackets {
		return
	}
	delete(globalVar.YyDatas.PasswdRedPackets, s.GetId())
}

func (s *YYPasswdRedPackets) initLoop() {
	if !s.IsOpen() {
		return
	}
	conf, ok := jsondata.GetYYPasswdRedPacketsConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	data := s.data()
	curTime := time_util.NowSec()

	for _, openTimer := range s.nextOpen {
		openTimer.Stop()
	}
	s.nextOpen = nil

	var nearTime uint32
	for _, str := range conf.ResetTime {
		openTime := time_util.ToTodayTime(str)
		if openTime <= curTime {
			nearTime = utils.MaxUInt32(nearTime, openTime)
			continue
		}
		nextTimer := timer.SetTimeout(time.Duration(openTime-time_util.NowSec())*time.Second, func() {
			s.changeLoop(openTime)
		})
		s.nextOpen = append(s.nextOpen, nextTimer)
	}

	if data.RefreshTime > 0 && data.RefreshTime < nearTime { //结束过
		s.changeLoop(nearTime)
	}
}

func (s *YYPasswdRedPackets) OnInit() {
	s.initLoop()
}

func (s *YYPasswdRedPackets) randRedPacketByLucky(luckyLv uint32) uint32 {
	conf, ok := jsondata.GetYYPasswdRedPacketsConf(s.ConfName, s.ConfIdx)
	if !ok {
		return 0
	}
	rateConf := conf.LuckyLevel[luckyLv].Rate
	pool := new(random.Pool)
	for i := 0; i < len(rateConf); i += 2 {
		pool.AddItem(rateConf[i], rateConf[i+1])
	}
	newQuality := pool.RandomOne().(uint32)
	return newQuality
}

func (s *YYPasswdRedPackets) changeLoop(timeStamp uint32) {
	data := s.data()
	if data.RefreshTime == timeStamp {
		return
	}
	conf, ok := jsondata.GetYYPasswdRedPacketsConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}

	s.LogInfo("口令红包活动:%d 开始切换轮次", s.GetId())

	data.RefreshTime = timeStamp

	var randPwd []uint32
	for pwdId := range conf.Passwords {
		if data.HistoryPwd[pwdId] {
			continue
		}
		randPwd = append(randPwd, pwdId)
	}
	if len(randPwd) < 1 {
		for pwdId := range conf.Passwords {
			randPwd = append(randPwd, pwdId)
		}
		s.LogError("random passwd not enough")
	}
	data.PasswdId = randPwd[random.Interval(0, len(randPwd)-1)]
	data.HistoryPwd[data.PasswdId] = true

	data.PlayerDatas = make(map[uint64]*pb3.PasswdRedPackets)
	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		s.sendPlayerData(player)
	})

	return
}

func (s *YYPasswdRedPackets) changeLoopPlayer(playerId uint64) {
	conf, ok := jsondata.GetYYPasswdRedPacketsConf(s.ConfName, s.ConfIdx)
	if !ok {
		s.LogError("YYPasswdRedPackets conf is nil")
		return
	}

	data := s.data()
	if _, ok := data.PlayerDatas[playerId]; !ok {
		data.PlayerDatas[playerId] = &pb3.PasswdRedPackets{}
	}

	pData := data.PlayerDatas[playerId]

	idx := uint32(len(data.PlayerDatas) % len(conf.Passwords[data.PasswdId].Password))
	pData.PasswdBits = utils.SetBit(0, idx)
	pData.LuckyRefreshTimes = 0
	pData.RedPacketQuailty = s.randRedPacketByLucky(0)
	pData.Record = nil
	pData.IsOpenRedPacket = false
	pData.LuckyLv = s.randomLucky()

	if player := manager.GetPlayerPtrById(playerId); nil != player {
		s.sendPlayerData(player)
	}
}

func (s *YYPasswdRedPackets) PlayerLogin(player iface.IPlayer) {
	s.sendPlayerData(player)
}

func (s *YYPasswdRedPackets) PlayerReconnect(player iface.IPlayer) {
	s.sendPlayerData(player)
}

func (s *YYPasswdRedPackets) getPlayerData(playerId uint64) (*pb3.PasswdRedPackets, bool) {
	pData, ok := s.data().PlayerDatas[playerId]
	if !ok {
		return nil, false
	}
	if nil == pData.AssistIds {
		pData.AssistIds = make(map[uint64]uint32)
	}
	return pData, ok
}

func (s *YYPasswdRedPackets) sendPlayerData(player iface.IPlayer) {
	if nil == player {
		return
	}
	data := s.data()
	rsp := &pb3.S2C_69_60{
		ActiveId: s.GetId(),
		PData:    data.PlayerDatas[player.GetId()],
		PwdId:    data.PasswdId,
	}
	if rsp.PData != nil {
		player.SetExtraAttr(attrdef.PasswordRedPacketLuckyLv, attrdef.AttrValueAlias(rsp.PData.LuckyLv))
	}
	player.SendProto3(69, 60, rsp)
	s.s2cNeedThankInfo(player)
	_ = s.s2cReqAssistList(player)
}

func (s *YYPasswdRedPackets) s2cNeedThankInfo(player iface.IPlayer) {
	rsp := &pb3.S2C_69_74{ActiveId: s.GetId()}
	pData, ok := s.getPlayerData(player.GetId())
	if !ok {
		player.SendProto3(69, 74, rsp)
		return
	}
	for assistId, raw := range pData.AssistIds {
		bits := uint32(utils.Low16(raw))
		if utils.IsSetBit(bits, pwdRedPacketsAssistStatusThankBan) {
			continue
		}
		target, ok := manager.GetData(assistId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
		if !ok {
			continue
		}
		st := &pb3.PwdRedPacketAssist{
			AssistId: assistId,
			Name:     target.GetName(),
			Quality:  uint32(utils.High16(raw)),
		}
		rsp.AssistList = append(rsp.AssistList, st)
	}
	player.SendProto3(69, 74, rsp)
}

func (s *YYPasswdRedPackets) c2sOpenRedPacket(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_69_61
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf, ok := jsondata.GetYYPasswdRedPacketsConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("YYPasswdRedPackets conf is nil")
	}

	data := s.data()
	if _, ok := conf.Passwords[data.PasswdId]; !ok {
		return neterror.ConfNotFoundError("YYPasswdRedPackets pwd %d is nil", data.PasswdId)
	}

	pData, ok := s.getPlayerData(player.GetId())
	if !ok {
		return neterror.InternalError("YYPasswdRedPackets player data not init")
	}

	allSet := uint32(len(conf.Passwords[data.PasswdId].Password))
	allSet = 1<<allSet - 1
	if pData.PasswdBits != allSet {
		return neterror.ParamsInvalidError("YYPasswdRedPackets redPacket pwd not equal")
	}

	if _, ok := conf.RedPackets[pData.RedPacketQuailty]; !ok {
		return neterror.ConfNotFoundError("YYPasswdRedPackets redPacket quality %d is nil", pData.RedPacketQuailty)
	}

	if pData.IsOpenRedPacket {
		player.SendTipMsg(tipmsgid.TpAwardIsReceive)
		return nil
	}

	pData.IsOpenRedPacket = true
	engine.GiveRewards(player, conf.RedPackets[pData.RedPacketQuailty].Rewards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogPasswdRedPacketsOpen,
	})

	player.SendProto3(69, 61, &pb3.S2C_69_61{
		ActiveId: s.GetId(),
		Awards:   jsondata.StdRewardVecToPb3RewardVec(conf.RedPackets[pData.RedPacketQuailty].Rewards),
	})
	return nil
}

func (s *YYPasswdRedPackets) c2sRefreshLucky(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_69_62
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf, ok := jsondata.GetYYPasswdRedPacketsConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("YYPasswdRedPackets conf is nil")
	}

	pData, ok := s.getPlayerData(player.GetId())
	if !ok {
		s.changeLoopPlayer(player.GetId())
		return nil
	}

	if pData.LuckyRefreshTimes >= conf.LuckyChangeTimes {
		return neterror.ParamsInvalidError("YYPasswdRedPackets LuckyChangeTimes limit")
	}

	var consume jsondata.ConsumeVec
	for i, v := range conf.RefreshLucky {
		if pData.LuckyRefreshTimes <= v.Times || i == (len(conf.RefreshLucky)-1) {
			consume = v.Consumes
			break
		}
	}

	if !player.ConsumeByConf(consume, false, common.ConsumeParams{LogId: pb3.LogId_LogPasswdRedPacketsRefreshLucky}) {
		player.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	pData.LuckyRefreshTimes++

	luckyLv := s.randomLucky()

	pData.LuckyLv = utils.MaxUInt32(pData.LuckyLv, luckyLv)
	player.SendProto3(69, 62, &pb3.S2C_69_62{
		ActiveId: s.GetId(),
		LuckyLv:  pData.LuckyLv,
	})
	player.SetExtraAttr(attrdef.PasswordRedPacketLuckyLv, attrdef.AttrValueAlias(pData.LuckyLv))

	return nil
}

func (s *YYPasswdRedPackets) randomLucky() uint32 {
	conf, ok := jsondata.GetYYPasswdRedPacketsConf(s.ConfName, s.ConfIdx)
	if !ok {
		return 0
	}
	pool := new(random.Pool)
	for _, v := range conf.LuckyLevel {
		if v.Lv == 0 || v.Weight == 0 {
			continue
		}
		pool.AddItem(v.Lv, v.Weight)
	}
	luckyLv := pool.RandomOne().(uint32)
	return luckyLv
}

func (s *YYPasswdRedPackets) c2sLuckyList(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_69_63
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	rsp := &pb3.S2C_69_63{
		ActiveId: s.GetId(),
	}

	pData, isInit := s.getPlayerData(player.GetId())

	if guild := guildmgr.GetGuildById(player.GetGuildId()); nil != guild {
		data := s.data()
		for memberId, member := range guild.Members {
			if mData, ok := data.PlayerDatas[memberId]; ok {
				st := &pb3.PasswdGuildLuckyInfo{
					ActorId:     memberId,
					LuckyLv:     mData.LuckyLv,
					AssistTimes: mData.AssistTime,
					Name:        member.PlayerInfo.Name,
				}
				if pie.Uint64s(mData.ReqAssistIds).Contains(player.GetId()) {
					st.Status = pwdRedPacketsAssistStatusClientReqAssist
				}
				if isInit {
					if _, isAssist := pData.AssistIds[memberId]; isAssist {
						st.Status = pwdRedPacketsAssistStatusClientUpSuccess
					}
				}

				rsp.LuckyList = append(rsp.LuckyList, st)
			}
		}
	}

	player.SendProto3(69, 63, rsp)
	return nil
}

const pwdRedPacketsAssistRecord = 10

func (s *YYPasswdRedPackets) c2sAssistRefresh(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_69_64
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf, ok := jsondata.GetYYPasswdRedPacketsConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("YYPasswdRedPackets conf is nil")
	}

	targetId := req.ActorId
	if targetId == player.GetId() {
		return neterror.ParamsInvalidError("YYPasswdRedPackets assist target is self")
	}

	targetData, ok := s.getPlayerData(targetId)
	if !ok {
		return neterror.ParamsInvalidError("YYPasswdRedPackets player %d not part", targetId)
	}

	if targetData.LuckyLv == 0 {
		return neterror.ParamsInvalidError("YYPasswdRedPackets player %d not part", targetId)
	}

	pData, ok := s.getPlayerData(player.GetId())
	if !ok {
		return neterror.InternalError("YYPasswdRedPackets player data not init")
	}

	delAssist := func(tid uint64) {
		pData.ReqAssistIds = pie.Uint64s(pData.ReqAssistIds).Filter(func(u uint64) bool {
			return u != tid
		})
		player.SendProto3(69, 72, &pb3.S2C_69_72{
			ActiveId: s.GetId(),
			ActorId:  tid,
		})
	}

	if targetData.IsOpenRedPacket {
		delAssist(targetId)
		return nil
	}

	if !req.IsAgree {
		delAssist(targetId)
		return nil
	}

	if conf.AssistTimes <= pData.AssistTime {
		return neterror.ParamsInvalidError("YYPasswdRedPackets assist times not enough")
	}

	if pData.LuckyLv == 0 {
		return neterror.ParamsInvalidError("YYPasswdRedPackets luckyLv is zero")
	}

	if targetData.RedPacketQuailty >= conf.GetRedPacketTopQuality() {
		delAssist(targetId)
		player.SendTipMsg(tipmsgid.TpSharePasswdmax)
		return nil
	}

	if !s.IsSameGuild(player, targetId) {
		return neterror.InternalError("YYPasswdRedPackets not same guild")
	}

	if !pie.Uint64s(pData.ReqAssistIds).Contains(targetId) {
		return neterror.InternalError("YYPasswdRedPackets target %d not in req assist list", targetId)
	}

	if _, isAssist := targetData.AssistIds[player.GetId()]; isAssist {
		return neterror.InternalError("YYPasswdRedPackets has assist target")
	}

	pData.AssistTime++

	newQuality := s.randRedPacketByLucky(pData.LuckyLv)

	isUpdate := false
	if newQuality > 0 && newQuality >= targetData.RedPacketQuailty {
		targetData.RedPacketQuailty = newQuality
		isUpdate = true
	}

	player.SendProto3(69, 65, &pb3.S2C_69_65{
		ActiveId:      s.GetId(),
		TargetId:      targetId,
		TargetQuality: targetData.RedPacketQuailty,
		AssistTimes:   pData.AssistTime,
	})

	delAssist(targetId)

	var status uint16
	if isUpdate {
		status = 1 << pwdRedPacketsAssistStatusUpSuccess
	}
	targetData.AssistIds[player.GetId()] = utils.Make32(status, uint16(targetData.RedPacketQuailty))

	target := manager.GetPlayerPtrById(targetId)
	if isUpdate {
		record := &pb3.PasswdRedPacketRefreshRecord{
			ActorId: player.GetId(),
			Name:    player.GetName(),
			Quality: targetData.RedPacketQuailty,
		}
		targetData.Record = append(targetData.Record, record)
		if len(targetData.Record) > pwdRedPacketsAssistRecord {
			targetData.Record = targetData.Record[1:]
		}
		if nil != target {
			target.SendProto3(69, 75, &pb3.S2C_69_75{
				ActiveId: s.GetId(),
				Quality:  targetData.RedPacketQuailty,
			})
			target.SendProto3(69, 64, &pb3.S2C_69_64{
				ActiveId: s.GetId(),
				Record:   record,
			})
		}
	}

	if nil != target {
		target.SendProto3(69, 74, &pb3.S2C_69_74{
			ActiveId: s.GetId(),
			AssistList: []*pb3.PwdRedPacketAssist{{
				AssistId: player.GetId(),
				Name:     player.GetName(),
				Quality:  targetData.RedPacketQuailty,
			}},
		})
	}

	engine.GiveRewards(player, conf.AssistAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPasswdRedPacketsAssist})

	return nil
}

func (s *YYPasswdRedPackets) c2sSendChatReq(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_69_67
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf, ok := jsondata.GetYYPasswdRedPacketsConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("YYPasswdRedPackets conf is nil")
	}

	pData, ok := s.getPlayerData(player.GetId())
	if !ok {
		return neterror.InternalError("YYPasswdRedPackets player data not init")
	}

	if pData.PasswdBits == 0 {
		return neterror.ParamsInvalidError("YYPasswdRedPackets pwd empty")
	}

	data := s.data()

	var tipStr strings.Builder

	allSet := uint32(len(conf.Passwords[data.PasswdId].Password))
	allSet = 1<<allSet - 1
	if pData.PasswdBits != allSet {
		tipStr.WriteString(fmt.Sprintf("%v,", tipmsgid.TpSharePasswd))
	} else {
		tipStr.WriteString(fmt.Sprintf("%v,", tipmsgid.TpSharePasswdfinish))
	}

	for i, str := range conf.Passwords[data.PasswdId].Password {
		if utils.IsSetBit(pData.PasswdBits, uint32(i)) {
			tipStr.WriteString(str)
		} else {
			tipStr.WriteString("*")
		}
	}

	player.ChannelChat(&pb3.C2S_5_1{
		Channel:     req.GetChannel(),
		Msg:         fmt.Sprintf(""),
		ToId:        0,
		ItemIds:     nil,
		Params:      tipStr.String(),
		ContentType: chatdef.ContentTipMsg,
	}, true)

	return nil
}

func (s *YYPasswdRedPackets) c2sSetPwd(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_69_68
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf, ok := jsondata.GetYYPasswdRedPacketsConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("YYPasswdRedPackets conf is nil")
	}

	idx := int(req.GetPwdIdx())
	data := s.data()
	if len(conf.Passwords[data.PasswdId].Password) <= idx || conf.Passwords[data.PasswdId].Password[idx] != req.GetStr() {
		return neterror.ParamsInvalidError("YYPasswdRedPackets pwd not equal")
	}

	pData, ok := s.getPlayerData(player.GetId())
	if !ok {
		return neterror.InternalError("YYPasswdRedPackets player data not init")
	}

	pData.PasswdBits = utils.SetBit(pData.PasswdBits, req.GetPwdIdx())
	player.SendProto3(69, 68, &pb3.S2C_69_68{
		ActiveId: s.GetId(),
		PwdIdx:   req.GetPwdIdx(),
	})

	return nil
}

func (s *YYPasswdRedPackets) IsSameGuild(player iface.IPlayer, targetId uint64) bool {
	guild := guildmgr.GetGuildById(player.GetGuildId())
	if nil == guild {
		return false
	}
	if nil == guild.GetMember(targetId) {
		return false
	}
	return true
}

func (s *YYPasswdRedPackets) c2sReqAssistList(player iface.IPlayer, msg *base.Message) error {
	return s.s2cReqAssistList(player)
}

func (s *YYPasswdRedPackets) s2cReqAssistList(player iface.IPlayer) error {
	conf, ok := jsondata.GetYYPasswdRedPacketsConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("YYPasswdRedPackets conf is nil")
	}
	rsp := &pb3.S2C_69_70{ActiveId: s.GetId()}
	pData, ok := s.getPlayerData(player.GetId())
	if !ok {
		player.SendProto3(69, 70, rsp)
		return nil
	}
	var newIds []uint64
	for _, targetId := range pData.ReqAssistIds {
		if !s.IsSameGuild(player, targetId) {
			continue
		}
		targetData, ok := s.getPlayerData(targetId)
		if !ok {
			continue
		}
		if targetData.RedPacketQuailty == conf.GetRedPacketTopQuality() {
			continue
		}
		if targetData.IsOpenRedPacket {
			continue
		}
		if target, ok := manager.GetData(targetId, gshare.ActorDataBase).(*pb3.PlayerDataBase); ok {
			rsp.ReqList = append(rsp.ReqList, &pb3.PwdRedPacketReqAssist{
				ActorId:   targetId,
				Name:      target.GetName(),
				HeadFrame: target.GetHeadFrame(),
				Head:      target.GetHead(),
				Job:       target.GetJob(),
				Quality:   targetData.GetRedPacketQuailty(),
			})
		}
		newIds = append(newIds, targetId)
	}
	pData.ReqAssistIds = newIds
	player.SendProto3(69, 70, rsp)
	return nil
}

func (s *YYPasswdRedPackets) c2sReqAssist(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_69_71
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	conf, ok := jsondata.GetYYPasswdRedPacketsConf(s.ConfName, s.ConfIdx)
	if !ok {
		return neterror.ConfNotFoundError("YYPasswdRedPackets conf is nil")
	}

	pData, ok := s.getPlayerData(player.GetId())
	if !ok {
		return neterror.InternalError("YYPasswdRedPackets player data not init")
	}

	if pData.RedPacketQuailty >= conf.GetRedPacketTopQuality() {
		return neterror.InternalError("YYPasswdRedPackets redPacket is full")
	}

	targetId := req.GetActorId()
	if _, isAssist := pData.AssistIds[targetId]; isAssist {
		return neterror.InternalError("YYPasswdRedPackets redPacket is already assist")
	}

	if !s.IsSameGuild(player, targetId) {
		return neterror.InternalError("YYPasswdRedPackets not same guild")
	}

	targetData, ok := s.getPlayerData(targetId)
	if !ok {
		return neterror.ParamsInvalidError("YYPasswdRedPackets target data not init")
	}

	if targetData.AssistTime >= conf.AssistTimes {
		return neterror.ParamsInvalidError("YYPasswdRedPackets target assist times is full")
	}

	if pie.Uint64s(targetData.ReqAssistIds).Contains(player.GetId()) {
		return nil
	}
	targetData.ReqAssistIds = append(targetData.ReqAssistIds, player.GetId())
	if target := manager.GetPlayerPtrById(targetId); nil != target {
		target.SendProto3(69, 71, &pb3.S2C_69_71{
			ActiveId: s.GetId(),
			Req: &pb3.PwdRedPacketReqAssist{
				ActorId:   player.GetId(),
				Name:      player.GetName(),
				HeadFrame: player.GetHeadFrame(),
				Head:      player.GetHead(),
				Job:       player.GetJob(),
				Quality:   pData.GetRedPacketQuailty(),
			},
		})
	}

	return nil
}

const (
	pwdRedPacketsAssistStatusThankBan  = 1
	pwdRedPacketsAssistStatusUpSuccess = 2

	pwdRedPacketsAssistStatusClientReqAssist = 1
	pwdRedPacketsAssistStatusClientUpSuccess = 2
)

func (s *YYPasswdRedPackets) c2sThankToAssist(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_69_73
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}

	pData, ok := s.getPlayerData(player.GetId())
	if !ok {
		return neterror.InternalError("YYPasswdRedPackets player data not init")
	}

	targetId := req.GetActorId()
	raw, isAssist := pData.AssistIds[targetId]
	if !isAssist {
		return neterror.ParamsInvalidError("YYPasswdRedPackets player %d not assist me", targetId)
	}
	bits := uint32(utils.Low16(raw))
	upQuality := uint32(utils.High16(raw))

	if !req.GetIsThank() {
		bits = utils.SetBit(bits, pwdRedPacketsAssistStatusThankBan)
		pData.AssistIds[targetId] = utils.Make32(uint16(bits), uint16(upQuality))
		return nil
	}

	if utils.IsSetBit(bits, pwdRedPacketsAssistStatusThankBan) {
		return neterror.ParamsInvalidError("YYPasswdRedPackets thank is handle")
	}

	bits = utils.SetBit(bits, pwdRedPacketsAssistStatusThankBan)
	pData.AssistIds[targetId] = utils.Make32(uint16(bits), uint16(upQuality))

	target := manager.GetPlayerPtrById(targetId)
	if nil != target {
		engine.SendPlayerMessage(req.GetActorId(), gshare.OfflinePwdPacketsSendThank, &pb3.CommonSt{U32Param: s.GetId(), U64Param: player.GetId()})
	}
	return nil
}

func (s *YYPasswdRedPackets) receiveThank(player iface.IPlayer, ThankId uint64) {
	conf, ok := jsondata.GetYYPasswdRedPacketsConf(s.ConfName, s.ConfIdx)
	if !ok {
		return
	}
	target, ok := manager.GetData(ThankId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
	if !ok {
		return
	}
	player.SendProto3(69, 73, &pb3.S2C_69_73{
		ActiveId: s.GetId(),
		ActorId:  target.GetId(),
		Name:     target.GetName(),
	})
	engine.GiveRewards(player, conf.ThankAwards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogPasswdRedPacketsThank})
	return
}

func (s *YYPasswdRedPackets) NewDay() {
	s.initLoop()
}

func offlinePwdPacketsSendThank(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.CommonSt)
	if !ok {
		return
	}
	yy := yymgr.GetYYByActId(st.U32Param)
	if nil == yy || !yy.IsOpen() {
		return
	}
	if yy.GetClass() != yydefine.YYPasswdRedPackets {
		return
	}
	yy.(*YYPasswdRedPackets).receiveThank(player, st.U64Param)
}

func init() {
	yymgr.RegisterYYType(yydefine.YYPasswdRedPackets, func() iface.IYunYing {
		return &YYPasswdRedPackets{}
	})

	engine.RegisterMessage(gshare.OfflinePwdPacketsSendThank, func() pb3.Message {
		return &pb3.CommonSt{}
	}, offlinePwdPacketsSendThank)

	net.RegisterGlobalYYSysProto(69, 61, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYPasswdRedPackets).c2sOpenRedPacket
	})
	net.RegisterGlobalYYSysProto(69, 62, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYPasswdRedPackets).c2sRefreshLucky
	})
	net.RegisterGlobalYYSysProto(69, 63, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYPasswdRedPackets).c2sLuckyList
	})
	net.RegisterGlobalYYSysProto(69, 64, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYPasswdRedPackets).c2sAssistRefresh
	})
	net.RegisterGlobalYYSysProto(69, 67, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYPasswdRedPackets).c2sSendChatReq
	})
	net.RegisterGlobalYYSysProto(69, 68, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYPasswdRedPackets).c2sSetPwd
	})
	net.RegisterGlobalYYSysProto(69, 70, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYPasswdRedPackets).c2sReqAssistList
	})
	net.RegisterGlobalYYSysProto(69, 71, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYPasswdRedPackets).c2sReqAssist
	})
	net.RegisterGlobalYYSysProto(69, 73, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*YYPasswdRedPackets).c2sThankToAssist
	})

	gmevent.Register("pwdRedPacket", func(player iface.IPlayer, args ...string) bool {
		allYY := yymgr.GetAllYY(yydefine.YYPasswdRedPackets)
		for _, iYunYing := range allYY {
			if !iYunYing.IsOpen() {
				continue
			}
			iYunYing.(*YYPasswdRedPackets).changeLoop(time_util.NowSec())
		}
		return true
	}, 1)
}
