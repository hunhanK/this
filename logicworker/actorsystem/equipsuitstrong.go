package actorsystem

import (
	"encoding/json"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"jjyz/gameserver/net"
)

type EquipSuitStrongSys struct {
	Base
}

func (s *EquipSuitStrongSys) OnInit() {
	binary := s.GetBinaryData()
	if nil == binary.EquipSuitStrong {
		binary.EquipSuitStrong = make(map[uint32]uint32)
	}
}

func (s *EquipSuitStrongSys) OnAfterLogin() {
	s.s2cInfo()
}

func (s *EquipSuitStrongSys) OnReconnect() {
	s.s2cInfo()
}

func (s *EquipSuitStrongSys) GetData() map[uint32]uint32 {
	binary := s.GetBinaryData()
	if nil == binary.EquipSuitStrong {
		binary.EquipSuitStrong = make(map[uint32]uint32)
	}
	return binary.EquipSuitStrong
}

func (s *EquipSuitStrongSys) s2cInfo() {
	s.SendProto3(11, 21, &pb3.S2C_11_21{EquipSuitStrong: s.GetData()})
}

func c2sSuitStrong(sys iface.ISystem) func(*base.Message) error {
	return func(msg *base.Message) error {
		s := sys.(*EquipSuitStrongSys)
		var req pb3.C2S_11_22
		err := pb3.Unmarshal(msg.Data, &req)
		if err != nil {
			return err
		}
		data := s.GetData()
		pos := req.GetPos()
		if pos < itemdef.EtBegin || pos > itemdef.EtEnd {
			return neterror.ParamsInvalidError("not equip pos(%d)", pos)
		}
		suitType := data[pos]
		ntType := suitType + 1
		conf := jsondata.GetEquipSuitStrongConf(ntType)
		if nil == conf {
			return neterror.ConfNotFoundError("equipsuitstrong conf(%d) is nil", ntType)
		}
		equipSys, ok := s.owner.GetSysObj(sysdef.SiEquip).(*EquipSystem)
		if !ok {
			return neterror.InternalError("equip sys is nil")
		}
		_, equip := equipSys.GetEquipByPos(pos)
		if nil == equip {
			return neterror.ParamsInvalidError("no equip in pos")
		}
		itemConf := jsondata.GetItemConfig(equip.ItemId)
		if nil == itemConf {
			return neterror.ConfNotFoundError("no equip item conf(%d)", equip.ItemId)
		}
		if itemConf.Star < conf.Star || itemConf.Quality < conf.Quality {
			s.owner.SendTipMsg(tipmsgid.EquipSuitStrongCondNotMeet)
			return nil
		}
		condConf := conf.GetEquipSuitStrongConf(pos, itemConf.Stage)
		if nil == condConf {
			return neterror.ConfNotFoundError("no equipsuitstrongcond conf stage(%d)", itemConf.Stage)
		}
		if !s.owner.ConsumeByConf(condConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogEquipSuitStrong}) {
			s.owner.SendTipMsg(tipmsgid.TpItemNotEnough)
			return nil
		}
		data[pos] = ntType
		s.SendProto3(11, 22, &pb3.S2C_11_22{
			Pos:      pos,
			SuitType: ntType,
		})
		s.ResetSysAttr(attrdef.SaEquipSuitStrong)
		s.afterSuitStrong()
		logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogEquipSuitStrong, &pb3.LogPlayerCounter{
			StrArgs: logworker.ConvertJsonStr(map[string]interface{}{
				"pos":      pos,
				"suitType": data[pos],
			}),
		})
		return nil
	}
}

