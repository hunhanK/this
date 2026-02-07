/**
 * @Author: LvYuMeng
 * @Date: 2025/4/16
 * @Desc: 求助2.0
**/

package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/series"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yy"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"sort"
)

type AskForHelpSys struct {
	Base
}

func (s *AskForHelpSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *AskForHelpSys) OnReconnect() {
	s.s2cInfo()
}

func (s *AskForHelpSys) getTargetData(actorId uint64) (*pb3.AskForHelpData, bool) {
	data, ok := manager.GetPlayerOnlineData(actorId)
	if !ok {
		return nil, false
	}

	if nil == data.AskForHelp {
		data.AskForHelp = &pb3.AskForHelpData{}
	}

	if nil == data.AskForHelp.AskForHelpSt {
		data.AskForHelp.AskForHelpSt = map[uint32]*pb3.AskForHelpSt{}
	}
	return data.AskForHelp, true
}

func (s *AskForHelpSys) findAskForHelpStByHdl(actorId, hdl uint64) (*pb3.AskForHelpSt, bool) {
	data, ok := s.getTargetData(actorId)
	if !ok {
		return nil, false
	}

	for _, v := range data.AskForHelpSt {
		st := v
		if st.AskRecordMap == nil {
			continue
		}
		if _, exist := v.AskRecordMap[hdl]; exist {
			return st, true
		}
	}
	return nil, false
}

func (s *AskForHelpSys) findAskForHelpStByType(actorId uint64, askType uint32) (*pb3.AskForHelpSt, bool) {
	data, ok := s.getTargetData(actorId)
	if !ok {
		return nil, false
	}

	info, ok := data.AskForHelpSt[askType]
	if !ok {
		info = &pb3.AskForHelpSt{AskType: askType}
		data.AskForHelpSt[askType] = info
	}
	if info.AskRecordMap == nil {
		info.AskRecordMap = make(map[uint64]*pb3.AskForHelpRecord)
	}
	return info, true
}

func (s *AskForHelpSys) getLogData() map[uint32]*pb3.AskForHelpLogs {
	binary := s.GetBinaryData()

	if nil == binary.AskForHelpLogs {
		binary.AskForHelpLogs = map[uint32]*pb3.AskForHelpLogs{}
	}
	return binary.AskForHelpLogs
}

func (s *AskForHelpSys) s2cInfo() {
	if data, ok := s.getTargetData(s.owner.GetId()); ok {
		s.SendProto3(42, 20, &pb3.S2C_42_20{Data: data})
	}
}

func (s *AskForHelpSys) c2sAskHelp(msg *base.Message) error {
	var req pb3.C2S_42_21
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return neterror.Wrap(err)
	}

	askId := req.AskId
	channelId := req.Channel
	targetId := req.TargetId

	bindConf, err := jsondata.GetAskForHelpBindInfo(askId)
	if err != nil {
		return neterror.Wrap(err)
	}

	typeConf, err := jsondata.GetAskForHelpTypeConfByType(bindConf.Type)
	if err != nil {
		return neterror.Wrap(err)
	}

	if !s.canAsk(askId) {
		return neterror.ParamsInvalidError("ask not allow")
	}

	data, ok := s.findAskForHelpStByType(s.owner.GetId(), bindConf.Type)
	if !ok {
		return neterror.InternalError("not found ask data")
	}

	nowSec := time_util.NowSec()

	if data.LastAskTime+typeConf.Cd > nowSec {
		s.owner.SendTipMsg(tipmsgid.Incd)
		return nil
	}

	if targetId > 0 && req.Channel == chatdef.CIPrivate {
		target := manager.GetPlayerPtrById(targetId)
		if nil == target {
			s.owner.SendTipMsg(tipmsgid.TpPlayerOfflineChat)
			return nil
		}
	}

	// 构造求助记录
	hdl, err := series.AllocSeries()
	if err != nil {
		return neterror.Wrap(err)
	}

	var askRecord = &pb3.AskForHelpRecord{
		Hdl:          hdl,
		AskPlayerId:  s.owner.GetId(),
		AskId:        askId,
		AskCreatedAt: nowSec,
		AskCount:     typeConf.BeGiftTimeLimit,
		Channel:      channelId,
	}

	data.AskRecordMap[hdl] = askRecord
	data.LastAskTime = nowSec

	// 推送消息
	s.owner.ChannelChat(&pb3.C2S_5_1{
		Channel:     req.Channel,
		Msg:         "",
		ToId:        req.TargetId,
		Params:      fmt.Sprintf("%d,%d", hdl, askId),
		ContentType: chatdef.ContentCollectCardAskHelp,
	}, !(req.Channel == chatdef.CIPrivate))

	s.SendProto3(42, 21, &pb3.S2C_42_21{
		AskRecord: askRecord,
	})

	s.SendProto3(42, 31, &pb3.S2C_42_31{Type: bindConf.Type, LastAskTime: data.LastAskTime})

	manager.SetOlineDataSaveFlag(s.owner.GetId())
	manager.SetOlineDataSaveFlag(targetId)

	err = s.checkAskRecordFull(bindConf.Type)
	if err != nil {
		return neterror.Wrap(err)
	}

	return nil
}

