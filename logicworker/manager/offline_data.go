/**
 * @Author: twl
 * @Desc: 玩家离线数据
 * @Date: 2023/03/21 20:35
 */

package manager

import (
	"google.golang.org/protobuf/proto"
	"jjyz/base"
	"jjyz/base/compress"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/db"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gcommon"
	"jjyz/gameserver/model"

	"github.com/gzjjyz/logger"
	"golang.org/x/exp/maps"
)

type offlineSysData struct {
	dirty  bool        // 是否已修改
	dataId uint32      // 数据id
	data   pb3.Message // 数据
}

// 玩家离线数据 map[actorId]map[dataId]proto.Message
var (
	offlineData = make(map[uint64]map[uint32]*offlineSysData)
	allBaseData = make(map[uint64]pb3.Message)
)

func GetOfflineCount() uint32 {
	return uint32(len(offlineData))
}

func IsActorActive(actorId uint64) bool {
	now := time_util.NowSec()
	time := GetLastLogoutTime(actorId)
	return now-time < gshare.DAY_SECOND*7
}

func GetLastLogoutTime(playerId uint64) uint32 {
	player := GetPlayerPtrById(playerId)
	if nil != player {
		return time_util.NowSec()
	} else {
		if data, ok := GetData(playerId, gshare.ActorDataBase).(*pb3.PlayerDataBase); ok {
			return data.GetLastLogoutTime()
		}
	}
	return 0
}

func GetActivePlayerCnt() uint32 {
	cnt := uint32(0)
	for actorId, _ := range offlineData {
		if IsActorActive(actorId) {
			cnt++
		}
	}
	return cnt
}

func LoadOfflineData() error {
	var datas []*model.OfflineData

	err := db.OrmEngine.Find(&datas)

	if nil != err {
		logger.LogError("load offline data error!!! err:%v", err)
		return err
	}

	offlineData = make(map[uint64]map[uint32]*offlineSysData)

	idMap := make(map[uint64]struct{})

	for _, line := range datas {
		playerId := line.ActorId

		if _, ok := offlineData[playerId]; !ok {
			offlineData[playerId] = make(map[uint32]*offlineSysData)
		}

		sysId := line.SysId

		fn := GetInitFn(sysId)
		if nil == fn {
			continue
		}

		pbData := fn()
		if nil == pbData {
			continue
		}

		err = pb3.Unmarshal(compress.UncompressPb(line.Data), pbData)
		if nil != err {
			logger.LogError("load offline data error! playerId=%d, dataId=%d %v", playerId, sysId, err)
			continue
		}

		offlineData[playerId][sysId] = &offlineSysData{
			dirty:  false,
			dataId: sysId,
			data:   pbData,
		}

		idMap[playerId] = struct{}{}
		if sysId == gshare.ActorDataBase {
			allBaseData[playerId] = pbData
		}
	}

	event.TriggerSysEvent(custom_id.SeOfflineDataLoadSucc, idMap)
	logger.LogInfo("load offline data finish")
	return nil
}

func GetOfflineData(actorId uint64, dataId uint32) pb3.Message {
	if actorData, ok := offlineData[actorId]; ok {
		if st, ok := actorData[dataId]; ok {
			return st.data
		}
	}
	return nil
}

func setOfflineData(actor iface.IPlayer, args ...interface{}) {
	actorId := actor.GetId()
	_, ok := offlineData[actorId]
	if !ok {
		offlineData[actorId] = make(map[uint32]*offlineSysData)
	}
	for dataId, fn := range mDataFn {
		offlineData[actorId][dataId] = &offlineSysData{
			dirty:  true,
			dataId: dataId,
			data:   fn(actor),
		}
	}
}

func AllOfflineDataBaseDo(fn func(p *pb3.PlayerDataBase) (notStop bool)) {
	for playerId := range offlineData {
		if data, ok := GetData(playerId, gshare.ActorDataBase).(*pb3.PlayerDataBase); ok {
			if !fn(data) {
				break
			}
		}
	}
	return
}

// GetOfflinePlayerByCond  通过筛选条件过滤符合条件的离线玩家
func GetOfflinePlayerByCond(condFn func(p *pb3.PlayerDataBase) bool, getNum uint32) []*pb3.PlayerDataBase {
	var playerList []*pb3.PlayerDataBase
	// 初始化
	for playerId := range offlineData {
		if getNum > 0 && uint32(len(playerList)) >= getNum { // 满了
			break
		}
		if data, ok := GetData(playerId, gshare.ActorDataBase).(*pb3.PlayerDataBase); ok {
			if condFn(data) {
				playerList = append(playerList, data)
			}
		}
	}
	return playerList
}

// CheckSaveOfflineData 发送到db协程保存
func CheckSaveOfflineData() {
	for playerId, dataMap := range offlineData {
		for dataId, st := range dataMap {
			if !st.dirty {
				continue
			}

			sData, err := compress.PB3ToByte(st.data)
			if nil != err {
				logger.LogError("player data to string error!!! playerId=%d, dataId=%d, err=%v", playerId, dataId, err)
				return
			}
			st.dirty = false // 先字节化，再入db协程，发到db协程处理，如果db协程保存失败，则重置为true
			m := model.OfflineData{
				ActorId: playerId,
				SysId:   dataId,
				Data:    sData,
			}
			gshare.SendDBMsg(custom_id.GMsgSaveOfflineData, m)
		}
	}
}

