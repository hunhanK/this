/**
 * @Author: zjj
 * @Date: 2024/5/9
 * @Desc: 战力冲榜
**/

package ranktype

type PowerRushRankType uint8

const (
	PowerRushRankTypeLevel                PowerRushRankType = 1  // 战力冲榜 - 等级
	PowerRushRankTypeEquip                PowerRushRankType = 2  // 战力冲榜 - 装备
	PowerRushRankTypeCharge               PowerRushRankType = 3  // 战力冲榜 - 充值
	PowerRushRankTypeFourSymbolsDragon    PowerRushRankType = 4  // 战力冲榜 - 四象青龙
	PowerRushRankTypeFourSymbolsTiger     PowerRushRankType = 5  // 战力冲榜 - 四象白虎
	PowerRushRankTypeFourSymbolsRoseFinch PowerRushRankType = 6  // 战力冲榜 - 四象朱雀
	PowerRushRankTypeFourSymbolsTortoise  PowerRushRankType = 7  // 战力冲榜 - 四象玄武
	PowerRushRankTypeFaBao                PowerRushRankType = 8  // 战力冲榜 - 法宝
	PowerRushRankTypeRider                PowerRushRankType = 9  // 战力冲榜 - 坐骑
	PowerRushRankTypeFairy                PowerRushRankType = 10 // 战力冲榜 - 仙灵
	PowerRushRankTypeWing                 PowerRushRankType = 11 // 战力冲榜 - 仙翼
	PowerRushRankTypeDailyUpPower         PowerRushRankType = 12 // 战力冲榜 - 单日提升总战力
)

type PowerRushRankSubType uint8

const (
	PowerRushRankSubTypeLevel                         PowerRushRankSubType = 1  // 战力冲榜子类 - 等级
	PowerRushRankSubTypeEquip                         PowerRushRankSubType = 2  // 战力冲榜子类 - 装备
	PowerRushRankSubTypeEquipStrong                   PowerRushRankSubType = 3  // 战力冲榜子类 - 装备强化
	PowerRushRankSubTypeEquipGem                      PowerRushRankSubType = 4  // 战力冲榜子类 - 装备宝石
	PowerRushRankSubTypeCharge                        PowerRushRankSubType = 5  // 战力冲榜子类 - 充值
	PowerRushRankSubTypeFourSymbolsDragon             PowerRushRankSubType = 6  // 战力冲榜子类 - 四象青龙
	PowerRushRankSubTypeFourSymbolsTiger              PowerRushRankSubType = 7  // 战力冲榜子类 - 四象白虎
	PowerRushRankSubTypeFourSymbolsRoseFinch          PowerRushRankSubType = 8  // 战力冲榜子类 - 四象朱雀
	PowerRushRankSubTypeFourSymbolsTortoise           PowerRushRankSubType = 9  // 战力冲榜子类 - 四象玄武
	PowerRushRankSubTypeFaBao                         PowerRushRankSubType = 10 // 战力冲榜子类 - 法宝
	PowerRushRankSubTypeRider                         PowerRushRankSubType = 11 // 战力冲榜子类 - 坐骑
	PowerRushRankSubTypeFairy                         PowerRushRankSubType = 12 // 战力冲榜子类 - 仙灵
	PowerRushRankSubTypeWing                          PowerRushRankSubType = 13 // 战力冲榜子类 - 仙翼
	PowerRushRankSubTypeDailyUpPower                  PowerRushRankSubType = 14 // 战力冲榜子类 - 单日提升总战力
	PowerRushRankSubTypeRiderFashion                  PowerRushRankSubType = 15 // 战力冲榜子类 - 坐骑外观
	PowerRushRankSubTypeGodRider                      PowerRushRankSubType = 16 // 战力冲榜子类 - 坐骑神兽
	PowerRushRankSubTypeRiderInternalFashion          PowerRushRankSubType = 17 // 战力冲榜子类 - 坐骑内部外观
	PowerRushRankSubTypeQiHunCultivationByRider       PowerRushRankSubType = 18 // 战力冲榜子类 - 器魂修炼-飞剑
	PowerRushRankSubTypeSaFairyWingFashion            PowerRushRankSubType = 19 // 战力冲榜子类 - 仙翼时装
	PowerRushRankSubTypeSaGodWing                     PowerRushRankSubType = 20 // 战力冲榜子类 - 仙翼神翼
	PowerRushRankSubTypeSaQiHunCultivationByFairyWing PowerRushRankSubType = 21 // 战力冲榜子类 - 器魂修炼-仙翼
)

// 映射关系
var powerRushRankTypeRefSubType = map[PowerRushRankType][]PowerRushRankSubType{
	PowerRushRankTypeLevel:                {PowerRushRankSubTypeLevel},
	PowerRushRankTypeEquip:                {PowerRushRankSubTypeEquip, PowerRushRankSubTypeEquipStrong, PowerRushRankSubTypeEquipGem},
	PowerRushRankTypeCharge:               {PowerRushRankSubTypeCharge},
	PowerRushRankTypeFourSymbolsDragon:    {PowerRushRankSubTypeFourSymbolsDragon},
	PowerRushRankTypeFourSymbolsTiger:     {PowerRushRankSubTypeFourSymbolsTiger},
	PowerRushRankTypeFourSymbolsRoseFinch: {PowerRushRankSubTypeFourSymbolsRoseFinch},
	PowerRushRankTypeFaBao:                {PowerRushRankSubTypeFaBao},
	PowerRushRankTypeRider:                {PowerRushRankSubTypeRider, PowerRushRankSubTypeRiderFashion, PowerRushRankSubTypeGodRider, PowerRushRankSubTypeRiderInternalFashion, PowerRushRankSubTypeQiHunCultivationByRider},
	PowerRushRankTypeFairy:                {PowerRushRankSubTypeFairy},
	PowerRushRankTypeWing:                 {PowerRushRankSubTypeWing, PowerRushRankSubTypeSaFairyWingFashion, PowerRushRankSubTypeSaGodWing, PowerRushRankSubTypeSaQiHunCultivationByFairyWing},
	PowerRushRankTypeDailyUpPower:         {PowerRushRankSubTypeDailyUpPower},
}

func GetPowerRushRankSubTypes(t PowerRushRankType) []PowerRushRankSubType {
	return powerRushRankTypeRefSubType[t]
}