func (s *AskForHelpSys) findAskForHelpStByAny(actorId, hdl uint64, askType uint32) (*pb3.AskForHelpSt, bool) {
	if hdl > 0 {
		return s.findAskForHelpStByHdl(actorId, hdl)
	}
	if askType > 0 {
		return s.findAskForHelpStByType(actorId, askType)
	}
	return nil, false
}

func (s *AskForHelpSys) findGiftInfo(record *pb3.AskForHelpRecord, actorId uint64) *pb3.AskForHelpGift {
	for _, gift := range record.GiftCount {
		if gift.ActorId == s.owner.GetId() {
			return gift
		}
	}
	return nil
}

func (s *AskForHelpSys) c2sToHelp(msg *base.Message) error {
	var req pb3.C2S_42_22
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return neterror.Wrap(err)
	}

	hdl := req.Hdl
	targetId := req.TargetId
	askId := req.AskId

	if targetId == s.owner.GetId() {
		s.owner.SendTipMsg(tipmsgid.AskHelpTips1)
		return nil
	}

	targetInfo, ok := manager.GetData(targetId, gshare.ActorDataBase).(*pb3.PlayerDataBase)
	if !ok {
		s.owner.SendTipMsg(tipmsgid.TpAddFriendNoOne)
		return nil
	}

	bindConf, err := jsondata.GetAskForHelpBindInfo(askId)
	if nil != err {
		return err
	}

	typeConf, err := jsondata.GetAskForHelpTypeConfByType(bindConf.Type)
	if nil != err {
		return err
	}

	targetAskInfo, ok := s.findAskForHelpStByAny(targetId, hdl, bindConf.Type)
	if !ok {
		return neterror.ParamsInvalidError("target not exist")
	}

	if targetAskInfo.AskType != bindConf.Type {
		return neterror.ParamsInvalidError("record ask type err")
	}

	myData, ok := s.findAskForHelpStByType(s.owner.GetId(), bindConf.Type)
	if !ok {
		return neterror.ParamsInvalidError("myData not exist")
	}

	var targetRecord *pb3.AskForHelpRecord
	var giftInfo *pb3.AskForHelpGift
	if hdl > 0 {
		targetRecord = targetAskInfo.AskRecordMap[hdl]
		// 对方的接受赠予已达上限
		if targetRecord.AcceptCount >= targetRecord.AskCount {
			s.owner.SendTipMsg(tipmsgid.AskHelpTips2)
			return nil
		}

		giftInfo = s.findGiftInfo(targetRecord, s.owner.GetId())
		if nil != giftInfo && giftInfo.Count >= typeConf.GftTimeLimit {
			s.owner.SendTipMsg(tipmsgid.AskHelpTips3)
			return neterror.ParamsInvalidError("该条求助达赠予上限")
		}

	} else { // 主动赠送
		var canHelp = true
		if !s.owner.IsExistFriend(req.TargetId) {
			s.owner.SendTipMsg(tipmsgid.NotFriend)
			canHelp = false
		}

		if !canHelp && targetInfo.GuildId != 0 && s.owner.GetGuildId() != targetInfo.GuildId {
			s.owner.SendTipMsg(tipmsgid.TpGuildPlayerIsntMember)
			canHelp = false
		}

		if !canHelp {
			return nil
		}
	}

	// 对方的每条求助，每个玩家最大赠予次数

	giftCount := myData.CompletedGiftCountMap[hdl]
	if giftCount != 0 && giftCount >= typeConf.GftTimeLimit {
		s.owner.SendTipMsg(tipmsgid.AskHelpTips3)
		return neterror.ParamsInvalidError("该条求助达赠予上限")
	}

	// 对方的获赠次数上限 (具体功能里去判断)
	// 自己的赠予次数上限（具体功能去判断）

	if !s.Send(askId, targetId) {
		return neterror.InternalError("send err")
	}

	// 新增求助记录
	nowSec := time_util.NowSec()
	var offer = hdl == 0
	if hdl == 0 {
		hdl, _ = series.AllocSeries()
	}

	targetLog := &pb3.AskForHelpLog{
		Hdl:        hdl,
		TargetId:   s.owner.GetId(),
		CreatedAt:  nowSec,
		AskId:      askId,
		IsAsk:      true,
		TargetName: s.owner.GetName(),
		ItemName:   "",
		Offer:      offer,
	}

	// 新增赠予记录
	myLog := &pb3.AskForHelpLog{
		Hdl:        hdl,
		TargetId:   targetId,
		CreatedAt:  nowSec,
		AskId:      askId,
		IsAsk:      false,
		TargetName: targetInfo.Name,
		Offer:      offer,
	}

	if targetRecord != nil {
		targetRecord.Logs = append(targetRecord.Logs, targetLog)
		targetRecord.AcceptCount += 1
		if nil == giftInfo {
			giftInfo = &pb3.AskForHelpGift{ActorId: targetId}
			targetRecord.GiftCount = append(targetRecord.GiftCount, giftInfo)
		}
		giftInfo.Count += 1
		engine.Broadcast(chatdef.CIWorld, 0, 42, 25, &pb3.S2C_42_25{
			AskRecord: targetRecord,
		}, 0)
	}

	engine.SendPlayerMessage(s.owner.GetId(), gshare.OfflineAddAskForHelpLog, myLog)

	engine.SendPlayerMessage(targetId, gshare.OfflineAddAskForHelpLog, targetLog)

	manager.SetOlineDataSaveFlag(s.owner.GetId())
	manager.SetOlineDataSaveFlag(targetId)
	return nil
}