func calcEquipSuitStrongAttr(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	itemPool := player.GetMainData().ItemPool
	if nil == itemPool || nil == itemPool.Equips {
		return
	}
	data := player.GetBinaryData().EquipSuitStrong
	if nil == data {
		return
	}
	confList := jsondata.GetEquipSuitStrongAttrConf()
	if nil == confList {
		return
	}
	suitMap := make(map[uint32]map[uint32]uint32)
	stageMark := make(map[uint32]struct{})
	for _, equip := range itemPool.Equips {
		itemConf := jsondata.GetItemConfig(equip.ItemId)
		if nil == itemConf {
			continue
		}
		equipType := itemdef.GetEquipTypeByPos(equip.Pos)
		if nil == suitMap[equipType] {
			suitMap[equipType] = make(map[uint32]uint32)
		}
		if data[equip.Pos] == 0 { //不存在装备套装阶级
			continue
		}
		for suitT := uint32(1); suitT <= data[equip.Pos]; suitT++ {
			suitMap[equipType][suitT]++ //类型数量
		}

		stageMark[itemConf.Stage] = struct{}{}
	}

	for equipType, line := range confList {
		if nil == suitMap[equipType] || nil == line.Suits {
			continue
		}
		for _, suitConf := range line.Suits {
			var maxSuit uint32
			var minn uint64
			var attrConf *jsondata.EquipSuitAttrConf
			for suitNum, num := range suitMap[equipType] {
				if num >= suitConf.SuitNum && suitNum > maxSuit { //符合此件数的最大类型
					maxSuit = suitNum
				}
			}
			if maxSuit > 0 { //x件套所述的最大类型的套装
				stageMap := make(map[uint32]uint32)
				for _, equip := range itemPool.Equips {
					itemConf := jsondata.GetItemConfig(equip.ItemId)
					if nil == itemConf {
						continue
					}
					if data[equip.Pos] < maxSuit {
						continue
					}
					for sk := range stageMark {
						if itemConf.Stage >= sk { //统计符合该套装类型的所有不小于该阶数的装备数量
							stageMap[sk]++
						}
					}
				}
				var maxStage uint32
				for sk, num := range stageMap {
					if num >= suitConf.SuitNum && sk > maxStage { //符合此件数和套装类型的最大件数
						maxStage = sk
					}
				}
				maxCond := utils.Make64(maxSuit, maxStage)
				for _, condConf := range suitConf.CondConf {
					staticCond := utils.Make64(condConf.Suit, condConf.Stage)
					if staticCond <= maxCond && (minn == 0 || minn < staticCond) {
						minn = staticCond
						attrConf = condConf
					}
				}
				if nil != attrConf {
					engine.CheckAddAttrsToCalc(player, calc, attrConf.Attrs)
				}
			}
		}
	}
}

func calcEquipSuitStrongAttrAddRate(player iface.IPlayer, totalSysCalc, calc *attrcalc.FightAttrCalc) {
	itemPool := player.GetMainData().ItemPool
	if nil == itemPool || nil == itemPool.Equips {
		return
	}

	data := player.GetBinaryData().EquipSuitStrong
	if nil == data {
		return
	}

	confList := jsondata.GetEquipSuitStrongAttrConf()
	if nil == confList {
		return
	}

	suitMap := make(map[uint32]map[uint32]uint32)
	stageMark := make(map[uint32]struct{})
	for _, equip := range itemPool.Equips {
		itemConf := jsondata.GetItemConfig(equip.ItemId)
		if nil == itemConf {
			continue
		}
		equipType := itemdef.GetEquipTypeByPos(equip.Pos)
		if nil == suitMap[equipType] {
			suitMap[equipType] = make(map[uint32]uint32)
		}
		if data[equip.Pos] == 0 { //不存在装备套装阶级
			continue
		}
		for suitT := uint32(1); suitT <= data[equip.Pos]; suitT++ {
			suitMap[equipType][suitT]++ //类型数量
		}

		stageMark[itemConf.Stage] = struct{}{}
	}

	addRate := totalSysCalc.GetValue(attrdef.EquipSuitStrongAddRate)
	addRate1 := totalSysCalc.GetValue(attrdef.EquipBasicSuitStrongAddRate)
	addRate2 := totalSysCalc.GetValue(attrdef.EquipJewelrySuitStrongAddRate)
	for equipType, line := range confList {
		if nil == suitMap[equipType] || nil == line.Suits {
			continue
		}
		for _, suitConf := range line.Suits {
			var maxSuit uint32
			var minn uint64
			var attrConf *jsondata.EquipSuitAttrConf
			for suitNum, num := range suitMap[equipType] {
				if num >= suitConf.SuitNum && suitNum > maxSuit { //符合此件数的最大类型
					maxSuit = suitNum
				}
			}
			if maxSuit > 0 { //x件套所述的最大类型的套装
				stageMap := make(map[uint32]uint32)
				for _, equip := range itemPool.Equips {
					itemConf := jsondata.GetItemConfig(equip.ItemId)
					if nil == itemConf {
						continue
					}
					if data[equip.Pos] < maxSuit {
						continue
					}
					for sk := range stageMark {
						if itemConf.Stage >= sk { //统计符合该套装类型的所有不小于该阶数的装备数量
							stageMap[sk]++
						}
					}
				}
				var maxStage uint32
				for sk, num := range stageMap {
					if num >= suitConf.SuitNum && sk > maxStage { //符合此件数和套装类型的最大件数
						maxStage = sk
					}
				}
				maxCond := utils.Make64(maxSuit, maxStage)
				for _, condConf := range suitConf.CondConf {
					staticCond := utils.Make64(condConf.Suit, condConf.Stage)
					if staticCond <= maxCond && (minn == 0 || minn < staticCond) {
						minn = staticCond
						attrConf = condConf
					}
				}
				// 装备套装固定属性加成
				var attrs jsondata.AttrVec
				for _, attr := range attrConf.Attrs {
					conf := jsondata.AttrFightArray[attr.Type]
					if conf.FormatType > 0 {
						continue
					}
					finialRate := addRate
					switch equipType {
					case itemdef.EtTypeBasic:
						finialRate += addRate1
					case itemdef.EtTypeJewelry:
						finialRate += addRate2
					}
					if finialRate == 0 {
						continue
					}
					value := utils.CalcMillionRate(attr.Value, uint32(finialRate))
					attrs = append(attrs, &jsondata.Attr{Type: attr.Type, Value: value, Job: attr.Job})
				}
				if len(attrs) > 0 {
					engine.CheckAddAttrsToCalc(player, calc, attrs)
				}
			}
		}
	}
}

