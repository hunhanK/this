package manager

import (
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"

	"github.com/gzjjyz/logger"
)

var FairylandRankMgr *base.Rank

func loadFairylandRank(args ...interface{}) {
	FairylandRankMgr = &base.Rank{}

	rankStored := gshare.GetStaticVar().FairylandRank

	if rankStored == nil {
		rankStored = make([]*pb3.OneRankItem, 0)
	}

	FairylandRankMgr.Init(jsondata.GetFairyLandCommonConf().RankLimit)

	for _, ori := range rankStored {
		FairylandRankMgr.Update(ori.Id, ori.Score)
	}
}

func saveFairyLandRank(args ...interface{}) {
	if FairylandRankMgr == nil {
		return
	}

	rankStored := make([]*pb3.OneRankItem, 0)

	foo := FairylandRankMgr.Map(func(rank int, item *pb3.OneRankItem) any {
		return item
	})

	for _, v := range foo {
		rankitem, ok := v.(*pb3.OneRankItem)
		if !ok {
			continue
		}
		rankStored = append(rankStored, rankitem)
	}
	gshare.GetStaticVar().FairylandRank = rankStored

}

func GetFairyLandRankMgr() *base.Rank {
	if FairylandRankMgr == nil {
		logger.LogError("FairyLandRankMgr is nil")
	}
	return FairylandRankMgr
}

func init() {
	event.RegSysEvent(custom_id.SeServerInit, loadFairylandRank)
	event.RegSysEvent(custom_id.SeBeforeSaveGlobalVar, saveFairyLandRank)
}
