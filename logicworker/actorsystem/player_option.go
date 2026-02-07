package actorsystem

import (
	"fmt"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/wordmonitor"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/engine/wordmonitoroption"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"unicode/utf8"

	"github.com/gzjjyz/random"

	"github.com/gzjjyz/srvlib/utils/pie"

	wordmonitor2 "github.com/gzjjyz/wordmonitor"
)

type PlayerOpSystem struct {
	Base
}

func SendViewPlayer(actor iface.IPlayer, playerId uint64) {
	var pRoleInfo = &pb3.CommonPlayerInfo{
		PlayerId: playerId,
		Name:     actor.GetName(),
		Lv:       actor.GetLevel(),
		Exp:      actor.GetExp(),
		ServerId: engine.GetServerId(),
	}
	rsp := &pb3.S2C_2_10{PlayerInfo: pRoleInfo}
	actor.SendProto3(2, 10, rsp)
}

func (pSys PlayerOpSystem) OnReconnect() {
	pSys.PushChatEmoji()
}

func (pSys PlayerOpSystem) OnLogin() {
	pSys.PushChatEmoji()
}

func (pSys PlayerOpSystem) PushChatEmoji() {
	binary := pSys.owner.GetBinaryData()
	if binary.ChatEmojiIds == nil {
		binary.ChatEmojiIds = make([]uint32, 0)
	}
	pSys.owner.SendProto3(2, 150, &pb3.S2C_2_150{EmojiData: binary.ChatEmojiIds})
}

// 查看玩家信息
func (pSys PlayerOpSystem) c2sViewPlayer(msg *base.Message) {
	var req pb3.C2S_2_10
	if err := pb3.Unmarshal(msg.Data, &req); nil != err {
		return
	}
	targetPlayerId := req.GetPlayerId()
	SendViewPlayer(pSys.owner, targetPlayerId)
}

func ViewData(id uint64) *pb3.PlayerDataBase {
	data, ok := manager.GetData(id, gshare.ActorDataBase).(*pb3.PlayerDataBase)
	if !ok {
		return nil
	}
	return data
}

// 玩家改名
func (pSys PlayerOpSystem) c2sPlayerChangeName(msg *base.Message) {
	var req pb3.C2S_2_11
	if err := pb3.Unmarshal(msg.Data, &req); nil != err {
		return
	}
	actor := pSys.owner
	if code := PlayerChangeName(actor, req.GetNewName()); code != 0 {
		pSys.GetOwner().SendTipMsg(tipmsgid.ChangeName)
		actor.SendProto3(2, 11, &pb3.S2C_2_11{
			Code:    code,
			NewName: req.GetNewName(),
		})
	}
}

func (pSys PlayerOpSystem) c2sPlayerUseSpringFestivalItem(msg *base.Message) {
	var req pb3.C2S_8_10
	if err := pb3.Unmarshal(msg.Data, &req); nil != err {
		return
	}
	actor := pSys.owner

	itemConf := jsondata.GetSpringFestivalItemConf(req.ItemId)
	if itemConf == nil {
		pSys.LogError("not found %d conf", req.ItemId)
		return
	}

	var desc *jsondata.SpringFestivalItemDesc
	for _, itemDesc := range itemConf.Desc {
		if itemDesc.SignId != req.DescIdx {
			continue
		}
		desc = itemDesc
		break
	}

	if desc == nil {
		actor.SendTipMsg(tipmsgid.TpUseItemFailed)
		return
	}

	if !actor.ConsumeByConf(jsondata.ConsumeVec{{Id: req.ItemId, Count: 1}}, false, common.ConsumeParams{LogId: pb3.LogId_LogUseSpringFestivalItem}) {
		actor.SendTipMsg(tipmsgid.TpItemNotEnough)
		return
	}

	for _, channel := range itemConf.Channels {
		actor.ChannelChat(&pb3.C2S_5_1{
			Msg:     fmt.Sprintf("%s%s", desc.Show1, desc.Show2),
			Channel: channel,
		}, false)
	}

	var randPool = new(random.Pool)
	for _, award := range itemConf.Awards {
		randPool.AddItem(award, award.Weight)
	}
	reward := randPool.RandomOne().(*jsondata.StdReward)

	engine.GiveRewards(actor, jsondata.StdRewardVec{reward}, common.EngineGiveRewardParam{LogId: pb3.LogId_LogUseSpringFestivalItem, NoTips: true})
	actor.SendShowRewardsPop(jsondata.StdRewardVec{reward})
	actor.SendProto3(8, 10, &pb3.S2C_8_10{
		ItemId:  req.ItemId,
		DescIdx: req.DescIdx,
	})
	logworker.LogPlayerBehavior(actor, pb3.LogId_LogUseSpringFestivalItem, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.ItemId),
		StrArgs: fmt.Sprintf("%d", req.DescIdx),
	})
}

func PlayerChangeName(actor iface.IPlayer, newName string) uint32 {
	vec := jsondata.GlobalU32Vec("changeNameItem")
	itemId, num := vec[0], int64(vec[1])
	if itemId <= 0 {
		return custom_id.ChangeNameNotFoundConsume
	}

	consume := jsondata.ConsumeVec{
		{Type: custom_id.ConsumeTypeItem, Id: itemId, Count: uint32(num)},
	}

	if !actor.CheckConsumeByConf(consume, false, 0) {
		return custom_id.ChangeNameConsumeNotEnough
	}

	nameLen := utf8.RuneCountInString(newName)
	nameLenLimit := jsondata.GetNameLenLimit()
	if nameLen > int(nameLenLimit) || nameLen <= 0 {
		return custom_id.ChangeNameLenLimitOrEmpty
	}

	if !engine.CheckNameRepeat(newName) {
		return custom_id.ChangeNameIsExist
	}

	if !engine.CheckNameSpecialCharacter(newName) {
		return custom_id.ChangeNameContainFilterChar
	}
	engine.AddPendingName(newName)

	engine.SendWordMonitor(wordmonitor.Name, wordmonitor.ChangePlayerName, newName,
		wordmonitoroption.WithPlayerId(actor.GetId()),
		wordmonitoroption.WithCommonData(actor.BuildChatBaseData(nil)),
		wordmonitoroption.WithDitchId(actor.GetExtraAttrU32(attrdef.DitchId)),
	)

	return 0
}

