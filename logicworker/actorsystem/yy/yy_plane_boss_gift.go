/**
 * @Author:
 * @Date: 2024/6/19
 * @Desc: 全服运营活动-魔王馈赠
**/

package yy

import (
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
)

type YYPlaneBossGift struct {
	YYBase
}

func (s *YYPlaneBossGift) OnOpen() {
	s.sendToLocalFightSrv(true)
	s.sendToSmallCrossSrv(true)
}

func (s *YYPlaneBossGift) OnEnd() {
	s.sendToLocalFightSrv(false)
	s.sendToSmallCrossSrv(false)
}

func (s *YYPlaneBossGift) sendToLocalFightSrv(isOpen bool) {
	var err error
	if isOpen {
		err = engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FYYActAddMonExtraDrops, &pb3.G2FYYActAddMonExtraDrops{
			ActivityId: s.GetId(),
			Data:       s.packMonsterDropData(),
		})
	} else {
		err = engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FYYActDelMonExtraDrops, &pb3.G2FYYActDelMonExtraDrops{
			ActivityId: s.GetId(),
		})
	}
	if err != nil {
		s.LogError("err:%v", err)
	}
}

func (s *YYPlaneBossGift) sendToSmallCrossSrv(isOpen bool) {
	var err error
	if isOpen {
		err = engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FYYActAddMonExtraDrops, &pb3.G2FYYActAddMonExtraDrops{
			ActivityId: s.GetId(),
			Data:       s.packMonsterDropData(),
		})
	} else {
		err = engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FYYActDelMonExtraDrops, &pb3.G2FYYActDelMonExtraDrops{
			ActivityId: s.GetId(),
		})
	}
	if err != nil {
		s.LogError("err:%v", err)
	}
}

func (s *YYPlaneBossGift) packMonsterDropData() map[uint64]*pb3.YYActMonExtraDrops {
	conf, ok := jsondata.GetYYPlaneBossGiftConf(s.ConfName, s.ConfIdx)
	if !ok {
		return nil
	}

	pkgMsg := make(map[uint64]*pb3.YYActMonExtraDrops)
	for _, mon := range conf.Monsters {
		pkgMsg[mon.MonsterId] = &pb3.YYActMonExtraDrops{DropIds: mon.Drops}
	}

	return pkgMsg
}

func init() {
	yymgr.RegisterYYType(yydefine.YYPlaneBossGift, func() iface.IYunYing {
		return &YYPlaneBossGift{}
	})

	event.RegSysEvent(custom_id.SeFightSrvConnSucc, func(args ...interface{}) {
		yyList := yymgr.GetAllYY(yydefine.YYPlaneBossGift)
		for _, yy := range yyList {
			if yy.IsOpen() {
				yy.(*YYPlaneBossGift).sendToLocalFightSrv(true)
			} else {
				yy.(*YYPlaneBossGift).sendToLocalFightSrv(false)
			}
		}
	})

	event.RegSysEvent(custom_id.SeCrossSrvConnSucc, func(args ...interface{}) {
		yyList := yymgr.GetAllYY(yydefine.YYPlaneBossGift)
		for _, yy := range yyList {
			if yy.IsOpen() {
				yy.(*YYPlaneBossGift).sendToSmallCrossSrv(true)
			} else {
				yy.(*YYPlaneBossGift).sendToSmallCrossSrv(false)
			}
		}
	})
}
