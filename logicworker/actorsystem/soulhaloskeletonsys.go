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
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/random"
)

type SoulHaloSkeletonSys struct {
	Base
}

func (s *SoulHaloSkeletonSys) getData() *pb3.SoulHaloSkeletonData {
	binary := s.GetBinaryData()
	if binary.SoulHaloSkeletonData == nil {
		binary.SoulHaloSkeletonData = &pb3.SoulHaloSkeletonData{}
	}
	if binary.SoulHaloSkeletonData.Skeleton == nil {
		binary.SoulHaloSkeletonData.Skeleton = map[uint32]*pb3.SimpleSoulHaloSkeletonItemSt{}
	}
	if binary.SoulHaloSkeletonData.Slot == nil {
		binary.SoulHaloSkeletonData.Slot = map[uint32]*pb3.SoulHaloSkeletonSlot{}
	}
	return binary.SoulHaloSkeletonData
}

func (s *SoulHaloSkeletonSys) OnLogin() {
	s.s2cInfo()
}

func (s *SoulHaloSkeletonSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *SoulHaloSkeletonSys) OnReconnect() {
	s.s2cInfo()
}

func (s *SoulHaloSkeletonSys) s2cInfo() {
	s.SendProto3(67, 60, &pb3.S2C_67_60{
		Data: s.getData(),
	})
}

func (s *SoulHaloSkeletonSys) GetSlotByItem(itemId uint32) uint32 {
	data := s.getData()
	if data.Skeleton == nil || len(data.Skeleton) == 0 {
		return 0
	}
	for pos, v := range data.Skeleton {
		if v.ItemId == itemId {
			return pos
		}
	}

	return 0
}

func (s *SoulHaloSkeletonSys) GetSoulHaloSkeletonData() *pb3.SoulHaloSkeletonData {
	return s.getData()
}

func (s *SoulHaloSkeletonSys) EquipOnP(itemId uint32) bool {
	return s.GetSlotByItem(itemId) > 0
}

func (s *SoulHaloSkeletonSys) TakeOnWithItemConfAndWithoutRewardToBag(player iface.IPlayer, itemConf *jsondata.ItemConf, logId uint32, itemHandles []uint64, bodySkeletonItem uint64) error {
	if itemConf == nil {
		return neterror.ParamsInvalidError("itemConf is nil")
	}
	data := s.getData().Skeleton
	var slot uint32

	for k, v := range data {
		if v.Handle == bodySkeletonItem {
			slot = k
			break
		}
	}

	if slot == 0 {
		return neterror.ParamsInvalidError("slot is nil")
	}

	data[slot].ItemId = itemConf.Id
	s.getData().Skeleton = data
	s.SendProto3(67, 61, &pb3.S2C_67_61{
		Solt:   slot,
		ItemSt: s.getData().Skeleton[slot],
	})
	s.ResetSysAttr(attrdef.SaSoulHaloSkeleton)

	return nil
}

func (s *SoulHaloSkeletonSys) GetBagSys() (*SoulHaloSkeletonBagSys, error) {
	bagSys, ok := s.owner.GetSysObj(sysdef.SiSoulHaloSkeletonBag).(*SoulHaloSkeletonBagSys)
	if !ok {
		return nil, neterror.SysNotExistError("SoulHaloSkeletonBagSys get err")
	}

	return bagSys, nil
}

func (s *SoulHaloSkeletonSys) getSoulHaloSkeletonBySlot(slot uint32) uint32 {
	data := s.GetSoulHaloSkeletonData()
	item, ok := data.Skeleton[slot]
	if ok {
		return item.ItemId
	}
	return 0
}

func (s *SoulHaloSkeletonSys) checkTakeOnSlotHandle(equip *pb3.ItemSt, slot uint32) error {
	itemConf := jsondata.GetItemConfig(equip.GetItemId())
	if nil == itemConf {
		return neterror.ConfNotFoundError("item itemConf(%d) nil", equip.GetItemId())
	}

	if !itemdef.IsSoulHaloSkeleton(itemConf.Type) {
		return neterror.SysNotExistError("not SoulHaloSkeleton item")
	}

	if !s.owner.CheckItemCond(itemConf) {
		s.owner.SendTipMsg(tipmsgid.TPTakenOn)
		return neterror.ParamsInvalidError("the wearing conditions are not met")
	}

	slotConf, ok := jsondata.GetSoulHaloSkeletonConfBySlot(slot)
	if !ok {
		return neterror.ParamsInvalidError("SoulHaloSkeleton(%d) conf is nil", slot)
	}
	if itemConf.SubType != slotConf.Type {
		return neterror.ParamsInvalidError("SoulHaloSkeleton  take pos is not equal")
	}
	return nil
}

