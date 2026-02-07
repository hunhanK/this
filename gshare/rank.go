package gshare

// 排行榜类型 任何不被rank_mgr.go 管理的榜都禁止在这里定义
const (
	RankTypePower            = 1  // 战力榜
	RankTypeLevel            = 2  // 等级榜
	RankTypeEquip            = 3  // 装备榜
	RankTypeGuild            = 4  // 仙盟榜
	RankTypeBoundary         = 5  // 境界榜
	RankTypePet              = 6  // 灵宠榜
	RankTypeGod              = 7  // 天神榜
	RankTypeMount            = 8  // 坐骑榜
	RankTypeWing             = 9  // 羽翼榜
	RankTypeFaBao            = 10 // 法宝榜
	RankTypeGodWeapon        = 11 // 神兵榜
	RankTypeGodBaby          = 12 // 仙娃榜
	RankTypeStarRiver        = 13 // 星河图关卡榜
	RankTypeBattleArena      = 14 // 竞技场
	RankTypeFairyMasterTrain = 15 // 仙尊试炼
	RankTypeFlyUpRoad        = 16 // 登仙榜(飞升之路)
	RankTypeBeastRampant     = 17 // 魂兽肆虐
	RankTypeFlyCamp          = 18 // 登仙榜
	RankAncientTower         = 19 // 大荒古塔
	RankSectHuntingDamage    = 20 // 仙宗狩猎伤害榜
	RankTypeMax              = iota + 1
)

const RankLikeDailyNumMax = 3 // 排行榜点赞每日上限
