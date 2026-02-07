package actorsystem

import (
	"encoding/json"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/actorfuncid"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/engine/miscitem"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/uplevelbase"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils/pie"

	"github.com/gzjjyz/srvlib/utils"
)

/**
 * @Author:  LvYuMeng;TangWenLong
 * @Desc: 战魂
 * @Date: 2024/8/5
 */

type ImmortalSoulSystem struct {
	Base
	*miscitem.EquipContainer
	expUpLv          uplevelbase.ExpUpLv
	battleSoulDetail map[uint32]*battleSoulDetail
}

type battleSoulDetail struct {
	expUpStage *uplevelbase.ExpUpLv
}

func (s *ImmortalSoulSystem) getBattleSoulData() *pb3.BattleSoulData {
	binary := s.GetBinaryData()
	if nil == binary.BattleSoulData {
		binary.BattleSoulData = &pb3.BattleSoulData{}
	}
	if nil == binary.BattleSoulData.BattleSoul {
		binary.BattleSoulData.BattleSoul = make(map[uint32]*pb3.BattleSoul)
	}
	if nil == binary.BattleSoulData.Medicine {
		binary.BattleSoulData.Medicine = make(map[uint32]*pb3.UseCounter)
	}
	if nil == binary.BattleSoulData.Slot {
		binary.BattleSoulData.Slot = make(map[uint32]uint32)
	}
	if nil == binary.BattleSoulData.BattleMeCha {
		binary.BattleSoulData.BattleMeCha = make(map[uint32]uint32)
	}
	if nil == binary.BattleSoulData.ExpLv {
		binary.BattleSoulData.ExpLv = &pb3.ExpLvSt{}
	}

	data := binary.BattleSoulData
	for k := range data.Slot {
		if _, ok := data.BattleMeCha[k]; !ok {
			data.BattleMeCha[k] = 0
		}
	}
	return binary.BattleSoulData
}

func (s *ImmortalSoulSystem) SaveSkillData(furry int64, readyPos uint32) {
	data := s.getBattleSoulData()
	data.Furry = furry
	data.ReadyPos = readyPos
}

func (s *ImmortalSoulSystem) getBattleSoulById(id uint32) (*pb3.BattleSoul, bool) {
	data := s.getBattleSoulData()
	if nil == data.BattleSoul {
		return nil, false
	}
	bs, ok := data.BattleSoul[id]
	if !ok {
		return nil, false
	}
	if nil == bs.ExpStage {
		bs.ExpStage = &pb3.ExpLvSt{}
	}
	return bs, true
}

func (s *ImmortalSoulSystem) newBattleSoul(id uint32) *pb3.BattleSoul {
	battleSoul := &pb3.BattleSoul{
		Id:       id,
		ExpStage: &pb3.ExpLvSt{},
		Star:     1,
	}
	return battleSoul
}

func (s *ImmortalSoulSystem) initBattleSoulExpUp(battleSoulId uint32) bool {
	battleSoul, ok := s.getBattleSoulById(battleSoulId)
	if !ok {
		return false
	}
	if nil == s.battleSoulDetail {
		s.battleSoulDetail = make(map[uint32]*battleSoulDetail)
	}
	s.battleSoulDetail[battleSoulId] = &battleSoulDetail{
		expUpStage: &uplevelbase.ExpUpLv{
			RefId:            battleSoulId,
			ExpLv:            battleSoul.ExpStage,
			AttrSysId:        attrdef.SaImmortalSoul,
			BehavAddExpLogId: pb3.LogId_LogBattleSoulStageUp,
			AfterUpLvCb:      s.AfterUpLevelBattleSoulStage,
			AfterAddExpCb:    s.AfterAddExpBattleSoulStage,
			GetLvConfHandler: func(lv uint32) *jsondata.ExpLvConf {
				return jsondata.GetBattleSoulStageConf(battleSoulId, lv)
			},
		},
	}
	return true
}

func (s *ImmortalSoulSystem) AfterUpLevel(oldLv uint32) {
}

func (s *ImmortalSoulSystem) AfterAddExp() {
	s.SendProto3(11, 71, &pb3.S2C_11_71{ExpLv: s.getBattleSoulData().ExpLv})
}

func (s *ImmortalSoulSystem) AfterUpLevelBattleSoulStage(oldLv uint32) {
}

func (s *ImmortalSoulSystem) AfterAddExpBattleSoulStage() {
}

func (s *ImmortalSoulSystem) OnInit() {
	mainData := s.GetMainData()

	itemPool := mainData.ItemPool
	if nil == itemPool {
		itemPool = &pb3.ItemPool{}
		mainData.ItemPool = itemPool
	}
	if nil == itemPool.ImmortalSouls {
		itemPool.ImmortalSouls = make([]*pb3.ItemSt, 0)
	}
	container := miscitem.NewEquipContainer(&mainData.ItemPool.ImmortalSouls)

	container.TakeOnLogId = pb3.LogId_LogImmortalSoulTakeOnLogId
	container.TakeOffLogId = pb3.LogId_LogImmortalSoulTakeOffLogId
	container.UpgradeLogId = pb3.LogId_LogImmortalSoulUpgrade
	container.AddItem = s.owner.AddItemPtr
	container.DelItem = s.owner.RemoveItemByHandle
	container.GetItem = s.owner.GetItemByHandle
	container.GetBagAvailable = s.owner.GetBagAvailableCount
	container.CheckTakeOnPosHandle = s.CheckTakeOnPosHandle
	container.ResetProp = s.ResetProp

	container.AfterTakeOn = s.AfterTakeOn
	container.AfterTakeOff = s.AfterTakeOff

	s.EquipContainer = container

	s.initBattleSoulExpUpLv()
}

