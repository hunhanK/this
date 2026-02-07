/**
 * @Author: LvYuMeng
 * @Date: 2025/11/28
 * @Desc:
**/

package yy

import (
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/ranktype"
)

type FightValueDynamic struct {
	YYBase
}

func (s *FightValueDynamic) OnInit() {

}

func (s *FightValueDynamic) OnOpen() {
	conf := jsondata.GetYYFightValueDynamicConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}

	manager.AllOnlinePlayerDo(func(player iface.IPlayer) {
		initValue := s.getFightValue(player)
		pData := s.getPlayerData(player.GetId())
		pData.InitFightValue = initValue
		pData.MaxFightValue = initValue
		s.updateMaxValueRank(player)
	})
}

func (s *FightValueDynamic) getData() *pb3.FightValueDynamicData {
	globalVar := gshare.GetStaticVar()
	if globalVar.YyDatas == nil {
		globalVar.YyDatas = &pb3.YYDatas{}
	}
	if globalVar.YyDatas.FightValueDynamicData == nil {
		globalVar.YyDatas.FightValueDynamicData = make(map[uint32]*pb3.FightValueDynamicData)
	}
	if globalVar.YyDatas.FightValueDynamicData[s.Id] == nil {
		globalVar.YyDatas.FightValueDynamicData[s.Id] = &pb3.FightValueDynamicData{}
	}
	if globalVar.YyDatas.FightValueDynamicData[s.Id].PlayerData == nil {
		globalVar.YyDatas.FightValueDynamicData[s.Id].PlayerData = make(map[uint64]*pb3.FightValueDynamicPlayerData)
	}
	return globalVar.YyDatas.FightValueDynamicData[s.Id]
}

func (s *FightValueDynamic) getPlayerData(playerId uint64) *pb3.FightValueDynamicPlayerData {
	actData := s.getData()
	if _, ok := actData.PlayerData[playerId]; !ok {
		actData.PlayerData[playerId] = &pb3.FightValueDynamicPlayerData{}
	}
	return actData.PlayerData[playerId]
}

func (s *FightValueDynamic) PlayerLogin(player iface.IPlayer) {
}

func (s *FightValueDynamic) PlayerReconnect(player iface.IPlayer) {
}

func (s *FightValueDynamic) onFightValueChange(player iface.IPlayer) {
	value := s.getFightValue(player)
	pData := s.getPlayerData(player.GetId())
	if pData.MaxFightValue < value {
		pData.MaxFightValue = value
	}
	s.updateMaxValueRank(player)
}

func (s *FightValueDynamic) updateMaxValueRank(player iface.IPlayer) {
	conf := jsondata.GetYYFightValueDynamicConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}
	pData := s.getPlayerData(player.GetId())
	if pData.InitFightValue > pData.MaxFightValue {
		player.LogError("value error:%+v", pData)
		return
	}
	diff := pData.MaxFightValue - pData.InitFightValue
	if diff == 0 {
		return
	}
	manager.UpdatePlayScoreRank(ranktype.PlayScoreRankType(conf.Type), player, diff, false, pData.MaxFightValue)
}

func (s *FightValueDynamic) getFightValue(player iface.IPlayer) (fightValue int64) {
	conf := jsondata.GetYYFightValueDynamicConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return
	}
	attrSys := player.GetAttrSys()
	for _, calc := range conf.SysCal {
		fightValue += attrSys.GetSysPower(calc)
	}
	return
}

func (s *FightValueDynamic) ResetData() {
	globalVar := gshare.GetStaticVar()
	if nil == globalVar.YyDatas || nil == globalVar.YyDatas.FightValueDynamicData {
		return
	}
	delete(globalVar.YyDatas.FightValueDynamicData, s.GetId())
}

func handleFightValueDynamicChange(player iface.IPlayer, _ ...interface{}) {
	yymgr.EachAllYYObj(yydefine.YYFightValueDynamic, func(obj iface.IYunYing) {
		sys, ok := obj.(*FightValueDynamic)
		if !ok || !sys.IsOpen() {
			return
		}
		sys.onFightValueChange(player)
	})
}

func init() {
	yymgr.RegisterYYType(yydefine.YYFightValueDynamic, func() iface.IYunYing {
		return &FightValueDynamic{}
	})

	event.RegActorEvent(custom_id.AeAfterUpdateSysPowerMap, handleFightValueDynamicChange)
}
