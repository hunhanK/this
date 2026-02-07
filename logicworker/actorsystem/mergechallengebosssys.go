/**
 * @Author: LvYuMeng
 * @Date: 2024/11/7
 * @Desc: 合服挑战boss
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils/pie"
)

type MergeCircleBossSys struct {
	Base
}

func (sys *MergeCircleBossSys) OnAfterLogin() {
	sys.s2cFightSrvInfo()
	sys.s2cInfo()
}

func (sys *MergeCircleBossSys) OnReconnect() {
	sys.s2cFightSrvInfo()
	sys.s2cInfo()
}

func (sys *MergeCircleBossSys) s2cFightSrvInfo() {
	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FMergeChallengeBossInfoReq, &pb3.G2FActorPfInfoReq{
		PfId:    engine.GetPfId(),
		SrvId:   engine.GetServerId(),
		ActorId: sys.GetOwner().GetId(),
	})
	if err != nil {
		sys.LogError("err:%v", err)
		return
	}
}
func (sys *MergeCircleBossSys) getData() *pb3.MergeChallengeBossData {
	binary := sys.GetBinaryData()
	if nil == binary.MergeChallengeBossData {
		binary.MergeChallengeBossData = &pb3.MergeChallengeBossData{}
	}
	return binary.MergeChallengeBossData
}

func (sys *MergeCircleBossSys) s2cInfo() {
	sys.SendProto3(17, 94, &pb3.S2C_17_94{Data: sys.getData()})
}

func (sys *MergeCircleBossSys) changeFollow(bossId uint32, need bool) {
	data := sys.getData()
	if need {
		if !pie.Uint32s(data.TipsIds).Contains(bossId) {
			data.TipsIds = append(data.TipsIds, bossId)
		}
	} else {
		data.TipsIds = pie.Uint32s(data.TipsIds).Filter(func(u uint32) bool {
			return bossId != u
		})
	}
}

func (sys *MergeCircleBossSys) c2sFollow(msg *base.Message) error {
	var req pb3.C2S_17_96
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	sys.changeFollow(req.GetBossId(), req.GetNeed())

	sys.SendProto3(17, 96, &pb3.S2C_17_96{
		BossId: req.GetBossId(),
		Need:   req.GetNeed(),
	})
	return nil
}

func (sys *MergeCircleBossSys) c2sEnter(msg *base.Message) error {
	var req pb3.C2S_17_97
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}
	mergeDay, mergeTimes := gshare.GetMergeSrvDay(), gshare.GetMergeTimes()
	conf := jsondata.GetMergeCircleBossConf()
	if mergeDay < conf.MergeDayStart || conf.MergeDayEnd < mergeDay {
		return neterror.ParamsInvalidError("merge day not enough")
	}

	layer := req.GetLayer()
	bossId := req.GetBossId()

	layerConf := jsondata.GetMergeCircleBossLayerConf(mergeTimes, layer)
	if layerConf == nil {
		return neterror.ConfNotFoundError("layerConf conf not found")
	}

	if sys.owner.GetCircle() < layerConf.CircleLv {
		sys.owner.SendTipMsg(tipmsgid.CircleNotEnough)
		return nil
	}

	if sys.owner.GetLevel() < layerConf.Level {
		sys.owner.SendTipMsg(tipmsgid.TpLevelNotReach)
		return nil
	}

	bossConf, ok := jsondata.GetMergeCircleBossBossConf(mergeTimes, layer, bossId)
	if !ok {
		return neterror.ConfNotFoundError("bossConf conf not found")
	}

	err = sys.owner.EnterFightSrv(base.LocalFightServer, fubendef.EnterMergeChallengeBoss, &pb3.CommonSt{
		U32Param:  layer,
		U32Param2: bossId,
		U32Param3: bossConf.SceneId,
	})
	if err != nil {
		return err
	}
	return nil
}

func init() {
	RegisterSysClass(sysdef.SiMergeChallengeBoss, func() iface.ISystem {
		return &MergeCircleBossSys{}
	})

	net.RegisterSysProtoV2(17, 96, sysdef.SiMergeChallengeBoss, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*MergeCircleBossSys).c2sFollow
	})
	net.RegisterSysProtoV2(17, 97, sysdef.SiMergeChallengeBoss, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*MergeCircleBossSys).c2sEnter
	})
}