func (s *ImmortalSoulSystem) initBattleSoulExpUpLv() {
	if !s.IsOpen() {
		return
	}
	data := s.getBattleSoulData()
	s.expUpLv = uplevelbase.ExpUpLv{
		ExpLv:            data.ExpLv,
		BehavAddExpLogId: pb3.LogId_LogBattleSoulLvUp,
		AttrSysId:        attrdef.SaImmortalSoul,
		AfterUpLvCb:      s.AfterUpLevel,
		AfterAddExpCb:    s.AfterAddExp,
		GetLvConfHandler: func(lv uint32) *jsondata.ExpLvConf {
			return jsondata.GetBattleSoulLvConf(lv)
		},
	}
	for id := range data.BattleSoul {
		s.initBattleSoulExpUp(id)
	}
}

func (s *ImmortalSoulSystem) OnLogin() {
}

func (s *ImmortalSoulSystem) OnAfterLogin() {
	s.checkAutoUnLock()
	s.s2CInfo()
}

func (s *ImmortalSoulSystem) OnReconnect() {
	s.s2CInfo()
}

func (s *ImmortalSoulSystem) ResetProp() {
	s.ResetSysAttr(attrdef.SaImmortalSoul)
}

func (s *ImmortalSoulSystem) s2CInfo() {
	if mData := s.GetMainData(); nil != mData {
		s.SendProto3(11, 7, &pb3.S2C_11_7{
			ImmortalSoul: mData.ItemPool.ImmortalSouls,
		})
	}
	s.SendProto3(11, 69, &pb3.S2C_11_69{Data: s.getBattleSoulData()})
}

func (s *ImmortalSoulSystem) c2sTakeOn(msg *base.Message) error {
	var req pb3.C2S_11_8
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	if _, oldEquip := s.GetEquipByPos(req.GetPos()); nil != oldEquip {
		if err := s.Replace(req.GetHandle(), req.GetPos()); nil != err {
			return err
		}
		return nil
	}
	if err, _ := s.TakeOn(req.GetHandle(), req.GetPos()); nil != err {
		return err
	}
	s.onTakeQuest()
	return nil
}

func onQuestTakenImmortalSoulStage(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	s, ok := actor.GetSysObj(sysdef.SiImmortalSoul).(*ImmortalSoulSystem)
	if !ok || !s.IsOpen() {
		return 0
	}
	itemPool := s.GetMainData().ItemPool
	if nil == itemPool.ImmortalSouls {
		return 0
	}
	var maxx uint32
	for _, soul := range itemPool.ImmortalSouls {
		conf := jsondata.GetItemConfig(soul.GetItemId())
		if nil == conf {
			continue
		}
		maxx = utils.MaxUInt32(maxx, conf.Stage)
	}
	return maxx
}

func onQuestImmortalSoulTakeNum(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	s, ok := actor.GetSysObj(sysdef.SiImmortalSoul).(*ImmortalSoulSystem)
	if !ok || !s.IsOpen() {
		return 0
	}
	itemPool := s.GetMainData().ItemPool
	if nil == itemPool.ImmortalSouls {
		return 0
	}
	return uint32(len(itemPool.ImmortalSouls))
}

// onQuestTakenImmortalSoulStageNum 统计穿戴任意X件Y阶战魂符文
func onQuestTakenImmortalSoulStageNum(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
	if len(ids) <= 0 {
		return 0
	}

	s, ok := actor.GetSysObj(sysdef.SiImmortalSoul).(*ImmortalSoulSystem)
	if !ok || !s.IsOpen() {
		return 0
	}
	itemPool := s.GetMainData().ItemPool

	var num uint32
	for _, v := range itemPool.ImmortalSouls {
		conf := jsondata.GetItemConfig(v.GetItemId())
		if nil == conf {
			continue
		}
		if conf.Stage >= ids[0] {
			num++
		}
	}

	return num
}

func (s *ImmortalSoulSystem) AfterTakeOn(equip *pb3.ItemSt) {
	s.SendProto3(11, 8, &pb3.S2C_11_8{ImmortalSoul: equip})
}

func (s *ImmortalSoulSystem) AfterTakeOff(equip *pb3.ItemSt, pos uint32) {
	s.SendProto3(11, 9, &pb3.S2C_11_9{Pos: pos})
}

func (s *ImmortalSoulSystem) onTakeQuest() {
	s.owner.TriggerQuestEventRange(custom_id.QttTakenImmortalSoulStage)
	s.owner.TriggerQuestEventRange(custom_id.QttImmortalSoulTakeNum)
	s.owner.TriggerQuestEventRange(custom_id.QttTakenImmortalSoulStageNum)
}

func (s *ImmortalSoulSystem) c2sTakeOff(msg *base.Message) error {
	var req pb3.C2S_11_9
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	if s.owner.GetBagAvailableCount() <= 0 {
		s.owner.SendTipMsg(tipmsgid.TpBagIsFull)
		return nil
	}
	if err := s.TakeOff(req.GetPos()); nil != err {
		return err
	}
	s.onTakeQuest()
	return nil
}

