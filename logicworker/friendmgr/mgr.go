/**
 * @Author: LvYuMeng
 * @Date: 2023/12/5
 * @Desc:
**/

package friendmgr

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/logicworker/manager"
)

func GenerateFriendshipId() uint64 {
	GetFriendShip().Series++

	var (
		serverId = engine.GetServerId()
		pfId     = engine.GetPfId()
		series   = GetFriendShip().Series
	)

	makeId := uint64(pfId)<<40 | uint64(serverId)<<24 | uint64(series)

	return makeId
}

func NewFriendship(actorId1, actorId2 uint64) *pb3.FriendCommonData {
	friendshipId := GenerateFriendshipId()
	data := GetFriendShip()

	data.FriendCommonData[friendshipId] = &pb3.FriendCommonData{
		Id:       friendshipId,
		ActorId1: actorId1,
		ActorId2: actorId2,
	}

	return data.FriendCommonData[friendshipId]
}

func GetFriendShip() *pb3.FriendShip {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.FriendShip {
		globalVar.FriendShip = &pb3.FriendShip{}
	}
	if nil == globalVar.FriendShip.FriendCommonData {
		globalVar.FriendShip.FriendCommonData = make(map[uint64]*pb3.FriendCommonData)
	}
	if nil == globalVar.FriendShip.MarryApply {
		globalVar.FriendShip.MarryApply = make(map[uint64]*pb3.MarryApply)
	}
	return globalVar.FriendShip
}

func IsExistStatus(id uint64, status uint32) bool {
	if fd, ok := GetFriendCommonDataById(id); ok {
		return utils.IsSetBit(fd.Status, status)
	}
	return false
}

func GetEngagementId(id, actorId uint64) uint64 {
	if fd, ok := GetFriendCommonDataById(id); ok {
		return utils.Ternary(fd.ActorId1 != actorId, fd.ActorId1, fd.ActorId2).(uint64)
	}
	return 0
}

func GetFriendCommonDataById(id uint64) (*pb3.FriendCommonData, bool) {
	data := GetFriendShip()
	fd, ok := data.FriendCommonData[id]
	return fd, ok
}

func BroadProto3ToCouple(id uint64, protoH, protoL uint16, msg pb3.Message) {
	data := GetFriendShip()
	fd, ok := data.FriendCommonData[id]
	if !ok {
		logger.LogError("commonId(%d) send msg(%v) err", id, msg)
		return
	}
	if actor1 := manager.GetPlayerPtrById(fd.ActorId1); nil != actor1 {
		actor1.SendProto3(protoH, protoL, msg)
	}
	if actor2 := manager.GetPlayerPtrById(fd.ActorId2); nil != actor2 {
		actor2.SendProto3(protoH, protoL, msg)
	}
	return
}

func OnDivorce(id uint64) {
	data := GetFriendShip()
	_, ok := data.FriendCommonData[id]
	if !ok {
		return
	}
	delete(data.FriendCommonData, id)
}

func OnCancel(id uint64) {
	data := GetFriendShip()
	_, ok := data.FriendCommonData[id]
	if !ok {
		return
	}
	delete(data.FriendCommonData, id)
}
