package actorsystem

/*
	desc:九州绘卷-个人履历
	author: twl
	time:	2023/04/07
*/

import (
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/tipmsgid"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"

	"github.com/gzjjyz/srvlib/utils"
)

// RoleRecSys 九州绘卷-个人履历
type RoleRecSys struct {
	Base
}

func (sys *RoleRecSys) OnInit() {
	if binary := sys.GetBinaryData(); nil != binary {
		if nil == binary.RoleRec {
			binary.RoleRec = &pb3.RoleRecData{}
		}
	}
	if playerAwardInfo := sys.GetBinaryData().GetRoleHandBookAwardInfo(); nil == playerAwardInfo {
		sys.GetBinaryData().RoleHandBookAwardInfo = make(map[uint32]*pb3.HandBookPlayerAwardInfo)
	}
}

func (sys *RoleRecSys) OnReconnect() {}

// 获取玩家的生涯记录
func (sys *RoleRecSys) c2sGetRoleRec(_ *base.Message) {
	var scrollLs []*pb3.Scroll
	player := sys.owner
	for id := custom_id.RoleRec1001; id < custom_id.RoleRecMax; id++ {
		if scroll := event.GetRecScroll(player, id); nil != scroll {
			scrollLs = append(scrollLs, scroll)
		}
	}
	sys.SendProto3(2, 50, &pb3.S2C_2_50{ScrollLs: scrollLs})
}

//func (sys *RoleRecSys) getHolyJade(id uint32, scrollLs []*pb3.Scroll) []*pb3.Scroll {
//	return scrollLs
//}

// 论道 todo 需要接入'在竞技场中获得x次胜利'
//func (sys *RoleRecSys) getArena(id uint32, scrollLs []*pb3.Scroll) []*pb3.Scroll {
//	return scrollLs
//}

// 公会(道盟) todo 需要接入'累计获得公会贡献点'
//func (sys *RoleRecSys) getGuildThing(id uint32, scrollLs []*pb3.Scroll) []*pb3.Scroll {
//	friendSys := sys.owner.GetSysObj(gshare.SiFriend).(*FriendSys)
//	if nil == friendSys {
//		return scrollLs
//	}
//	var friendAcceptName string
//	friendAcceptName = "张三" // todo
//	firstSendGiftFriendName := sys.owner.GetBinaryData().GetRoleRec().FirstSendGiftFriendName
//	strArgs := []string{firstSendGiftFriendName, friendAcceptName}
//	scroll := &pb3.Scroll{
//		RecId: id,
//		Args:  strArgs,
//	}
//	scrollLs = append(scrollLs, scroll)
//	return scrollLs
//}

// 灵宠 todo 灵宠系统完成后 去接入对应数据
//func (sys *RoleRecSys) getPet(id uint32, scrollLs []*pb3.Scroll) []*pb3.Scroll {
//	roleRec := sys.owner.GetBinaryData().GetRoleRec()
//	MainTaskName := roleRec.MainTaskName
//	PetName := roleRec.FirstPetName
//	strArgs := []string{MainTaskName, PetName}
//	scroll := &pb3.Scroll{
//		RecId: id,
//		Args:  strArgs,
//	}
//	scrollLs = append(scrollLs, scroll)
//	return scrollLs
//}

func setAwardFlag(playerId uint64, isAward bool, playerIds []uint64) uint32 {
	if isAward {
		return custom_id.GlobalFirstAwardAwarded
	} else if utils.SliceContainsUint64(playerIds, playerId) { // 判断是否有资格领取
		return custom_id.GlobalFirstAwardNone
	} else {
		return custom_id.GlobalFirstAwardBan
	}
}