// 使用回城石: 回到主城
func useHomeStone(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) < 1 {
		player.LogError("useItem:%d, Wrong number of parameters!", param.ItemId)
		return false, false, 0
	}
	if player.InDartCar() {
		player.SendTipMsg(tipmsgid.DartCarIngNotUseItem)
		return
	}
	msg := &pb3.CheckItemUse{ItemId: param.ItemId, ItemType: itemdef.BackToMain}
	err := player.CallActorFunc(actorfuncid.ReqCheckUseMoveItem, msg)
	if err != nil {
		player.LogError("useItem:%d, actorfuncid.ReqCheckUseMoveItem failed!", param.ItemId)
		return false, false, 0
	}
	return true, true, 1
}

// 使用场景传送石: 在当前场景随机传送到可行走点
func useRandTeleportStone(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) < 1 {
		player.LogError("useItem:%d, Wrong number of parameters!", param.ItemId)
		return false, false, 0
	}
	if player.InDartCar() {
		player.SendTipMsg(tipmsgid.DartCarIngNotUseItem)
		return
	}
	msg := &pb3.CheckItemUse{ItemId: param.ItemId, ItemType: itemdef.RandTransfer}
	err := player.CallActorFunc(actorfuncid.ReqCheckUseMoveItem, msg)
	if err != nil {
		player.LogError("useItem:%d, actorfuncid.ReqCheckUseMoveItem failed!", param.ItemId)
		return false, false, 0
	}
	return true, true, 1
}

func useUnLockChatEmoji(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	if len(conf.Param) < 1 {
		player.LogError("useItem:%d, Wrong number of parameters!", param.ItemId)
		return false, false, 0
	}
	eId := conf.Param[0]
	eConf := jsondata.GetChatExpressionConf(eId)
	if eConf == nil || eConf.IsLock == 0 {
		player.LogError("useItem:%d, GetChatExpressionConf failed", param.ItemId)
		return false, false, 0
	}
	binary := player.GetBinaryData()
	if binary.ChatEmojiIds == nil {
		binary.ChatEmojiIds = make([]uint32, len(jsondata.ChatExpressionConfMgr))
	}
	binary.ChatEmojiIds = pie.Uint32s(binary.ChatEmojiIds).Append(eId).Unique()
	player.SendProto3(2, 151, &pb3.S2C_2_151{EmojiData: []uint32{eId}})
	return true, true, 1
}

func onChangeNameMonitorRet(word *wordmonitor.Word) error {
	engine.DelPendingName(word.Content)

	player := manager.GetPlayerPtrById(word.PlayerId)
	if nil == player {
		return nil
	}

	if word.Ret != wordmonitor2.Success {
		player.SendTipMsg(tipmsgid.ChangeName)
		player.SendProto3(2, 11, &pb3.S2C_2_11{
			Code:    custom_id.ChangeNameContainFilterChar,
			NewName: word.Content,
		})
		return nil
	}

	vec := jsondata.GlobalU32Vec("changeNameItem")
	itemId, num := vec[0], int64(vec[1])
	if itemId <= 0 {
		return nil
	}

	consume := jsondata.ConsumeVec{
		{Type: custom_id.ConsumeTypeItem, Id: itemId, Count: uint32(num)},
	}

	if !player.ConsumeByConf(consume, false, common.ConsumeParams{LogId: pb3.LogId_LogPlayerChangeName}) {
		return nil
	}

	newName := word.Content
	DoChangeName(player, newName)
	return nil
}

func DoChangeName(player iface.IPlayer, newName string) {
	engine.AddPlayerName(newName)

	engine.RemovePlayerName(player.GetName())

	player.SetName(newName)

	player.SendProto3(2, 11, &pb3.S2C_2_11{
		Code:    0,
		NewName: player.GetName(),
	})

	player.TriggerEvent(custom_id.AeChangeName)
	player.CallActorFunc(actorfuncid.ChangeName, &pb3.ChangeName{NewName: newName})
}

func init() {
	RegisterSysClass(sysdef.SiRoleInfo, func() iface.ISystem {
		return &PlayerOpSystem{}
	})
	RegisterSysClass(sysdef.SiPlayerChangeName, func() iface.ISystem {
		return &PlayerOpSystem{}
	})

	miscitem.RegCommonUseItemHandle(itemdef.UseItemHomeStone, useHomeStone)
	miscitem.RegCommonUseItemHandle(itemdef.UseItemRandTeleportStone, useRandTeleportStone)
	miscitem.RegCommonUseItemHandle(itemdef.UseItemUnLockChatEmoji, useUnLockChatEmoji)

	net.RegisterSysProto(2, 10, sysdef.SiRoleInfo, (*PlayerOpSystem).c2sViewPlayer)
	net.RegisterSysProto(2, 11, sysdef.SiPlayerChangeName, (*PlayerOpSystem).c2sPlayerChangeName)
	net.RegisterSysProto(8, 10, sysdef.SiRoleInfo, (*PlayerOpSystem).c2sPlayerUseSpringFestivalItem)

	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		engine.RegWordMonitorOpCodeHandler(wordmonitor.ChangePlayerName, onChangeNameMonitorRet)
	})
}
