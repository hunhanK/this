package yy

import (
	"errors"
	"fmt"
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	wordmonitor2 "github.com/gzjjyz/wordmonitor"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/base/wordmonitor"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/wordmonitoroption"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"jjyz/gameserver/ranktype"
)

type QiXiConfession struct {
	YYBase
}

func (s *QiXiConfession) getPlayerData(playerId uint64) *pb3.QiXiPlayerData {
	data := s.getData()
	pData, ok := data.PlayerData[playerId]
	if !ok {
		pData = new(pb3.QiXiPlayerData)
		data.PlayerData[playerId] = pData
	}
	return pData
}

func (s *QiXiConfession) getData() *pb3.YYQiXiData {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}
	if nil == globalVar.YyDatas.QiXiData {
		globalVar.YyDatas.QiXiData = make(map[uint32]*pb3.YYQiXiData)
	}
	if globalVar.YyDatas.QiXiData[s.Id] == nil {
		globalVar.YyDatas.QiXiData[s.Id] = &pb3.YYQiXiData{}
	}
	if nil == globalVar.YyDatas.QiXiData[s.Id].PlayerData {
		globalVar.YyDatas.QiXiData[s.Id].PlayerData = make(map[uint64]*pb3.QiXiPlayerData)
	}
	return globalVar.YyDatas.QiXiData[s.Id]
}

func (s *QiXiConfession) s2cInfo(player iface.IPlayer) {
	tmp := s.getPlayerData(player.GetId())
	player.SendProto3(75, 136, &pb3.S2C_75_136{
		ActiveId: s.GetId(),
		Record:   tmp.QiXiGiftRecords,
	})
}

func (s *QiXiConfession) OnOpen() {
	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		s.s2cInfo(player)
	})
}

func (s *QiXiConfession) ResetData() {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.YyDatas || nil == globalVar.YyDatas.QiXiData {
		return
	}
	delete(globalVar.YyDatas.QiXiData, s.GetId())
}

func (s *QiXiConfession) PlayerReconnect(player iface.IPlayer) {
	s.s2cInfo(player)
}

func (s *QiXiConfession) PlayerLogin(player iface.IPlayer) {
	s.s2cInfo(player)
}

func (s *QiXiConfession) clearAll(player iface.IPlayer) {
	pData := s.getPlayerData(player.GetId())
	pData.QiXiGiftRecords = nil
	player.SendProto3(75, 136, &pb3.S2C_75_136{
		ActiveId: s.Id,
	})
}

func (s *QiXiConfession) c2sRead(player iface.IPlayer, msg *base.Message) error {
	pData := s.getPlayerData(player.GetId())
	pData.QiXiGiftRecords = nil
	player.SendProto3(75, 136, &pb3.S2C_75_136{
		ActiveId: s.Id,
	})
	return nil
}

func (s *QiXiConfession) c2sConfession(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_75_134
	if err := pb3.Unmarshal(msg.Data, &req); nil != err {
		return neterror.ParamsInvalidError("UnpackPb3Msg SetTags :%v", err)
	}

	conf := jsondata.GetQiXiConfessionConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return neterror.ConfNotFoundError("QiXiConfession conf is nil")
	}

	targetId := req.TargetId
	//不能表白自己
	if player.GetId() == targetId {
		return errors.New("cannot confess oneself")
	}
	//校验文本
	if req.TextId == 0 && req.Text == "" {
		return fmt.Errorf("表白赠言不能为空")
	}
	if req.TextId != 0 {
		//读配置取文本
		statementConf := conf.GetQiXiStatementConf(req.TextId)
		if statementConf == nil {
			return neterror.ConfNotFoundError("Statement:%v Config Not Found", req.TextId)
		}
		return s.doConfession(player, &req)
	}
	//校验文本
	engine.SendWordMonitor(wordmonitor.QiXiConfession, wordmonitor.QiXiConfessionWord, req.GetText(),
		wordmonitoroption.WithPlayerId(player.GetId()),
		wordmonitoroption.WithRawData(&req),
		wordmonitoroption.WithCommonData(player.BuildChatBaseData(nil)),
		wordmonitoroption.WithDitchId(player.GetExtraAttrU32(attrdef.DitchId)),
	)
	return nil
}