func (s *ImmortalSoulSystem) c2sCompose(msg *base.Message) error {
	var req pb3.C2S_11_10
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	pos := req.GetPos()
	_, immortalSoul := s.GetEquipByPos(pos)
	if nil == immortalSoul {
		return neterror.ParamsInvalidError("No immortalSoul in pos: %d", pos)
	}
	takeOnItemId := immortalSoul.GetItemId()
	if takeOnItemId > 0 {
		composeConf := jsondata.GetImmortalSoulComposeConf(pos, takeOnItemId)
		if nil == composeConf {
			return neterror.ConfNotFoundError("composeConf nil: (%d,%d)", pos, takeOnItemId)
		}
		if composeConf.Circle > s.owner.GetCircle() { //转生限制
			return neterror.InternalError("circle not reached")
		}
		if composeConf.Level > s.owner.GetLevel() { //转生限制
			s.owner.SendTipMsg(tipmsgid.TpLevelNotReach)
			return nil
		}
		itemConf := jsondata.GetItemConfig(composeConf.NItemId)
		if nil == itemConf {
			return neterror.ConfNotFoundError("item conf(%d) is nil", composeConf.NItemId)
		}
		if st := s.UpGradeEquip(s.owner, pos,
			composeConf.NeedTakeOnItem, composeConf.NItemId, composeConf.Consume, false); st != nil {
			s.SendProto3(11, 10, &pb3.S2C_11_10{ImmortalSoul: st})
			logArg, _ := json.Marshal(map[string]interface{}{
				"oldItem": takeOnItemId,
				"newItem": st.GetItemId(),
			})
			logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogImmortalSoulUpgrade, &pb3.LogPlayerCounter{
				NumArgs: st.Handle,
				StrArgs: string(logArg),
			})
			s.onTakeQuest()
		} else {
			return neterror.InternalError("ImmortalSoul compose failed")
		}
	}
	return nil
}

// 激活战魂
func (s *ImmortalSoulSystem) c2sActivity(msg *base.Message) error {
	var req pb3.C2S_11_13
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	return s.ActivitySpirits(req.Id, false)
}

func (s *ImmortalSoulSystem) ActivitySpirits(id uint32, isItemAct bool) error {
	conf := jsondata.GetBattleSpiritsConf(id)
	if nil == conf {
		return neterror.ConfNotFoundError("BattleSpirits conf not find")
	}
	if isItemAct && s.isBattleSpiritAct(id) { // 已经激活 转化为碎片
		award := make(jsondata.StdRewardVec, 0)
		award = append(award, &jsondata.StdReward{Id: conf.SpiritsDebris, Count: int64(conf.Num)})
		engine.GiveRewards(s.owner, award, common.EngineGiveRewardParam{LogId: pb3.LogId_LogBattleSpiritsAct})
		return nil
	}

	if s.isBattleSpiritAct(id) { // 重复发激活
		return neterror.ParamsInvalidError("BattleSpirits Has Been activity!")
	}

	if !isItemAct { // 不是道具激活  -- 消耗
		var cost []*jsondata.Consume
		cost = append(cost, &jsondata.Consume{Id: conf.SpiritsDebris, Count: conf.Cost})
		if !s.owner.ConsumeByConf(cost, false, common.ConsumeParams{LogId: pb3.LogId_LogBattleSpiritsAct}) {
			s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
			return nil
		}
	}

	data := s.getBattleSoulData()

	newBattleSoul := s.newBattleSoul(id)

	data.BattleSoul[id] = newBattleSoul

	var num int
	for _, v := range data.BattleSoul {
		if v.Star >= 1 {
			num++
		}
	}

	s.initBattleSoulExpUp(id)

	s.ResetSysAttr(attrdef.SaImmortalSoul)

	st := &pb3.CommonSt{
		U32Param: id,
	}
	if num == 1 {
		st.BParam = true
		st.U32Param2 = 1
	}

	err := s.owner.CallActorFunc(actorfuncid.G2FBattleSoulSlotActive, st)
	if nil != err {
		s.LogError("G2FBattleSoulSlotUnLock err:%v", err)
	}

	err = s.owner.CallActorFunc(actorfuncid.G2FBattleSoulToStar, &pb3.CommonSt{U32Param: id, U32Param2: 1})
	if nil != err {
		s.LogError("G2FBattleSoulToStar err:%v", err)
	}

	err = s.owner.CallActorFunc(actorfuncid.G2FBattleSoulAddFurry, &pb3.CommonSt{U32Param: jsondata.GetBattleSoulConf().MaxFury})
	if nil != err {
		s.LogError("G2FBattleSoulAddFurry err:%v", err)
	}

	s.battleSpiritsUpdate(id, battleSoulUpWayActive)

	s.owner.TriggerQuestEvent(custom_id.QttImmortalSoulAnyUpTo, 0, 1)
	s.owner.TriggerQuestEvent(custom_id.QttImmortalSoulAppointUpTo, id, 1)
	s.owner.TriggerEvent(custom_id.AeActiveFashion, &custom_id.FashionSetEvent{
		SetId:     conf.SetId,
		FType:     conf.FType,
		FashionId: id,
	})
	return nil
}

const (
	battleSoulUpWayActive  = 1
	battleSoulUpWayUpStar  = 2
	battleSoulUpWayUpStage = 3
	battleSoulUpWayEquip   = 4
)

