/**
 * @Author: beiming
 * @Desc: 神兵附魂
 * @Date: 2023/11/21
 */
package actorsystem

import (
	"encoding/json"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
	"math/rand"

	"golang.org/x/exp/slices"
)

func init() {
	RegisterSysClass(sysdef.SiWeaponSoul, newWeaponSoulSystem)
	gmevent.Register("WeaponSoul.finishAllQuest", func(player iface.IPlayer, args ...string) bool {
		system := player.GetSysObj(sysdef.SiWeaponSoul).(*WeaponSoulSystem)
		if system == nil {
			return false
		}
		for _, quest := range system.getData().Quests {
			system.GmFinishQuest(quest)
		}
		return true
	}, 1)
	engine.RegAttrCalcFn(attrdef.SaWeaponSoul, calcWeaponSoulSysAttr)

	net.RegisterSysProtoV2(165, 2, sysdef.SiWeaponSoul, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*WeaponSoulSystem).c2sUnlock
	})
	net.RegisterSysProtoV2(165, 3, sysdef.SiWeaponSoul, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*WeaponSoulSystem).c2sLevelUp
	})
	net.RegisterSysProtoV2(165, 4, sysdef.SiWeaponSoul, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*WeaponSoulSystem).c2sActiveStar
	})
	net.RegisterSysProtoV2(165, 5, sysdef.SiWeaponSoul, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*WeaponSoulSystem).c2sActiveQuest
	})
}

type WeaponSoulSystem struct {
	*QuestTargetBase
}

func (s *WeaponSoulSystem) OnOpen() {
	s.getData() // 初始化数据

	// 接受所有任务
	s.acceptAllQuest()

	// 设置已开启的神兵
	s.resetOpenedWeaponSoul()

	s.s2cInfo()
}

func (s *WeaponSoulSystem) OnLogin() {
	s.acceptAllQuest()
	s.resetOpenedWeaponSoul()
	s.s2cInfo()
}

func (s *WeaponSoulSystem) OnReconnect() {
	s.ResetSysAttr(attrdef.SaWeaponSoul)
	s.s2cInfo()
}

func (s *WeaponSoulSystem) getData() *pb3.WeaponSoul {
	if s.GetBinaryData().WeaponSoul == nil {
		s.GetBinaryData().WeaponSoul = new(pb3.WeaponSoul)
	}

	if s.GetBinaryData().WeaponSoul.TrainRecords == nil {
		s.GetBinaryData().WeaponSoul.TrainRecords = make(map[uint32]*pb3.WeaponSoulTrain)
	}

	return s.GetBinaryData().WeaponSoul
}

func (s *WeaponSoulSystem) s2cInfo() {
	s.SendProto3(165, 1, &pb3.S2C_165_1{
		State: s.getData(),
	})
}

// c2sActiveQuest 激活神兵
func (s *WeaponSoulSystem) c2sUnlock(msg *base.Message) error {
	var req pb3.C2S_165_2
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("weaponSoul c2sUnlock unpack msg, err: %w", err)
	}

	cfg, exist := jsondata.GetWeaponSoulConfigByID(req.Id)
	if !exist {
		return neterror.ConfNotFoundError("weaponSoul c2sUnlock, id %d", req.Id)
	}

	data := s.getData()

	// 检查任务是否都已经激活
	for _, quest := range cfg.UnlockTargets {
		var active bool
		for _, record := range data.GetActiveQuestRecords() {
			if record.GetQuestId() == quest.ID && record.GetId() == req.Id {
				active = true
				break
			}
		}
		if !active {
			return neterror.ParamsInvalidError("weaponSoul c2sUnlock, id %d, quest %d 任务未激活", req.Id, quest.ID)
		}
	}

	// 解锁神兵后，加入养成记录
	data.TrainRecords[req.Id] = &pb3.WeaponSoulTrain{Id: req.Id}

	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogWeaponSoulUnlock, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.Id),
	})

	s.SendProto3(165, 2, &pb3.S2C_165_2{Id: req.Id})

	return nil
}

