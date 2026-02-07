package engine

import (
	"encoding/json"
	"jjyz/base/common"
	"jjyz/base/custom_id/bagdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/mailargs"
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logworker"
	"time"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

type BroadcastRewardsHandler func(actorId uint64, actorName string, id uint32, count int64, serverId uint32, param common.EngineGiveRewardParam) ([]interface{}, bool)

var broadcastHandler = make(map[uint32]BroadcastRewardsHandler)

var (
	CommonYYDrawBroadcast = func(actorId uint64, actorName string, id uint32, count int64, serverId uint32, param common.EngineGiveRewardParam) ([]interface{}, bool) {
		if len(param.BroadcastExt) < 1 {
			return nil, false
		}
		return []interface{}{actorId, actorName, param.BroadcastExt[0], id}, true //玩家id，玩家名，活动id，道具
	}
)

func RegRewardsBroadcastHandler(tipsId uint32, fn BroadcastRewardsHandler) {
	if nil == fn {
		return
	}
	broadcastHandler[tipsId] = fn
}

func checkBroadcast(actor iface.IPlayer, broadcastMap map[uint32]uint32, itemId uint32, count int64, param common.EngineGiveRewardParam) {
	if broadcastMap == nil {
		return
	}

	broId, ok := broadcastMap[itemId]
	if !ok {
		return
	}

	if fn, ok := broadcastHandler[broId]; ok {
		if params, canBro := fn(actor.GetId(), actor.GetName(), itemId, count, actor.GetServerId(), param); canBro {
			BroadcastTipMsgById(broId, params...)
		}
		return
	}

	args := []interface{}{
		actor.GetId(),
		actor.GetName(),
		itemId,
		count,
	}
	args = append(args, param.BroadcastExt...)

	BroadcastTipMsgById(broId, args...)
}

func CheckRewards(player iface.IPlayer, rewards []*jsondata.StdReward) bool {
	if nil == player {
		logger.LogStack("no player")
		return false
	}

	rewards = FilterRewardByPlayer(player, rewards)
	if len(rewards) <= 0 {
		logger.LogStack("rewards empty")
		return true
	}

	if !checkBagSpace(player, rewards) {
		return false
	}

	return true
}

type bagSt struct {
	checkSpaceFn bagCheckRewardSpaceFn
}

type bagCheckRewardSpaceFn func(player iface.IPlayer, rewards jsondata.StdRewardVec) bool

func makeGenericCheckSpaceFn(bagType uint32, checkFn func(uint32) bool) func(player iface.IPlayer, rewards jsondata.StdRewardVec) bool {
	return func(player iface.IPlayer, rewards jsondata.StdRewardVec) bool {
		bagSys := player.GetBagSysByBagType(bagType)
		if nil == bagSys {
			return false
		}

		availableCount := bagSys.AvailableCount()
		if availableCount == 0 {
			return false
		}

		count := uint32(0)
		for _, reward := range rewards {
			itemConf := jsondata.GetItemConfig(reward.Id)
			if itemConf == nil {
				continue
			}
			if !checkFn(itemConf.Type) {
				continue
			}
			count++
		}

		if count > 0 && count > availableCount {
			return false
		}

		return true
	}
}

var bags = func() map[uint32]bagSt {
	m := map[uint32]bagSt{
		bagdef.BagType: {
			checkSpaceFn: roleBagCheckSpaceFn,
		},
		bagdef.BagFairyType: {
			checkSpaceFn: fairyBagCheckSpaceFn,
		},
	}

	for _, rule := range gshare.SpBagRules {
		if _, exist := m[rule.Bag]; exist { // 跳过特殊处理的背包
			continue
		}
		m[rule.Bag] = bagSt{
			checkSpaceFn: makeGenericCheckSpaceFn(rule.Bag, rule.Check),
		}
	}
	return m
}()

// FilterRewardByPlayer 根据人物的性别，职业筛选出符合条件的奖励
func FilterRewardByPlayer(player iface.IPlayer, rewards jsondata.StdRewardVec) jsondata.StdRewardVec {
	var ret jsondata.StdRewardVec
	level := player.GetLevel()
	for _, reward := range rewards {
		if reward.Sex > 0 && reward.Sex != player.GetSex() {
			continue
		}
		if reward.Job > 0 && reward.Job != player.GetJob() {
			continue
		}
		if reward.MinOpenDay > 0 && reward.MinOpenDay > gshare.GetOpenServerDay() {
			continue
		}
		if reward.MaxOpenDay > 0 && reward.MaxOpenDay < gshare.GetOpenServerDay() {
			continue
		}
		if reward.WeekCycle > 0 && reward.InCycleIdx > 0 {
			_, week := time.Now().ISOWeek()
			w := uint32(week) % reward.WeekCycle
			if w == 0 {
				w = reward.WeekCycle
			}
			if w != reward.InCycleIdx {
				continue
			}
		}

		if reward.MinOpenWeek > 0 && reward.MinOpenWeek > gshare.GetOpenServerWeeks() {
			continue
		}
		if reward.MaxOpenWeek > 0 && reward.MaxOpenWeek < gshare.GetOpenServerWeeks() {
			continue
		}
		if reward.OpenWeekCycle > 0 && reward.InOpenWeekCycleIdx > 0 {
			week := gshare.GetOpenServerWeeks()
			w := week % reward.OpenWeekCycle
			if w == 0 {
				w = reward.OpenWeekCycle
			}
			if w != reward.InOpenWeekCycleIdx {
				continue
			}
		}

		if len(reward.LvRange) == 2 {
			if level < reward.LvRange[0] || level > reward.LvRange[1] {
				continue
			}
		}
		ret = append(ret, reward)
	}
	return ret
}

func fairyBagCheckSpaceFn(player iface.IPlayer, rewards jsondata.StdRewardVec) bool {
	fairyNum := uint32(0)
	for _, reward := range rewards {
		if jData := jsondata.GetItemConfig(reward.Id); nil != jData {
			if jData.Type == itemdef.ItemFairy {
				fairyNum += uint32(reward.Count)
			}
		}
	}

	if fairyNum > 0 && fairyNum > player.GetFairyBagAvailableCount() {
		return false
	}

	return true
}

func roleBagCheckSpaceFn(player iface.IPlayer, rewards jsondata.StdRewardVec) bool {
	if nil == player {
		return false
	}

	bindMap := make(map[uint32]int64)
	unBindMap := make(map[uint32]int64)
	for _, reward := range rewards {
		if jData := jsondata.GetItemConfig(reward.Id); nil != jData {
			if jData.Type == itemdef.ItemFairy {
			} else if itemdef.IsGodBeastBagItem(jData.Type) {
			} else if jData.Type != itemdef.ItemTypeMoney {
				if reward.Bind {
					bindMap[reward.Id] += reward.Count
				} else {
					unBindMap[reward.Id] += reward.Count
				}
			}
		}
	}

	available := player.GetBagAvailableCount()
	need := uint32(0)
	for itemId, cnt := range bindMap {
		if jData := jsondata.GetItemConfig(itemId); nil != jData {
			st := &itemdef.ItemParamSt{}
			st.ItemId = itemId
			st.Count = cnt
			st.Bind = true
			need += uint32(player.GetAddItemNeedGridCount(st, true))
		}

		if need > available {
			return false
		}
	}
	for itemId, cnt := range unBindMap {
		if jData := jsondata.GetItemConfig(itemId); nil != jData {
			st := &itemdef.ItemParamSt{}
			st.ItemId = itemId
			st.Count = cnt
			st.Bind = false
			need += uint32(player.GetAddItemNeedGridCount(st, true))
		}
		if need > available {
			return false
		}
	}
	return true
}

func buildBroadCastMap(rewards jsondata.StdRewardVec) map[uint32]uint32 {
	ret := make(map[uint32]uint32)
	for _, reward := range rewards {
		if reward.Broadcast > 0 {
			if _, ok := ret[reward.Id]; ok {
				continue
			}

			ret[reward.Id] = reward.Broadcast
		}
	}
	return ret
}

func mergeRewardsToMap(rewards jsondata.StdRewardVec) map[uint32]map[bool]int64 {
	ret := make(map[uint32]map[bool]int64)
	for _, reward := range rewards {
		if jData := jsondata.GetItemConfig(reward.Id); nil != jData {
			if _, ok := ret[reward.Id]; !ok {
				ret[reward.Id] = make(map[bool]int64)
			}
			ret[reward.Id][reward.Bind] += reward.Count
		}
	}
	return ret
}

func sendRewardsToBag(player iface.IPlayer, bagRewards jsondata.StdRewardVec, param common.EngineGiveRewardParam) {
	broadCastMap := buildBroadCastMap(bagRewards)
	rewardsMap := mergeRewardsToMap(bagRewards)

	for itemId, bindAndUnBindNum := range rewardsMap {
		totalCount := int64(0)
		for bind, count := range bindAndUnBindNum {
			player.AddItem(&itemdef.ItemParamSt{
				ItemId: itemId,
				Count:  count,
				Bind:   bind,
				LogId:  param.LogId,
				NoTips: param.NoTips,
			})
			totalCount += count
		}
		checkBroadcast(player, broadCastMap, itemId, totalCount, param)
	}
}

func SendRewardsByEmail(player iface.IPlayer, mailId uint16, mailArg interface{}, bagRewards jsondata.StdRewardVec, param common.EngineGiveRewardParam) {
	broadCastMap := buildBroadCastMap(bagRewards)
	rewardsMap := mergeRewardsToMap(bagRewards)

	items := make([]*jsondata.StdReward, 0, gshare.MaxMailFileCount)
	files := 0

	mails := make([]*mailargs.SendMailSt, 0)
	for itemId, bindAndUnBindNum := range rewardsMap {
		itemConf := jsondata.GetItemConfig(itemId)
		if nil == itemConf {
			continue
		}
		for bind, count := range bindAndUnBindNum {
			dup := utils.MaxInt64(1, itemConf.Dup)
			for count > 0 {
				curCount := int64(0)
				if count > dup {
					curCount = count - dup
				} else {
					curCount = count
				}
				items = append(items, &jsondata.StdReward{Id: itemId, Count: curCount, Bind: bind})
				files++
				if files > 0 && files == gshare.MaxMailFileCount {
					files = 0
					mails = append(mails, &mailargs.SendMailSt{ConfId: mailId, Rewards: items, Content: mailArg})
					items = make([]*jsondata.StdReward, 0, gshare.MaxMailFileCount)
				}
				count -= curCount
			}
		}
	}

	// 防止上面未达到maxMailFileCount的奖励被漏掉
	if files > 0 {
		mails = append(mails, &mailargs.SendMailSt{ConfId: mailId, Rewards: items})
	}

	// 大于20封邮件的，会被截断
	if len(mails) > 20 {
		excedMails := mails[20:]
		mails = mails[:20]
		logger.LogError("player(%d) mail reward exception which length more than 20!", player.GetId())
		logArg, _ := json.Marshal(excedMails)
		logworker.LogPlayerBehavior(player, pb3.LogId_LogMailRewardException, &pb3.LogPlayerCounter{
			NumArgs: uint64(param.LogId),
			StrArgs: string(logArg),
		})
	}

	if len(mails) > 0 {
		for _, sms := range mails {
			player.SendMail(sms)
			for _, reward := range sms.Rewards {
				checkBroadcast(player, broadCastMap, reward.Id, reward.Count, param)
			}
		}
	}
}

func GiveRewards(player iface.IPlayer, rewards jsondata.StdRewardVec, param common.EngineGiveRewardParam) bool {
	if nil == player {
		logger.LogStack("no player")
		return false
	}

	rewards = FilterRewardByPlayer(player, rewards)
	if len(rewards) <= 0 {
		logger.LogStack("rewards empty")
		return true
	}

	bagSpaceEnough := checkBagSpace(player, rewards)

	if bagSpaceEnough {
		sendRewardsToBag(player, rewards, param)
		return true
	} else {
		SendRewardsByEmail(player, common.Mail_BagInsufficient, nil, rewards, param)
		player.SendTipMsg(tipmsgid.BagIsFullAwardSendByMail)
		return true
	}
}

func CheckBagSpaceByRewards(player iface.IPlayer, rewards jsondata.StdRewardVec) bool {
	if nil == player {
		logger.LogStack("no player")
		return false
	}

	rewards = FilterRewardByPlayer(player, rewards)
	if len(rewards) <= 0 {
		logger.LogStack("rewards empty")
		return true
	}

	if !checkBagSpace(player, rewards) {
		return false
	}

	return true
}

func checkBagSpace(player iface.IPlayer, rewards jsondata.StdRewardVec) bool {
	// 拆分rewards
	bagRewards := classifyRewards(rewards)

	for bagType, tmpRewards := range bagRewards {
		enough := bags[bagType].checkSpaceFn(player, tmpRewards)
		if !enough {
			return false
		}
	}

	return true
}

func classifyRewards(rewards jsondata.StdRewardVec) map[uint32]jsondata.StdRewardVec {
	bagRewards := make(map[uint32]jsondata.StdRewardVec)
	for _, reward := range rewards {
		itemConf := jsondata.GetItemConfig(reward.Id)
		if itemConf == nil {
			continue
		}

		classified := false
		for _, rule := range gshare.SpBagRules {
			if rule.Check(itemConf.Type) {
				if bagRewards[rule.Bag] == nil {
					bagRewards[rule.Bag] = make(jsondata.StdRewardVec, 0)
				}
				bagRewards[rule.Bag] = append(bagRewards[rule.Bag], reward)
				classified = true
				break
			}
		}

		// 未匹配任何特定分类的默认处理
		if !classified {
			if bagRewards[bagdef.BagType] == nil {
				bagRewards[bagdef.BagType] = make(jsondata.StdRewardVec, 0)
			}
			bagRewards[bagdef.BagType] = append(bagRewards[bagdef.BagType], reward)
		}
	}
	return bagRewards
}

func StdRewardToBroadcast(player iface.IPlayer, rewards jsondata.StdRewardVec) string {
	fRewards := FilterRewardByPlayer(player, rewards)
	return jsondata.StdRewardToBroadcastStr(fRewards)
}

var StdRewardToBroadcastV2 func(actorId uint64, rewards jsondata.StdRewardVec) string

func FilterRewardsByCond(rewards jsondata.StdRewardVec, cond *jsondata.FilerRewardsCond) jsondata.StdRewardVec {
	filterRewards := jsondata.FilterRewardByOption(rewards,
		jsondata.WithFilterRewardOptionByJob(cond.Job),
		jsondata.WithFilterRewardOptionBySex(cond.Sex),
		jsondata.WithFilterRewardOptionByLvRange(cond.Level),
		jsondata.WithFilterRewardOptionByOpenDayRange(gshare.GetOpenServerDay()),
		jsondata.WithFilterRewardOptionByOpenWeekRange(gshare.GetOpenServerWeeks()),
		jsondata.WithFilterRewardOptionByWeekCycle(),
		jsondata.WithFilterRewardOptionByOpenWeekCycle(gshare.GetOpenServerWeeks()),
	)
	return filterRewards
}
