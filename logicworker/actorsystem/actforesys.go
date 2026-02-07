/**
 * @Author: lzp
 * @Date: 2023/11/27
 * @Desc:
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/activitydef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/activity"
	"jjyz/gameserver/net"
)

type ActForeSys struct {
	Base
}

func (sys *ActForeSys) OnReconnect() {
	sys.S2CInfo()
}

func (sys *ActForeSys) OnLogin() {
	sys.S2CInfo()
}

func (sys *ActForeSys) S2CInfo() {
	data := sys.GetData()
	msg := &pb3.S2C_31_7{
		ActIds: data,
	}
	sys.owner.SendProto3(31, 7, msg)
}

func (sys *ActForeSys) GetData() []uint32 {
	if sys.GetBinaryData().ActForeRewards == nil {
		sys.GetBinaryData().ActForeRewards = []uint32{}
	}
	return sys.GetBinaryData().ActForeRewards
}

func (sys *ActForeSys) c2sFetchReward(msg *base.Message) error {
	var req pb3.C2S_31_6
	if err := msg.UnpackagePbmsg(&req); nil != err {
		return neterror.ParamsInvalidError("c2sFetchRewards error:%v", err)
	}

	actId := req.GetActId()
	conf := jsondata.GetActivityConf(actId)
	if conf == nil {
		return neterror.ParamsInvalidError("c2sFetchRewards error:conf is nil")
	}

	rewardConf := jsondata.GetForeActivityConf(actId)
	if rewardConf == nil {
		return neterror.ParamsInvalidError("c2sFetchRewards error:rewardConf is nil")
	}

	if activity.GetActStatus(actId) != activitydef.ActStart {
		return neterror.ParamsInvalidError("c2sFetchRewards error:activity is not start")
	}

	idL := sys.GetData()

	if utils.SliceContainsUint32(idL, req.ActId) {
		return neterror.ParamsInvalidError("c2sFetchRewards error:reward is fetched")
	}

	idL = append(idL, req.ActId)
	sys.GetBinaryData().ActForeRewards = idL

	// 发送奖励
	if len(rewardConf.Awards) > 0 {
		engine.GiveRewards(sys.owner, rewardConf.Awards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogActForeRewards,
		})
	}

	sys.owner.SendProto3(31, 6, &pb3.S2C_31_6{
		ActId: actId,
	})

	return nil
}

func init() {
	RegisterSysClass(sysdef.SiActivityFore, func() iface.ISystem {
		return &ActForeSys{}
	})

	net.RegisterSysProto(31, 6, sysdef.SiActivityFore, (*ActForeSys).c2sFetchReward)
}
