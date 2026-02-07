/**
 * @Author: zjj
 * @Date: 2024/5/9
 * @Desc: 玩法分数比拼
**/

package ranktype

type PlayScoreRankType uint8

const (
	PlayScoreRankTypeFairy                PlayScoreRankType = 1  // 玩法分数比拼 - 仙灵比拼
	PlayScoreRankTypeFlyUpLoadItem        PlayScoreRankType = 2  // 玩法分数比拼 - 飞升令比拼
	PlayScoreRankTypeNewFaBao             PlayScoreRankType = 3  // 玩法分数比拼 - 新法宝
	PlayScoreRankTypeSoulHalo             PlayScoreRankType = 4  // 玩法分数比拼 - 武魂比拼
	PlayScoreRankTypeConsumeDiamonds      PlayScoreRankType = 5  // 玩法分数比拼 - 消费非绑玉比拼
	PlayScoreRankTypeGlobalCollectCards   PlayScoreRankType = 6  // 玩法分数比拼 - 集卡榜
	PlayScoreRankTypeDemonSubduing        PlayScoreRankType = 7  // 玩法分数比拼 - 全民伏魔榜
	PlayScoreRankTypeFightValueSoulHalo   PlayScoreRankType = 8  // 玩法分数比拼 - 限时战力排行-魂环战力
	PlayScoreRankTypeFightValueCollection PlayScoreRankType = 9  // 玩法分数比拼 - 限时战力排行-藏品战力
	PlayScoreRankTypeFightValueDragonEqu  PlayScoreRankType = 10 // 玩法分数比拼 - 限时战力排行-龙装战力
	PlayScoreRankTypeFightValueGem        PlayScoreRankType = 11 // 玩法分数比拼 - 限时战力排行-宝石战力
	PlayScoreRankTypeFightValueFairySword PlayScoreRankType = 12 // 玩法分数比拼 - 限时战力排行-剑装战力
	PlayScoreRankTypeFightValueFaShen     PlayScoreRankType = 13 // 玩法分数比拼 - 限时战力排行-法身战力
	PlayScoreRankTypeFightValueFeather    PlayScoreRankType = 14 // 玩法分数比拼 - 限时战力排行-阴阳五行战力
	PlayScoreRankTypeFightValueDomain     PlayScoreRankType = 15 // 玩法分数比拼 - 限时战力排行-领域战力
)