// 获取全服记录信息
func (sys *RoleRecSys) c2sGetHandBookMsg(_ *base.Message) {
	allHandBookConfLs := jsondata.GetAllHandBookConfig()
	var globalScrollLs []*pb3.GlobalScroll
	handBook := gshare.GetStaticVar().GetGlobalHandBook()
	playerAwardInfo := sys.owner.GetBinaryData().GetRoleHandBookAwardInfo()
	for cfgId, _ := range allHandBookConfLs {
		if handBookInfo := handBook[cfgId]; handBookInfo != nil {
			targetCount := uint32(len(handBookInfo.PlayerIds))
			info := playerAwardInfo[cfgId]
			if nil == info {
				info = &pb3.HandBookPlayerAwardInfo{}
			}
			awardFlag := setAwardFlag(sys.owner.GetId(), info.FirstAward, handBookInfo.PlayerIds)
			globalScroll := &pb3.GlobalScroll{
				TargetId:      cfgId,
				TargetCount:   targetCount,
				IsAwardMemory: info.NormalAward,
				IsAwardLimit:  awardFlag,
			}
			globalScrollLs = append(globalScrollLs, globalScroll)
		}
	}
	rsp := &pb3.S2C_2_51{GlobalScrollLs: globalScrollLs}
	sys.SendProto3(2, 51, rsp)
}

// 领取奖励
func (sys *RoleRecSys) c2sGetHandBookAward(msg *base.Message) {
	var req pb3.C2S_2_52
	if err := pb3.Unmarshal(msg.Data, &req); nil != err {
		return
	}
	actor := sys.owner
	handBookConf := jsondata.GetHandBookConfig(req.GetTargetId())
	if handBookConf == nil {
		actor.SendTipMsg(tipmsgid.TpRoleRecConfErr)
		return
	}
	handBook := gshare.GetStaticVar().GetGlobalHandBook()
	handBookInfo := handBook[req.TargetId]
	if handBook == nil {
		actor.SendTipMsg(tipmsgid.TpRoleRecCanNotAward)
		return // 无法领取
	}
	playerHandBookInfoLs := actor.GetBinaryData().GetRoleHandBookAwardInfo()
	playerHandBookInfo := playerHandBookInfoLs[req.TargetId]
	if nil == playerHandBookInfo {
		// 初始化
		playerHandBookInfo = &pb3.HandBookPlayerAwardInfo{}
	}
	vec := make([]*jsondata.StdReward, 0)
	targetCount := uint32(len(handBookInfo.PlayerIds))
	normalFlag := playerHandBookInfo.NormalAward
	firstFlag := playerHandBookInfo.FirstAward
	switch req.Type {
	case custom_id.HandBookAwardNormal: // 判断达标人数是否满了配置
		if handBookConf.Number == targetCount && !playerHandBookInfo.NormalAward {
			// 满了可以领取
			vec = handBookConf.CommonReward
			normalFlag = true

		} else {
			actor.SendTipMsg(tipmsgid.TpRoleRecCanNotAward)
			return
		}
	case custom_id.HandBookAwardLimit: // 抢先奖励(在里面就可以领取)
		if utils.SliceContainsUint64(handBookInfo.PlayerIds, actor.GetId()) { // 判断是否有资格领取
			vec = handBookConf.FirstReward
			firstFlag = true
		} else {
			actor.SendTipMsg(tipmsgid.TpRoleRecCanNotAward)
			return
		}
	}
	if flag := engine.CheckRewards(actor, vec); !flag {
		actor.SendTipMsg(tipmsgid.TpBagIsFull)
	}
	// 赋值
	playerHandBookInfo.NormalAward = normalFlag
	playerHandBookInfo.FirstAward = firstFlag
	playerHandBookInfoLs[req.TargetId] = playerHandBookInfo
	engine.GiveRewards(actor, vec, common.EngineGiveRewardParam{LogId: pb3.LogId_LogGlobalHandBook})

	awardFlag := setAwardFlag(sys.owner.GetId(), playerHandBookInfo.FirstAward, handBookInfo.PlayerIds)
	globalScroll := &pb3.GlobalScroll{
		TargetId:      req.GetTargetId(),
		TargetCount:   targetCount,
		IsAwardMemory: playerHandBookInfo.NormalAward,
		IsAwardLimit:  awardFlag,
	}
	sys.SendProto3(2, 52, &pb3.S2C_2_52{GlobalScrollLs: globalScroll})
}