func (s *SoulHaloSkeletonSys) c2sTakeOn(msg *base.Message) error {
	var req pb3.C2S_67_61
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	err = s.takeOn(req.Solt, req.Hdl)
	if err != nil {
		return err
	}

	s.ResetSysAttr(attrdef.SaSoulHaloSkeleton)
	return nil
}

func (s *SoulHaloSkeletonSys) takeOn(slot uint32, hdl uint64) error {
	bagSys, err := s.GetBagSys()
	if nil != err {
		return err
	}

	newEquip := bagSys.FindItemByHandle(hdl)
	if nil == newEquip {
		return neterror.ParamsInvalidError("item handle(%d) not exist", hdl)
	}

	newEquipId := newEquip.ItemId

	err = s.checkTakeOnSlotHandle(newEquip, slot) //检查装备是否符合穿戴条件
	if err != nil {
		return neterror.Wrap(err)
	}

	eqData := s.GetSoulHaloSkeletonData()
	if eqData == nil {
		return neterror.ParamsInvalidError("SoulHaloSkeletonData is nil")
	}

	oldEq := s.getSoulHaloSkeletonBySlot(slot)
	if oldEq != 0 {
		if err := s.takeOff(slot); err != nil {
			return err
		}
	}

	if removeSucc := bagSys.RemoveItemByHandle(hdl, pb3.LogId_LogSoulHaloSkeletonTakeOn); !removeSucc {
		return neterror.InternalError("remove SoulHaloSkeleton hdl:%d item:%d failed", newEquip.GetHandle(), newEquip.GetItemId())
	}
	slotData, ok := eqData.Skeleton[slot]
	if !ok {
		slotData = &pb3.SimpleSoulHaloSkeletonItemSt{}
		eqData.Skeleton[slot] = slotData

	}
	eqData.Skeleton[slot].ItemId = newEquipId
	eqData.Skeleton[slot].Handle = hdl
	s.SendProto3(67, 61, &pb3.S2C_67_61{
		Solt:   slot,
		ItemSt: s.getData().Skeleton[slot],
	})
	return nil
}

func (s *SoulHaloSkeletonSys) c2sTakeOff(msg *base.Message) error {
	var req pb3.C2S_67_62
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}

	err = s.takeOff(req.Solt)
	if err != nil {
		s.LogError("err:%v", err)
		return err
	}
	s.ResetSysAttr(attrdef.SaSoulHaloSkeleton)
	return nil
}

func (s *SoulHaloSkeletonSys) takeOff(pos uint32) error {
	eqData := s.GetSoulHaloSkeletonData()
	_, exist := eqData.Skeleton[pos]
	if !exist {
		return neterror.ParamsInvalidError("SoulHaloSkeleton is nil")
	}

	oldItemId := eqData.Skeleton[pos].ItemId
	if oldItemId == 0 {
		return neterror.ParamsInvalidError("SoulHaloSkeleton is nil")
	}

	bagSys, err := s.GetBagSys()
	if nil != err {
		return neterror.Wrap(err)
	}

	if bagSys.AvailableCount() <= 0 {
		s.owner.SendTipMsg(tipmsgid.TpBagIsFull)
		return neterror.ParamsInvalidError("bag availablecount <=0 ")
	}
	delete(eqData.Skeleton, pos)
	engine.GiveRewards(s.GetOwner(), []*jsondata.StdReward{{Id: oldItemId, Count: 1}}, common.EngineGiveRewardParam{
		LogId: pb3.LogId_LogSoulHaloSkeletonTakeOff,
	})
	s.SendProto3(67, 62, &pb3.S2C_67_62{Solt: pos})
	return nil
}

func (s *SoulHaloSkeletonSys) getSlotData(slot uint32) *pb3.SoulHaloSkeletonSlot {
	data := s.getData()
	slotData, ok := data.Slot[slot]
	if !ok {
		slotData = &pb3.SoulHaloSkeletonSlot{
			Pos: slot,
		}
		data.Slot[slot] = slotData
	}
	return slotData
}

