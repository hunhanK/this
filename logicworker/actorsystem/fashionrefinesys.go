package actorsystem

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type FashionRefineSys struct {
	Base
}

func (s *FashionRefineSys) OnLogin() {

}

func (s *FashionRefineSys) OnAfterLogin() {

}

func (s *FashionRefineSys) OnReconnect() {

}

func (s *FashionRefineSys) c2sRefine(msg *base.Message) error {
	var req pb3.C2S_13_10
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}
	bagSys, ok := s.owner.GetSysObj(sysdef.SiBag).(*BagSystem)
	if !ok {
		return neterror.ParamsInvalidError("get bag fail")
	}
	var flag bool
	var itemIds []uint32
	var totalAwards jsondata.StdRewardVec
	var awards jsondata.StdRewardVec
	for _, hdl := range req.Handles {
		item := bagSys.FindItemByHandle(hdl)
		if nil == item {
			continue
		}
		itemIds = append(itemIds, item.ItemId)
		conf := jsondata.GetFashionRefinementConf(item.ItemId)
		if conf == nil {
			s.LogError("item %d can't refine", item.ItemId)
			continue
		}
		flag = s.checkMaxStar(conf.SysId, item.ItemId)
		if !flag {
			s.LogError("sysId %d not full of stars", conf.SysId, item.ItemId)
			continue
		}
		if !s.owner.RemoveItemByHandle(hdl, pb3.LogId_LogFashionRefineConsume) {
			s.LogError("remove %d item fail", hdl)
			continue
		}
		awards = jsondata.StdRewardMulti(conf.Rewards, item.Count)
		totalAwards = append(totalAwards, awards...)
	}
	engine.GiveRewards(s.owner, totalAwards, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogFashionRefine,
	})
	s.SendProto3(13, 10, &pb3.S2C_13_10{})
	return nil
}

func (s *FashionRefineSys) checkMaxStar(sysId uint32, objId uint32) bool {
	targetSys := s.owner.GetSysObj(sysId)
	if targetSys == nil || !targetSys.IsOpen() {
		return false
	}

	checker, ok := targetSys.(iface.IMaxStarChecker)
	if !ok {
		return false
	}

	return checker.IsMaxStar(objId)
}

func init() {
	RegisterSysClass(sysdef.SiFashionRefine, func() iface.ISystem {
		return &FashionRefineSys{}
	})
	net.RegisterSysProtoV2(13, 10, sysdef.SiFashionRefine, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*FashionRefineSys).c2sRefine
	})
}
