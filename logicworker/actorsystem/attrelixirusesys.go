/**
 * @Author: zjj
 * @Date: 2023年10月30日
 * @Desc: 属性丹使用系统
**/

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type AttrElixirUseSys struct {
	Base
}

func (s *AttrElixirUseSys) GetData() *pb3.AttrElixirUseState {
	if s.GetBinaryData().AttrElixirUseState == nil {
		s.GetBinaryData().AttrElixirUseState = &pb3.AttrElixirUseState{}
	}
	state := s.GetBinaryData().AttrElixirUseState
	if state.UseMap == nil {
		state.UseMap = make(map[uint32]uint32)
	}
	return state
}

func (s *AttrElixirUseSys) S2CInfo() {
	s.SendProto3(156, 1, &pb3.S2C_156_1{
		State: s.GetData(),
	})
}

func (s *AttrElixirUseSys) OnLogin() {
	s.S2CInfo()
}

func (s *AttrElixirUseSys) OnReconnect() {
	s.S2CInfo()
}

func (s *AttrElixirUseSys) OnInit() {
	s.S2CInfo()
}

func (s *AttrElixirUseSys) c2sOneClickUse(msg *base.Message) error {
	var req pb3.C2S_156_2
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.Wrap(err)
	}

	items := req.Items
	if len(items) == 0 {
		return nil
	}

	jingjieLv := s.owner.GetExtraAttrU32(attrdef.Circle)
	data := s.GetData()

	var consumeVec jsondata.ConsumeVec
	for _, item := range items {
		useConf, ok := jsondata.GetAttrElixirUseConf(item.ItemId)
		if !ok {
			return neterror.ConfNotFoundError("not found item conf , id is %d", item.ItemId)
		}
		useUpLimit := useConf.UseUpLimit

		// 境界
		if useConf.JingJIeLv > 0 && jingjieLv < useConf.JingJIeLv {
			return neterror.ConsumeFailedError("jing jie lv not reach , jing jie lv is %d", jingjieLv)
		}

		count := data.UseMap[item.ItemId]

		// 达到上限
		if useUpLimit > 0 && count > useUpLimit {
			return neterror.ConsumeFailedError("use up limit , up limit is %d ,cur is %d", useUpLimit, count)
		}
		if useUpLimit > 0 && count+uint32(item.Count) > useUpLimit {
			return neterror.ConsumeFailedError("use up limit , up limit is %d ,cur + count is %d", useUpLimit, count+uint32(item.Count))
		}

		consumeVec = append(consumeVec, &jsondata.Consume{
			Id:    item.ItemId,
			Count: uint32(item.Count),
		})
	}

	if len(consumeVec) > 0 {
		if !s.GetOwner().ConsumeByConf(consumeVec, false, common.ConsumeParams{LogId: pb3.LogId_LogConsumeByOneClickUseAlchemy}) {
			s.GetOwner().SendTipMsg(tipmsgid.TpUseItemFailed)
			return neterror.ConsumeFailedError("one click attr elixir to consume failed")
		}
	}

	for _, consume := range consumeVec {
		data.UseMap[consume.Id] = data.UseMap[consume.Id] + consume.Count
	}

	s.ResetSysAttr(attrdef.SaAttrElixirUse)

	s.SendProto3(156, 2, &pb3.S2C_156_2{
		Items: items,
	})

	s.SendProto3(156, 1, &pb3.S2C_156_1{
		State: s.GetData(),
	})
	return nil
}

func useAttrElixir(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	obj := player.GetSysObj(sysdef.SiAttrElixirUse)
	if obj == nil || !obj.IsOpen() {
		return
	}
	s, ok := obj.(*AttrElixirUseSys)
	if !ok {
		return
	}

	useConf, ok := jsondata.GetAttrElixirUseConf(param.ItemId)
	if !ok {
		return
	}
	data := s.GetData()
	count := data.UseMap[param.ItemId]
	if useConf.JingJIeLv > 0 {
		jingjieLv := s.owner.GetExtraAttrU32(attrdef.Circle)

		if jingjieLv < useConf.JingJIeLv {
			s.LogWarn("jing jie lv not reach , jing jie lv is %d", jingjieLv)
			return
		}
	}

	useUpLimit := useConf.UseUpLimit
	// 达到上限
	if useUpLimit > 0 && count > useUpLimit {
		s.LogWarn("use up limit , up limit is %d ,cur is %d", useUpLimit, count)
		return
	}

	if useUpLimit > 0 && count+uint32(param.Count) > useUpLimit {
		s.LogWarn("use up limit , up limit is %d ,cur + count is %d", useUpLimit, count, param.Count)
		return
	}

	if useConf.TipsId > 0 {
		itemConf := jsondata.GetItemConfig(useConf.ItemId)
		if itemConf != nil {
			engine.BroadcastTipMsgById(useConf.TipsId, itemConf.Name)
		}
	}

	data.UseMap[param.ItemId] = count + uint32(param.Count)
	s.SendProto3(156, 1, &pb3.S2C_156_1{
		State: s.GetData(),
	})
	s.ResetSysAttr(attrdef.SaAttrElixirUse)
	return true, true, param.Count
}

func calcSaAttrElixirUseSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiAttrElixirUse).(*AttrElixirUseSys)
	if !ok {
		return
	}

	var attrs jsondata.AttrVec
	for itemId, count := range s.GetData().UseMap {
		conf, ok := jsondata.GetAttrElixirUseConf(itemId)
		if !ok {
			continue
		}
		for i := uint32(0); i < count; i++ {
			attrs = append(attrs, conf.Attrs...)
		}
	}

	// 合并一下
	attrs = jsondata.MergeAttrVec(attrs...)

	// 获取加成
	var rate uint32
	for _, attr := range attrs {
		if attr.Type != attrdef.EatElixirDrugAddRate {
			continue
		}
		rate += attr.Value
	}

	var addRateAttrs jsondata.AttrVec
	for _, attr := range attrs {
		na := &jsondata.Attr{
			Type:           attr.Type,
			Value:          attr.Value,
			Job:            attr.Job,
			EffectiveLimit: attr.EffectiveLimit,
		}
		if attr.Type != attrdef.EatElixirDrugAddRate {
			na.Value = na.Value * (10000 + rate) / 10000
		}
		addRateAttrs = append(addRateAttrs, na)
	}

	// 加属性
	if len(addRateAttrs) > 0 {
		engine.CheckAddAttrsToCalc(player, calc, addRateAttrs)
	}
}

func init() {
	RegisterSysClass(sysdef.SiAttrElixirUse, func() iface.ISystem {
		return &AttrElixirUseSys{}
	})

	net.RegisterSysProtoV2(156, 2, sysdef.SiAttrElixirUse, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*AttrElixirUseSys).c2sOneClickUse
	})

	// 注册道具使用
	miscitem.RegCommonUseItemHandle(itemdef.UseItemAttrElixirUse, useAttrElixir)

	// 重算属性
	engine.RegAttrCalcFn(attrdef.SaAttrElixirUse, calcSaAttrElixirUseSysAttr)

}