// c2sLevelUp 神兵进阶
func (s *WeaponSoulSystem) c2sLevelUp(msg *base.Message) error {
	var req pb3.C2S_165_3
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("weaponSoul c2sLevelUp unpack msg, err: %w", err)
	}

	// 检查是否有养成记录, 未有养成记录表示还未解锁
	data := s.getData()
	trainRecord, exist := data.GetTrainRecords()[req.Id]
	if !exist {
		return neterror.ParamsInvalidError("weaponSoul c2sLevelUp, id %d, 未解锁", req.Id)
	}

	// 当前槽位信息
	var slot *pb3.WeaponSoulSlot
	for _, v := range trainRecord.Slots {
		if v.Slot == req.Slot {
			slot = v
			break
		}
	}
	if slot == nil {
		slot = &pb3.WeaponSoulSlot{Slot: req.Slot, Level: 0, Star: 0}
		trainRecord.Slots = append(trainRecord.Slots, slot)
	}

	trainCfg, exist := jsondata.GetWeaponSoulTrainConfig(req.Id, slot.Slot, slot.Level+1) // 读取下一阶的配置
	if !exist {
		return neterror.ConfNotFoundError(
			"weaponSoul c2sLevelUp, id %d, level %d, star %d", req.Id, slot.Level, slot.Star)
	}

	if err := s.levelUpEquipCheck(trainCfg, req.GetHandles()); err != nil {
		return err
	}

	// 消耗道具 + 装备
	for _, handle := range req.GetHandles() {
		if ok := s.GetOwner().RemoveItemByHandle(handle, pb3.LogId_LogWeaponSoulLevelUp); !ok {
			return neterror.ConsumeFailedError("weaponSoul c2sLevelUp, remove item failed, handle %d", handle)
		}
	}
	if len(trainCfg.Consume) > 0 {
		if ok := s.GetOwner().ConsumeByConf(trainCfg.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogWeaponSoulLevelUp}); !ok {
			return neterror.ConsumeFailedError(
				"weaponSoul c2sLevelUp, consume failed id is %d, level %d, star %d,", req.Id, slot.Level, slot.Star)
		}
	}

	// 进阶并获得属性
	slot.Level++
	attrs, hasBest := s.levelUpAttrs(trainCfg, slot.GetAttrs())
	if hasBest {
		slot.Star++
	}
	slot.Attrs = append(slot.Attrs, attrs...)

	// 重算属性
	s.ResetSysAttr(attrdef.SaWeaponSoul)

	logParam := map[string]any{
		"id":    req.Id,
		"slot":  req.Slot,
		"level": slot.Level,
		"star":  slot.Star,
	}
	bt, _ := json.Marshal(logParam)
	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogWeaponSoulLevel, &pb3.LogPlayerCounter{
		NumArgs: uint64(req.Id),
		StrArgs: string(bt),
	})

	s.SendProto3(165, 3, &pb3.S2C_165_3{
		Id:   req.Id,
		Slot: slot,
	})

	return nil
}

// levelUpEquipCheck 检查装备是否满足升级条件
func (s *WeaponSoulSystem) levelUpEquipCheck(trainCfg *jsondata.WeaponSoulTrain, handles []uint64) error {
	needExp := trainCfg.LevelUpExp - trainCfg.DefaultExp
	if needExp <= 0 {
		return nil
	}

	equipIds := make([]uint32, 0, len(handles))
	for _, handle := range handles {
		item := s.GetOwner().GetItemByHandle(handle)
		if item == nil {
			return neterror.ParamsInvalidError("weaponSoul levelUpEquipCheck, handle %d, item not found", handle)
		}
		equipIds = append(equipIds, item.ItemId)
	}

	ownerSex, ownerJob := s.GetOwner().GetSex(), s.GetOwner().GetJob()

	var totalExp uint32
	for _, equipId := range equipIds {
		itemCfg := jsondata.GetItemConfig(equipId)
		if itemCfg == nil {
			return neterror.ConfNotFoundError("weaponSoul levelUpEquipCheck, equipId %d", equipId)
		}
		if (itemCfg.Sex > 0 && uint32(itemCfg.Sex) != ownerSex) || (itemCfg.Job > 0 && uint32(itemCfg.Job) != ownerJob) {
			return neterror.ParamsInvalidError("weaponSoul levelUpEquipCheck, equipId %d, 职业或性别与当前角色不符", equipId)
		}

		for _, v := range jsondata.GetWeaponSoulEquipConfig() {
			// 根据装备的星级、阶数、品质来确定经验值
			if v.Star == itemCfg.Star && v.Stage == itemCfg.Stage && v.Quality == itemCfg.Quality {
				totalExp += v.Exp
				break
			}
		}
	}

	if totalExp < needExp {
		return neterror.ParamsInvalidError("weaponSoul levelUpEquipCheck, totalExp %d, needExp %d", totalExp, needExp)
	}

	return nil
}

