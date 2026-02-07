/**
 * @Author: lzp
 * @Date: 2024/11/18
 * @Desc:
**/

package pyy

import (
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
)

type DoubleDrops struct {
	PlayerYYBase
	isSync bool
}

func (s *DoubleDrops) OnReconnect() {
}

func (s *DoubleDrops) NewDay() {
	if !s.IsOpen() {
		return
	}
	day := s.GetOpenDay()
	s.syncDelMonDropRateMulti(day - 1)
	s.syncAddMonDropRateMulti(day)
	s.isSync = true
}

func (s *DoubleDrops) OnAfterLogin() {
	if !s.IsOpen() {
		return
	}
	if s.isSync {
		return
	}
	day := s.GetOpenDay()
	s.syncDelMonDropRateMulti(day - 1)
	s.syncAddMonDropRateMulti(day)
}

func (s *DoubleDrops) OnOpen() {
	day := s.GetOpenDay()
	s.syncAddMonDropRateMulti(day)
}

func (s *DoubleDrops) OnEnd() {
	day := s.GetOpenDay()
	s.syncDelMonDropRateMulti(day)
}

func (s *DoubleDrops) syncAddMonDropRateMulti(day uint32) {
	conf := jsondata.GetPYYDoubleDropsConfByDay(s.ConfName, s.ConfIdx, day)
	if conf == nil {
		return
	}

	for _, mon := range conf.MonsterDrops {
		err := s.GetPlayer().CallActorFunc(actorfuncid.AddMonDropRateMulti, &pb3.AddMonDropRateMulti{
			MonId:    mon.MonId,
			SysId:    s.Id,
			AddRate:  mon.AddDropRate,
			AddMulti: mon.AddDropMulti,
		})
		if err != nil {
			s.GetPlayer().LogWarn("err:%v", err)
		}
	}
}

func (s *DoubleDrops) syncDelMonDropRateMulti(day uint32) {
	conf := jsondata.GetPYYDoubleDropsConfByDay(s.ConfName, s.ConfIdx, day)
	if conf == nil {
		return
	}

	for _, mon := range conf.MonsterDrops {
		err := s.GetPlayer().CallActorFunc(actorfuncid.DelMonDropRateMulti, &pb3.DelMonDropRateMulti{
			MonId:    mon.MonId,
			SysId:    s.Id,
			AddRate:  mon.AddDropRate,
			AddMulti: mon.AddDropMulti,
		})
		if err != nil {
			s.GetPlayer().LogWarn("err:%v", err)
		}
	}
}

func init() {
	pyymgr.RegPlayerYY(yydefine.PYYDoubleDrops, func() iface.IPlayerYY {
		return &DoubleDrops{}
	})
}