// 玩家升级
func onPlayerLvUp(player iface.IPlayer, args ...interface{}) {
	handBook := gshare.GetStaticVar().GetGlobalHandBook()
	if handBook == nil {
		gshare.GetStaticVar().GlobalHandBook = make(map[uint32]*pb3.HandBookPlayerIds)
		handBook = gshare.GetStaticVar().GetGlobalHandBook()
	}
	allHandBookConf := jsondata.GetAllHandBookConfig()
	currLv := player.GetLevel()
	for cfgId, conf := range allHandBookConf {
		if conf.Achievement == custom_id.GlobalRecTypeLv && conf.Parameter == currLv {
			// 找到了 检查该玩家是不是前几名
			onPlayerIds := handBook[cfgId].GetPlayerIds()
			if nil == onPlayerIds { // 	第一位
				onPlayerIds = []uint64{player.GetId()}
				handBook[cfgId] = &pb3.HandBookPlayerIds{PlayerIds: onPlayerIds}
			} else if uint32(len(onPlayerIds)) < conf.Number { // 不是第一位检查是否能上
				onPlayerIds = append(onPlayerIds, player.GetId())
				handBook[cfgId].PlayerIds = onPlayerIds
			}
			// 上不了直接退出循环了
			break
		}
	}
}

// 玩家货币变化 - 累计到玩家的生涯记录中
func onPlayerMoneyChange(player iface.IPlayer, args ...interface{}) {
	mt := args[0].(uint32)
	changerNum := args[1].(int64)
	if changerNum < 0 {
		return
	}
	switch mt {
	case moneydef.YuanBao:
		roleRecInfo := player.GetBinaryData().GetRoleRec()
		roleRecInfo.AccCoinNum += uint64(changerNum)
	case moneydef.Diamonds:
		roleRecInfo := player.GetBinaryData().GetRoleRec()
		roleRecInfo.AccDiamond += uint64(changerNum)
	}
	if mt != moneydef.YuanBao && changerNum <= 0 {
		return
	}

}

// 玩家激活第一个称号
func onPlayerActFirstTitle(player iface.IPlayer, args ...interface{}) {
	if len(args) == 0 {
		return
	}

	roleRecInfo := player.GetBinaryData().GetRoleRec()
	if roleRecInfo.FirstTitleCfgId == 0 {
		titleCfgId := args[0].(uint32)
		roleRecInfo.FirstTitleCfgId = titleCfgId
	}
}

// 玩家添加好友回调
func onPlayerAddFriends(player iface.IPlayer, args ...interface{}) {
	if len(args) == 0 {
		return
	}
	roleRecInfo := player.GetBinaryData().GetRoleRec()
	if roleRecInfo.FirstFriendsName == "" { //  还没第一个
		name := args[0].(string)
		roleRecInfo.FirstFriendsName = name
	}
	roleRecInfo.HisMaxFriendsNum += 1 // 累计好友数量+1
}

func onPlayerFinishMainQuest(player iface.IPlayer, args ...interface{}) {
	player.GetBinaryData().RoleRec.MainQuestFinishTimes++
}

func onPlayerActiveNewFaBao(player iface.IPlayer, args ...interface{}) {
	itemId, ok := args[0].(uint32)
	if !ok {
		return
	}
	data := player.GetBinaryData().RoleRec
	if data.FirstActiveNewFabaoTime > 0 {
		return
	}
	data.FirstActiveNewFabaoTime = time_util.NowSec()
	data.FirstActiveNewFabaoItemId = itemId
}

func onPlayerBattleAreaWin(player iface.IPlayer, args ...interface{}) {
	player.GetBinaryData().RoleRec.BattleArenaWinTimes++
}

func onPlayerSendConfession(player iface.IPlayer, args ...interface{}) {
	if len(args) < 2 {
		return
	}
	name := args[1].(string)
	player.GetBinaryData().RoleRec.FirstConfessionPlayerName = name
}

