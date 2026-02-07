/**
 * @Author: zjj
 * @Date: 2024/8/1
 * @Desc: 屠龙BOSS
**/

package yy

import (
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/manager"
)

type CelebrationFreePrivilege struct {
	YYBase
}

func (k *CelebrationFreePrivilege) OnOpen() {
	manager.AllOfflineDataBaseDo(func(p *pb3.PlayerDataBase) bool {
		st := &pb3.CommonSt{
			U32Param: k.GetId(),
			BParam:   true,
		}
		engine.SendPlayerMessage(p.Id, gshare.OfflineYYCelebrationFreePrivilege, st)
		return true
	})
}

func (k *CelebrationFreePrivilege) PlayerLogin(player iface.IPlayer) {
	player.SetExtraAttr(attrdef.CelebrationFreePrivilege, attrdef.AttrValueAlias(k.GetId()))
}

func (k *CelebrationFreePrivilege) OnEnd() {
	manager.AllOfflineDataBaseDo(func(p *pb3.PlayerDataBase) bool {
		st := &pb3.CommonSt{
			U32Param: k.GetId(),
			BParam:   false,
		}
		engine.SendPlayerMessage(p.Id, gshare.OfflineYYCelebrationFreePrivilege, st)
		return true
	})
}

func handleOfflineYYCelebrationFreePrivilege(player iface.IPlayer, msg pb3.Message) {
	st, ok := msg.(*pb3.CommonSt)
	if !ok {
		return
	}
	if st.BParam {
		player.SetExtraAttr(attrdef.CelebrationFreePrivilege, attrdef.AttrValueAlias(st.U32Param))
	} else {
		player.SetExtraAttr(attrdef.CelebrationFreePrivilege, attrdef.AttrValueAlias(0))
	}
}

func init() {
	yymgr.RegisterYYType(yydefine.YYCelebrationFreePrivilege, func() iface.IYunYing {
		return &CelebrationFreePrivilege{}
	})
	engine.RegisterMessage(gshare.OfflineYYCelebrationFreePrivilege, func() pb3.Message {
		return &pb3.CommonSt{}
	}, handleOfflineYYCelebrationFreePrivilege)
}
