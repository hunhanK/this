/**
 * @Author: lzp
 * @Date: 2023/12/5
 * @Desc: 宗门灵兽
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/activity"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils"
)

type SectBeastSys struct {
	Base
}

func (sys *SectBeastSys) OnLogin() {
	sys.PushSectBeast()
	sys.PushSectBeastDamage()
}

func (sys *SectBeastSys) OnReconnect() {
	sys.PushSectBeast()
	sys.PushSectBeastDamage()
}

func (sys *SectBeastSys) OnOpen() {
	sys.PushSectBeast()
}

func (sys *SectBeastSys) GetData() *pb3.SectBeastData {
	if sys.GetBinaryData().SectData == nil {
		sys.GetBinaryData().SectData = &pb3.SectBeastData{}
	}

	sectData := sys.GetBinaryData().SectData

	if sectData.CanGetRewards == nil {
		sectData.CanGetRewards = make([]uint32, 0)
	}

	if sectData.IsRewards == nil {
		sectData.IsRewards = make([]uint32, 0)
	}

	return sectData
}

func (sys *SectBeastSys) AddExp(param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	useConf := jsondata.GetUseItemConfById(conf.ItemId)
	if useConf == nil || len(useConf.Param) <= 0 {
		return false, false, 0
	}

	if len(conf.Param) < 1 {
		return false, false, 0
	}

	sectConf := jsondata.GetSectBeastConf()
	if sectConf == nil {
		return false, false, 0
	}

	expConf, ok := sectConf.ExpItems[conf.ItemId]
	if !ok {
		return false, false, 0
	}

	// 获得奖励
	if !engine.GiveRewards(sys.owner, expConf.Awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogSectBeastFeedReward}) {
		return false, false, 0
	}

	activity.AddBeastExp(uint64(conf.Param[0]) * uint64(param.Count))

	sys.PushSectBeast()
	return true, true, param.Count
}

func (sys *SectBeastSys) PushSectBeast() {
	sys.SendProto3(31, 152, &pb3.S2C_31_152{
		ExpLv: activity.GetSectBeastExpLv(),
	})
}

func (sys *SectBeastSys) PushSectBeastDamage() {
	sys.SendProto3(31, 155, &pb3.S2C_31_155{
		PerDamage:  activity.GetActorDamage(sys.owner.GetId()),
		SectDamage: activity.GetSectDamage(),
	})
}

func (sys *SectBeastSys) c2sGetDamage(msg *base.Message) error {
	sys.PushSectBeastDamage()
	return nil
}

func (sys *SectBeastSys) c2sGetExpLv(msg *base.Message) error {
	sys.PushSectBeast()
	return nil
}

func (sys *SectBeastSys) c2sGetCrossRank(msg *base.Message) error {
	activity.GetCrossRank(sys.owner.GetId())
	return nil
}

func addExp(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	sectBeastSys, ok := player.GetSysObj(sysdef.SiSectBeast).(*SectBeastSys)
	if !ok || !sectBeastSys.IsOpen() {
		return false, false, 0
	}
	return sectBeastSys.AddExp(param, conf)
}

func init() {
	RegisterSysClass(sysdef.SiSectBeast, func() iface.ISystem {
		return &SectBeastSys{}
	})

	net.RegisterSysProtoV2(31, 155, sysdef.SiSectBeast, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SectBeastSys).c2sGetDamage
	})
	net.RegisterSysProtoV2(31, 152, sysdef.SiSectBeast, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SectBeastSys).c2sGetExpLv
	})
	net.RegisterSysProtoV2(31, 158, sysdef.SiSectBeast, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SectBeastSys).c2sGetCrossRank
	})

	miscitem.RegCommonUseItemHandle(itemdef.UseItemAddSectBeastExp, addExp)

	gmevent.Register("sect.OnNewDay", func(player iface.IPlayer, args ...string) bool {
		sys, ok := player.GetSysObj(sysdef.SiSectBeast).(*SectBeastSys)
		if !ok || !sys.IsOpen() {
			return false
		}
		sys.OnNewDay()
		return true
	}, 1)

	gmevent.Register("sect.AddExp", func(player iface.IPlayer, args ...string) bool {
		sys, ok := player.GetSysObj(sysdef.SiSectBeast).(*SectBeastSys)
		if !ok || !sys.IsOpen() {
			return false
		}
		if len(args) <= 0 {
			return false
		}
		activity.AddBeastExp(utils.AtoUint64(args[0]))
		sys.PushSectBeast()
		return true
	}, 1)
}
