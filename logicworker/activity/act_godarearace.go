/**
 * @Author: lzp
 * @Date: 2024/5/23
 * @Desc:
**/

package activity

import (
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
)

func init() {
	engine.RegisterActorCallFunc(playerfuncid.GodAreaKillActor, func(player iface.IPlayer, buf []byte) {
		player.TriggerEvent(custom_id.AeFashionTalentEvent, &custom_id.FashionTalentEvent{
			Cond:  custom_id.FashionSetGodAreaEvent,
			Count: 1,
		})
		player.TriggerQuestEvent(custom_id.QttGodAreaRaceActKillActorOrMonster, 0, 1)
	})
	engine.RegisterActorCallFunc(playerfuncid.GodAreaKillMonster, func(player iface.IPlayer, buf []byte) {
		player.TriggerQuestEvent(custom_id.QttGodAreaRaceActKillActorOrMonster, 0, 1)
	})
	event.RegActorEvent(custom_id.AePassFbByHelper, func(player iface.IPlayer, args ...interface{}) {
		player.TriggerQuestEvent(custom_id.QttAchievementsPassFbByHelper, 0, 1)
	})
	event.RegActorEvent(custom_id.AeJoinGodAreaRaceAct, func(player iface.IPlayer, args ...interface{}) {
		player.TriggerQuestEvent(custom_id.QttJoinGodAreaRaceAct, 0, 1)
	})
}