func handleReceiveCrossConfession(buf []byte) {
	//构造表白信息
	msg := pb3.C2GCrossConfessNotify{}
	if err := pb3.Unmarshal(buf, &msg); err != nil {
		logger.LogDebug("confession unmarshal fail")
		return
	}
	success := func() bool {
		if !gshare.IsActorInThisServer(msg.TargetActorId) {
			return false
		}
		obj := yymgr.GetYYByActId(msg.ActId)
		if obj == nil || !obj.IsOpen() {
			return false
		}
		s, ok := obj.(*QiXiConfession)
		if !ok {
			return false
		}
		s.addCharm(msg.TargetActorId, msg.AddScore)
		s.addRecord(msg.TargetActorId, &pb3.QiXiGiftRecord{
			TargetId:  msg.ActorId,
			NickName:  msg.Name,
			ItemId:    msg.ItemId,
			ItemNum:   msg.ItemNum,
			Text:      msg.Text,
			TextId:    msg.TextId,
			HeadFrame: msg.HeadFrame,
			Head:      msg.Head,
			Job:       msg.Job,
			TimeStamp: time_util.NowSec(),
			AddScore:  uint64(msg.AddScore),
			Level:     msg.Level,
			XianZong:  msg.XianZong,
		})

		if targetData, ok := manager.GetData(msg.TargetActorId, gshare.ActorDataBase).(*pb3.PlayerDataBase); ok {
			s.broadcast(msg.ActorId, msg.TargetActorId, msg.ItemId, msg.Name, targetData.Name)
		}

		return true
	}

	rsp := &pb3.G2CRetCrossConfessNotify{
		ActorId:       msg.ActorId,       // 发起查询的玩家ID
		TargetActorId: msg.TargetActorId, // 目标玩家ID
		TargetPfId:    msg.TargetPfId,    // 平台ID
		TargetSrvId:   msg.TargetSrvId,   // 本服服务器ID
		ServerType:    msg.ServerType,    // 跨服节点类型
		RankType:      ranktype.CommonRankTypeByCharm,
		ItemId:        msg.ItemId,
		ItemNum:       msg.ItemNum,
		AddScore:      msg.AddScore,
		Text:          msg.Text,
		TextId:        msg.TextId,
		Name:          msg.Name,
		ActId:         msg.ActId,
		AddConfession: msg.AddConfession,
		ConfessionId:  msg.ConfessionId,
		Success:       success(),
	}

	if targetData, ok := manager.GetData(msg.TargetActorId, gshare.ActorDataBase).(*pb3.PlayerDataBase); ok {
		rsp.TarName = targetData.Name
	}

	// 发回跨服
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CRetCrossConfession, rsp)
	if err != nil {
		logger.LogError("cross server issues")
	}
}

func handleConfessionCrossSuccess(buf []byte) {
	msg := pb3.G2CRetCrossConfessNotify{}
	if err := pb3.Unmarshal(buf, &msg); err != nil {
		return
	}

	obj := yymgr.GetYYByActId(msg.ActId)
	if obj == nil || !obj.IsOpen() {
		return
	}

	s, ok := obj.(*QiXiConfession)
	if !ok {
		logger.LogError("获取活动实例失败: actId=%d", msg.ActId)
		return
	}
	if !msg.Success {
		s.returnConfessionCost(msg.ActorId, msg.ItemId, msg.ItemNum)
		return
	}
	s.addConfession(msg.ActorId, msg.AddConfession)
	s.getRewards(msg.ActorId, msg.ItemId, msg.ItemNum)
	s.broadcast(msg.ActorId, msg.TargetActorId, msg.ItemId, msg.Name, msg.TarName)
}

func (s *QiXiConfession) getRewards(actorId uint64, itemId uint32, itemNum uint32) {
	conf := jsondata.GetQiXiConfessionConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		return
	}
	giftConf := conf.GetQiXiGiftsConf(itemId)
	if giftConf == nil {
		return
	}
	var awards jsondata.StdRewardVec
	for _, reward := range giftConf.Rewards {
		var award = &jsondata.StdReward{
			Id:    reward.Id,
			Count: int64(reward.Count),
		}
		awards = append(awards, award)
	}
	awards = jsondata.StdRewardMulti(awards, int64(itemNum))
	player := manager.GetPlayerPtrById(actorId)
	if player == nil {
		return
	}
	engine.GiveRewards(player, awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogQiXiReturn, NoTips: false})
}

func onQiXiConfessionWordCheckRet(word *wordmonitor.Word) error {
	player := manager.GetPlayerPtrById(word.PlayerId)
	if nil == player {
		return nil
	}

	if word.Ret != wordmonitor2.Success {
		player.SendTipMsg(tipmsgid.QiXisensitive)

		return nil
	}
	req, ok := word.Data.(*pb3.C2S_75_134)
	if !ok {
		return errors.New("not *pb3.C2S_75_134")
	}
	obj := yymgr.GetYYByActId(req.Base.ActiveId)
	if obj == nil || !obj.IsOpen() {
		return nil
	}

	err := obj.(*QiXiConfession).doConfession(player, req)
	if err != nil {
		return err
	}
	return nil
}