// 入世
func onGetRoleCreate(actor iface.IPlayer, id uint32) *pb3.Scroll {
	career := actor.GetMainData().GetJob()
	name := actor.GetName()
	createTime := actor.GetCreateTime()
	strArgs := []string{utils.ToStr(career), name, utils.ToStr(createTime)}
	scroll := &pb3.Scroll{
		RecId: id,
		Args:  strArgs,
	}
	return scroll
}

// 修为
func onGetLoginBound(actor iface.IPlayer, id uint32) *pb3.Scroll {
	sys, ok := actor.GetSysObj(sysdef.SiNewJingJie).(*NewJingJieSys)
	if !ok || !sys.IsOpen() {
		return nil
	}
	level := sys.GetData().Level
	lvConf := sys.getLevelConf(level)
	if nil == lvConf {
		return nil
	}
	LoginDays := actor.GetMainData().GetLoginedDays()
	strArgs := []string{utils.ToStr(LoginDays), lvConf.LevelName}
	scroll := &pb3.Scroll{
		RecId: id,
		Args:  strArgs,
	}
	return scroll
}

// 历练
func onGetMainTask(player iface.IPlayer, id uint32) *pb3.Scroll {
	strArgs := []string{utils.ToStr(player.GetBinaryData().FinMainQuestId)}
	scroll := &pb3.Scroll{
		RecId: id,
		Args:  strArgs,
	}
	return scroll
}

// 新法宝
func onGetNewFaBao(player iface.IPlayer, id uint32) *pb3.Scroll {
	data := player.GetBinaryData().RoleRec
	sys, ok := player.GetSysObj(sysdef.SiNewFabao).(*FaBaoSys)
	if !ok || !sys.IsOpen() {
		return nil
	}
	fabaoNum := len(sys.state().FaBaoMap)
	if fabaoNum <= 0 {
		return nil
	}
	strArgs := []string{utils.ToStr(data.FirstActiveNewFabaoTime), utils.ToStr(data.FirstActiveNewFabaoItemId), utils.ToStr(fabaoNum)}
	scroll := &pb3.Scroll{
		RecId: id,
		Args:  strArgs,
	}
	return scroll
}

// 道友
func onGetFriends(player iface.IPlayer, id uint32) *pb3.Scroll {
	friendsNum := player.GetBinaryData().GetRoleRec().HisMaxFriendsNum
	friendsName := player.GetBinaryData().GetRoleRec().FirstFriendsName
	if friendsNum == 0 {
		return nil
	}
	strArgs := []string{friendsName, utils.ToStr(friendsNum)}
	scroll := &pb3.Scroll{
		RecId: id,
		Args:  strArgs,
	}
	return scroll
}

// 表白
func onGetConfession(player iface.IPlayer, id uint32) *pb3.Scroll {
	data := player.GetBinaryData().GetRoleRec()
	if data.FirstConfessionPlayerName == "" {
		return nil
	}
	strArgs := []string{data.FirstConfessionPlayerName}
	scroll := &pb3.Scroll{
		RecId: id,
		Args:  strArgs,
	}
	return scroll
}

func onGetGuildMsg(actor iface.IPlayer, id uint32) *pb3.Scroll {
	if sys, ok := actor.GetSysObj(sysdef.SiGuild).(*GuildSys); ok && sys.IsOpen() {
		if guild := sys.GetGuild(); nil != guild {
			memberInfo := guild.Members[actor.GetId()]
			strArgs := []string{utils.ToStr(memberInfo.JoinTime), guild.GetName(), utils.ToStr(memberInfo.Donate)}
			scroll := &pb3.Scroll{
				RecId: id,
				Args:  strArgs,
			}
			return scroll

		}
	}
	return nil
}