var askCheck = map[uint32]func(player iface.IPlayer, askId uint32, bindConf *jsondata.AskHelpItemConfig) bool{
	custom_id.AskTypeCollectCard: func(player iface.IPlayer, askId uint32, bindConf *jsondata.AskHelpItemConfig) bool {
		s, exist := player.GetSysObj(sysdef.SiCollectCard).(*CollectCardSys)
		return exist && s.IsOpen()
	},
	custom_id.AskTypeYYZongziFeast: func(player iface.IPlayer, askId uint32, bindConf *jsondata.AskHelpItemConfig) bool {
		for _, yyObj := range yymgr.GetAllYY(yydefine.YYZongziFeast) {
			if yyzf, ok := yyObj.(*yy.YYZongziFeast); ok && yyzf.IsOpen() && yyzf.CanAskItem(player.GetId(), bindConf.BindId) {
				return true
			}
		}
		return false
	},
}

func (s *AskForHelpSys) canAsk(askId uint32) bool {
	bindConf, err := jsondata.GetAskForHelpBindInfo(askId)
	if nil != err {
		return false
	}

	if check, ok := askCheck[bindConf.Type]; ok {
		return check(s.owner, askId, bindConf)
	}

	return true
}

var askSend = map[uint32]func(player iface.IPlayer, targetId uint64, askId uint32, bindConf *jsondata.AskHelpItemConfig) bool{
	custom_id.AskTypeCollectCard: func(player iface.IPlayer, targetId uint64, askId uint32, bindConf *jsondata.AskHelpItemConfig) bool {
		if ccSys, exist := player.GetSysObj(sysdef.SiCollectCard).(*CollectCardSys); exist && ccSys.IsOpen() {
			return ccSys.sendCard(targetId, bindConf.BindId)
		}
		return false
	},
	custom_id.AskTypeYYZongziFeast: func(player iface.IPlayer, targetId uint64, askId uint32, bindConf *jsondata.AskHelpItemConfig) bool {
		for _, yyObj := range yymgr.GetAllYY(yydefine.YYZongziFeast) {
			if yyzf, ok := yyObj.(*yy.YYZongziFeast); ok && yyzf.IsOpen() && yyzf.SendItem(player, targetId, bindConf.BindId) {
				return true
			}
		}
		return false
	},
}