func (s *QiXiConfession) addConfession(actorId uint64, score int64) {
	event.TriggerSysEvent(custom_id.SeCrossCommonRankUpdate, &pb3.YYCrossRankParams{
		RankType: ranktype.CommonRankTypeByConfession,
		ActorId:  actorId,
		Score:    score,
	})
	if player := manager.GetPlayerPtrById(actorId); nil != player {
		player.SendProto3(75, 134, &pb3.S2C_75_134{})
	}

}

func (s *QiXiConfession) addCharm(actorId uint64, score int64) {
	event.TriggerSysEvent(custom_id.SeCrossCommonRankUpdate, &pb3.YYCrossRankParams{
		RankType: ranktype.CommonRankTypeByCharm,
		ActorId:  actorId,
		Score:    score,
	})
}

// 添加被表白记录
func (s *QiXiConfession) addRecord(playerId uint64, record *pb3.QiXiGiftRecord) bool {
	pData := s.getPlayerData(playerId)
	pData.QiXiGiftRecords = append(pData.QiXiGiftRecords, record)
	player := manager.GetPlayerPtrById(playerId)
	if player != nil { // 在线
		player.SendProto3(75, 137, &pb3.S2C_75_137{
			ActiveId: s.GetId(),
			Record:   record,
		})
	}
	return true
}

func (s *QiXiConfession) broadcast(playerId, targetId uint64, itemId uint32, playerName, tarName string) {
	conf := jsondata.GetQiXiConfessionConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		logger.LogError("not found config")
		return
	}
	giftConf := conf.GetQiXiGiftsConf(itemId)
	if giftConf == nil {
		logger.LogError("not found config")
		return
	}

	engine.BroadcastTipMsgById(conf.Tips, playerName, tarName, itemId)

	msg := &pb3.S2C_75_138{
		ActiveId: s.Id,
		ItemId:   itemId,
	}

	broToPlayer := func(playerId uint64) {
		player := manager.GetPlayerPtrById(playerId)
		if nil == player {
			return
		}
		player.SendProto3(75, 138, msg)
	}

	if giftConf.IsGlobal {
		engine.Broadcast(chatdef.CIWorld, 0, 75, 138, msg, 0)
	} else {
		broToPlayer(playerId)
		broToPlayer(targetId)
	}
}

func (s *QiXiConfession) doConfession(player iface.IPlayer, req *pb3.C2S_75_134) error {
	conf := jsondata.GetQiXiConfessionConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return neterror.ConfNotFoundError("conf is nil")
	}

	giftsConf := conf.GetQiXiGiftsConf(req.ItemId)
	if giftsConf == nil {
		return neterror.ConfNotFoundError("Gift:%v Config Not Found", req.ItemId)
	}

	consume := jsondata.ConsumeVec{
		&jsondata.Consume{
			Id:    req.ItemId,
			Count: req.ItemNum,
		},
	}

	targetId := req.TargetId
	isSameSrv := gshare.IsActorInThisServer(targetId)
	if !player.ConsumeByConf(consume, true, common.ConsumeParams{LogId: pb3.LogId_LogQiXiConfession}) {
		player.SendTipMsg(tipmsgid.TpItemNotEnough)
		return neterror.ConsumeFailedError("consume-failed")
	}
	charmVal := giftsConf.CharmValue * req.ItemNum
	confessionVal := giftsConf.ConfessionValue * req.ItemNum

	if isSameSrv {
		s.addConfession(player.GetId(), int64(confessionVal))
		s.getRewards(player.GetId(), req.ItemId, req.ItemNum)
		s.addCharm(targetId, int64(charmVal))
		s.addRecord(targetId, &pb3.QiXiGiftRecord{
			TargetId:  player.GetId(),
			NickName:  player.GetName(),
			ItemId:    req.ItemId,
			ItemNum:   req.ItemNum,
			Text:      req.Text,
			TextId:    req.TextId,
			HeadFrame: player.GetHeadFrame(),
			Head:      player.GetHead(),
			Job:       player.GetJob(),
			TimeStamp: time_util.NowSec(),
			AddScore:  uint64(charmVal),
			Level:     player.GetLevel(),
			XianZong:  uint32(player.GetSmallCrossCamp()),
		})

		if targetData, ok := manager.GetData(targetId, gshare.ActorDataBase).(*pb3.PlayerDataBase); ok {
			s.broadcast(player.GetId(), targetId, req.ItemId, player.GetName(), targetData.GetName())
		}

		return nil
	}
	// 向跨服节点发送查询请求
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CCrossConfession, &pb3.G2CCrossConfessNotify{
		ActorId:       player.GetId(),                 // 发起查询的玩家ID
		TargetActorId: targetId,                       // 目标玩家ID
		TargetPfId:    engine.GetPfId(),               // 平台ID
		TargetSrvId:   engine.GetServerId(),           // 本服服务器ID
		RankType:      ranktype.CommonRankTypeByCharm, //  ?
		ItemId:        req.ItemId,
		ItemNum:       req.ItemNum,
		AddScore:      int64(charmVal),
		Text:          req.Text,
		Name:          player.GetName(),
		ActId:         s.Id,
		TextId:        req.TextId,
		ConfIdx:       s.ConfIdx,
		ConfName:      s.ConfName,
		AddConfession: int64(confessionVal),
		Head:          player.GetHead(),
		HeadFrame:     player.GetHeadFrame(),
		Job:           player.GetJob(),
		Level:         player.GetLevel(),
		XianZong:      uint32(player.GetSmallCrossCamp()),
	})
	if err != nil {
		return neterror.Wrap(err)
	}
	return nil
}

