package fairydynastyjobrank

import (
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/logicworker/manager"
)

var zongMenRank = &base.Rank{}

func UpdateZongMenRank(playerId uint64, score int64) {
	if zongMenRank == nil {
		return
	}

	conf := jsondata.GetFairyDynastyJobCommonConf()
	if conf == nil {
		return
	}

	if score < int64(conf.ZongMenRankLimit) {
		return
	}

	zongMenRank.Update(playerId, score)
}

func LoadFairyDynastyDomainZongMenJobRank() {
	commonConf := jsondata.GetFairyDynastyJobCommonConf()
	if commonConf == nil {
		return
	}

	zongMenRank.Init(commonConf.MaxRank)

	staticVar := gshare.GetStaticVar()
	if staticVar.FairyDynastyZongMenJobRank == nil {
		return
	}

	for _, ori := range staticVar.FairyDynastyZongMenJobRank {
		zongMenRank.Update(ori.Id, ori.Score)
	}
}

func SaveFairyDynastyDomainRank(args ...interface{}) {
	tmpRanks := zongMenRank.Map(func(rank int, ori *pb3.OneRankItem) any {
		return ori
	})

	staticVar := gshare.GetStaticVar()

	staticVar.FairyDynastyZongMenJobRank = make([]*pb3.OneRankItem, 0, len(tmpRanks))

	for _, v := range tmpRanks {
		foo := v.(*pb3.OneRankItem)
		if foo != nil {
			staticVar.FairyDynastyZongMenJobRank = append(staticVar.FairyDynastyZongMenJobRank, foo)
		}
	}
}

func PackZongMenRank() []*pb3.FairyDynastyJobRankInfo {
	if zongMenRank == nil {
		return nil
	}

	ranksTmp := zongMenRank.Map(func(rank int, ori *pb3.OneRankItem) any {
		playerDataBase, ok := manager.GetData(ori.Id, gshare.ActorDataBase).(*pb3.PlayerDataBase)
		if !ok {
			return nil
		}

		rankInfo := &pb3.FairyDynastyJobRankInfo{
			Camp: uint32(playerDataBase.SmallCrossCamp),
		}

		rankInfo.Base = &pb3.RankInfo{
			Rank:      uint32(rank),
			Key:       ori.Id,
			Value:     ori.Score,
			PlayerId:  ori.Id,
			Head:      playerDataBase.Head,
			VipLv:     playerDataBase.VipLv,
			Name:      playerDataBase.Name,
			Job:       playerDataBase.Job,
			GuildName: playerDataBase.GuildName,
			HeadFrame: playerDataBase.HeadFrame,
			Appear:    make(map[uint32]int64),
		}

		for k, sas := range playerDataBase.AppearInfo {
			rankInfo.Base.Appear[k] = int64(sas.SysId)<<32 | int64(sas.AppearId)
		}

		return rankInfo
	})

	ranks := make([]*pb3.FairyDynastyJobRankInfo, 0, len(ranksTmp))

	for _, v := range ranksTmp {
		foo, ok := v.(*pb3.FairyDynastyJobRankInfo)

		if !ok {
			continue
		}
		ranks = append(ranks, foo)
	}

	return ranks
}

func init() {
	// register server init load rank
	event.RegSysEvent(custom_id.SeServerInit, func(args ...interface{}) {
		LoadFairyDynastyDomainZongMenJobRank()
	})

	event.RegSysEvent(custom_id.SeBeforeSaveGlobalVar, SaveFairyDynastyDomainRank)
}
