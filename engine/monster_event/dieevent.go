/**
 * @Author: HeXinLi
 * @Desc: 监听战斗服怪物死亡调用
 * @Date: 2021/9/11 14:57
 */

package monster_event

import (
	"github.com/gzjjyz/logger"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
)

type monDieCbFunc func(layer, sceneId, monId uint32, killerId uint64, args []uint32)

var (
	monDieCallBackMap = make(map[uint32][]monDieCbFunc)
)

func OnFightSrvMonsterDie(buf []byte) {
	msg := &pb3.MonsterDie_Common{}
	if err := pb3.Unmarshal(buf, msg); nil != err {
		logger.LogError("战斗服怪物死亡事件 %v", err)
		return
	}

	if fns, exist := monDieCallBackMap[msg.FbId]; exist {
		for _, fn := range fns {
			fn(msg.Layer, msg.SceneId, msg.MonId, msg.KillerId, msg.ExtraData)
		}
	}
}

func RegisterMonDieEvent(fbId uint32, fn monDieCbFunc) {
	_, exist := monDieCallBackMap[fbId]
	if !exist {
		monDieCallBackMap[fbId] = make([]monDieCbFunc, 0)
	}

	monDieCallBackMap[fbId] = append(monDieCallBackMap[fbId], fn)

}

func init() {
	event.RegSysEventH(custom_id.SeReloadJson, func(args ...interface{}) {
		monDieCallBackMap = make(map[uint32][]monDieCbFunc)
	})
	engine.RegisterSysCall(sysfuncid.FGOnMonsterDie, OnFightSrvMonsterDie)
}
