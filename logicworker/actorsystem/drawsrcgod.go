package actorsystem

import (
	"github.com/gzjjyz/random"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type DrawSrcGodSys struct {
	Base
}

func (sys *DrawSrcGodSys) OnReconnect() {

}

func (sys *DrawSrcGodSys) drawSrcGodSys() error {
	conf := jsondata.GetDrawSrcGodConf()
	if nil == conf {
		return neterror.ConfNotFoundError("drawsrcgod conf is nil")
	}
	hasSrcGod := make(map[uint32]struct{})
	mainData := sys.GetMainData()
	itemPool := mainData.ItemPool
	for _, v := range itemPool.FairyBag {
		if v.Ext.FairyGrade == custom_id.FairyGradeYuanShen {
			hasSrcGod[v.ItemId] = struct{}{}
		}
	}
	pool := new(random.Pool)
	for _, v := range conf.Rewards {
		if _, ok := hasSrcGod[v.Id]; ok {
			continue
		}
		pool.AddItem(v, v.Weight)
	}
	if pool.Size() == 0 {
		return neterror.ParamsInvalidError("all srcgod is clear")
	}
	if !sys.owner.ConsumeByConf(conf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogDrawSrcGod}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	line := pool.RandomOne()
	if award, ok := line.(*jsondata.StdReward); ok {
		if !engine.GiveRewards(sys.owner, []*jsondata.StdReward{award}, common.EngineGiveRewardParam{LogId: pb3.LogId_LogDrawSrcGod}) {
			return neterror.InternalError("drawSrcGod GiveRewards has err")
		}
		itemPb := jsondata.StdRewardToPb3ShowFairyItem(award)
		sys.SendProto3(27, 121, &pb3.S2C_27_121{Item: itemPb})
	} else {
		return neterror.InternalError("drawSrcGod has err")
	}
	return nil
}

func c2sDrawSrcGod(sys iface.ISystem) func(*base.Message) error {
	return func(msg *base.Message) error {
		err := sys.(*DrawSrcGodSys).drawSrcGodSys()
		if err != nil {
			return err
		}
		return nil
	}
}

func (sys *DrawSrcGodSys) changeSrcGod(msg *base.Message) error {
	var req pb3.C2S_27_122
	if err := msg.UnPackPb3Msg(&req); err != nil {
		return err
	}
	conf := jsondata.GetDrawSrcGodConf()
	if nil == conf {
		return neterror.ConfNotFoundError("drawsrcgod conf is nil")
	}
	id := req.GetId()
	changeConf, ok := conf.Change[id]
	if !ok || nil == changeConf {
		return neterror.ConfNotFoundError("no changeSrcGod conf(%d)", id)
	}
	fairyBagSys, ok := sys.owner.GetSysObj(sysdef.SiFairyBag).(*FairyBagSystem)
	if !ok {
		return neterror.InternalError("fairybag is nil")
	}
	fairy := fairyBagSys.FindItemByHandle(req.GetCosHdl())
	if nil == fairy {
		return neterror.ParamsInvalidError("fairy(%d) not fount", req.GetCosHdl())
	}
	if fairy.Ext.FairyGrade != custom_id.FairyGradeYuanShen || fairy.ItemId == req.GetId() {
		return neterror.ParamsInvalidError("fairy item(%d) not allow change", fairy.ItemId)
	}
	if itemdef.IsFairyPos(fairy.Pos) {
		return neterror.ParamsInvalidError("cant cos fairy hdl(%d) in pos", req.GetCosHdl())
	}
	if fairy.Union1 > 0 || fairy.Ext.FairyBackNum > 0 {
		return neterror.InternalError("fairy which developed need back")
	}
	if !sys.owner.ConsumeByConf(changeConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogChangeSrcGod}) {
		sys.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	if !sys.owner.RemoveFairyByHandle(req.GetCosHdl(), pb3.LogId_LogChangeSrcGod) {
		return neterror.InternalError("remove fairy(%d) failed", fairy.GetHandle())
	}
	newFairy := &jsondata.StdReward{
		Id:    id,
		Count: 1,
		Bind:  true,
	}
	if !engine.GiveRewards(sys.owner, []*jsondata.StdReward{newFairy}, common.EngineGiveRewardParam{LogId: pb3.LogId_LogDrawSrcGod}) {
		return neterror.InternalError("drawSrcGod GiveRewards has err")
	}
	itemPb := jsondata.StdRewardToPb3ShowFairyItem(newFairy)
	sys.SendProto3(27, 122, &pb3.S2C_27_122{Item: itemPb})
	return nil
}

func c2sChangeSrcGod(sys iface.ISystem) func(*base.Message) error {
	return func(msg *base.Message) error {
		err := sys.(*DrawSrcGodSys).changeSrcGod(msg)
		if err != nil {
			return err
		}
		return nil
	}
}

func init() {
	RegisterSysClass(sysdef.SiDrawSrcGod, func() iface.ISystem {
		return &DrawSrcGodSys{}
	})
	net.RegisterSysProtoV2(27, 121, sysdef.SiDrawSrcGod, c2sDrawSrcGod)

	net.RegisterSysProtoV2(27, 122, sysdef.SiDrawSrcGod, c2sChangeSrcGod)

}