// levelUpAttrs 升级获得属性
func (s *WeaponSoulSystem) levelUpAttrs(trainCfg *jsondata.WeaponSoulTrain, ownerAttrs []*pb3.WeaponSoulAttr) ([]*pb3.WeaponSoulAttr, bool) {
	jsonAttrs := make([]*pb3.WeaponSoulAttr, 0, 2)
	// 基础属性
	for _, v := range trainCfg.BaseAttrs {
		attr := s.convertAttr(v)
		attr.IsBase = true
		jsonAttrs = append(jsonAttrs, attr)
	}

	// 随机属性, 需要排除已有的随机属性
	ownerRandAttrs := make(map[uint32]struct{})
	for _, attr := range ownerAttrs {
		if attr.IsBase {
			continue
		}
		ownerRandAttrs[attr.Id] = struct{}{}
	}

	var randAttrs []*jsondata.WeaponSoulAttr
	for _, v := range trainCfg.RandAttrs {
		if _, exist := ownerRandAttrs[v.ID]; exist {
			continue
		}
		randAttrs = append(randAttrs, v)
	}

	var hasBest bool
	if len(randAttrs) > 0 {
		idx := rand.Intn(len(randAttrs))
		randAttr := randAttrs[idx]
		hasBest = randAttr.IsBest
		jsonAttrs = append(jsonAttrs, s.convertAttr(randAttr))
	}

	return jsonAttrs, hasBest
}

func (s *WeaponSoulSystem) convertAttr(attr *jsondata.WeaponSoulAttr) *pb3.WeaponSoulAttr {
	return &pb3.WeaponSoulAttr{
		Attr: &pb3.AttrSt{
			Type:  attr.Type,
			Value: attr.Value,
		},
		Id:        attr.ID,
		IsAdvance: attr.IsBest,
	}
}

// c2sActiveStar 星级大师激活
func (s *WeaponSoulSystem) c2sActiveStar(_ *base.Message) error {
	if !s.starMasterGtCurrent() {
		return neterror.ParamsInvalidError("weaponSoul c2sActiveStar, 未满足星级大师激活条件")
	}

	data := s.getData()
	data.StarMaster++

	// 重算属性
	s.ResetSysAttr(attrdef.SaWeaponSoul)

	logworker.LogPlayerBehavior(s.GetOwner(), pb3.LogId_LogWeaponSoulMasterStar, &pb3.LogPlayerCounter{
		NumArgs: uint64(data.StarMaster),
	})

	s.SendProto3(165, 4, &pb3.S2C_165_4{StarMaster: data.StarMaster})
	return nil
}

// starMasterLtCurrent 达到星级大师是否大于当前已激活的
func (s *WeaponSoulSystem) starMasterGtCurrent() bool {
	data := s.getData()

	var currentStar uint32
	for _, record := range data.GetTrainRecords() {
		for _, slot := range record.GetSlots() {
			currentStar += slot.GetStar()
		}
	}

	for _, v := range jsondata.GetWeaponSoulStarConfig() {
		if currentStar >= v.NeedStar && v.Star > data.StarMaster {
			return true
		}
	}

	return false
}

