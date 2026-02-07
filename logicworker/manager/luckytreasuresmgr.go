/**
 * @Author: zjj
 * @Date: 2025/2/10
 * @Desc: 招财进宝
**/

package manager

import (
	"fmt"
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"sync"
	"time"
)

type LuckyTreasuresMgr struct{}

var onceLuckyTreasures sync.Once
var singleLuckyTreasures *LuckyTreasuresMgr

func GetLuckyTreasures() *LuckyTreasuresMgr {
	if singleLuckyTreasures == nil {
		onceLuckyTreasures.Do(func() {
			singleLuckyTreasures = &LuckyTreasuresMgr{}
		})
	}
	return singleLuckyTreasures
}

func (m *LuckyTreasuresMgr) GetData() *pb3.LuckyTreasuresGlobalData {
	staticVar := gshare.GetStaticVar()
	if staticVar.LuckyTreasuresData == nil {
		staticVar.LuckyTreasuresData = &pb3.LuckyTreasuresGlobalData{}
	}
	if staticVar.LuckyTreasuresData.PlayerData == nil {
		staticVar.LuckyTreasuresData.PlayerData = make(map[uint64]*pb3.LuckyTreasuresData)
	}
	return staticVar.LuckyTreasuresData
}

func (m *LuckyTreasuresMgr) GetPlayerData(playerId uint64) *pb3.LuckyTreasuresData {
	data := m.GetData()
	treasuresData := data.PlayerData[playerId]
	if treasuresData == nil {
		data.PlayerData[playerId] = &pb3.LuckyTreasuresData{}
		treasuresData = data.PlayerData[playerId]
	}
	return treasuresData
}

func (m *LuckyTreasuresMgr) InActTime() bool {
	nowSec := time_util.NowSec()
	data := m.GetData()
	if data.IsSettle {
		return false
	}
	return data.StartAt <= nowSec && nowSec <= data.EndAt
}

func (m *LuckyTreasuresMgr) GetTimesConf() (*jsondata.LuckyTreasuresTimesConf, error) {
	confIdx := m.GetData().TimesConfIdx
	conf := jsondata.GetLuckyTreasuresTimesConf(confIdx)
	if conf == nil {
		return nil, neterror.ConfNotFoundError("%d conf not found", confIdx)
	}
	return conf, nil
}

func (m *LuckyTreasuresMgr) Input(player iface.IPlayer) error {
	if !m.InActTime() {
		return neterror.ParamsInvalidError("not in act time")
	}

	data := m.GetPlayerData(player.GetId())
	conf, err := m.GetTimesConf()
	if err != nil {
		logger.LogError("err:%v", err)
		return err
	}

	// 校验今日奖励是否领取
	format := time.Now().Format("20060102")
	dailyInputDay := utils.AtoUint32(format)
	if data.LastRecDay != 0 && data.LastRecDay != dailyInputDay {
		return neterror.ParamsInvalidError("daily awards not rec")
	}

	if data.Money >= conf.MaxMoney {
		return neterror.ParamsInvalidError("input money is full")
	}

	if !player.ConsumeByConf(jsondata.ConsumeVec{{
		Type:  custom_id.ConsumeTypeMoney,
		Id:    conf.MoneyType,
		Count: conf.SingleMoney,
	}}, false, common.ConsumeParams{LogId: pb3.LogId_LogLuckyTreasuresInput}) {
		return neterror.ConsumeFailedError("consume failed")
	}

	if data.FirstJoinDay == 0 {
		data.FirstJoinDay = dailyInputDay
	}

	data.Money += conf.SingleMoney
	player.SendProto3(8, 61, &pb3.S2C_8_61{
		Money:        data.Money,
		FirstJoinDay: data.FirstJoinDay,
	})

	logworker.LogPlayerBehavior(player, pb3.LogId_LogLuckyTreasuresInput, &pb3.LogPlayerCounter{
		NumArgs: uint64(conf.SingleMoney),
		StrArgs: fmt.Sprintf("%d", data.Money),
	})
	return nil
}

func (m *LuckyTreasuresMgr) RecDailyAwards(player iface.IPlayer) error {
	getData := m.GetData()
	if getData.IsSettle {
		return neterror.ParamsInvalidError("LuckyTreasuresMgr settle")
	}

	data := m.GetPlayerData(player.GetId())
	conf, err := m.GetTimesConf()
	if err != nil {
		logger.LogError("err:%v", err)
		return err
	}

	format := time.Now().Format("20060102")
	dailyInputDay := utils.AtoUint32(format)

	// 没投入过
	if data.FirstJoinDay == 0 {
		return neterror.ParamsInvalidError("not input money")
	}

	// 今天投入 今天不能领
	if data.FirstJoinDay != 0 && data.FirstJoinDay == dailyInputDay {
		return neterror.ParamsInvalidError("%d %d is same not can rec")
	}

	// 已经领过
	if data.LastRecDay == dailyInputDay {
		return neterror.ParamsInvalidError("%d not can rec", dailyInputDay)
	}

	// 最后一次领取校验
	t := time.Unix(int64(getData.EndAt), 0)
	format = t.AddDate(0, 0, 1).Format("20060102")
	lastCanRecDay := utils.AtoUint32(format)
	if data.LastRecDay >= lastCanRecDay {
		return neterror.ParamsInvalidError("EndAt:%d,dailyInputDay:%d,lastCanRecDay:%d", getData.EndAt, data.LastRecDay, lastCanRecDay)
	}

	if data.LastRecDay == 0 {
		parse, _ := time.Parse("20060102", fmt.Sprintf("%d", data.FirstJoinDay))
		data.LastRecDay = utils.AtoUint32(parse.AddDate(0, 0, 1).Format("20060102"))
	} else {
		parse, _ := time.Parse("20060102", fmt.Sprintf("%d", data.LastRecDay))
		data.LastRecDay = utils.AtoUint32(parse.AddDate(0, 0, 1).Format("20060102"))
	}

	var awards = jsondata.StdRewardVec{{Id: conf.MoneyItemId, Count: int64(data.Money * conf.Ratio / 10000)}}

	// 领取最后一天的奖励 需要把本金也返还
	if lastCanRecDay == data.LastRecDay {
		awards = append(awards, &jsondata.StdReward{Id: conf.MoneyItemId, Count: int64(data.Money)})
		awards = jsondata.MergeStdReward(awards)
	}
	engine.GiveRewards(player, awards, common.EngineGiveRewardParam{LogId: pb3.LogId_LogLuckyTreasuresOutput})
	player.SendProto3(8, 62, &pb3.S2C_8_62{
		LastRecDay: data.LastRecDay,
	})
	logworker.LogPlayerBehavior(player, pb3.LogId_LogLuckyTreasuresOutput, &pb3.LogPlayerCounter{
		NumArgs: uint64(data.LastRecDay),
		StrArgs: fmt.Sprintf("%d", lastCanRecDay),
	})
	return nil
}