func (s *AskForHelpSys) Send(askId uint32, targetId uint64) bool {
	bindConf, err := jsondata.GetAskForHelpBindInfo(askId)
	if nil != err {
		return false
	}

	if send, ok := askSend[bindConf.Type]; ok {
		return send(s.owner, targetId, askId, bindConf)
	}

	return false
}

func (s *AskForHelpSys) checkAskRecordFull(askType uint32) error {
	typeConf, err := jsondata.GetAskForHelpTypeConfByType(askType)
	if err != nil {
		return neterror.Wrap(err)
	}

	data, ok := s.findAskForHelpStByType(s.owner.GetId(), askType)
	if !ok {
		return neterror.InternalError("not found ask data")
	}

	askRecordMap := data.AskRecordMap
	if typeConf.AskHelpTimes == 0 {
		return nil
	}

	if uint32(len(askRecordMap)) < typeConf.AskHelpTimes {
		return nil
	}

	var timeSec = time_util.NowSec()
	var delHdl uint64
	for hdl, record := range askRecordMap {
		if record.AskCreatedAt > timeSec {
			continue
		}
		timeSec = record.AskCreatedAt
		delHdl = hdl
	}
	if delHdl != 0 {
		delete(askRecordMap, delHdl)
	}

	return nil
}

func (s *AskForHelpSys) c2sGetAskRecord(msg *base.Message) error {
	var req pb3.C2S_42_24
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}

	info, ok := s.findAskForHelpStByHdl(req.TargetId, req.Hdl)
	if !ok {
		s.owner.SendTipMsg(tipmsgid.AskHelpRecordNotFound)
		return nil
	}

	record, ok := info.AskRecordMap[req.Hdl]
	if !ok {
		s.owner.SendTipMsg(tipmsgid.AskHelpRecordNotFound)
		return nil
	}
	s.owner.SendProto3(42, 24, &pb3.S2C_42_24{
		Record: record,
	})
	return nil
}

func (s *AskForHelpSys) c2sRead(msg *base.Message) error {
	var req pb3.C2S_42_28
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return neterror.Wrap(err)
	}
	data := s.getLogData()
	var log *pb3.AskForHelpLog
	for _, logs := range data {
		for _, l := range logs.ReceiveLogs {
			if l.Hdl != req.Hdl {
				continue
			}
			log = l
			break
		}
	}

	if log == nil {
		return neterror.ParamsInvalidError("not found record %d", req.Hdl)
	}

	log.ReadAt = time_util.NowSec()
	s.SendProto3(42, 28, &pb3.S2C_42_28{
		Hdl:    req.Hdl,
		ReadAt: log.ReadAt,
	})

	manager.SetOlineDataSaveFlag(s.owner.GetId())
	return nil
}

func (s *AskForHelpSys) c2sLogs(msg *base.Message) error {
	var req pb3.C2S_42_30
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return neterror.Wrap(err)
	}

	data := s.getLogData()
	logs, ok := data[req.Type]
	if !ok {
		return nil
	}

	s.SendProto3(42, 30, &pb3.S2C_42_30{
		Type: req.Type,
		Logs: logs,
	})
	return nil
}