// c2sActiveQuest 激活任务
func (s *WeaponSoulSystem) c2sActiveQuest(msg *base.Message) error {
	var req pb3.C2S_165_5
	if err := msg.UnpackagePbmsg(&req); err != nil {
		return neterror.ParamsInvalidError("weaponSoul c2sActiveQuest unpack msg, err: %w", err)
	}

	data := s.getData()
	// 检查任务是否已经激活
	for _, record := range data.ActiveQuestRecords {
		if record.GetQuestId() == req.QuestId && record.GetId() == req.Id {
			return neterror.ParamsInvalidError("weaponSoul c2sActiveQuest, id %d, quest %d 任务已激活", req.Id, req.QuestId)
		}
	}

	// 检查任务是否已经完成
	if !s.CheckFinishQuest(s.getUnFinishQuestData(req.QuestId)) {
		return neterror.ParamsInvalidError("weaponSoul c2sActiveQuest, id %d, quest %d 任务未完成", req.Id, req.QuestId)
	}

	data.ActiveQuestRecords = append(data.ActiveQuestRecords, &pb3.WeaponSoulActiveQuest{Id: req.Id, QuestId: req.QuestId})

	// 重算属性
	s.ResetSysAttr(attrdef.SaWeaponSoul)

	s.SendProto3(165, 5, &pb3.S2C_165_5{Id: req.Id, QuestId: req.QuestId})

	return nil
}

// calcWeaponSoulSysAttr 计算神兵附魂系统属性
func calcWeaponSoulSysAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	weaponSoul := player.GetBinaryData().GetWeaponSoul()

	var attrs []*jsondata.Attr
	// 升阶属性
	for _, record := range weaponSoul.GetTrainRecords() {
		for _, slot := range record.GetSlots() {
			for _, attr := range slot.GetAttrs() {
				attrs = append(attrs, &jsondata.Attr{
					Type:  attr.GetAttr().GetType(),
					Value: attr.GetAttr().GetValue(),
				})
			}
		}
	}

	// 激活任务属性
	for _, v := range weaponSoul.GetActiveQuestRecords() {
		attrs = append(attrs, jsondata.GetWeaponSoulQuestAttrs(v.GetQuestId())...)
	}

	// 星级大师属性
	starCfg := jsondata.GetWeaponSoulStarConfig()
	for _, cfg := range starCfg {
		if weaponSoul.GetStarMaster() == cfg.Star { // 星级大师等级属性需要修改为替换，而非叠加
			for _, v := range cfg.Attrs {
				attrs = append(attrs, v.Attr)
			}
			break
		}
	}

	engine.CheckAddAttrsToCalc(player, calc, attrs)
}

func newWeaponSoulSystem() iface.ISystem {
	var s WeaponSoulSystem

	s.QuestTargetBase = &QuestTargetBase{
		GetQuestIdSetFunc:        s.getQuestIdSet,
		GetUnFinishQuestDataFunc: s.getUnFinishQuestData,
		GetTargetConfFunc:        s.getTargetConf,
		OnUpdateTargetDataFunc:   s.onUpdateTargetData,
	}

	return &s
}

// acceptAllQuest 接受所有任务
func (s *WeaponSoulSystem) acceptAllQuest() {
	quests := jsondata.GetWeaponSoulAllQuests()

	data := s.getData()

	var idSet = make(map[uint32]struct{})
	for _, quest := range data.Quests {
		idSet[quest.Id] = struct{}{}
	}

	for _, target := range quests {
		if _, ok := idSet[target.ID]; ok {
			continue
		}
		data.Quests = append(data.Quests, &pb3.QuestData{
			Id: target.ID,
		})
	}

	for _, quest := range data.Quests {
		s.OnAcceptQuest(quest)
	}
}

func (s *WeaponSoulSystem) resetOpenedWeaponSoul() {
	data := s.getData()
	data.OpenedWeaponSouls = append(data.OpenedWeaponSouls, s.openWeaponSoul()...)
}

