/**
 * @Author: zjj
 * @Date: 2025/1/21
 * @Desc:
**/

package jobchange

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base/custom_id"
	"jjyz/base/neterror"
	"jjyz/gameserver/iface"
)

const (
	BaseEquip           = 1 // 装备
	BasicSkill          = 2 // 基础技能
	SoulHalo            = 3 // 魂环
	Fashion             = 4 // 时装
	YYConfessionForLove = 5 // 爱的表白活动
)

type jobChangeFunc func(player iface.IPlayer, job uint32) bool

type Fn struct {
	Fn      jobChangeFunc
	CheckFn jobChangeFunc
}

var jobChangeFuncMap = make(map[uint32]*Fn)

func RegJobChangeFunc(cType uint32, fn *Fn) {
	if nil == fn {
		logger.LogDebug("转职注册类型 %d, 函数为空", cType)
		return
	}

	if nil != jobChangeFuncMap[cType] {
		logger.LogFatal("转职注册函数重复, type:%d", cType)
		return
	}

	jobChangeFuncMap[cType] = fn
}

func JobChange(player iface.IPlayer, job uint32) error {
	if job == player.GetJob() {
		return neterror.ParamsInvalidError("job is same %d", job)
	}

	if job < custom_id.JobIdMin || custom_id.JobIdMax < job {
		return neterror.ParamsInvalidError("job not found %d", job)
	}

	// 先检查
	for jType, line := range jobChangeFuncMap {
		if nil == line.CheckFn {
			continue
		}

		if !line.CheckFn(player, job) {
			return neterror.ParamsInvalidError("玩家:%s 转职检查不过, 检查类型:%d", player.GetName(), jType)
		}
	}

	// 数据转换
	for _, line := range jobChangeFuncMap {
		utils.ProtectRun(func() {
			line.Fn(player, job)
		})
	}

	return nil
}
