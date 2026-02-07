package cache

import (
	"github.com/gzjjyz/srvlib/utils"
	"io/ioutil"
	"jjyz/base/compress"
	"jjyz/base/custom_id"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/objversionworker"
	"os"

	"github.com/gzjjyz/logger"
)

type cacheSt struct {
	Pb3Data    *pb3.PlayerData
	RemoteAddr string
	Dirty      bool
}

var (
	actorCaches map[uint64]*cacheSt
)

func SaveActorDataToCache(actorId uint64, playerData *pb3.PlayerData, remoteAddr string) {
	if nil == actorCaches {
		return
	}

	if _, ok := actorCaches[actorId]; !ok {
		tmp := new(cacheSt)
		tmp.RemoteAddr = remoteAddr
		actorCaches[actorId] = tmp
	}

	actorCaches[actorId].Pb3Data = playerData
	actorCaches[actorId].Dirty = true
	// 写消息队列放到最后
	objversionworker.PostObjVersionData(actorId, playerData)
}

func LoadActorDataFromCache(actorId uint64) *pb3.PlayerData {
	if nil == actorCaches {
		return nil
	}

	if cache, ok := actorCaches[actorId]; ok {
		return cache.Pb3Data
	}
	return nil
}

func GetActorCaches() map[uint64]*cacheSt {
	return actorCaches
}

func DirtyAllCache() {
	for _, v := range actorCaches {
		v.Dirty = true
	}
}

// LoadActorCacheFromFile 后台指令
func LoadActorCacheFromFile(actorId uint64, fileName string) {
	data, err := ioutil.ReadFile(utils.GetCurrentDir() + fileName)
	if nil != err && os.IsNotExist(err) {
		logger.LogError("load actor cache from file error!! %v", err)
		return
	}

	if len(data) <= 0 {
		logger.LogError("load actor cache from file error!! buffer is nil")
		return
	}

	playerData := &pb3.PlayerData{}
	if err = pb3.Unmarshal(compress.UncompressPb(data), playerData); nil != err {
		logger.LogError("reload actor cache error! %v", err)
		return
	}

	playerData.ActorId = actorId
	SaveActorDataToCache(actorId, playerData, "")

	gshare.SendGameMsg(custom_id.GMsgReloadActorFinish, actorId, data)
}

func init() {
	actorCaches = make(map[uint64]*cacheSt)
}
