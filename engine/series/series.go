package series

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/db"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/model"
	"math"
)

var (
	time_       uint32 //当前时间戳
	series_     uint32 //序列号
	server_id_  uint32 //服务器id
	MailSeries_ uint32 //邮件序列号
)

var (
	mActorIdSeries = make(map[uint32]uint32) //服务器ID、玩家序列号映射表
)

func UpdateTime(t uint32) {
	if t != time_ {
		time_ = t
		series_ = 1
	}
}

func SetServerId(srvId uint32) {
	server_id_ = srvId
}

func AllocSeries() (uint64, error) {
	series := series_
	series_++
	return utils.Make64(time_, server_id_<<16|series), nil
}

// 合服取最大的值
func AllocMailIdSeries() uint64 {
	MailSeries_++
	return utils.Make64(MailSeries_, server_id_)
}

func GetActorIdSeries(sId uint32) (uint32, uint32) {
	if _, ok := mActorIdSeries[sId]; !ok {
		return 0, custom_id.ErrSrvId
	}
	id := mActorIdSeries[sId]
	if id >= math.MaxUint32 {
		return 0, custom_id.MaxActorId
	}
	mActorIdSeries[sId]++
	return mActorIdSeries[sId], 0
}

func UpdateActorSeries(id uint64) {
	series := base.GetSeriesByPlayerId(id)
	sId := engine.GetServerId()
	if mActorIdSeries[sId] < series {
		mActorIdSeries[sId] = series
	}
}

func LoadSeries() error {
	mActorIdSeries[engine.GetServerId()] = 0

	var series []*model.ActorSeries
	err := db.OrmEngine.Find(&series)
	if nil != err {
		return err
	}
	for _, s := range series {
		mActorIdSeries[s.ServerId] = s.Series
	}
	return nil
}

func SaveSeries() error {
	var list []*model.ActorSeries
	err := db.OrmEngine.Find(&list)
	if nil != err {
		return err
	}

	tmp := make(map[uint32]*model.ActorSeries)
	for _, s := range list {
		tmp[s.ServerId] = s
	}

	for sId, series := range mActorIdSeries {
		if data, ok := tmp[sId]; !ok {
			// 理论上只有新服第一次才会有插入，所以不做批量了
			if _, err = db.OrmEngine.Insert(&model.ActorSeries{ServerId: sId, Series: series}); nil != err {
				logger.LogError("update actor series error. serverId:%d, series:%d, err:%v", sId, series, err)
			}
		} else {
			if data.Series != series {
				if _, err = db.OrmEngine.ID(data.Id).Update(&model.ActorSeries{Series: series}); nil != err {
					logger.LogError("update actor series error. serverId:%d, series:%d, err:%v", sId, series, err)
				}
			}
		}
	}

	return nil
}
