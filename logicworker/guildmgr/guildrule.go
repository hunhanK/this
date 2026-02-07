/**
 * @Author: zjj
 * @Date: 2025/5/12
 * @Desc:
**/

package guildmgr

import (
	"google.golang.org/protobuf/proto"
	"jjyz/base/custom_id"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
)

var pfRule *pb3.BackStageGuildRule

func GuildRuleCheck(lv uint32, memberNum uint32) (full bool, existRule bool) {
	if pfRule == nil {
		return
	}
	if pfRule.InitNum == 0 {
		return
	}
	existRule = true
	var maxMemberNum = pfRule.InitNum
	if lv > 1 {
		maxMemberNum += pfRule.AddNum * (lv - 1)
	}
	full = memberNum >= maxMemberNum
	return
}

func GetPfRule() *pb3.BackStageGuildRule {
	return pfRule
}

func init() {
	event.RegSysEvent(custom_id.SeDBWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterGameMsgHandler(custom_id.GMsgReloadGuildRuleRet, func(param ...interface{}) {
			if len(param) < 1 {
				return
			}
			rule := param[0].(*pb3.BackStageGuildRule)
			if rule == nil {
				return
			}
			pfRule = proto.Clone(rule).(*pb3.BackStageGuildRule)
		})
	})
}