func (s *QiXiConfession) returnConfessionCost(actorId uint64, itemId, itemNum uint32) {
	// 返还道具
	conf := jsondata.GetItemConfig(itemId)
	if conf == nil {
		return
	}
	var award = jsondata.StdRewardVec{{
		Id:    itemId,
		Count: int64(itemNum),
	}}
	player := manager.GetPlayerPtrById(actorId)
	if player == nil {
		return
	}
	engine.GiveRewards(player, award, common.EngineGiveRewardParam{LogId: pb3.LogId_LogQiXiReturn, NoTips: true})
}

func (s *QiXiConfession) GMAddScore(id uint64, tmp uint32) {
	s.addCharm(id, int64(tmp))
	s.addConfession(id, int64(tmp))
}

func (s *QiXiConfession) GMRecord(player uint64, itemId uint32, itemNum uint32) {
	pData := s.getPlayerData(player)
	playerT := manager.GetPlayerPtrById(player)
	tmp := &pb3.QiXiGiftRecord{
		TargetId:  player,
		NickName:  "name",
		ItemId:    itemId,
		ItemNum:   itemNum,
		Text:      "",
		TextId:    1,
		HeadFrame: playerT.GetHeadFrame(),
		Head:      playerT.GetHead(),
		Job:       playerT.GetJob(),
		TimeStamp: time_util.NowSec(),
		AddScore:  100,
		Level:     playerT.GetLevel(),
	}
	pData.QiXiGiftRecords = append(pData.QiXiGiftRecords, tmp)
}

func init() {
	yymgr.RegisterYYType(yydefine.YYQiXiConfession, func() iface.IYunYing {
		return &QiXiConfession{}
	})
	net.RegisterGlobalYYSysProto(75, 134, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*QiXiConfession).c2sConfession
	})
	net.RegisterGlobalYYSysProto(75, 139, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*QiXiConfession).c2sRead
	})

	engine.RegisterSysCall(sysfuncid.C2GCrossConfession, handleReceiveCrossConfession)
	engine.RegisterSysCall(sysfuncid.C2GRetCrossConfession, handleConfessionCrossSuccess)

	engine.RegWordMonitorOpCodeHandler(wordmonitor.QiXiConfessionWord, onQiXiConfessionWordCheckRet)
	initConfessionGm()
}

func initConfessionGm() {
	gmevent.Register("SiConfession.addScore", func(player iface.IPlayer, args ...string) bool {
		allYY := yymgr.GetAllYY(yydefine.YYQiXiConfession)
		for _, v := range allYY {
			if !v.IsOpen() {
				continue
			}
			s, ok := v.(*QiXiConfession)
			if !ok {
				continue
			}
			utils.ProtectRun(func() {
				s.GMAddScore(utils.AtoUint64(args[0]), utils.AtoUint32(args[1]))
			})
		}
		return true
	}, 1)

	gmevent.Register("SiConfession.addRecord", func(player iface.IPlayer, args ...string) bool {
		allYY := yymgr.GetAllYY(yydefine.YYQiXiConfession)
		for _, v := range allYY {
			if !v.IsOpen() {
				continue
			}
			s, ok := v.(*QiXiConfession)
			if !ok {
				continue
			}
			utils.ProtectRun(func() {
				s.GMRecord(utils.AtoUint64(args[0]), utils.AtoUint32(args[1]), utils.AtoUint32(args[2]))
			})
		}
		return true
	}, 1)
}