func (s *AskForHelpSys) checkAskHelpLogFull(logs []*pb3.AskForHelpLog, recordLimit uint32) ([]*pb3.AskForHelpLog, error) {
	if uint32(len(logs)) <= recordLimit {
		return logs, nil
	}

	// 从大到小排序
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].CreatedAt > logs[j].CreatedAt
	})
	var ret = make([]*pb3.AskForHelpLog, recordLimit)
	copy(ret, logs)
	return ret, nil
}

func (s *AskForHelpSys) addLog(log *pb3.AskForHelpLog) {
	if nil == log {
		return
	}

	bindConf, err := jsondata.GetAskForHelpBindInfo(log.AskId)
	if nil != err {
		return
	}

	typeConf, err := jsondata.GetAskForHelpTypeConfByType(bindConf.Type)
	if nil != err {
		return
	}

	data := s.getLogData()
	logs, ok := data[bindConf.Type]
	if !ok {
		logs = &pb3.AskForHelpLogs{}
		data[bindConf.Type] = logs
	}

	if !log.IsAsk {
		logs.SendLogs = append(logs.SendLogs, log)
		ret, err := s.checkAskHelpLogFull(logs.SendLogs, typeConf.RecordLimit)
		if err != nil {
			s.LogError("err:%v", err)
		} else {
			logs.SendLogs = ret
		}
		s.owner.SendProto3(42, 22, &pb3.S2C_42_22{
			Log: log,
		})
	} else {
		logs.ReceiveLogs = append(logs.ReceiveLogs, log)
		ret, err := s.checkAskHelpLogFull(logs.ReceiveLogs, typeConf.RecordLimit)
		if err != nil {
			s.LogError("err:%v", err)
		} else {
			logs.ReceiveLogs = ret
		}
		s.owner.SendProto3(42, 23, &pb3.S2C_42_23{
			Log: log,
		})
	}

	manager.SetOlineDataSaveFlag(s.owner.GetId())
	return
}

func offlineAddAskForHelpLog(player iface.IPlayer, msg pb3.Message) {
	log, ok := msg.(*pb3.AskForHelpLog)
	if !ok {
		return
	}

	if s, exist := player.GetSysObj(sysdef.SiAskForHelp).(*AskForHelpSys); exist && s.IsOpen() {
		s.addLog(log)
		return
	}

	return
}

func clearAskForHelpDataByType(askType uint32) {
	manager.AllOnlineDataDo(func(actorId uint64, data *pb3.PlayerOnlineData) {
		if nil == data.AskForHelp || nil == data.AskForHelp.AskForHelpSt {
			return
		}
		delete(data.AskForHelp.AskForHelpSt, askType)

		manager.SetOlineDataSaveFlag(actorId)
	})
}

func init() {
	RegisterSysClass(sysdef.SiAskForHelp, func() iface.ISystem {
		return &AskForHelpSys{}
	})

	event.RegSysEvent(custom_id.SeClearAskForHelpData, func(args ...interface{}) {
		if len(args) < 1 {
			return
		}
		askType, ok := args[0].(uint32)
		if !ok {
			return
		}
		clearAskForHelpDataByType(askType)
	})

	engine.RegisterMessage(gshare.OfflineAddAskForHelpLog, func() pb3.Message {
		return &pb3.AskForHelpLog{}
	}, offlineAddAskForHelpLog)

	net.RegisterSysProtoV2(42, 21, sysdef.SiAskForHelp, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*AskForHelpSys).c2sAskHelp
	})
	net.RegisterSysProtoV2(42, 22, sysdef.SiAskForHelp, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*AskForHelpSys).c2sToHelp
	})
	net.RegisterSysProtoV2(42, 24, sysdef.SiAskForHelp, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*AskForHelpSys).c2sGetAskRecord
	})
	net.RegisterSysProtoV2(42, 28, sysdef.SiAskForHelp, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*AskForHelpSys).c2sRead
	})
	net.RegisterSysProtoV2(42, 30, sysdef.SiAskForHelp, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*AskForHelpSys).c2sLogs
	})

}