func (s *ImmortalSoulSystem) c2sStarUp(msg *base.Message) error {
	var req pb3.C2S_11_70
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	conf := jsondata.GetBattleSpiritsConf(req.Id)
	if nil == conf {
		return neterror.ConfNotFoundError("BattleSpirits conf not find")
	}

	id := req.GetId()
	battleSoul, ok := s.getBattleSoulById(id)
	if !ok {
		return neterror.ParamsInvalidError("BattleSpirits need activity first!")
	}
	if battleSoul.Star <= 0 {
		return neterror.InternalError("BattleSpirits not active but init")
	}
	ntStarConf := jsondata.GetBattleStarConf(id, battleSoul.Star+1)
	if nil == ntStarConf {
		return neterror.ConfNotFoundError("star conf is nil")
	}
	curStarConf := jsondata.GetBattleStarConf(id, battleSoul.Star)
	if nil == curStarConf {
		return neterror.ConfNotFoundError("star conf is nil")
	}

	if !s.owner.ConsumeByConf(curStarConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogBattleSpiritsUpLv}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}
	battleSoul.Star++
	s.battleSpiritsUpdate(id, battleSoulUpWayUpStar)

	if s.IsBattleSoulOnBattle(id) {
		s.owner.LearnSkill(ntStarConf.SkillId, ntStarConf.Lv, true)
	}

	s.ResetSysAttr(attrdef.SaImmortalSoul)
	err = s.owner.CallActorFunc(actorfuncid.G2FBattleSoulToStar, &pb3.CommonSt{U32Param: battleSoul.Id, U32Param2: battleSoul.Star})
	if nil != err {
		s.LogError("G2FBattleSoulToStar err:%v", err)
	}
	s.owner.SendTipMsg(tipmsgid.BattleSpiritsStarUp, conf.Name)
	s.owner.TriggerQuestEvent(custom_id.QttImmortalSoulAnyUpTo, 0, int64(battleSoul.Star))
	s.owner.TriggerQuestEvent(custom_id.QttImmortalSoulAppointUpTo, id, int64(battleSoul.Star))
	return nil
}

func (s *ImmortalSoulSystem) IsBattleSoulOnBattle(id uint32) bool {
	if _, ok := s.getBattleSoulById(id); !ok {
		return false
	}
	data := s.getBattleSoulData()
	for _, v := range data.Slot {
		if v == id {
			return true
		}
	}
	return false
}

func (s *ImmortalSoulSystem) IsBattleMeChaOnBattle(id uint32) bool {
	data := s.getBattleSoulData()
	for _, v := range data.BattleMeCha {
		if v == id {
			return true
		}
	}
	return false
}

func (s *ImmortalSoulSystem) isBattleSpiritAct(id uint32) bool {
	_, ok := s.getBattleSoulById(id)
	return ok
}

// 战魂信息更新
func (s *ImmortalSoulSystem) battleSpiritsUpdate(id, from uint32) {
	if battleSoul, ok := s.getBattleSoulById(id); ok {
		rsp := &pb3.S2C_11_14{
			BattleSoul: battleSoul,
			UpWay:      from,
		}
		s.owner.SendProto3(11, 14, rsp)
	}
}

func (s *ImmortalSoulSystem) CheckTakeOnPosHandle(st *pb3.ItemSt, pos uint32) bool {
	itemId := st.GetItemId()
	itemConf := jsondata.GetItemConfig(itemId)
	if nil == itemConf {
		return false
	}
	if !itemdef.IsImmortalSoul(itemConf.Type, itemConf.SubType) {
		return false
	}
	if pos != itemConf.SubType {
		return false
	}

	immortalSoulConf := jsondata.GetImmortalSoulConf(pos)
	if immortalSoulConf == nil {
		return false
	}

	//槽位开启状态
	if s.owner.GetLevel() < immortalSoulConf.DeblockLv || s.owner.GetCircle() < immortalSoulConf.DeblockOrder {
		return false
	}

	return true
}

func (s *ImmortalSoulSystem) OnOpen() {
	s.initBattleSoulExpUpLv()
	s.getBattleSoulData().ExpLv.Lv = 1
	s.checkAutoUnLock()
	s.ResetSysAttr(attrdef.SaImmortalSoul)
	s.s2CInfo()
}

func (s *ImmortalSoulSystem) checkAutoUnLock() {
	conf := jsondata.GetBattleSoulConf()
	if nil == conf {
		return
	}
	for id := range conf.Slot {
		s.isSlotUnLock(id)
	}
}

func (s *ImmortalSoulSystem) isSlotUnLock(slot uint32) bool {
	data := s.getBattleSoulData()
	if _, ok := data.Slot[slot]; ok {
		return true
	}

	conf := jsondata.GetBattleSoulSlotConf(slot)
	if nil == conf {
		return false
	}

	if len(conf.Cond) == 0 {
		if nil != conf.Consume { //道具解锁
			return false
		} else { //默认开
			s.openSlot(slot)
			return true
		}
	}

	for _, v := range conf.Cond {
		if CheckReach(s.owner, v.Type, v.Val) {
			s.openSlot(slot)
			return true
		}
	}

	return false
}

