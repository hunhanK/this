/**
 * @Author: ChenJunJi
 * @Date: 2023/11/06
 * @Desc:
**/

package model

type ActorStatics struct {
	Id                   int32  `xorm:"pk 'id'"`
	ActorId              uint64 `xorm:"'actor_id'"`
	QuestId              uint32 `xorm:"'quest_id'"`
	SceneId              uint32 `xorm:"'scene_id'"`
	ChargeDiamond        uint32 `xorm:"'charge_diamond'"`
	LastChargeTime       uint32 `xorm:"'last_charge_time'"`
	UseChargeItem        uint32 `xorm:"'use_charge_item'"`
	device               string `xorm:"varchar(64) 'device'"`
	JyTimestamp          string `xorm:"'jy_timestamp'"`
	OnlineMinutes        uint32 `xorm:"'online_minutes'"`          // 在线时长(分钟)
	JingJieLv            uint32 `xorm:"'jing_jie_lv'"`             // 境界(层)
	AllEquipStrongLv     uint32 `xorm:"'all_equip_strong_lv'"`     // 装备强化总等级
	AllAccessoryStrongLv uint32 `xorm:"'all_accessory_strong_lv'"` // 饰品强化总等级
	DestinedFaBaoLv      uint32 `xorm:"'destined_fa_bao_lv'"`      // 本命法宝等级
	MeridiansTotalLv     uint32 `xorm:"'meridians_total_lv'"`      // 经脉等级
	MageBodyLv           uint32 `xorm:"'mage_body_lv'"`            // 炼体等级
	FairyWingLv          uint32 `xorm:"'fairy_wing_lv'"`           // 仙翼等级
	FairyMasterTrainLv   uint32 `xorm:"'fairy_master_train_lv'"`   // 仙尊试炼进度
	RiderLv              uint32 `xorm:"'rider_lv'"`                // 坐骑等级
	FourSymbolsDragon    uint32 `xorm:"'four_symbols_dragon'"`     // 青龙等级
	FourSymbolsTiger     uint32 `xorm:"'four_symbols_tiger'"`      // 白虎等级
	FourSymbolsRoseFinch uint32 `xorm:"'four_symbols_rose_finch'"` // 朱雀等级
	FourSymbolsTortoise  uint32 `xorm:"'four_symbols_tortoise'"`   // 玄武等级
	nirvanaLv            uint32 `xorm:"'nirvana_lv'"`              // 转生
	nirvanaSubLv         uint32 `xorm:"'nirvana_sub_lv'"`          // 转生子等级
}

func (m ActorStatics) TableName() string {
	return "actor_statics"
}
