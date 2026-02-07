/**
 * @Author: zjj
 * @Date: 2024年4月8日
 * @Desc: 仙宗回收
**/

package pyy

import (
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
)

// 仙宗回收

type GuildReCycleSys struct {
	PlayerYYBase
}

func (s *GuildReCycleSys) OnOpen() {
	conf := jsondata.GetGuildRecycleConf(s.ConfName, s.ConfIdx)
	if conf == nil {
		s.LogWarn("%s GuildReCycleSys ot found conf", s.GetPrefix())
		return
	}
	s.GetPlayer().ResetSpecCycleBuy(custom_id.StoreTypeGuildRecycle, conf.SubStoreType)
	data := s.GetData()
	data.SubStoreType = conf.SubStoreType
	s.S2CInfo()
}

func (s *GuildReCycleSys) OnReconnect() {
	if !s.IsOpen() {
		return
	}
	s.S2CInfo()
}

func (s *GuildReCycleSys) OnAfterLogin() {
	if !s.IsOpen() {
		return
	}
	s.addMonExtraDrops()
	s.S2CInfo()
}

func (s *GuildReCycleSys) OnEnd() {
	s.delMonExtraDrops()
}

func (s *GuildReCycleSys) ResetData() {
	yyData := s.GetYYData()
	if nil == yyData.GuildRecycle {
		return
	}
	delete(yyData.GuildRecycle, s.Id)
}

func (s *GuildReCycleSys) GetData() *pb3.PYY_GuildRecycle {
	yyData := s.GetYYData()
	if nil == yyData.GuildRecycle {
		yyData.GuildRecycle = make(map[uint32]*pb3.PYY_GuildRecycle)
	}
	if nil == yyData.GuildRecycle[s.Id] {
		yyData.GuildRecycle[s.Id] = &pb3.PYY_GuildRecycle{}
	}
	return yyData.GuildRecycle[s.Id]
}

func (s *GuildReCycleSys) S2CInfo() {
	s.SendProto3(60, 0, &pb3.S2C_60_0{
		ActiveId:     s.Id,
		GuildRecycle: s.GetData(),
	})
}

func (s *GuildReCycleSys) OnLoginFight() {
	s.addMonExtraDrops()
}

func (s *GuildReCycleSys) addMonExtraDrops() {
	conf := jsondata.GetGuildRecycleConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		s.LogError("guild recycle conf is nil")
		return
	}
	for _, mon := range conf.Monsters {
		err := s.GetPlayer().CallActorFunc(actorfuncid.AddMonExtraDrops, &pb3.AddActorMonExtraDrops{
			SysId:   s.Id,
			MonId:   mon.MonsterId,
			DropIds: mon.Drops,
		})
		if err != nil {
			s.LogWarn("err:%v", err)
		}

	}
}

func (s *GuildReCycleSys) delMonExtraDrops() {
	conf := jsondata.GetGuildRecycleConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		s.LogError("guild recycle conf is nil")
		return
	}
	for _, mon := range conf.Monsters {
		err := s.GetPlayer().CallActorFunc(actorfuncid.DelMonExtraDrops, &pb3.DelActorMonExtraDrops{
			SysId:   s.Id,
			MonId:   mon.MonsterId,
			DropIds: mon.Drops,
		})
		if err != nil {
			s.GetPlayer().LogWarn("err:%v", err)
		}
	}
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYGuildRecycle, func() iface.IPlayerYY {
		return &GuildReCycleSys{}
	})
}
