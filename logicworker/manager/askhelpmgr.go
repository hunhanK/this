/**
 * @Author: zjj
 * @Date: 2024/5/9
 * @Desc:
**/

package manager

import (
	"encoding/json"
	log "github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/pb3"
	"jjyz/base/syncmsg"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

var askHelpInfoIns map[uint64]*pb3.AskHelpInfo
var askHelpSaveFlag = map[uint64]struct{}{}

func LoadAskHelpMgr() error {
	askHelpInfoIns = make(map[uint64]*pb3.AskHelpInfo)

	syncMsg := syncmsg.NewSyncMsg()
	gshare.SendDBMsg(custom_id.GMsgSyncLoadAskHelpData, syncMsg)
	ret, err := syncMsg.Ret()
	if err != nil {
		log.LogError("err:%v", err)
		return err
	}

	lists := ret.(map[uint64]*pb3.AskHelpInfo)
	for k, v := range lists {
		askHelpInfoIns[k] = v
	}
	return nil
}

func SaveAskHelpMgr(sync bool) error {
	m := make(map[uint64]*pb3.AskHelpInfo)
	for actorId := range askHelpSaveFlag {
		m[actorId] = askHelpInfoIns[actorId]
	}

	bytes, err := json.Marshal(m)
	if err != nil {
		return err
	}

	copyData := make(map[uint64]*pb3.AskHelpInfo)
	err = json.Unmarshal(bytes, &copyData)
	if err != nil {
		return err
	}

	askHelpSaveFlag = map[uint64]struct{}{}

	if !sync {
		gshare.SendDBMsg(custom_id.GMsgAskHelpData, copyData)
		return nil
	}

	syncMsg := syncmsg.NewSyncMsg(copyData)
	gshare.SendDBMsg(custom_id.GMsgSyncSaveAskHelpData, syncMsg)
	_, err = syncMsg.Ret()
	if err != nil {
		log.LogError("err:%v", err)
		return err
	}
	return nil
}

func SetAskHelpSaveFlag(actorId uint64) {
	if actorId == 0 {
		return
	}
	askHelpSaveFlag[actorId] = struct{}{}
}

func GetAskHelpInfo(actorId uint64) *pb3.AskHelpInfo {
	info, ok := askHelpInfoIns[actorId]
	if !ok {
		askHelpInfoIns[actorId] = newAskHelpInfo()
		info = askHelpInfoIns[actorId]
	}
	if info.AskRecordMap == nil {
		info.AskRecordMap = make(map[uint64]*pb3.AskRecord)
	}
	if info.CompletedAskCountMap == nil {
		info.CompletedAskCountMap = make(map[uint64]uint32)
	}
	if info.CompletedGiftCountMap == nil {
		info.CompletedGiftCountMap = make(map[uint64]uint32)
	}
	if info.OfflineCompleteAskHelpTimesMap == nil {
		info.OfflineCompleteAskHelpTimesMap = make(map[uint32]uint32)
	}
	if info.QuestMap == nil {
		info.QuestMap = make(map[uint32]*pb3.QuestData)
	}
	return info
}

func newAskHelpInfo() *pb3.AskHelpInfo {
	val := &pb3.AskHelpInfo{}
	return val
}

func init() {
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		err := LoadAskHelpMgr()
		if err != nil {
			log.LogError("LoadAskHelpMgr err:%v", err)
			return
		}
	})

	// 跨周重置次数
	event.RegSysEvent(custom_id.SeNewWeekArrive, func(args ...interface{}) {
		for _, info := range askHelpInfoIns {
			info.CompletedAskCountMap = make(map[uint64]uint32)
			info.CompletedGiftCountMap = make(map[uint64]uint32)
		}
	})
	net.RegisterProto(67, 24, c2SGetAskRecord)
}

func c2SGetAskRecord(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_67_24
	if err := pb3.Unmarshal(msg.Data, &req); nil != err {
		return nil
	}
	info := GetAskHelpInfo(req.TargetId)
	if info == nil {
		player.SendTipMsg(tipmsgid.AskHelpRecordNotFound)
		return nil
	}

	record, ok := info.AskRecordMap[req.Hdl]
	if !ok {
		player.SendTipMsg(tipmsgid.AskHelpRecordNotFound)
		return nil
	}
	player.SendProto3(67, 24, &pb3.S2C_67_24{
		Record: record,
	})
	return nil
}
