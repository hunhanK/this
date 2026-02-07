/**
 * @Author: zjj
 * @Date: 2025/5/8
 * @Desc: 仙宗狩猎 全服数据
**/

package manager

import (
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/chatdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"time"
)

func GetSectHuntingGlobalData() *pb3.SectHuntingGlobalData {
	staticVar := gshare.GetStaticVar()
	if staticVar == nil {
		return nil
	}
	if staticVar.SectHuntingGlobalData == nil {
		staticVar.SectHuntingGlobalData = &pb3.SectHuntingGlobalData{}
	}
	return staticVar.SectHuntingGlobalData
}

func RotateTheNextSectHuntingBoss() {
	var nextBossIdx = -1
	config := jsondata.GetSectHuntingConfig()
	if config == nil {
		logger.LogError("not found config")
		return
	}
	data := GetSectHuntingGlobalData()
	bossId := data.Boss.BossId
	for idx, boss := range config.BossList {
		if boss.BossId != bossId {
			continue
		}
		nextBossIdx = idx + 1
		break
	}
	// 特殊情况
	if nextBossIdx <= 0 || nextBossIdx >= len(config.BossList) {
		data.NotNextBoss = true
		return
	}
	data.NotNextBoss = false
	data.Boss.BossId = config.BossList[nextBossIdx].BossId
	data.Boss.BuckleHp = 0
	engine.Broadcast(chatdef.CIWorld, 0, 8, 210, &pb3.S2C_8_210{GlobalData: data}, 0)
}

func handleSectHuntingServerInit(_ ...interface{}) {
	sectHuntingConfig := jsondata.GetSectHuntingConfig()
	if sectHuntingConfig == nil {
		logger.LogWarn("not found sectHuntingConfig, not init boss info")
		return
	}
	if len(sectHuntingConfig.BossList) == 0 {
		logger.LogWarn("not found sectHunting boss conf")
		return
	}
	data := GetSectHuntingGlobalData()
	if data.Boss == nil {
		data.Boss = &pb3.SectHuntingBoss{}
		bossInfo := sectHuntingConfig.BossList[0]
		data.Boss.BossId = bossInfo.BossId
	}
	// 通知战斗服创建副本
	if data.NotNextBoss {
		RotateTheNextSectHuntingBoss()
	}
	if data.NotNextBoss {
		logger.LogError("没有找到下一轮怪物")
		return
	}
	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FCreateSectHuntingMon, &pb3.G2FCreateSectHuntingMonReq{
		BossId:   data.Boss.BossId,
		BuckleHp: data.Boss.BuckleHp,
	})
	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
}

func checkSectHuntingRefreshRankAt() {
	sectHuntingConfig := jsondata.GetSectHuntingConfig()
	if sectHuntingConfig == nil {
		return
	}
	data := GetSectHuntingGlobalData()
	nowSec := time_util.NowSec()

	// 今天已经刷新过了
	if time_util.IsSameDay(data.RefreshRankAt, nowSec) {
		return
	}

	// 解析刷新时间
	pTime, err := time.Parse("15:04:05", sectHuntingConfig.RefreshRank)
	if err != nil {
		return
	}

	currentTime := time.Now()
	rTime := time.Date(currentTime.Year(), currentTime.Month(), currentTime.Day(), pTime.Hour(), pTime.Minute(), pTime.Second(), 0, currentTime.Location())
	if currentTime.Before(rTime) {
		return
	}
	data.RefreshRankAt = nowSec
	rRank := GRankMgrIns.GetRankByType(gshare.RankSectHuntingDamage)
	rRank.Clear()
	AllOfflineDataBaseDo(func(p *pb3.PlayerDataBase) bool {
		engine.SendPlayerMessage(p.Id, gshare.OfflineSectHuntingSysResetLiveTime, &pb3.CommonSt{})
		return true
	})
	engine.Broadcast(chatdef.CIWorld, 0, 8, 206, &pb3.S2C_8_206{
		RefreshRankAt: data.RefreshRankAt,
	}, 0)
}

func init() {
	event.RegSysEvent(custom_id.SeFightSrvConnSucc, handleSectHuntingServerInit)
	event.RegSysEvent(custom_id.SeMerge, handleSectHuntingServerInit)
}
