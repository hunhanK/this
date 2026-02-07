/**
 * @Author: zjj
 * @Date: 2024/9/26
 * @Desc: 协助
**/

package assistancemgr

import (
	"jjyz/base/pb3"
)

var singleList []*pb3.AssistanceInfo

func GetList() []*pb3.AssistanceInfo {
	return singleList
}

func GetListByWorldAndGuild(guildId uint64) []*pb3.AssistanceInfo {
	list := GetList()
	var copyList []*pb3.AssistanceInfo
	for _, info := range list {
		// 世界求助
		if info.GuildId == 0 {
			copyList = append(copyList, info)
			continue
		}
		// 帮会求助
		if info.GuildId == guildId {
			copyList = append(copyList, info)
			continue
		}
	}
	return copyList
}

func GetAssistanceInfo(hdl uint64) *pb3.AssistanceInfo {
	list := GetList()
	for _, entry := range list {
		if entry.Hdl == hdl {
			return entry
		}
	}
	return nil
}

func GetAssistanceInfoByActorId(actorId uint64) *pb3.AssistanceInfo {
	list := GetList()
	for _, entry := range list {
		if entry.ActorId == actorId {
			return entry
		}
	}
	return nil
}

func DelAssistanceInfo(hdl uint64) *pb3.AssistanceInfo {
	list := GetList()
	var index = -1
	for idx, fightingEntry := range list {
		if fightingEntry.Hdl != hdl {
			continue
		}
		index = idx
		break
	}
	if index >= 0 {
		list = append(list[:index], list[index+1:]...)
	}
	saveList(list)
	return nil
}

func saveList(val []*pb3.AssistanceInfo) {
	singleList = val
}

func AppendNewAssistanceInfo(actorId uint64, entry *pb3.AssistanceInfo) bool {
	list := GetList()
	var index = -1
	for idx, fightingEntry := range list {
		if fightingEntry.ActorId != actorId {
			continue
		}
		index = idx
		break
	}
	if index >= 0 {
		list = append(list[:index], list[index+1:]...)
	}
	list = append(list, entry)
	saveList(list)
	return true
}
