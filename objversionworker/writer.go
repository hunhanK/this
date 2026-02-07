/**
 * @Author: ChenJunJi
 * @Date: 2024/09/09
 * @Desc:
**/

package objversionworker

import (
	"encoding/json"
	"errors"
	"github.com/gzjjyz/counter-go-sdk/counter"
	"github.com/gzjjyz/logger"
	micro "github.com/gzjjyz/simple-micro"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"time"
)

var (
	producer counter.Producer
)

type VersionData struct {
	PfId    uint32 `json:"pf_id"`
	ActorId uint64 `json:"actor_id"`
	Version uint32 `json:"version"`
	Data    []byte `json:"data"`
}

func Init() error {
	cfg, flag := micro.MustMeta().GetRedisConn("objversion")
	if !flag {
		return errors.New("meta can not find redis.objversion config")
	}
	var err error
	producer, err = counter.NewRedisProducer(counter.RedisConfig{
		Host:     cfg.Host,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	return err
}

func Flush() {
	if nil != producer {
		producer.Close()
	}
}

func PostObjVersionData(actorId uint64, playerData *pb3.PlayerData) {
	data := &VersionData{
		PfId:    engine.GetPfId(),
		ActorId: actorId,
		Version: uint32(time.Now().Unix()),
		Data:    pb3.CompressByte(playerData),
	}

	buf, err := json.Marshal(data)
	if nil != err {
		logger.LogError("marshal actor version data error! %v", err)
		return
	}
	if err = producer.Add("actor_version_data", buf); nil != err {
		logger.LogError("producer add actor version data error! %v", err)
	}
}

func PostObjVersionDataWithVersion(actorId uint64, version uint32, playerData *pb3.PlayerData) {
	data := &VersionData{
		PfId:    engine.GetPfId(),
		ActorId: actorId,
		Version: version,
		Data:    pb3.CompressByte(playerData),
	}

	buf, err := json.Marshal(data)
	if nil != err {
		logger.LogError("marshal actor version data error! %v", err)
		return
	}
	if err = producer.Add("actor_version_data", buf); nil != err {
		logger.LogError("producer add actor version data error! %v", err)
	}
}

func PostObjVersionGlobalVarData(srvId uint32, globalVarData []byte) {
	if utils.IsDev() {
		return
	}
	data := &VersionData{
		PfId:    engine.GetPfId(),
		ActorId: uint64(srvId),
		Version: uint32(time.Now().Unix()),
		Data:    globalVarData,
	}

	buf, err := json.Marshal(data)
	if nil != err {
		logger.LogError("marshal actor version data error! %v", err)
		return
	}
	if err = producer.Add("actor_version_data", buf); nil != err {
		logger.LogError("producer add actor version data error! %v", err)
	}
}