func (s *SoulHaloSkeletonSys) c2sUpLv(msg *base.Message) error {
	var req pb3.C2S_67_63
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}
	slot := req.Solt
	data := s.getData()
	slotItem := s.getSoulHaloSkeletonBySlot(slot)
	if slotItem == 0 {
		return neterror.ParamsInvalidError("slot(%d) is nil", slot)
	}
	slotData := s.getSlotData(slot)
	nextLv := slotData.Lv + 1
	nextLvConf, ok := jsondata.GetSoulHaloSkeletonLvConf(req.Solt, nextLv)
	if !ok {
		return neterror.ConfNotFoundError("SoulHaloSkeletonNextLvConf is nil")
	}
	if nextLvConf.Stage > slotData.Stage {
		return neterror.ParamsInvalidError("stage not enough")
	}
	if !s.owner.ConsumeByConf(nextLvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogSoulHaloSkeletonUpLv}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	slotData.Lv = nextLv
	s.SendProto3(67, 63, &pb3.S2C_67_63{
		Solt:   slot,
		SlotSt: slotData,
	})

	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogSoulHaloSkeletonUpLv, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"itemId": slotItem,
			"handle": data.Skeleton[slot].Handle,
			"level":  slotData.Lv,
		}),
	})
	return nil
}

func (s *SoulHaloSkeletonSys) c2sBreak(msg *base.Message) error {
	var req pb3.C2S_67_64
	err := msg.UnPackPb3Msg(&req)
	if nil != err {
		return err
	}
	slot := req.Solt
	data := s.getData()
	slotItem := s.getSoulHaloSkeletonBySlot(slot)
	if slotItem == 0 {
		return neterror.ParamsInvalidError("slot(%d) is nil", slot)
	}
	conf := jsondata.GetSoulHaloSkeletonConf()
	if conf == nil {
		return neterror.ConfNotFoundError("SoulHaloSkeletonConf is nil")
	}
	slotConf, ok := jsondata.GetSoulHaloSkeletonSlotConf(slot)
	if !ok {
		return neterror.ConfNotFoundError("SoulHaloSkeletonSlotConf is nil")
	}
	//检查同魂环的另一个槽位
	elseSlotConf, ok := jsondata.GetSoulHaloSkeletonSameSoulHalo(slot)
	if !ok {
		return neterror.ConfNotFoundError("else SoulHaloSkeletonSlotConf is nil")
	}
	elseSlot := s.getSlotData(elseSlotConf.Pos).Stage
	slotData := s.getSlotData(slot)

	if slotData.Stage > elseSlot && (slotData.Stage-elseSlot) >= conf.BreakDiffLimit {
		return neterror.ParamsInvalidError("two slots abs > %d ", conf.BreakDiffLimit)
	}

	binary := s.GetBinaryData()
	monAddRate := binary.BeastRampantTimes * conf.MonthAddRate

	nextStage := slotData.Stage + 1
	nextStageConf, ok := jsondata.GetSoulHaloSkeletonStageConf(slot, nextStage)
	if !ok {
		return neterror.ConfNotFoundError("SoulHaloSkeletonNextStageConf is nil")
	}

	failRate := slotData.FailTimes * slotConf.BreakConf[nextStage].FailAddSucRate
	addRate := monAddRate + slotConf.BreakConf[nextStage].InitRate + failRate
	sys, ok := s.owner.GetSysObj(sysdef.SiSoulHalo).(*SoulHaloSys)
	if !ok || !sys.IsOpen() {
		return neterror.ParamsInvalidError("soulHaloSys is no open ")
	}
	soulHalo := sys.getSoulHaloInfoBySlot(slotConf.SoulHaloPos)
	if soulHalo == nil || soulHalo.SoulHalo == nil {
		return neterror.ParamsInvalidError("soulHalo is nil ")
	}
	itemConf := jsondata.GetItemConfig(soulHalo.SoulHalo.ItemId)
	if nil == itemConf {
		return neterror.ParamsInvalidError("soulHalo item is nil ")
	}
	var soulHaloAddRate uint32
	for i := 0; i < len(conf.SoulHaloAddRate); i += 2 {
		if itemConf.Stage != conf.SoulHaloAddRate[i] {
			continue
		}
		soulHaloAddRate = conf.SoulHaloAddRate[i+1]
		break
	}

	addRate += soulHaloAddRate
	if random.Hit(addRate, 10000) {
		slotData.Stage = nextStage
		slotData.FailTimes = 0
	} else {
		slotData.FailTimes++
	}
	if !s.owner.ConsumeByConf(nextStageConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogSoulHaloSkeletonBreak}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	s.SendProto3(67, 63, &pb3.S2C_67_63{
		Solt:   slot,
		SlotSt: slotData,
	})
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogSoulHaloSkeletonBreak, &pb3.LogPlayerCounter{
		StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
			"itemId": slotItem,
			"handle": data.Skeleton[slot].Handle,
			"break":  slotData.Stage,
		}),
	})

	return nil
}

