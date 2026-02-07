/**
 * @Author: LvYuMeng
 * @Date: 2023/12/7
 * @Desc:
**/

package friendmgr

import (
	"jjyz/base/custom_id"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/logworker"
)

type FriendKey struct {
	ActorIdMin uint64
	ActorIdMax uint64
}

type FriendVal struct {
	Intimacy   uint32
	IntimacyLv uint32
}

type IntimacyMgr struct {
	fMap map[FriendKey]FriendVal
}

var intimacyMgr = &IntimacyMgr{
	fMap: make(map[FriendKey]FriendVal),
}

func GetIntimacyMgr() *IntimacyMgr {
	return intimacyMgr
}

func (im *IntimacyMgr) Save() {
	for key, val := range im.fMap {
		gshare.SendDBMsg(custom_id.GMsgSaveFriendIntimacy, &pb3.CommonSt{
			U32Param:  val.Intimacy,
			U64Param:  key.ActorIdMin,
			U64Param2: key.ActorIdMax,
		})
		gshare.SendDBMsg(custom_id.GMsgSaveFriendIntimacy, &pb3.CommonSt{
			U32Param:  val.Intimacy,
			U64Param:  key.ActorIdMax,
			U64Param2: key.ActorIdMin,
		})
	}
}

func (im *IntimacyMgr) IsIntimacyLoad(actorId1, actorId2 uint64) (bool, uint32, uint32) {
	if actorId1 > actorId2 {
		actorId1, actorId2 = actorId2, actorId1
	}
	if res, ok := im.fMap[FriendKey{
		ActorIdMin: actorId1,
		ActorIdMax: actorId2,
	}]; ok {
		return ok, res.Intimacy, res.IntimacyLv
	}
	return false, 0, 0
}

func (im *IntimacyMgr) DelIntimacy(actorId1, actorId2 uint64) {
	if actorId1 > actorId2 {
		actorId1, actorId2 = actorId2, actorId1
	}
	key := FriendKey{
		ActorIdMin: actorId1,
		ActorIdMax: actorId2,
	}

	var oldLv, oldIntimacy uint32
	if v, ok := im.fMap[key]; !ok {
		oldLv, oldIntimacy = v.IntimacyLv, v.Intimacy
	}

	delete(im.fMap, key)

	im.onIntimacyChange(actorId1, actorId2, 0, 0, oldLv, oldIntimacy, true)
	im.onIntimacyChange(actorId2, actorId1, 0, 0, oldLv, oldIntimacy, true)
}

func (im *IntimacyMgr) onIntimacyChange(actorId, targetId uint64, lv, intimacy, oldLv, oldIntimacy uint32, isSend bool) {
	player := manager.GetPlayerPtrById(actorId)
	if nil == player {
		return
	}

	player.TriggerEvent(custom_id.AeIntimacyChange, targetId, lv, intimacy, oldLv, oldIntimacy)
	im.logIntimacy(player, targetId, 0, oldIntimacy)
	if isSend {
		player.SendProto3(14, 14, &pb3.S2C_14_13{
			Intimacy:   intimacy,
			IntimacyLv: lv,
		})
	}
}

func (im *IntimacyMgr) logIntimacy(player iface.IPlayer, targetId uint64, intimacy, oldIntimacy uint32) {
	logworker.LogPlayerBehavior(player, pb3.LogId_LogAddIntimacy, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"targetId":    targetId,
			"intimacy":    intimacy,
			"oldIntimacy": oldIntimacy,
		}),
	})
}

func (im *IntimacyMgr) AddIntimacy(actorId1, actorId2 uint64, add uint32, isAdd, isSend bool) {
	if actorId1 > actorId2 {
		actorId1, actorId2 = actorId2, actorId1
	}
	key := FriendKey{
		ActorIdMin: actorId1,
		ActorIdMax: actorId2,
	}
	if _, ok := im.fMap[key]; !ok {
		im.fMap[key] = FriendVal{}
	}

	val := im.fMap[key]
	oldLv, oldIntimacy := val.IntimacyLv, val.Intimacy

	if isAdd {
		val.Intimacy += add
	} else {
		val.Intimacy = add
	}

	val.IntimacyLv = jsondata.GetIntimacyLevelByExp(val.Intimacy)

	im.fMap[key] = val

	im.onIntimacyChange(actorId1, actorId2, val.IntimacyLv, val.Intimacy, oldLv, oldIntimacy, isSend)
	im.onIntimacyChange(actorId2, actorId1, val.IntimacyLv, val.Intimacy, oldLv, oldIntimacy, isSend)
}
