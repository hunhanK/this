package manager

import (
	"jjyz/base/cmd"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/iface"
	"time"

	"modernc.org/mathutil"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

type SettingBitChangeTriggerFunc func(player iface.IPlayer, old bool, new bool)

var settingBitChangeTriggerFuncs map[int]SettingBitChangeTriggerFunc = make(map[int]SettingBitChangeTriggerFunc)

var RobotRankPlayerDataSetter = func(playerId uint64, info *pb3.RankInfo) bool {
	return false
}

var (
	playerMap = make(map[uint64]iface.IPlayer) //玩家列表
)

func RunOne() {
	AllOnlinePlayerDo(func(player iface.IPlayer) {
		player.RunOne()
	})
}

func Checker1sRunOne() {
	checkSectHuntingRefreshRankAt()
}

// SetPlayerRankInfo 根据玩家id设置排行榜数据
func SetPlayerRankInfo(playerId uint64, info *pb3.RankInfo, rankType uint32) {
	if nil == info {
		return
	}

	info.PlayerId = playerId

	switch rankType {
	case gshare.RankTypeBattleArena:
		if RobotRankPlayerDataSetter(playerId, info) {
			return
		}
	}

	packetPlayerInfoToRankInfo(playerId, info)
}

func packetPlayerInfoToRankInfo(playerId uint64, info *pb3.RankInfo) {
	if player := GetPlayerPtrById(playerId); nil != player {
		info.Head = player.GetHead()
		info.HeadFrame = player.GetHeadFrame()
		info.VipLv = player.GetBinaryData().Vip
		info.Name = player.GetName()
		info.Job = player.GetJob()
		info.Appear = GetExtraAppearAttr(playerId)
		info.Circle = player.GetCircle()
		info.GuildName = player.GetGuildName()
		info.FlyCamp = player.GetFlyCamp()
		info.DragonEqData = player.GetRankDragonEqData()
		info.FairyData = player.GetRankFairyData()
		info.SoulHaloData = player.GetRankSoulHaloData()
		info.SysOpenStatus = player.GetSysStatusData()
		info.FairyCWData = player.GetRankFairyCWData()
		info.KDragonEqData = player.GetRankKDragonEqData()
	} else {
		if data := GetData(playerId, gshare.ActorDataBase); data != nil {
			if baseData, ok := data.(*pb3.PlayerDataBase); ok {
				info.Name = baseData.Name
				info.Job = baseData.Job
				info.Head = baseData.Head
				info.HeadFrame = baseData.HeadFrame
				info.VipLv = baseData.VipLv
				info.Appear = GetExtraAppearAttr(playerId)
				info.Circle = baseData.Circle
				info.GuildName = baseData.GuildName
				info.FlyCamp = baseData.FlyCamp
				info.DragonEqData = GetRankDragonEqData(baseData)
				info.FairyData = GetRankFairyData(baseData)
				info.SoulHaloData = GetRankSoulHaloData(baseData)
				info.SysOpenStatus = baseData.SysOpenStatus
				info.FairyCWData = GetRankFairyCWData(baseData)
				info.KDragonEqData = GetRankKDragonEqData(baseData)
			}
		}
	}
}

func GetSimplyData(playerId uint64) *pb3.SimplyPlayerData {
	if engine.IsRobot(playerId) {
		return GetRobotSimplyData(playerId)
	}
	if data := GetData(playerId, gshare.ActorDataBase); data != nil {
		if bData, ok := data.(*pb3.PlayerDataBase); ok {
			return &pb3.SimplyPlayerData{
				Id:             bData.Id,
				Name:           bData.Name,
				Circle:         bData.Circle,
				Lv:             bData.Lv,
				VipLv:          bData.VipLv,
				Job:            bData.Job,
				Sex:            bData.Sex,
				GuildId:        bData.GuildId,
				GuildName:      bData.GuildName,
				LastLogoutTime: bData.LastLogoutTime,
				BubbleFrame:    bData.BubbleFrame,
				HeadFrame:      bData.HeadFrame,
				LoginTime:      bData.LoginTime,
				Head:           bData.Head,
				Power:          bData.Power,
				CharacterTags:  bData.CharacterTags,
				FlyCamp:        bData.FlyCamp,
				MarryId:        bData.MarryId,
				MarryName:      bData.MarryName,
				MarryCommonId:  bData.MarryCommonId,
			}
		}
	}
	return nil
}

// GetData 根据玩家Id和数据Id获取玩家数据
func GetData(actorId uint64, dataId uint32) pb3.Message {
	player := GetPlayerPtrById(actorId)
	if nil != player { // 在线
		if fn := GetDataFn(dataId); nil != fn {
			return fn(player)
		}
		return nil
	}
	return GetOfflineData(actorId, dataId)
}

// GetFightAttr 根据玩家Id和属性prop获取玩家战斗属性
func GetFightAttr(actorId uint64, prop uint32) attrdef.AttrValueAlias {
	player := GetPlayerPtrById(actorId)
	if nil != player { // 在线
		return player.GetFightAttr(prop)
	}
	if property, ok := GetData(actorId, gshare.ActorDataProperty).(*pb3.OfflineProperty); ok {
		if nil != property.FightAttr {
			return property.FightAttr[prop]
		}
	}
	return 0
}

// GetExtraAttr 根据玩家Id和属性prop获取玩特殊属性
func GetExtraAttr(actorId uint64, prop uint32) attrdef.AttrValueAlias {
	player := GetPlayerPtrById(actorId)
	if nil != player { // 在线
		return player.GetExtraAttr(prop)
	}
	if property, ok := GetData(actorId, gshare.ActorDataProperty).(*pb3.OfflineProperty); ok {
		if nil != property.ExtraAttr {
			return property.ExtraAttr[prop]
		}
	}
	return 0
}

// GetExtraAppearAttr 根据玩家Id和属性prop获取玩家的外观属性
func GetExtraAppearAttr(actorId uint64) map[uint32]int64 {
	appMap := make(map[uint32]int64, len(appeardef.AppearPosMapToExtraAttr))
	player := GetPlayerPtrById(actorId)
	if nil != player { // 在线
		for _, id := range appeardef.AppearPosMapToExtraAttr {
			appMap[id] = player.GetExtraAttr(id)
		}
	} else {
		if property, ok := GetData(actorId, gshare.ActorDataProperty).(*pb3.OfflineProperty); ok {
			if nil != property.ExtraAttr {
				for _, id := range appeardef.AppearPosMapToExtraAttr {
					appMap[id] = property.ExtraAttr[id]
				}
			}
		}
	}
	return appMap
}

// GetPlayerPtrById 根据玩家id获取player对象
func GetPlayerPtrById(playerId uint64) iface.IPlayer {
	return playerMap[playerId]
}

// GetPlayerPtrByName 根据玩家名字获取player对象
func GetPlayerPtrByName(name string) iface.IPlayer {
	for _, player := range playerMap {
		if player.GetName() == name {
			return player
		}
	}
	return nil
}

func GetRankDragonEqData(data *pb3.PlayerDataBase) *pb3.RankDragonEqData {
	return &pb3.RankDragonEqData{Equips: data.DragonEquips}
}

func GetRankFairyData(data *pb3.PlayerDataBase) *pb3.RankFairyData {
	posMap := make(map[uint32]*pb3.RankFairy)
	for pos, posData := range data.BattleFairy {
		if !itemdef.IsFairyMainPos(pos) {
			continue
		}
		posMap[pos] = &pb3.RankFairy{
			ItemId: posData.ItemId,
			Lv:     posData.Union1,
			Star:   posData.Union2,
		}
	}
	return &pb3.RankFairyData{BattleFairy: posMap}
}

func GetRankSoulHaloData(data *pb3.PlayerDataBase) *pb3.RankSoulHaloData {
	return &pb3.RankSoulHaloData{SlotInfo: data.SoulHalo}
}

func GetRankFairyCWData(data *pb3.PlayerDataBase) *pb3.RankFairyColdWeaponData {
	return &pb3.RankFairyColdWeaponData{WeaponMap: data.FairyCWData}
}

func GetRankKDragonEqData(data *pb3.PlayerDataBase) *pb3.RankKillDragonEqData {
	return &pb3.RankKillDragonEqData{EquipCastMap: data.KillDragonEqs}
}

func AllOnlinePlayerDoCond(fn func(iface.IPlayer) bool) {
	flag := true
	for _, player := range playerMap {
		if !flag {
			break
		}
		utils.ProtectRun(func() {
			flag = fn(player)
		})
	}
}

func AllOnlinePlayerDo(fn func(iface.IPlayer)) {
	for _, player := range playerMap {
		utils.ProtectRun(func() {
			fn(player)
		})
	}
}

// GetOnlinePlayerByCond  通过筛选条件过滤符合条件的在线玩家
func GetOnlinePlayerByCond(condFn func(iface.IPlayer) bool, getNum uint32) []iface.IPlayer {
	mapLen := uint32(len(playerMap))
	if getNum > 0 {
		mapLen = mathutil.MinUint32(getNum, mapLen)
	}
	// 初始化
	playerList := make([]iface.IPlayer, 0)
	for _, player := range playerMap {
		if uint32(len(playerList)) >= mapLen {
			break
		}
		flag := false
		utils.ProtectRun(func() {
			flag = condFn(player)
		})
		if flag {
			playerList = append(playerList, player)
		}
	}
	return playerList
}

func GetAllOnlinePlayerCount() uint32 {
	return uint32(len(playerMap))
}

func GetOnlinePlayerCountWithDitch() map[uint32]map[uint32]uint32 {
	dm := make(map[uint32]map[uint32]uint32)
	for _, player := range playerMap {
		if nil != player {
			ditchId := player.GetExtraAttrU32(attrdef.DitchId)
			subDitchId := player.GetExtraAttrU32(attrdef.SubDitchId)
			s, ok := dm[ditchId]
			if !ok {
				s = make(map[uint32]uint32)
				dm[ditchId] = s
			}
			s[subDitchId]++
		}
	}
	return dm
}

var (
	logOnlinePeriod = getLogOnlinePeriod(time.Now())
)

func getLogOnlinePeriod(now time.Time) uint32 {
	minutes := now.Hour()*60 + now.Minute()
	return uint32(minutes / 3)
}

func LogDitchOnlinePlayer(cb func(st *pb3.LogOnline)) {
	now := time.Now()
	curPeriod := getLogOnlinePeriod(now)
	if logOnlinePeriod == curPeriod {
		return
	}

	logTime := uint32(now.Unix()) / 180 * 180
	logOnlinePeriod = curPeriod

	dm := GetOnlinePlayerCountWithDitch()
	for ditch, line := range dm {
		for subDitch, count := range line {
			cb(&pb3.LogOnline{
				Time:       logTime,
				Count:      count,
				DitchId:    ditch,
				SubDitchId: subDitch,
			})
		}
	}
}

// OnPlayerLogin 玩家上线
func OnPlayerLogin(player iface.IPlayer) {
	playerId := player.GetId()
	if _, exists := playerMap[player.GetId()]; exists {
		logger.LogError("error!! has player data in hashtable!! playerId:%d", playerId)
	}
	playerMap[playerId] = player
}

func OnPlayerClosed(player iface.IPlayer) {
	playerId := player.GetId()

	if _, exist := playerMap[playerId]; exist {
		delete(playerMap, playerId)
	} else {
		logger.LogError("can't find player[playerId=%d] in playerMap", playerId)
	}
}

// CloseAllPlayer 关闭网关所有玩家
func CloseAllPlayer(gateId int) {
	logger.LogInfo("EntityMgr CloseAllPlayer, gateworker=%d", gateId)
	AllOnlinePlayerDo(func(player iface.IPlayer) {
		id, _ := player.GetGateInfo()
		if -1 == gateId || int(id) == gateId {
			player.ClosePlayer(cmd.DCRCloseServer)
		}
	})
}

func OnServerMailLoaded() {
	AllOnlinePlayerDo(func(player iface.IPlayer) {
		if sys, ok := player.GetSysObj(sysdef.SiMail).(iface.IMailSys); ok {
			sys.OnServerMailLoaded()
		}
	})
}

func RegisterSettingChangeTriggerFunc(bitPos int, fn SettingBitChangeTriggerFunc) {
	if bitPos > 31 {
		logger.LogFatal("invalid bitPos %v", bitPos)
	}
	_, ok := settingBitChangeTriggerFuncs[bitPos]
	if ok {
		logger.LogFatal("bitPos %v already registered", bitPos)
	}
	settingBitChangeTriggerFuncs[bitPos] = fn
}

func TriggerSettingChange(player iface.IPlayer, oldBits uint64, newBits uint64) {
	changedBits := oldBits ^ newBits
	for bit, fn := range settingBitChangeTriggerFuncs {
		if changedBits&(1<<bit) != 0 {
			fn(player, oldBits&(1<<bit) != 0, newBits&(1<<bit) != 0)
		}
	}
}

func init() {
	engine.StdRewardToBroadcastV2 = func(actorId uint64, rewards jsondata.StdRewardVec) string {
		cond := &jsondata.FilerRewardsCond{}
		if bData, ok := GetData(actorId, gshare.ActorDataBase).(*pb3.PlayerDataBase); ok {
			cond.Level = bData.Lv
			cond.Job = bData.Job
			cond.Sex = bData.Sex
		}
		fRewards := engine.FilterRewardsByCond(rewards, cond)
		return jsondata.StdRewardToBroadcastStr(fRewards)
	}
}