func (s *ImmortalSoulSystem) openSlot(slot uint32) {
	data := s.getBattleSoulData()
	if _, ok := data.Slot[slot]; !ok {
		data.Slot[slot] = 0
		data.BattleMeCha[slot] = 0
	}
	err := s.owner.CallActorFunc(actorfuncid.G2FBattleSoulSlotUnLock, &pb3.CommonSt{
		U32Param: slot,
	})
	if nil != err {
		s.LogError("G2FBattleSoulSlotUnLock err:%v", err)
	}
}

func (s *ImmortalSoulSystem) c2sUnLock(msg *base.Message) error {
	var req pb3.C2S_11_74
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	slot := req.GetId()

	conf := jsondata.GetBattleSoulSlotConf(slot)
	if nil == conf {
		return neterror.ConfNotFoundError("battleSoul slot conf %d is nil", slot)
	}

	if s.isSlotUnLock(slot) {
		return neterror.ParamsInvalidError("battleSoul slot(%d) is unlock", slot)
	}

	if len(conf.Consume) == 0 {
		return neterror.ParamsInvalidError("battleSoul slot(%d) not allow use item unlock", slot)
	}

	if !s.owner.ConsumeByConf(conf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogUnLockSoulHaloSlot}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	s.openSlot(slot)

	s.SendProto3(11, 74, &pb3.S2C_11_74{Id: slot})

	return nil
}

func (s *ImmortalSoulSystem) c2sAddExp(msg *base.Message) error {
	var req pb3.C2S_11_71
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	if nil == req.Item {
		return neterror.ParamsInvalidError("item is nil")
	}

	lvUpItem := jsondata.GetBattleSoulConf().LevelUpItem

	var exp uint64
	var consume jsondata.ConsumeVec

	for id, count := range req.Item {
		if !pie.Uint32s(lvUpItem).Contains(id) {
			return neterror.ParamsInvalidError("item %d not point", id)
		}
		itemConf := jsondata.GetItemConfig(id)
		if nil == itemConf {
			return neterror.ConfNotFoundError("item %d conf is nil", id)
		}
		exp += uint64(itemConf.CommonField * count)
		consume = append(consume, &jsondata.Consume{
			Id:    id,
			Count: count,
		})
	}
	if !s.owner.ConsumeByConf(consume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogBattleSoulLvUp,
	}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	err = s.expUpLv.AddExp(s.GetOwner(), exp)
	if err != nil {
		return err
	}

	return nil
}

func (s *ImmortalSoulSystem) c2sStageUp(msg *base.Message) error {
	var req pb3.C2S_11_72
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	if nil == req.Item {
		return neterror.ParamsInvalidError("item is nil")
	}

	conf := jsondata.GetBattleSpiritsConf(req.GetId())
	if nil == conf {
		return neterror.ConfNotFoundError("BattleSpirits conf not find")
	}

	if !s.isBattleSpiritAct(req.GetId()) {
		return neterror.ParamsInvalidError("not active")
	}

	if nil == s.battleSoulDetail[req.GetId()] || nil == s.battleSoulDetail[req.GetId()].expUpStage {
		return neterror.InternalError("not init stage exp")
	}
	stageUpItem := conf.StageUpItem

	var exp uint64
	var consume jsondata.ConsumeVec

	for id, count := range req.Item {
		if !pie.Uint32s(stageUpItem).Contains(id) {
			return neterror.ParamsInvalidError("item %d not point", id)
		}
		itemConf := jsondata.GetItemConfig(id)
		if nil == itemConf {
			return neterror.ConfNotFoundError("item %d conf is nil", id)
		}
		exp += uint64(itemConf.CommonField * count)
		consume = append(consume, &jsondata.Consume{
			Id:    id,
			Count: count,
		})
	}
	if !s.owner.ConsumeByConf(consume, false, common.ConsumeParams{
		LogId: pb3.LogId_LogBattleSoulStageUp,
	}) {
		s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
		return nil
	}

	err = s.battleSoulDetail[req.GetId()].expUpStage.AddExp(s.GetOwner(), exp)
	if err != nil {
		return err
	}

	s.battleSpiritsUpdate(req.GetId(), battleSoulUpWayUpStage)

	return nil
}

func (s *ImmortalSoulSystem) c2sDress(msg *base.Message) error {
	var req pb3.C2S_11_75
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}

	battleSoul, ok := s.getBattleSoulById(req.Id)
	if !ok {
		return neterror.ParamsInvalidError("battle soul not activated: %d", req.Id)
	}

	if req.BattleSoulEId > 0 {
		eConf := jsondata.GetImmortalSoulConf(req.BattleSoulEId)
		if eConf == nil {
			return neterror.ConfNotFoundError("battle soul equip not found: %d", req.BattleSoulEId)
		}

		eSys, ok := s.owner.GetSysObj(sysdef.SiImmortalSoulEquip).(*ImmortalSoulEquipSys)
		if !ok {
			return neterror.ParamsInvalidError("battle soul equip system not open")
		}

		eData := eSys.GetData()
		_, ok = eData[req.BattleSoulEId]
		if !ok {
			return neterror.ParamsInvalidError("battle soul equip not activated: %d", req.BattleSoulEId)
		}
	}

	battleSoul.BattleSoulEId = req.BattleSoulEId

	s.battleSpiritsUpdate(req.Id, battleSoulUpWayEquip)
	err = s.owner.CallActorFunc(actorfuncid.G2FBattleSoulOnEquip, &pb3.CommonSt{U32Param: battleSoul.Id, U32Param2: battleSoul.BattleSoulEId})
	if nil != err {
		s.LogError("G2FBattleSoulToStar err:%v", err)
	}
	return nil
}