func onGetBattleArea(player iface.IPlayer, id uint32) *pb3.Scroll {
	data := player.GetBinaryData().GetRoleRec()
	if data.BattleArenaWinTimes <= 0 {
		return nil
	}
	strArgs := []string{utils.ToStr(data.BattleArenaWinTimes)}
	scroll := &pb3.Scroll{
		RecId: id,
		Args:  strArgs,
	}
	return scroll
}

// 财富
func onGetCoins(actor iface.IPlayer, id uint32) *pb3.Scroll {
	SumCoin := actor.GetBinaryData().GetRoleRec().GetAccCoinNum()
	strArgs := []string{utils.ToStr(SumCoin)}
	scroll := &pb3.Scroll{
		RecId: id,
		Args:  strArgs,
	}
	return scroll
}

func onGetHolyJades(actor iface.IPlayer, id uint32) *pb3.Scroll {
	SumJade := actor.GetBinaryData().GetRoleRec().GetAccDiamond()
	strArgs := []string{utils.ToStr(SumJade)}
	scroll := &pb3.Scroll{
		RecId: id,
		Args:  strArgs,
	}
	return scroll
}

// 名号
func onGetTitle(actor iface.IPlayer, id uint32) *pb3.Scroll {
	firstTitleId := actor.GetBinaryData().GetRoleRec().GetFirstTitleCfgId()
	titleLs := actor.GetBinaryData().GetTitleLs()
	if nil == titleLs {
		return nil
	}
	titleConf := jsondata.GetTitleConfig(firstTitleId)
	if nil == titleConf {
		return nil
	}
	firstTitleName := titleConf.TitleName
	strArgs := []string{firstTitleName, utils.ToStr(len(titleLs))}
	scroll := &pb3.Scroll{
		RecId: id,
		Args:  strArgs,
	}
	return scroll
}

func init() {
	RegisterSysClass(sysdef.SiNinePlaceRoleRec, func() iface.ISystem {
		return &RoleRecSys{}
	})
	net.RegisterSysProto(2, 50, sysdef.SiNinePlaceRoleRec, (*RoleRecSys).c2sGetRoleRec)
	net.RegisterSysProto(2, 51, sysdef.SiNinePlaceRoleRec, (*RoleRecSys).c2sGetHandBookMsg)
	net.RegisterSysProto(2, 52, sysdef.SiNinePlaceRoleRec, (*RoleRecSys).c2sGetHandBookAward)

	event.RegActorEvent(custom_id.AeLevelUp, onPlayerLvUp)
	event.RegActorEvent(custom_id.AeMoneyChange, onPlayerMoneyChange)
	event.RegActorEvent(custom_id.AeActFirstTitle, onPlayerActFirstTitle)
	event.RegActorEvent(custom_id.AeAddFriends, onPlayerAddFriends)
	event.RegActorEvent(custom_id.AeFinishMainQuest, onPlayerFinishMainQuest)
	event.RegActorEvent(custom_id.AeActiveNewFaBao, onPlayerActiveNewFaBao)
	event.RegActorEvent(custom_id.AeBattleAreaWin, onPlayerBattleAreaWin)
	event.RegActorEvent(custom_id.AeSendConfession, onPlayerSendConfession)

	event.RegRecFunc(custom_id.RoleRec1001, onGetRoleCreate)
	event.RegRecFunc(custom_id.RoleRec1002, onGetLoginBound)
	event.RegRecFunc(custom_id.RoleRec1003, onGetMainTask)
	event.RegRecFunc(custom_id.RoleRec1004, onGetNewFaBao)
	event.RegRecFunc(custom_id.RoleRec1005, onGetFriends)
	event.RegRecFunc(custom_id.RoleRec1006, onGetConfession)
	event.RegRecFunc(custom_id.RoleRec1007, onGetGuildMsg)
	event.RegRecFunc(custom_id.RoleRec1008, onGetBattleArea)
	event.RegRecFunc(custom_id.RoleRec1009, onGetCoins)
	event.RegRecFunc(custom_id.RoleRec1010, onGetHolyJades)
	event.RegRecFunc(custom_id.RoleRec1011, onGetTitle)
}
