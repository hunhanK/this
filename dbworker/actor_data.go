/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2021/8/30 15:50
 */

package dbworker

import (
	"fmt"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/appeardef"
	"jjyz/base/custom_id/moneydef"
	"jjyz/base/db"
	"jjyz/base/db/mysql"
	"jjyz/base/pb3"
	"jjyz/base/time_util"
	"jjyz/gameserver/dbworker/cache"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/logicworker/gcommon"
	"jjyz/gameserver/model"
	"jjyz/gameserver/objversionworker"
	"time"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

func updateActorLogin(args ...interface{}) {
	if !gcommon.CheckArgsCount("updateActorLogin", 1, len(args)) {
		return
	}
	actorId, ok := args[0].(uint64)
	if !ok || actorId <= 0 {
		return
	}

	if _, err := db.OrmEngine.Table("actors").Where("actor_id = ?", actorId).
		Update(map[string]interface{}{"online_flag": 1, "lastlogintime": time.Now()}); nil != err {
		logger.LogError("update actor login error:%v", err)
	}
}

func updateActorLogout(args ...interface{}) {
	if !gcommon.CheckArgsCount("updateActorLogout", 1, len(args)) {
		return
	}
	actorId, ok := args[0].(uint64)
	if !ok || actorId <= 0 {
		return
	}

	if _, err := db.OrmEngine.Exec("update actors set `online_flag`=0 where actor_id = ? limit 1;", actorId); nil != err {
		logger.LogError("update actor logout error:%v", err)
	}
}

func loadActorCacheV2(args ...interface{}) {
	if !gcommon.CheckArgsCount("loadActorCache", 2, len(args)) {
		return
	}

	actorId, ok := args[0].(uint64)
	if !ok {
		logger.LogError("重载玩家数据失败, 参数1 actorId 不是uint64")
		return
	}

	fileName, ok := args[1].(string)
	if !ok {
		logger.LogError("重载玩家数据失败, 参数2 不是string")
		return
	}

	//cache.LoadActorCacheFromDB(actorId, version)

	cache.LoadActorCacheFromFile(actorId, fileName)
}

func saveToCache(args ...interface{}) {
	if !gcommon.CheckArgsCount("saveToCache", 2, len(args)) {
		return
	}

	ip, ok := args[0].(string)
	if !ok {
		logger.LogError("保存玩家数据失败, 参数1 ip 不是字符串类型")
		return
	}

	pb3DataBuf, ok := args[1].([]byte)
	if !ok {
		logger.LogError("保存玩家数据失败, 参数2 不是[]byte")
		return
	}

	pb3Data := &pb3.PlayerData{}
	err := pb3.Unmarshal(pb3DataBuf, pb3Data)
	if err != nil {
		return
	}

	actorId := pb3Data.ActorId
	if actorId <= 0 {
		logger.LogError("保存玩家数据失败, 玩家id=%d", actorId)
		return
	}

	appearInfoBytes, err := pb3.Marshal(pb3Data.MainData.AppearInfo)
	if err != nil {
		appearInfoBytes = []byte{}
		logger.LogError("序列化玩家外观信息失败,外观信息置空, 玩家id=%d, err=%v", actorId, err)
	}

	imedUpdateData := map[string]interface{}{
		"level":       pb3Data.MainData.Level,
		"actor_name":  pb3Data.MainData.ActorName,
		"circle":      pb3Data.MainData.Circle,
		"appear_info": appearInfoBytes,
	}

	_, err = db.OrmEngine.Table("actors").Where("actor_id = ?", actorId).Update(imedUpdateData)
	if err != nil {
		logger.LogError("保存玩家选角界面数据失败, 玩家id=%d, err=%v", actorId, err)
	}

	cache.SaveActorDataToCache(actorId, pb3Data, ip)
}

func saveToObjVersion(args ...interface{}) {
	if !gcommon.CheckArgsCount("saveToObjVersion", 2, len(args)) {
		return
	}

	version, ok := args[0].(uint32)
	if !ok {
		logger.LogError("保存玩家数据失败, 参数1 version 不是整形类型")
		return
	}

	pb3DataBuf, ok := args[1].([]byte)
	if !ok {
		logger.LogError("保存玩家数据失败, 参数2 不是[]byte")
		return
	}

	pb3Data := &pb3.PlayerData{}
	err := pb3.Unmarshal(pb3DataBuf, pb3Data)
	if err != nil {
		return
	}
	objversionworker.PostObjVersionDataWithVersion(pb3Data.ActorId, version, pb3Data)
}

func LoadActorDataFromMysql(actorId uint64) (*pb3.PlayerData, error) {
	//如果查到mysql有玩家数据就把数据存到redis，并且返回给逻辑协程
	var actors []mysql.Actors
	if err := db.OrmEngine.SQL("call loadbaseactor(?)", actorId).Find(&actors); nil != err {
		logger.LogError("%s", err)
		return nil, err
	}

	if len(actors) <= 0 {
		return nil, fmt.Errorf("can't find actor data from mysql!! actorId=%d", actorId)
	}

	// TODO 先保证能运行, 这个 sql 一会儿改

	actor := actors[0]

	mainData := &pb3.PlayerMainData{}

	mainData.ActorName = actor.ActorName
	mainData.Job = actor.Job
	mainData.Sex = actor.Sex
	mainData.Level = actor.Level
	mainData.Circle = actor.Circle
	mainData.CreateTime = uint32(actor.CreateTime.Unix())
	mainData.YuanBao = actor.YuanBao
	mainData.BindDiamonds = actor.BindDiamonds
	mainData.Diamonds = actor.Diamonds
	mainData.DitchId = actor.DitchId
	mainData.SubDitchId = actor.SubDitchId
	mainData.LoginedDays = actor.LoginedDays
	mainData.NewDayResetTime = actor.NewDayResetTime
	mainData.DayOnlineTime = actor.DayOnlineTime
	mainData.Exp = actor.Exp
	mainData.LastLogoutTime = actor.LastLogoutTime
	mainData.FairyStone = actor.FairyStone
	mainData.AppearInfo = &pb3.AppearInfo{}
	if err := pb3.Unmarshal(actor.AppearInfo, mainData.AppearInfo); err != nil {
		logger.LogError("unmarshal appear info error!!! %v", err)
	}

	binaryData := &pb3.PlayerBinaryData{}

	// 20241219: 修复旧数据 更换存储类型
	if actor.MediumBinaryPb3Data == nil || len(actor.MediumBinaryPb3Data) == 0 {
		actor.MediumBinaryPb3Data = actor.BinaryPb3Data
	}

	if actor.MediumBinaryPb3Data != nil {
		if err := pb3.Unmarshal(pb3.UnCompress(actor.MediumBinaryPb3Data), binaryData); nil != err {
			return nil, err
		}
	}

	mainData.Skills = make(map[uint32]*pb3.SkillInfo)
	skills, err := loadActorSkill(actorId)
	if nil != err {
		logger.LogError("load actor skill error!!! %v", err)
	} else {
		for _, st := range skills {
			mainData.Skills[st.SkillId] = &pb3.SkillInfo{
				Id:    st.SkillId,
				Level: st.Level,
				Cd:    st.Cd,
			}
		}
	}

	mainData.ItemPool = new(pb3.ItemPool)
	loadAllItem(mainData, actorId)

	playerData := &pb3.PlayerData{MainData: mainData, BinaryData: binaryData}
	playerData.ActorId = actor.ActorId

	return playerData, nil
}

func loadActorData(args ...interface{}) {
	logger.LogInfo("【登录流程】DB线程开始加载玩家数据")
	if !gcommon.CheckArgsCount("loadActorData", 1, len(args)) {
		return
	}

	actorId, ok := args[0].(uint64)
	if !ok || actorId <= 0 {
		logger.LogError("加载玩家数据出错, 玩家id不是uint64")
		return
	}
	flag := true
	pb3Data := cache.LoadActorDataFromCache(actorId)
	logger.LogInfo("【登录流程】从缓存中加载玩家数据 %d", actorId)
	if nil == pb3Data {
		logger.LogInfo("【登录流程】缓存中没有玩家数据，去mysql加载 %d", actorId)
		if tmpPb3Data, err := LoadActorDataFromMysql(actorId); nil != err {
			logger.LogError("loadActorData Error!!! error:%v", err)
			flag = false
		} else {
			pb3Data = tmpPb3Data
		}
	}

	logger.LogInfo("【登录流程】DB线程加载玩家数据完成 %d", actorId)
	gshare.SendGameMsg(custom_id.GMsgLoadActorDataRet, actorId, flag, pb3Data)
}

func SaveActorData(ip string, pb3Data *pb3.PlayerData) {
	actorId := pb3Data.GetActorId()
	if actorId <= 0 {
		logger.LogError("保存玩家数据失败, 玩家id=%d", actorId)
		return
	}

	var vip uint32
	binaryData := pb3Data.BinaryData
	mainData := pb3Data.MainData

	if nil != binaryData {
		vip = binaryData.GetVip()
	}

	appearInfoBytes, err := pb3.Marshal(mainData.AppearInfo)

	if err != nil {
		appearInfoBytes = []byte{}
	}

	money := binaryData.Money
	var bindDiamonds, diamonds, fairyStone int64
	if money != nil {
		bindDiamonds = money[moneydef.BindDiamonds]
		diamonds = money[moneydef.Diamonds]
		fairyStone = money[moneydef.FairyStone]
	}

	if _, err := db.OrmEngine.Exec("call updateactordata(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", actorId, mainData.GetJob(),
		mainData.GetSex(), mainData.GetActorName(), vip, mainData.GetCircle(), mainData.GetLevel(),
		0, bindDiamonds, diamonds,
		mainData.LastLogoutTime, mainData.GetFightValue(), utils.Ip2int(ip),
		mainData.LoginedDays, mainData.DayOnlineTime, mainData.NewDayResetTime, mainData.Exp,
		pb3.CompressByte(binaryData), appearInfoBytes, fairyStone); nil != err {
		logger.LogError("save actor error!!! actorId:%d %v", actorId, err)
	}

	saveActorSkill(actorId, mainData.Skills) //保存技能
	err = saveItemPool(actorId, mainData.ItemPool)
	if nil != err {
		logger.LogError("actor %d save item pool error %v", actorId, err)
	}

	var onlineMinutes = uint32(0)
	if time_util.NowSec() > mainData.LoginTime {
		onlineMinutes = (time_util.NowSec() - mainData.LoginTime) / 60
	}
	staticsData := map[string]interface{}{
		model.FieldOnlineMinutes_: onlineMinutes,
	}
	if nil != binaryData {
		staticsData["quest_id"] = binaryData.GetFinMainQuestId()
		if pos := binaryData.GetPos(); nil != pos {
			staticsData["scene_id"] = pos.GetSceneId()
		}
	}
	updateActorStatics(actorId, staticsData)
}

func recoveryPlayer(args ...interface{}) {
	if !gcommon.CheckArgsCount("recoveryPlayer", 2, len(args)) {
		return
	}
	playerId, ok := args[0].(uint64)
	userId, ok1 := args[1].(uint32)
	if !ok || !ok1 {
		return
	}

	if rows, err := db.OrmEngine.QueryString("call recoveryplayer(?, ?)", playerId, userId); nil != err {
		logger.LogError("%s", err)
		return
	} else {
		if len(rows) <= 0 || 1 != utils.Atoi(rows[0]["ret"]) {
			logger.LogError("恢复角色失败，当前有效角色>=3 playerId:%d, userId:%d", playerId, userId)
			return
		}

		// 恢复成功，返回逻辑服恢复排行榜数据
		gshare.SendGameMsg(custom_id.GMsgRecoveryPlayer, playerId)
	}
}

func deletePlayer(args ...interface{}) {
	if !gcommon.CheckArgsCount("deletePlayer", 2, len(args)) {
		return
	}

	playerId, ok := args[0].(uint64)
	userId, ok1 := args[1].(uint32)
	if !ok || !ok1 {
		return
	}

	if _, err := db.OrmEngine.Exec("call deleteplayer(?,?)", playerId, userId); nil != err {
		logger.LogError("deleteplayer error!!! playerId:%d userId:%d %v", playerId, userId, err)
		return
	}
	gshare.SendGameMsg(custom_id.GMsgDeletePlayerRet, playerId, userId)
}

func getPlayerBasicInfo(args ...interface{}) {
	if !gcommon.CheckArgsCount("getPlayerBasicInfo", 2, len(args)) {
		return
	}

	guildId, guildOk := args[0].(uint64)
	playerId, ok := args[1].(uint64)
	if !ok || !guildOk {
		return
	}

	type tmpPlayerBasicInfo struct {
		Id             uint64 `xorm:"actor_id"`
		Sex            uint32 `xorm:"sex"`
		Job            uint32 `xorm:"job"`
		Circle         uint32 `xorm:"circle"`
		Level          uint32 `xorm:"level"`
		ActorName      string `xorm:"actor_name"`
		Vip            uint32 `xorm:"vip"`
		FightValue     uint64 `xorm:"fight_value"`
		LastLogoutTime uint32 `xorm:"last_logout_time"`
		AppearInfo     []byte `xorm:"appear_info"`
	}

	var tpi tmpPlayerBasicInfo

	has, err := db.OrmEngine.Table("actors").Where("actor_id = ?", playerId).Get(&tpi)
	if nil != err {
		logger.LogError("load loadplayerbasic message error!!! playerID=%d, err:%v", playerId, err)
		return
	}

	if !has {
		logger.LogError("load loadplayerbasic message failed record not found for playerId = %v", playerId)
		return
	}

	playerData := &pb3.SimplyPlayerData{
		Id:             playerId,
		Name:           tpi.ActorName,
		Circle:         tpi.Circle,
		Lv:             tpi.Level,
		VipLv:          tpi.Vip,
		GuildId:        guildId,
		LastLogoutTime: tpi.LastLogoutTime,
		Power:          tpi.FightValue,
		Job:            tpi.Job,
		Sex:            tpi.Sex,
	}

	appearInfo := &pb3.AppearInfo{}
	err = pb3.Unmarshal(tpi.AppearInfo, appearInfo)
	if err != nil {
		headFrameInfo, ok := appearInfo.Appear[appeardef.AppearPos_HeadFrame]
		if ok {
			playerData.HeadFrame = headFrameInfo.AppearId
		}
		bubbleFrameInfo, ok := appearInfo.Appear[appeardef.AppearPos_BubbleFrame]
		if ok {
			playerData.BubbleFrame = bubbleFrameInfo.AppearId
		}
	}

	gshare.SendGameMsg(custom_id.GMsgGetPlayerBasicInfoRet, playerData)
}

func dirtyActorCache(...interface{}) {
	cache.DirtyAllCache()
}

func updateActorStatus(args ...interface{}) {
	if !gcommon.CheckArgsCount("updateActorStatus", 2, len(args)) {
		return
	}

	actorId, ok1 := args[0].(uint64)
	status, ok2 := args[1].(uint32)
	if !ok1 || !ok2 {
		return
	}

	if _, err := db.OrmEngine.Table("actors").Where("actor_id = ?", actorId).
		Update(map[string]interface{}{"status": status}); nil != err {
		logger.LogError("update actor status message error!!! actorId=%d, err:%v", actorId, err)
	}
}

func instantlySaveDB(args ...interface{}) {
	if !gcommon.CheckArgsCount("instantlySaveDB", 1, len(args)) {
		return
	}

	actorId, ok := args[0].(uint64)
	if !ok {
		return
	}

	data, ok := cache.GetActorCaches()[actorId]
	if ok {
		SaveActorData(data.RemoteAddr, data.Pb3Data)
	}
}

func init() {
	event.RegSysEvent(custom_id.SeDBWorkerInitFinish, func(args ...interface{}) {
		gshare.RegisterDBMsgHandler(custom_id.GMsgUpdateActorLogin, updateActorLogin)
		gshare.RegisterDBMsgHandler(custom_id.GMsgUpdateActorLogout, updateActorLogout)
		gshare.RegisterDBMsgHandler(custom_id.GMsgLoadActorData, loadActorData)
		gshare.RegisterDBMsgHandler(custom_id.GMsgSaveActorDataToCache, saveToCache)
		gshare.RegisterDBMsgHandler(custom_id.GMsgLoadActorCache, loadActorCacheV2)
		gshare.RegisterDBMsgHandler(custom_id.GMsgRecoveryPlayer, recoveryPlayer)
		gshare.RegisterDBMsgHandler(custom_id.GMsgDeletePlayer, deletePlayer)
		gshare.RegisterDBMsgHandler(custom_id.GMsgGetPlayerBasicInfo, getPlayerBasicInfo)
		gshare.RegisterDBMsgHandler(custom_id.GMsgDirtyActorCache, dirtyActorCache)
		gshare.RegisterDBMsgHandler(custom_id.GMsgUpdateActorStatus, updateActorStatus)
		gshare.RegisterDBMsgHandler(custom_id.GMsgInstantlySaveDB, instantlySaveDB)
		gshare.RegisterDBMsgHandler(custom_id.GMsgActorSaveToObjVersion, saveToObjVersion)
	})
}