func (s *ImmortalSoulSystem) useMedicine(param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	medicineConf := jsondata.GetBattleSoulConf().Medicine[conf.ItemId]
	if medicineConf == nil {
		return false, false, 0
	}

	data := s.getBattleSoulData()
	medicine, ok := data.Medicine[conf.ItemId]
	if !ok {
		medicine = &pb3.UseCounter{
			Id: conf.ItemId,
		}
		data.Medicine[conf.ItemId] = medicine
	}

	var limitConf *jsondata.MedicineUseLimit

	for _, mul := range medicineConf.UseLimit {
		if s.owner.GetLevel() <= mul.LevelLimit {
			limitConf = mul
			break
		}
	}

	if limitConf == nil && len(medicineConf.UseLimit) > 0 {
		limitConf = medicineConf.UseLimit[len(medicineConf.UseLimit)-1]
	}

	if limitConf == nil {
		return false, false, 0
	}

	if medicine.Count+uint32(param.Count) > uint32(limitConf.Limit) {
		s.owner.LogError("useMedicine failed, medicine.Count >= limitConf.Limit, medicine.Count: %d, limitConf.Limit: %d", medicine.Count, limitConf.Limit)
		return false, false, 0
	}

	medicine.Count += uint32(param.Count)

	s.ResetSysAttr(attrdef.SaImmortalSoul)
	s.SendProto3(11, 73, &pb3.S2C_11_73{
		Medicines: data.Medicine,
	})
	return true, true, int64(param.Count)
}

func (s *ImmortalSoulSystem) syncSlotInfo(st *pb3.SyncBattleSoulSlotInfoChange) {
	data := s.getBattleSoulData()
	for id := range data.Slot {
		data.Slot[id] = st.Slot[id]
	}
	s.SendProto3(11, 15, &pb3.S2C_11_15{Slot: data.Slot})
	if st.NewTakeOffId > 0 {
		s.forgetBattleSoulSkill(st.NewTakeOffId)
	}
	if st.NewTakeId > 0 {
		s.learnBattleSoulSkill(st.NewTakeId)
	}
}

func (s *ImmortalSoulSystem) learnBattleSoulSkill(id uint32) {
	if !s.IsBattleSoulOnBattle(id) {
		s.LogError("not in battle")
		return
	}
	battleSoul, ok := s.getBattleSoulById(id)
	if !ok {
		return
	}
	starConf := jsondata.GetBattleStarConf(id, battleSoul.GetStar())
	if nil == starConf {
		return
	}
	s.owner.LearnSkill(starConf.SkillId, starConf.Lv, true)
}

func (s *ImmortalSoulSystem) forgetBattleSoulSkill(id uint32) {
	conf := jsondata.GetBattleSpiritsConf(id)
	if nil == conf {
		return
	}
	s.owner.ForgetSkill(conf.SkillId, true, true, true)
}

func (s *ImmortalSoulSystem) PackFightSrvBattleSoul(createData *pb3.CreateActorData) {
	if nil == createData {
		return
	}
	createData.BattleSoulInfo = &pb3.BattleSoulInfo{}
	if !s.IsOpen() {
		return
	}
	fData := createData.BattleSoulInfo
	data := s.getBattleSoulData()
	fData.Slot = data.Slot
	fData.BattleSoulFurry = data.Furry
	fData.BattleSoulReadyPos = data.ReadyPos
	fData.BattleMeCha = data.BattleMeCha

	fData.BattleLv = make(map[uint32]uint32)
	for id, val := range data.BattleSoul {
		fData.ActiveIds = append(fData.ActiveIds, id)
		fData.BattleLv[id] = val.Star
	}

	fData.MeCharStar = make(map[uint32]uint32)
	if meChaSys, ok := s.owner.GetSysObj(sysdef.SiMeCha).(*MeChaSys); ok && meChaSys.IsOpen() {
		data := meChaSys.GetData()
		for _, meChaId := range fData.BattleMeCha {
			mData, ok := data[meChaId]
			if ok {
				fData.MeCharStar[meChaId] = mData.Star
			}
		}
	}
}

func calcImmortalSoulSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	calcRuneAttr(player, calc)
	calcBattleSoulAttr(player, calc)
}

func calcImmortalSoulSysAttrAddRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	calcBattleSoulAttrAddRate(player, totalSysCalc, calc)
}

func calcBattleSoulAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiImmortalSoul).(*ImmortalSoulSystem)
	if !ok || !s.IsOpen() {
		return
	}

	data := s.getBattleSoulData()
	//神通
	for id, medicine := range data.Medicine {
		medicineConf := jsondata.GetBattleSoulConf().Medicine[id]
		if medicineConf == nil {
			continue
		}

		// 基本属性百分比加成
		engine.CheckAddAttrsTimes(s.owner, calc, medicineConf.RateAttrs, medicine.Count)

		// 计算丹药的固定数值加成
		engine.CheckAddAttrsTimes(s.owner, calc, medicineConf.Attrs, medicine.Count)
	}

	//战魂升星/升阶
	for id, v := range data.BattleSoul {
		if v.Star <= 0 {
			continue
		}
		if starConf := jsondata.GetBattleStarConf(id, v.Star); nil != starConf {
			engine.CheckAddAttrsToCalc(s.owner, calc, starConf.Attrs)
		}
		if st := v.GetExpStage(); nil != st {
			if stageConf := jsondata.GetBattleSoulStageConf(id, st.GetLv()); nil != stageConf {
				engine.CheckAddAttrsToCalc(s.owner, calc, stageConf.Attrs)
			}
		}

		// 等级特性属性
		battleSpiritsConf := jsondata.GetBattleSpiritsConf(id)
		if battleSpiritsConf == nil || len(battleSpiritsConf.LvFeatureConf) == 0 {
			continue
		}
		for _, featureConf := range battleSpiritsConf.LvFeatureConf {
			if featureConf.Level > v.Star {
				continue
			}
			engine.CheckAddAttrsToCalc(player, calc, featureConf.Attrs)
		}
	}

	if st := data.GetExpLv(); nil != st {
		if lvConf := jsondata.GetBattleSoulLvConf(st.GetLv()); nil != lvConf {
			engine.CheckAddAttrsToCalc(s.owner, calc, lvConf.Attrs)
		}
	}
}
func calcBattleSoulAttrAddRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	medicineAddRate := uint32(totalSysCalc.GetValue(attrdef.BattleSoulBaseAttrRate))
	if medicineAddRate == 0 {
		return
	}
	s, ok := player.GetSysObj(sysdef.SiImmortalSoul).(*ImmortalSoulSystem)
	if !ok || !s.IsOpen() {
		return
	}
	data := s.getBattleSoulData()
	if st := data.GetExpLv(); nil != st {
		if lvConf := jsondata.GetBattleSoulLvConf(st.GetLv()); nil != lvConf {
			//这个要放整个计算器最后
			engine.CheckAddAttrsRateRoundingUp(s.owner, calc, lvConf.Attrs, medicineAddRate)
		}
	}
}

func calcRuneAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	s, ok := player.GetSysObj(sysdef.SiImmortalSoul).(*ImmortalSoulSystem)
	if !ok || !s.IsOpen() {
		return
	}
	mData := player.GetMainData()
	stageTotalMap := make(map[uint32]uint8)
	for _, runes := range mData.ItemPool.ImmortalSouls {
		conf := jsondata.GetItemConfig(runes.GetItemId())
		if nil == conf {
			continue
		}
		//基础属性
		engine.CheckAddAttrsToCalc(player, calc, conf.StaticAttrs)
		stageTotalMap[conf.Stage] = 0
	}
	for _, runes := range mData.ItemPool.ImmortalSouls {
		conf := jsondata.GetItemConfig(runes.GetItemId())
		if nil == conf {
			continue
		}
		for stage := range stageTotalMap {
			if stage <= conf.Stage {
				stageTotalMap[stage]++
			}
		}
	}
	suitConfList := jsondata.GetImmortalSoulSuitConf()
	if nil == suitConfList {
		return
	}
	for _, suitConf := range suitConfList {
		if nil == suitConf.ImmortalSoulLevelConf {
			continue
		}
		var mxStage, minn uint32
		for stage, num := range stageTotalMap {
			if num >= suitConf.SuitNum && stage > mxStage {
				mxStage = stage
			}
		}
		if mxStage > 0 {
			for _, lvConf := range suitConf.ImmortalSoulLevelConf {
				if lvConf.Level <= mxStage && (minn == 0 || minn < lvConf.Level) {
					minn = lvConf.Level
				}
			}
			//套装属性
			if v, ok := suitConf.ImmortalSoulLevelConf[minn]; ok {
				engine.CheckAddAttrsToCalc(player, calc, v.Attrs)
			}
		}
	}
}

// 使用道具激活
func useItemActBattleSpirits(player iface.IPlayer, _ *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	s := player.GetSysObj(sysdef.SiImmortalSoul).(*ImmortalSoulSystem)
	player.LogInfo("player %d use item active ")
	err := s.ActivitySpirits(conf.ItemId, true)
	if err != nil {
		return false, false, 0
	}
	return true, true, 1
}

func qttImmortalSoulUpTo(player iface.IPlayer, ids []uint32, _ ...interface{}) uint32 {
	if len(ids) < 1 {
		return 0
	}
	s, ok := player.GetSysObj(sysdef.SiImmortalSoul).(*ImmortalSoulSystem)
	if !ok {
		return 0
	}

	appointId := ids[0]
	if appointId > 0 {
		if battleSoul, ok := s.getBattleSoulById(appointId); ok {
			return battleSoul.Star
		}
		return 0
	}

	data := s.getBattleSoulData()

	var maxLv uint32
	for _, v := range data.BattleSoul {
		maxLv = utils.MaxUInt32(v.Star, maxLv)
	}

	return 0
}

func useItemBattleSoulMedicine(player iface.IPlayer, param *miscitem.UseItemParamSt, conf *jsondata.BasicUseItemConf) (success, del bool, cnt int64) {
	sys, ok := player.GetSysObj(sysdef.SiImmortalSoul).(*ImmortalSoulSystem)
	if !ok || !sys.IsOpen() {
		return false, false, 0
	}
	return sys.useMedicine(param, conf)
}