// getQuestIdSet 实现 QuestTargetBase 方法
func (s *WeaponSoulSystem) getQuestIdSet(qtt uint32) map[uint32]struct{} {
	set := make(map[uint32]struct{})

	quests := jsondata.GetWeaponSoulAllQuests()
	for id, target := range quests {
		if target.Type != qtt {
			continue
		}
		if _, ok := set[id]; ok {
			continue
		}
		set[id] = struct{}{}
	}

	return set
}

// isUnlockQuest 是否是解锁神兵的任务
func (s *WeaponSoulSystem) isUnlockQuest(id uint32) bool {
	cm := jsondata.GetWeaponSoulConfigMap()
	for _, conf := range cm {
		for _, v := range conf.UnlockTargets {
			if v.ID == id {
				return true
			}
		}
	}
	return false
}

// isOpenQuest 是否是开启神兵的任务
func (s *WeaponSoulSystem) isOpenQuest(id uint32) bool {
	cm := jsondata.GetWeaponSoulConfigMap()
	for _, conf := range cm {
		for _, v := range conf.OpenTargets {
			if v.ID == id {
				return true
			}
		}
	}
	return false
}

// getUnFinishQuestData 实现 QuestTargetBase 方法
func (s *WeaponSoulSystem) getUnFinishQuestData(id uint32) *pb3.QuestData {
	data := s.getData()

	for _, quest := range data.Quests {
		if quest.GetId() == id {
			return quest
		}
	}
	return nil
}

// getTargetConf 实现 QuestTargetBase 方法
func (s *WeaponSoulSystem) getTargetConf(id uint32) []*jsondata.QuestTargetConf {
	quests := jsondata.GetWeaponSoulAllQuests()

	quest, ok := quests[id]
	if !ok {
		return nil
	}

	return []*jsondata.QuestTargetConf{quest.QuestTargetConf}
}

// onUpdateTargetData 实现 QuestTargetBase 方法
func (s *WeaponSoulSystem) onUpdateTargetData(id uint32) {
	quest := s.getUnFinishQuestData(id)
	if quest != nil && s.isUnlockQuest(id) { // 只有解锁的任务才通知客户端
		s.SendProto3(165, 6, &pb3.S2C_165_6{Quest: quest})
	}

	if !s.CheckFinishQuest(quest) || !s.isOpenQuest(id) {
		return
	}

	// 检查是否有新的神兵开启，并通知客户端
	ids := s.openWeaponSoul()
	if len(ids) > 0 {
		data := s.getData()
		data.OpenedWeaponSouls = append(data.OpenedWeaponSouls, ids...)

		s.SendProto3(165, 7, &pb3.S2C_165_7{Ids: ids})
	}
}

// openWeaponSoul 通过任务开启神兵
// 返回新开启的神兵id列表
func (s *WeaponSoulSystem) openWeaponSoul() []uint32 {
	unOpenedWeaponSouls := s.getUnOpenedWeaponSouls()

	var openedWeaponSouls []uint32
	for _, id := range unOpenedWeaponSouls {
		conf, exist := jsondata.GetWeaponSoulConfigByID(id)
		if !exist {
			continue
		}

		// 检查任务是否都已经完成
		allFinish := true
		for _, t := range conf.OpenTargets {
			if !s.CheckFinishQuest(s.getUnFinishQuestData(t.ID)) {
				allFinish = false
				break
			}
		}

		if allFinish {
			openedWeaponSouls = append(openedWeaponSouls, id)
		}
	}

	return openedWeaponSouls
}

// getUnOpenedWeaponSouls 获取未开启的神兵
func (s *WeaponSoulSystem) getUnOpenedWeaponSouls() []uint32 {
	var ids []uint32

	sm := jsondata.GetWeaponSoulConfigMap()
	for id := range sm {
		if !slices.Contains(s.getData().GetOpenedWeaponSouls(), id) {
			ids = append(ids, id)
		}
	}

	return ids
}