func (s *EquipSuitStrongSys) back(pos, stage uint32) {
	if pos < itemdef.EtBegin && pos > itemdef.EtEnd {
		return
	}
	data := s.GetData()
	suitType := data[pos]
	if suitType <= 0 {
		return
	}
	data[pos] = 0
	var rewards []*jsondata.StdReward
	moneyMap := make(map[uint32]int64)
	for i := uint32(1); i <= suitType; i++ {
		conf := jsondata.GetEquipSuitStrongConf(i)
		if nil == conf {
			s.LogError("not find equipsuitstrong conf(%d) pos(%d) when reback material", i, pos)
			return
		}
		condConf := conf.GetEquipSuitStrongConf(pos, stage)
		if nil == condConf {
			s.LogError("not find equipsuitstrong conf(%d) pos(%d) when reback material", i, pos)
			return
		}
		for _, line := range condConf.Consume {
			if line.Job > 0 && line.Job != s.owner.GetJob() {
				continue
			}
			if line.Type == custom_id.ConsumeTypeItem {
				rewards = append(rewards, &jsondata.StdReward{
					Id:    line.Id,
					Count: int64(line.Count),
				})
			} else if line.Type == custom_id.ConsumeTypeMoney {
				moneyMap[line.Id] += int64(line.Count)
			}
		}
	}
	engine.GiveRewards(s.owner, rewards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogEquipSuitTakeOffBack})
	for mt, count := range moneyMap {
		s.owner.AddMoney(mt, count, false, pb3.LogId_LogEquipSuitTakeOffBack)
	}
	s.SendProto3(11, 22, &pb3.S2C_11_22{
		Pos:      pos,
		SuitType: 0,
	})
	s.ResetSysAttr(attrdef.SaEquipSuitStrong)
	logArg, _ := json.Marshal(&pb3.KeyValue{
		Key:   pos,
		Value: data[pos],
	})
	logworker.LogPlayerBehavior(s.owner, pb3.LogId_LogEquipSuitTakeOffBack, &pb3.LogPlayerCounter{
		StrArgs: string(logArg),
	})
}

func getEquipSuitMap(player iface.IPlayer) map[uint32]map[uint32]uint32 {
	itemPool := player.GetMainData().ItemPool
	if nil == itemPool || nil == itemPool.Equips {
		return nil
	}

	data := player.GetBinaryData().EquipSuitStrong
	if nil == data {
		return nil
	}

	suitMap := make(map[uint32]map[uint32]uint32)
	for _, equip := range itemPool.Equips {
		itemConf := jsondata.GetItemConfig(equip.ItemId)
		if nil == itemConf {
			continue
		}
		equipType := itemdef.GetEquipTypeByPos(equip.Pos)
		if nil == suitMap[equipType] {
			suitMap[equipType] = make(map[uint32]uint32)
		}
		if data[equip.Pos] == 0 { //不存在装备套装阶级
			continue
		}
		for suitT := uint32(1); suitT <= data[equip.Pos]; suitT++ {
			suitMap[equipType][suitT]++ //类型数量
		}
	}
	return suitMap
}