func onBattleSoulSlotInfoChange(player iface.IPlayer, buf []byte) {
	st := &pb3.SyncBattleSoulSlotInfoChange{}
	if err := pb3.Unmarshal(buf, st); err != nil {
		player.LogError("unmarshal err: %v", err)
		return
	}
	sys, ok := player.GetSysObj(sysdef.SiImmortalSoul).(*ImmortalSoulSystem)
	if !ok || !sys.IsOpen() {
		return
	}
	sys.syncSlotInfo(st)
}

func onTryBattleSoulSlotUnLock(player iface.IPlayer, buf []byte) {
	st := &pb3.CommonSt{}
	if err := pb3.Unmarshal(buf, st); err != nil {
		player.LogError("unmarshal err: %v", err)
		return
	}
	sys, ok := player.GetSysObj(sysdef.SiImmortalSoul).(*ImmortalSoulSystem)
	if !ok || !sys.IsOpen() {
		return
	}
	pos := st.GetU32Param()
	isUnLock := sys.isSlotUnLock(pos)
	if !isUnLock {
		sys.LogError("to battle but not unlock pos %d", pos)
		return
	}
	err := player.CallActorFunc(actorfuncid.G2FBattleSoulToBattle, st)
	if nil != err {
		sys.LogError("G2FBattleSoulToBattle err:%v", err)
	}
}

func handleImmortalSoulAfterUseItemQuickUpLvAndQuest(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiImmortalSoul).(*ImmortalSoulSystem)
	if !ok || !sys.IsOpen() {
		return
	}
	if !sys.isBattleSpiritAct(40010001) {
		sys.ActivitySpirits(40010001, false)
	}
}

func handleBattleMaChaBattle(player iface.IPlayer, args ...interface{}) {
	sys, ok := player.GetSysObj(sysdef.SiImmortalSoul).(*ImmortalSoulSystem)
	if !ok || !sys.IsOpen() {
		return
	}

	msg, ok := args[0].(*pb3.SyncBattleSoulSlotMeCha)
	if !ok {
		return
	}

	data := sys.getBattleSoulData()
	for id := range data.BattleMeCha {
		data.BattleMeCha[id] = msg.Slot[id]
	}
	sys.SendProto3(11, 150, &pb3.S2C_11_150{SlotMeCha: data.BattleMeCha})
}

func init() {
	RegisterSysClass(sysdef.SiImmortalSoul, func() iface.ISystem {
		return &ImmortalSoulSystem{}
	})

	engine.RegAttrCalcFn(attrdef.SaImmortalSoul, calcImmortalSoulSysAttr)
	engine.RegAttrAddRateCalcFn(attrdef.SaImmortalSoul, calcImmortalSoulSysAttrAddRate)

	engine.RegisterActorCallFunc(playerfuncid.SyncBattleSoulSlotInfoChange, onBattleSoulSlotInfoChange)
	engine.RegisterActorCallFunc(playerfuncid.TryBattleSoulSlotUnLockToBattle, onTryBattleSoulSlotUnLock)

	engine.RegQuestTargetProgress(custom_id.QttTakenImmortalSoulStage, onQuestTakenImmortalSoulStage)
	engine.RegQuestTargetProgress(custom_id.QttImmortalSoulTakeNum, onQuestImmortalSoulTakeNum)
	engine.RegQuestTargetProgress(custom_id.QttTakenImmortalSoulStageNum, onQuestTakenImmortalSoulStageNum)

	engine.RegQuestTargetProgress(custom_id.QttImmortalSoulAnyUpTo, qttImmortalSoulUpTo)
	engine.RegQuestTargetProgress(custom_id.QttImmortalSoulAppointUpTo, qttImmortalSoulUpTo)

	miscitem.RegCommonUseItemHandle(itemdef.UseItemActBattleSpirits, useItemActBattleSpirits)
	miscitem.RegCommonUseItemHandle(itemdef.UseItemBattleSoulMedicine, useItemBattleSoulMedicine)
	//符石
	net.RegisterSysProto(11, 8, sysdef.SiImmortalSoul, (*ImmortalSoulSystem).c2sTakeOn)
	net.RegisterSysProto(11, 9, sysdef.SiImmortalSoul, (*ImmortalSoulSystem).c2sTakeOff)
	net.RegisterSysProto(11, 10, sysdef.SiImmortalSoul, (*ImmortalSoulSystem).c2sCompose)

	//战魂-通用升级
	net.RegisterSysProtoV2(11, 71, sysdef.SiImmortalSoul, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*ImmortalSoulSystem).c2sAddExp
	})

	net.RegisterSysProtoV2(11, 74, sysdef.SiImmortalSoul, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*ImmortalSoulSystem).c2sUnLock
	})
	//战魂个体
	net.RegisterSysProtoV2(11, 13, sysdef.SiImmortalSoul, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*ImmortalSoulSystem).c2sActivity
	})
	net.RegisterSysProtoV2(11, 70, sysdef.SiImmortalSoul, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*ImmortalSoulSystem).c2sStarUp
	})
	net.RegisterSysProtoV2(11, 72, sysdef.SiImmortalSoul, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*ImmortalSoulSystem).c2sStageUp
	})

	net.RegisterSysProtoV2(11, 75, sysdef.SiImmortalSoul, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*ImmortalSoulSystem).c2sDress
	})
	event.RegActorEvent(custom_id.AeAfterUseItemQuickUpLvAndQuest, handleImmortalSoulAfterUseItemQuickUpLvAndQuest)
	event.RegActorEvent(custom_id.AeMeChaBattle, handleBattleMaChaBattle)
}
