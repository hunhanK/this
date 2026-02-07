/**
 * @Author: zjj
 * @Date: 2025/1/16
 * @Desc: 年兽大作战
**/

package yy

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/net"
)

type GroupOfNiEnBeast struct {
	YYBase
}

func (yy *GroupOfNiEnBeast) OnInit() {
	yy.callCreateFb()
}

func (yy *GroupOfNiEnBeast) s2cInfo(player iface.IPlayer) {
	err := player.CallActorSmallCrossFunc(actorfuncid.G2FGetGroupOfNiEnBeastMonReq, &pb3.CommonSt{
		U32Param:  yy.GetId(),
		U32Param2: yy.GetConfIdx(),
	})
	if err != nil {
		player.LogError("err:%v", err)
		return
	}
}

func (yy *GroupOfNiEnBeast) PlayerLogin(player iface.IPlayer) {
	yy.s2cInfo(player)
	yy.callCrossPoint(player)
}

func (yy *GroupOfNiEnBeast) PlayerReconnect(player iface.IPlayer) {
	yy.s2cInfo(player)
	yy.callCrossPoint(player)
}

func (yy *GroupOfNiEnBeast) callCreateFb() {
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FCreateGroupOfNiEnBeastFb, &pb3.CommonSt{
		U32Param:  yy.GetId(),
		U32Param2: yy.GetConfIdx(),
	})
	if err != nil {
		yy.LogError("err:%v", err)
		return
	}
}

func (yy *GroupOfNiEnBeast) enterFb(player iface.IPlayer, msg *base.Message) error {
	err := player.EnterFightSrv(base.SmallCrossServer, fubendef.EnterGroupOfNiEnBeastFb, &pb3.CommonSt{
		U32Param:  yy.GetId(),
		U32Param2: yy.GetConfIdx(),
	})
	if err != nil {
		player.LogError("err:%v", err)
		return err
	}
	return nil
}

func (yy *GroupOfNiEnBeast) OnEnd() {
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FCloseGroupOfNiEnBeastFb, &pb3.CommonSt{
		U32Param:  yy.GetId(),
		U32Param2: yy.GetConfIdx(),
	})
	if err != nil {
		yy.LogError("err:%v", err)
		return
	}
}

func (yy *GroupOfNiEnBeast) callCrossPoint(player iface.IPlayer) {
	err := player.CallActorSmallCrossFunc(actorfuncid.G2FGetGroupOfNiEnBeastPointReq, &pb3.CommonSt{})
	if err != nil {
		player.LogError("err:%v", err)
		return
	}
}

func rangeAllGroupOfNiEnBeast(doLogic func(ying iface.IYunYing)) {
	allYY := yymgr.GetAllYY(yydefine.YYGroupOfNiEnBeast)
	for _, v := range allYY {
		if !v.IsOpen() {
			continue
		}
		utils.ProtectRun(func() {
			doLogic(v)
		})
	}
}

func init() {
	yymgr.RegisterYYType(yydefine.YYGroupOfNiEnBeast, func() iface.IYunYing {
		return &GroupOfNiEnBeast{}
	})
	event.RegSysEvent(custom_id.SeCrossSrvConnSucc, func(args ...interface{}) {
		rangeAllGroupOfNiEnBeast(func(ying iface.IYunYing) {
			ying.(*GroupOfNiEnBeast).callCreateFb()
		})
	})
	net.RegisterGlobalYYSysProto(8, 30, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return sys.(*GroupOfNiEnBeast).enterFb
	})
	net.RegisterGlobalYYSysProto(8, 31, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return func(player iface.IPlayer, msg *base.Message) error {
			sys.(*GroupOfNiEnBeast).s2cInfo(player)
			return nil
		}
	})
	net.RegisterGlobalYYSysProto(8, 34, func(sys iface.IYunYing) func(player iface.IPlayer, msg *base.Message) error {
		return func(player iface.IPlayer, msg *base.Message) error {
			sys.(*GroupOfNiEnBeast).callCrossPoint(player)
			return nil
		}
	})
	gmevent.Register("GroupOfNiEnBeast", func(player iface.IPlayer, args ...string) bool {
		rangeAllGroupOfNiEnBeast(func(ying iface.IYunYing) {
			ying.(*GroupOfNiEnBeast).callCreateFb()
		})
		return true
	}, 1)
	gmevent.Register("GroupOfNiEnBeast.enter", func(player iface.IPlayer, args ...string) bool {
		rangeAllGroupOfNiEnBeast(func(ying iface.IYunYing) {
			ying.(*GroupOfNiEnBeast).enterFb(player, nil)
		})
		return true
	}, 1)
}
