/**
 * @Author: zjj
 * @Desc: 铸龙系统
 * @Date:
 */

package actorsystem

import (
	"jjyz/base"
	"jjyz/base/attrcalc"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type CastDragonSys struct {
	Base
}

func (sys *CastDragonSys) OnReconnect() {
	sys.ReCalcProperty()
	sys.s2cSendCastDragonInfo()
}

func (sys *CastDragonSys) OnLogin() {
	sys.s2cSendCastDragonInfo()
}

func (sys *CastDragonSys) OnOpen() {
	binary := sys.GetBinaryData()
	castDragonData := binary.CastDragonDatas
	if nil == castDragonData {
		castDragonData = make([]*pb3.CastDragonData, 0)
		binary.CastDragonDatas = castDragonData
	}
}

func (sys *CastDragonSys) getCastDragonInfo(castType, pos uint32) uint32 {
	binary := sys.GetBinaryData()
	for _, line := range binary.CastDragonDatas {
		if line.GetType() == castType {
			for _, line2 := range line.Pos {
				if line2.GetKey() == pos {
					return line2.GetValue()
				}
			}
		}
	}

	return 0
}

func (sys *CastDragonSys) ReCalcProperty() {
	sys.ResetSysAttr(attrdef.CastDragonProperty)
}

func (sys *CastDragonSys) CastDragon(castType, pos uint32) error {
	conf := jsondata.GetCastDragonConf(castType)
	posConf := jsondata.GetCastDragonPosConf(castType, pos)
	if nil == posConf || nil == conf {
		return neterror.ConfNotFoundError("%d %d not found pos", castType, pos)
	}

	if gshare.GetOpenServerDay() < conf.OpenDay {
		return neterror.ParamsInvalidError("%d < %d open server day", gshare.GetOpenServerDay(), conf.OpenDay)
	}

	// 检查是否穿戴装备
	itemId := sys.getTypeEquip(castType, pos)
	if itemId == 0 {
		return neterror.ParamsInvalidError("%d %d not take on equip", castType, pos)
	}
	itemConf := jsondata.GetItemConfig(itemId)
	if nil == itemConf {
		return neterror.ConfNotFoundError("%d not found item", itemId)
	}

	binary := sys.GetBinaryData()
	var data *pb3.CastDragonData
	for _, line := range binary.CastDragonDatas {
		if line.GetType() == castType {
			data = line
		}
	}

	if nil == data {
		data = &pb3.CastDragonData{
			Type: castType,
		}
		data.Pos = make([]*pb3.KeyValue, 0)
		binary.CastDragonDatas = append(binary.CastDragonDatas, data)
	}

	var keyValue *pb3.KeyValue
	for _, line := range data.Pos {
		if line.GetKey() == pos {
			keyValue = line
		}
	}

	if nil == keyValue {
		keyValue = &pb3.KeyValue{
			Key: pos,
		}
		data.Pos = append(data.Pos, keyValue)
	}

	// 检查穿戴装备等级
	lv := keyValue.GetValue() + 1
	lvConf := jsondata.GetCastDragonLvConf(castType, pos, lv)
	if nil == lvConf || lv > itemConf.Stage {
		return neterror.ConfNotFoundError("%d %d %d not found conf or lv %d > state %d", castType, pos, lv, lv, itemConf.Stage)
	}

	if !sys.owner.ConsumeByConf(lvConf.Consume, false, common.ConsumeParams{LogId: pb3.LogId_LogCastDragon}) {
		return neterror.ConsumeFailedError("%d %d consume failed", castType, pos)
	}

	keyValue.Value = lv
	sys.ReCalcProperty()

	engine.BroadcastTipMsgById(lvConf.Broadcast, sys.owner.GetId(), sys.owner.GetName(), itemId, lv)

	return nil
}

func (sys *CastDragonSys) getTypeEquip(castType, pos uint32) uint32 {
	var itemId uint32
	binary := sys.GetBinaryData()
	switch castType {
	case custom_id.CastDragonTypeWingAccessory: // 神翼副装
		if binary.FairyWingData != nil && binary.FairyWingData.DeputyEquips != nil {
			for takeOnPos, takeOnItemId := range binary.FairyWingData.DeputyEquips {
				if takeOnPos == pos {
					itemId = takeOnItemId
					break
				}
			}
		}
	case custom_id.CastDragonTypeWingGodEquip: // 神翼神装
		if binary.FairyWingData != nil && binary.FairyWingData.DeputyEquips != nil {
			for takeOnPos, takeOnItemId := range binary.FairyWingData.PrincipaleEquips {
				if takeOnPos == pos {
					itemId = takeOnItemId
					break
				}
			}
		}
	case custom_id.CastDragonTypeDragonAccessory: // 青龙副装
		symbols := binary.FourSymbols
		if symbols == nil || symbols[custom_id.FourSymbolsDragon] == nil || symbols[custom_id.FourSymbolsDragon].FourSymbolsEquipMap == nil {
			return 0
		}
		for _, tokeOnItemId := range symbols[custom_id.FourSymbolsDragon].FourSymbolsEquipMap {
			conf := jsondata.GetItemConfig(tokeOnItemId)
			if conf == nil {
				continue
			}
			if conf.Type == itemdef.ItemFsDragonDeputyEquip && conf.SubType == pos {
				itemId = tokeOnItemId
				break
			}
		}
	case custom_id.CastDragonTypeDragonGodEquip: // 青龙神装
		symbols := binary.FourSymbols
		if symbols == nil || symbols[custom_id.FourSymbolsDragon] == nil || symbols[custom_id.FourSymbolsDragon].FourSymbolsEquipMap == nil {
			return 0
		}
		for _, tokeOnItemId := range symbols[custom_id.FourSymbolsDragon].FourSymbolsEquipMap {
			conf := jsondata.GetItemConfig(tokeOnItemId)
			if conf == nil {
				continue
			}
			if conf.Type == itemdef.ItemFsDragonPrincipalEquip && conf.SubType == pos {
				itemId = tokeOnItemId
				break
			}
		}
	case custom_id.CastDragonTypeTigerAccessory: // 白虎副装
		symbols := binary.FourSymbols
		if symbols == nil || symbols[custom_id.FourSymbolsTiger] == nil || symbols[custom_id.FourSymbolsTiger].FourSymbolsEquipMap == nil {
			return 0
		}
		for _, tokeOnItemId := range symbols[custom_id.FourSymbolsTiger].FourSymbolsEquipMap {
			conf := jsondata.GetItemConfig(tokeOnItemId)
			if conf == nil {
				continue
			}
			if conf.Type == itemdef.ItemFsTigerDeputyEquipEquip && conf.SubType == pos {
				itemId = tokeOnItemId
				break
			}
		}
	case custom_id.CastDragonTypeTigerGodEquip: // 白虎神装
		symbols := binary.FourSymbols
		if symbols == nil || symbols[custom_id.FourSymbolsTiger] == nil || symbols[custom_id.FourSymbolsTiger].FourSymbolsEquipMap == nil {
			return 0
		}
		for _, tokeOnItemId := range symbols[custom_id.FourSymbolsTiger].FourSymbolsEquipMap {
			conf := jsondata.GetItemConfig(tokeOnItemId)
			if conf == nil {
				continue
			}
			if conf.Type == itemdef.ItemFsTigerPrincipalEquip && conf.SubType == pos {
				itemId = tokeOnItemId
				break
			}
		}
	case custom_id.CastDragonTypeRoseFinchAccessory: // 朱雀副装
		symbols := binary.FourSymbols
		if symbols == nil || symbols[custom_id.FourSymbolsRosefinch] == nil || symbols[custom_id.FourSymbolsRosefinch].FourSymbolsEquipMap == nil {
			return 0
		}
		for _, tokeOnItemId := range symbols[custom_id.FourSymbolsRosefinch].FourSymbolsEquipMap {
			conf := jsondata.GetItemConfig(tokeOnItemId)
			if conf == nil {
				continue
			}
			if conf.Type == itemdef.ItemFsRosefinchDeputyEquip && conf.SubType == pos {
				itemId = tokeOnItemId
				break
			}
		}
	case custom_id.CastDragonTypeRoseFinchGodEquip: // 朱雀神装
		symbols := binary.FourSymbols
		if symbols == nil || symbols[custom_id.FourSymbolsRosefinch] == nil || symbols[custom_id.FourSymbolsRosefinch].FourSymbolsEquipMap == nil {
			return 0
		}
		for _, tokeOnItemId := range symbols[custom_id.FourSymbolsRosefinch].FourSymbolsEquipMap {
			conf := jsondata.GetItemConfig(tokeOnItemId)
			if conf == nil {
				continue
			}
			if conf.Type == itemdef.ItemFsRosefinchPrincipalEquip && conf.SubType == pos {
				itemId = tokeOnItemId
				break
			}
		}
	case custom_id.CastDragonTypeTortoiseAccessory: // 玄武副装
		symbols := binary.FourSymbols
		if symbols == nil || symbols[custom_id.FourSymbolsTortoise] == nil || symbols[custom_id.FourSymbolsTortoise].FourSymbolsEquipMap == nil {
			return 0
		}
		for _, tokeOnItemId := range symbols[custom_id.FourSymbolsTortoise].FourSymbolsEquipMap {
			conf := jsondata.GetItemConfig(tokeOnItemId)
			if conf == nil {
				continue
			}
			if conf.Type == itemdef.ItemFsTortoiseDeputyEquip && conf.SubType == pos {
				itemId = tokeOnItemId
				break
			}
		}
	case custom_id.CastDragonTypeTortoiseGodEquip: // 玄武神装
		symbols := binary.FourSymbols
		if symbols == nil || symbols[custom_id.FourSymbolsTortoise] == nil || symbols[custom_id.FourSymbolsTortoise].FourSymbolsEquipMap == nil {
			return 0
		}
		for _, tokeOnItemId := range symbols[custom_id.FourSymbolsTortoise].FourSymbolsEquipMap {
			conf := jsondata.GetItemConfig(tokeOnItemId)
			if conf == nil {
				continue
			}
			if conf.Type == itemdef.ItemFsTortoisePrincipalEquip && conf.SubType == pos {
				itemId = tokeOnItemId
				break
			}
		}
	case custom_id.CastDragonTypeRiderAccessory: // 坐骑副装
		if binary.RiderData != nil && binary.RiderData.DeputyEquips != nil {
			for takeOnPos, takeOnItemId := range binary.RiderData.DeputyEquips {
				if takeOnPos == pos {
					itemId = takeOnItemId
					break
				}
			}
		}
	case custom_id.CastDragonTypeRiderGodEquip: // 坐骑神装
		if binary.RiderData != nil && binary.RiderData.DeputyEquips != nil {
			for takeOnPos, takeOnItemId := range binary.RiderData.PrincipaleEquips {
				if takeOnPos == pos {
					itemId = takeOnItemId
					break
				}
			}
		}
	}

	return itemId
}

func (sys *CastDragonSys) s2cSendCastDragonInfo() {
	binary := sys.GetBinaryData()

	sys.SendProto3(11, 50, &pb3.S2C_11_50{
		DataList: binary.CastDragonDatas,
	})
}

func (sys *CastDragonSys) s2cSendOneCastDragonInfo(castType, pos uint32) {
	lv := sys.getCastDragonInfo(castType, pos)

	sys.SendProto3(11, 51, &pb3.S2C_11_51{
		CastType: castType,
		Pos:      pos,
		Lv:       lv,
	})
}

func (sys *CastDragonSys) castDragon(msg *base.Message) error {
	var req pb3.C2S_11_51
	if err := msg.UnpackagePbmsg(&req); nil != err {
		return err
	}

	castType, pos := req.CastType, req.Pos
	err := sys.CastDragon(castType, pos)
	if err != nil {
		sys.GetOwner().LogError("err:%v", err)
		return err
	}

	sys.s2cSendOneCastDragonInfo(castType, pos)
	return nil
}

func calcCastDragonProperty(player iface.IPlayer, calc *attrcalc.FightAttrCalc) {
	if nil == player {
		return
	}

	sys, ok := player.GetSysObj(sysdef.SiCastDragon).(*CastDragonSys)
	if !ok {
		return
	}

	binary := player.GetBinaryData()
	for _, line := range binary.CastDragonDatas {
		castType := line.GetType()
		var minLv uint32
		var isAllTake = true
		for _, line2 := range line.Pos {
			pos := line2.GetKey()
			if curItem := sys.getTypeEquip(castType, pos); curItem == 0 {
				isAllTake = false
				continue
			}

			lv := line2.GetValue()
			if conf := jsondata.GetCastDragonLvConf(castType, pos, lv); nil != conf {
				engine.AddAttrsToCalc(player, calc, conf.Attrs)
			}

			if isAllTake && (minLv == 0 || minLv > lv) {
				minLv = lv
			}
		}

		// 套装属性
		if isAllTake {
			if conf := jsondata.GetCastDragonConf(castType); nil != conf && minLv > 0 {
				var attr []*jsondata.Attr
				if len(conf.PosConf) <= len(line.Pos) {
					for _, line2 := range conf.SuitConf {
						if line2.SuitLv <= minLv {
							attr = line2.Attrs
						}
					}
				}

				if nil != attr {
					engine.AddAttrsToCalc(player, calc, attr)
				}
			}
		}
	}
}

func init() {
	RegisterSysClass(sysdef.SiCastDragon, func() iface.ISystem {
		return &CastDragonSys{}
	})

	event.RegActorEvent(custom_id.AeChangeCastDragonEquip, func(player iface.IPlayer, args ...interface{}) {
		sys, ok := player.GetSysObj(sysdef.SiCastDragon).(*CastDragonSys)
		if !ok {
			return
		}
		sys.ResetSysAttr(attrdef.CastDragonProperty)
	})
	engine.RegAttrCalcFn(attrdef.CastDragonProperty, calcCastDragonProperty)
	//manager.RegisterViewFunc(common.ViewPlayerCastDragon, viewOtherPlayerCastDragon)
	net.RegisterSysProtoV2(11, 51, sysdef.SiCastDragon, func(sys iface.ISystem) func(*base.Message) error {
		return sys.(*CastDragonSys).castDragon
	})
}