func (s *EquipSuitStrongSys) afterSuitStrong() {
	player := s.GetOwner()
	suitMap := getEquipSuitMap(player)
	for equipType, m := range suitMap {
		var qtttype uint32
		switch equipType {
		case itemdef.EtTypeBasic:
			qtttype = custom_id.QttBaseEquipSuitStrong
		case itemdef.EtTypeJewelry:
			qtttype = custom_id.QttJewelryEquipSuitStrong
		}
		if qtttype == 0 {
			continue
		}
		for suitT, count := range m {
			if count == 0 {
				continue
			}
			player.TriggerQuestEvent(qtttype, suitT, int64(count))
		}
	}
	player.TriggerQuestEventRange(custom_id.QttAllEquipSuitStrong)
	player.TriggerQuestEventRange(custom_id.QttAnyEquipSuitStrong)
}

func onEquipTakeOff(player iface.IPlayer, args ...interface{}) {
	if len(args) < 2 {
		return
	}
	oldEquip, ok := args[0].(*pb3.ItemSt)
	if !ok {
		return
	}
	pos, ok := args[1].(uint32)
	if !ok {
		return
	}
	s, ok := player.GetSysObj(sysdef.SiEquipSuitStrong).(*EquipSuitStrongSys)
	if !ok {
		return
	}
	if itemConf := jsondata.GetItemConfig(oldEquip.ItemId); nil != itemConf {
		s.back(pos, itemConf.Stage)
	}
}

func onEquipTakeReplace(player iface.IPlayer, args ...interface{}) {
	if len(args) < 3 {
		return
	}
	newequip, ok := args[0].(*pb3.ItemSt)
	if !ok {
		return
	}

	oldEquip, ok := args[1].(*pb3.ItemSt)
	if !ok {
		return
	}
	pos, ok := args[2].(uint32)
	if !ok {
		return
	}
	newItemConf := jsondata.GetItemConfig(newequip.ItemId)
	oldItemConf := jsondata.GetItemConfig(oldEquip.ItemId)
	if nil == newItemConf || nil == oldItemConf {
		player.LogError("not find equip conf: %d or %d", newequip.ItemId, oldEquip.ItemId)
		return
	}
	s, ok := player.GetSysObj(sysdef.SiEquipSuitStrong).(*EquipSuitStrongSys)
	if !ok {
		return
	}
	if newItemConf.Stage != oldItemConf.Stage {
		s.back(pos, oldItemConf.Stage)
		return
	}
	data := s.GetData()
	suitType := data[pos]
	if suitType <= 0 {
		return
	}
	conf := jsondata.GetEquipSuitStrongConf(suitType)
	if nil == conf {
		player.LogError("equipsuitstrong conf(%d) is nil", suitType)
		return
	}
	//阶数相同
	if newItemConf.Star < conf.Star || newItemConf.Quality < conf.Quality {
		s.back(pos, oldItemConf.Stage)
		return
	}
}

func init() {
	RegisterSysClass(sysdef.SiEquipSuitStrong, func() iface.ISystem {
		return &EquipSuitStrongSys{}
	})

	engine.RegAttrCalcFn(attrdef.SaEquipSuitStrong, calcEquipSuitStrongAttr)
	engine.RegAttrAddRateCalcFn(attrdef.SaEquipSuitStrong, calcEquipSuitStrongAttrAddRate)

	//返还材料
	event.RegActorEvent(custom_id.AeTakeOffEquip, onEquipTakeOff)
	event.RegActorEvent(custom_id.AeTakeReplaceEquip, onEquipTakeReplace)

	engine.RegQuestTargetProgress(custom_id.QttAllEquipSuitStrong, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		if len(ids) != 2 {
			return 0
		}
		suitMap := getEquipSuitMap(actor)
		var count uint32
		suitT1 := ids[0]
		suitT2 := ids[1]
		m, ok := suitMap[itemdef.EtTypeBasic]
		if ok {
			count += m[suitT1]
		}
		m, ok = suitMap[itemdef.EtTypeJewelry]
		if ok {
			count += m[suitT2]
		}
		return count
	})

	engine.RegQuestTargetProgress(custom_id.QttAnyEquipSuitStrong, func(actor iface.IPlayer, ids []uint32, args ...interface{}) uint32 {
		if len(ids) != 3 {
			return 0
		}
		suitMap := getEquipSuitMap(actor)
		suitT1 := ids[0]
		suitT2 := ids[1]
		count := ids[2]
		m, ok := suitMap[itemdef.EtTypeBasic]
		if ok && count <= m[suitT1] {
			return 1
		}
		m, ok = suitMap[itemdef.EtTypeJewelry]
		if ok && count <= m[suitT2] {
			return 1
		}
		return 0
	})

	net.RegisterSysProtoV2(11, 22, sysdef.SiEquipSuitStrong, c2sSuitStrong)
}