func HasOfflineData(actorId uint64) bool {
	_, ok := offlineData[actorId]
	return ok
}

func onSaveOfflineDataFailed(args ...interface{}) {
	if !gcommon.CheckArgsCount("onSaveOfflineDataFailed", 2, len(args)) {
		logger.LogError("---------onSaveOfflineDataFailed  args params != 2")
		return
	}

	playerId, ok := args[0].(uint64)
	if !ok {
		logger.LogError("onSaveOfflineDataFailed, playerId is not uint64")
		return
	}
	dataId, ok := args[1].(uint32)
	if !ok {
		logger.LogError("onSaveOfflineDataFailed, dataId is not uint32")
		return
	}

	if dataMap, ok := offlineData[playerId]; ok {
		if st, ok := dataMap[dataId]; ok {
			if !st.dirty {
				st.dirty = true
				logger.LogInfo("reset player offline data dirty! playerId=%d, dataId=%d", playerId, dataId)
			}
		}
	}
}

func syncBaseData2Cross(serverType base.ServerType) {
	st := pb3.SyncPlayerBaseData{
		PfId:  engine.GetPfId(),
		SrvId: engine.GetServerId(),
	}
	for _, line := range allBaseData {
		if bytes, err := pb3.Marshal(line); nil == err {
			st.Datas = append(st.Datas, bytes)
		} else {
			logger.LogError("sync player base data to cross error! %v", err.Error())
		}
	}
	engine.CallFightSrvFunc(serverType, sysfuncid.G2FPlayerBaseData, &st)
}

// 玩家基础数据获取函数
func baseData(actor iface.IPlayer) pb3.Message {
	st := actor.ToPlayerDataBase()
	return st
}

// 玩家基础数据获取函数
func actorShowStrAttr(actor iface.IPlayer) pb3.Message {
	var msg = &pb3.PlayerShowStrAttr{
		ShowStr: actor.PackageShowStr(),
	}
	return msg
}

// 玩家属性获取函数
func propertyData(actor iface.IPlayer) pb3.Message {
	st := &pb3.OfflineProperty{}
	if sys := actor.GetAttrSys(); nil != st {
		sys.PackPropertyData(st)
	}
	return st
}

// 玩家获取基础装备
func equipData(actor iface.IPlayer) pb3.Message {
	st := &pb3.OfflineEquipData{}
	itemPool := actor.GetMainData().ItemPool
	var copyEquips = make([]*pb3.ItemSt, len(itemPool.Equips))
	for i, v := range itemPool.Equips {
		copyEquips[i] = proto.Clone(v).(*pb3.ItemSt)
	}
	var copyMarryEquips = make([]*pb3.ItemSt, len(itemPool.MarryEquips))
	for i, v := range itemPool.MarryEquips {
		copyMarryEquips[i] = proto.Clone(v).(*pb3.ItemSt)
	}
	st.Equip = copyEquips
	st.MarryEquips = copyMarryEquips
	st.EquipSuit = maps.Clone(actor.GetBinaryData().EquipSuitStrong)
	st.Intensify = maps.Clone(actor.GetBinaryData().Intensify)
	st.EquipAwaken = proto.Clone(actor.GetBinaryData().EquipAwaken).(*pb3.EquipAwaken)
	st.KillDragonEquipSuitData = proto.Clone(actor.GetBinaryData().KillDragonEquipSuitData).(*pb3.KillDragonEquipSuitData)
	st.ExclusiveSign = maps.Clone(actor.GetBinaryData().GetExclusiveSign().GetLv())
	return st
}

func init() {
	event.RegActorEvent(custom_id.AeLogout, setOfflineData)
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		gshare.RegisterGameMsgHandler(custom_id.GMsgSaveOfflineDataFailed, onSaveOfflineDataFailed)
	})
	Register(gshare.ActorDataBase, func() pb3.Message {
		return &pb3.PlayerDataBase{}
	}, baseData)

	Register(gshare.ActorShowStr, func() pb3.Message {
		return &pb3.PlayerShowStrAttr{}
	}, actorShowStrAttr)

	Register(gshare.ActorDataProperty, func() pb3.Message {
		return &pb3.OfflineProperty{}
	}, propertyData)

	Register(gshare.ActorDataEquip, func() pb3.Message {
		return &pb3.OfflineEquipData{}
	}, equipData)

	event.RegSysEvent(custom_id.SeCrossSrvConnSucc, func(args ...interface{}) {
		st := args[0].(*pb3.SyncConnectCrossSuccess)
		if nil == st {
			return
		}

		srvType := base.ServerType(st.CrossType)
		switch srvType {
		case base.SmallCrossServer:
			syncBaseData2Cross(srvType)
		}
	})
}