func (s *SoulHaloSkeletonSys) calcSoulHaloSkeletonAttr(calc *attrcalc.FightAttrCalc) {
	data := s.getData()
	conf := jsondata.GetSoulHaloSkeletonConf()
	if conf == nil {
		return
	}

	for _, skeletonItem := range data.Skeleton {
		if skeletonItem.ItemId == 0 {
			continue
		}
		itemConf := jsondata.GetItemConfig(skeletonItem.ItemId)
		if itemConf == nil {
			continue
		}
		engine.CheckAddAttrsToCalc(s.owner, calc, itemConf.StaticAttrs)
		engine.CheckAddAttrsToCalc(s.owner, calc, itemConf.PremiumAttrs)
	}

	for slot, slotConf := range conf.SlotConf {
		slotDat := s.getSlotData(slot)
		lv := slotDat.Lv
		if slotConf.LvConf[lv] == nil {
			continue
		}
		engine.CheckAddAttrsToCalc(s.owner, calc, slotConf.LvConf[lv].Attrs)
	}
}

func (s *SoulHaloSkeletonSys) calcSoulHaloSkeletonAttrAddRate(totalSysCalc, calc *attrcalc.FightAttrCalc) {
	addRate := totalSysCalc.GetValue(attrdef.SoulHaloSkeletonAttrAddRate)
	if addRate == 0 {
		return
	}
	sys, ok := s.owner.GetSysObj(sysdef.SiSoulHalo).(*SoulHaloSys)
	if !ok || !sys.IsOpen() {
		return
	}
	shData := sys.GetData()

	for _, slotInfo := range shData.SoltInfo {
		soulHalo := slotInfo.SoulHalo
		if nil == soulHalo {
			continue
		}

		soulHaloConf := jsondata.GetSoulHaloConfByItemId(soulHalo.GetItemId())
		if soulHaloConf == nil {
			s.LogError("soul halo conf(%d) is nil", soulHalo.GetItemId())
			continue
		}
		//魂环基础属性
		itemConf := jsondata.GetItemConfig(soulHalo.GetItemId())
		if nil == itemConf {
			s.LogError("soul halo item conf(%d) is nil", soulHalo.GetItemId())
			continue
		}
		engine.CheckAddAttrsRateRoundingUp(s.owner, calc, itemConf.StaticAttrs, uint32(addRate))

		//等级属性
		lv := sys.getSoulHaloLv(soulHalo)
		if lv > 0 && lv <= uint32(len(soulHaloConf.LvConf)) {
			lvConf := soulHaloConf.LvConf[lv-1]
			if lvConf != nil {
				engine.CheckAddAttrsRateRoundingUp(s.owner, calc, lvConf.Attrs, uint32(addRate))
			}
		}
	}
}

func calcSoulHaloSkeletonAddRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiSoulHaloSkeleton).(*SoulHaloSkeletonSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcSoulHaloSkeletonAttrAddRate(totalSysCalc, calc)
}

func calcSoulHaloSkeletonAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	sys, ok := player.GetSysObj(sysdef.SiSoulHaloSkeleton).(*SoulHaloSkeletonSys)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.calcSoulHaloSkeletonAttr(calc)
}

func init() {
	RegisterSysClass(sysdef.SiSoulHaloSkeleton, func() iface.ISystem {
		return &SoulHaloSkeletonSys{}
	})
	engine.RegAttrCalcFn(attrdef.SaSoulHaloSkeleton, calcSoulHaloSkeletonAttr)
	engine.RegAttrAddRateCalcFn(attrdef.SaSoulHaloSkeleton, calcSoulHaloSkeletonAddRate)

	net.RegisterSysProtoV2(67, 61, sysdef.SiSoulHaloSkeleton, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SoulHaloSkeletonSys).c2sTakeOn
	})
	net.RegisterSysProtoV2(67, 62, sysdef.SiSoulHaloSkeleton, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SoulHaloSkeletonSys).c2sTakeOff
	})
	net.RegisterSysProtoV2(67, 63, sysdef.SiSoulHaloSkeleton, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SoulHaloSkeletonSys).c2sUpLv
	})
	net.RegisterSysProtoV2(67, 64, sysdef.SiSoulHaloSkeleton, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*SoulHaloSkeletonSys).c2sBreak
	})
}
