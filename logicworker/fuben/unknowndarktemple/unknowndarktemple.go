/*
* @Author:
* @Desc: 圣域
* @Date:
 */

package unknowndarktemple

import (
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
	"github.com/gzjjyz/srvlib/utils/pie"
	"jjyz/base"
	"jjyz/base/argsdef"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/fubendef"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/gmevent"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
)

func GetSceneSec(player iface.IPlayer, sceneId uint32) uint32 {
	data := player.GetBinaryData()
	if data.UnknownDarkTempleSec == nil {
		data.UnknownDarkTempleSec = make(map[uint32]uint32)
	}
	return data.UnknownDarkTempleSec[sceneId]
}

func checkTime(player iface.IPlayer, sceneId uint32, isCross bool) bool {
	conf := jsondata.GetUnknownDarkTempleSceneConf(sceneId, isCross)
	if conf.EnterTotal == 0 {
		return true
	}
	if GetSceneSec(player, sceneId) >= conf.EnterTotal {
		return false
	}
	return true
}

func checkEnterMergeCond(mergeConf *jsondata.UnknownDarkTempleMergeConf) bool {
	if mergeConf.OpenDay != 0 && gshare.GetOpenServerDay() < mergeConf.OpenDay {
		logger.LogWarn("open day not reach %d %d", mergeConf.Layer, gshare.GetOpenServerDay())
		return false
	}

	if mergeConf.MergeTimes != 0 && gshare.GetMergeTimes() != mergeConf.MergeTimes {
		logger.LogWarn("merge times not reach %d %d", mergeConf.Layer, gshare.GetMergeTimes())
		return false
	}

	if len(mergeConf.MergeDayRange) == 2 && !(gshare.GetMergeSrvDay() >= mergeConf.MergeDayRange[0] && gshare.GetMergeSrvDay() <= mergeConf.MergeDayRange[1]) {
		return false
	}
	return true
}

func enterFight(player iface.IPlayer, sceneId uint32, cross bool) error {
	mergeConf, conf := jsondata.GetUnknownDarkTempleWxTypeConf(sceneId, cross)
	if mergeConf == nil || conf == nil {
		return neterror.ConfNotFoundError("not found conf")
	}

	if !checkEnterMergeCond(mergeConf) {
		return neterror.ParamsInvalidError("cond not reach")
	}

	// 时间用完了
	if !checkTime(player, sceneId, cross) {
		return neterror.ParamsInvalidError("not time")
	}

	var srvType = base.LocalFightServer
	if cross {
		srvType = base.SmallCrossServer
	}

	err := player.EnterFightSrv(srvType, fubendef.EnterUnknownDarkTemple,
		&pb3.CommonSt{
			U32Param: sceneId,
		},
		&argsdef.ConsumesSt{
			Consumes: conf.Consume,
			LogId:    pb3.LogId_LogUnknownDarkTemple,
		})
	if err != nil {
		player.LogError("err:%v", err)
		return err
	}
	return nil
}

func enterUnknownDarkTemple(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_130_143
	if err := msg.UnpackagePbmsg(&req); nil != err {
		return err
	}
	return enterFight(player, req.GetSceneId(), req.GetCross())
}

func f2gBossInfo(buf []byte) {
	msg := &pb3.SyncUnknownDarkTempleBossInfo{}
	if err := pb3.Unmarshal(buf, msg); err != nil {
		return
	}
	actorId := msg.GetActorId()
	if player := manager.GetPlayerPtrById(actorId); nil != player {
		player.SendProto3(130, 144, &pb3.S2C_130_144{
			LayerInfo: msg.LayerInfo,
			Cross:     msg.Cross,
		})
	}
}

func onSyncUnknownDarkTempleSceneSec(player iface.IPlayer, buf []byte) {
	var st pb3.CommonSt
	if err := pb3.Unmarshal(buf, &st); nil != err {
		player.LogError("onSyncUnknownDarkTempleSceneSec error:%v", err)
		return
	}
	sceneId := st.U32Param
	sec := st.U32Param2
	data := player.GetBinaryData()
	data.UnknownDarkTempleSec[sceneId] = sec
}

func s2cSceneDayTime(player iface.IPlayer) {
	data := player.GetBinaryData()

	msg := &pb3.S2C_130_146{
		SceneSec: data.UnknownDarkTempleSec,
	}
	player.SendProto3(130, 146, msg)
}

func handleAeAfterLogin(player iface.IPlayer, _ ...interface{}) {
	err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FReqUnknownDarkTemple, &pb3.CommonSt{U64Param: player.GetId(), U32Param: engine.GetPfId(), U32Param2: engine.GetServerId()})
	if err != nil {
		player.LogError("err:%v,enter local %d failed", err, sysfuncid.G2FReqUnknownDarkTemple)
	}
	err = engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FReqUnknownDarkTemple, &pb3.CommonSt{U64Param: player.GetId(), U32Param: engine.GetPfId(), U32Param2: engine.GetServerId()})
	if err != nil {
		player.LogError("err:%v,enter cross %d failed", err, sysfuncid.G2FReqUnknownDarkTemple)
	}
	player.SendProto3(130, 149, &pb3.S2C_130_149{
		BossIds: player.GetBinaryData().FollowUnknownDarkTempleBossIds,
	})
}

func handleAeNewDay(player iface.IPlayer, _ ...interface{}) {
	player.GetBinaryData().UnknownDarkTempleSec = make(map[uint32]uint32)
}

func c2sSceneDayTime(player iface.IPlayer, msg *base.Message) error {
	s2cSceneDayTime(player)
	return nil
}

func f2gSceneInfo(buf []byte) {
	msg := &pb3.SyncUnknownDarkTempleSceneInfo{}
	if err := pb3.Unmarshal(buf, msg); err != nil {
		return
	}
	actorId := msg.GetActorId()
	if player := manager.GetPlayerPtrById(actorId); nil != player {
		player.SendProto3(130, 141, &pb3.S2C_130_141{
			SceneId:    msg.SceneId,
			MonsterMap: msg.MonsterMap,
			Cross:      msg.Cross,
		})
	}
}

func c2sFollow(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_130_148
	err := msg.UnPackPb3Msg(&req)
	if err != nil {
		return err
	}
	need := req.Need
	bossId := req.BossId
	bossIds := player.GetBinaryData().FollowUnknownDarkTempleBossIds
	if need {
		if !pie.Uint32s(bossIds).Contains(bossId) {
			bossIds = append(bossIds, bossId)
		}
	} else {
		bossIds = pie.Uint32s(bossIds).Filter(func(u uint32) bool {
			return bossId != u
		})
	}
	player.GetBinaryData().FollowUnknownDarkTempleBossIds = bossIds
	player.SendProto3(130, 148, &pb3.S2C_130_148{
		BossId: req.GetBossId(),
		Need:   req.GetNeed(),
	})
	return nil
}

func c2sGetSceneNum(player iface.IPlayer, msg *base.Message) error {
	var req pb3.C2S_130_141
	if err := msg.UnpackagePbmsg(&req); nil != err {
		return err
	}
	if req.GetCross() {
		err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2FReqUnknownDarkTempleScene, &pb3.CommonSt{U64Param: player.GetId(), U32Param: req.SceneId, U32Param2: engine.GetPfId(), U32Param3: engine.GetServerId()})
		if err != nil {
			player.LogError("err:%v,enter local %d failed", err, sysfuncid.G2FReqUnknownDarkTempleScene)
		}
	} else {
		err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FReqUnknownDarkTempleScene, &pb3.CommonSt{U64Param: player.GetId(), U32Param: req.SceneId, U32Param2: engine.GetPfId(), U32Param3: engine.GetServerId()})
		if err != nil {
			player.LogError("err:%v,enter local %d failed", err, sysfuncid.G2FReqUnknownDarkTempleScene)
		}
	}
	return nil
}

func init() {
	event.RegActorEvent(custom_id.AeNewDay, handleAeNewDay)
	event.RegActorEventL(custom_id.AeAfterLogin, handleAeAfterLogin)
	event.RegActorEventL(custom_id.AeReconnect, handleAeAfterLogin)
	engine.RegisterSysCall(sysfuncid.F2GUnknownDarkTempleBossInfo, f2gBossInfo)
	engine.RegisterSysCall(sysfuncid.F2GUnknownDarkTempleSceneInfo, f2gSceneInfo)
	engine.RegisterActorCallFunc(playerfuncid.F2GSyncUnknownDarkTempleSceneSec, onSyncUnknownDarkTempleSceneSec)
	net.RegisterProto(130, 141, c2sGetSceneNum)
	net.RegisterProto(130, 143, enterUnknownDarkTemple)
	net.RegisterProto(130, 146, c2sSceneDayTime)
	net.RegisterProto(130, 148, c2sFollow)
	initUdtGm()
}

func initUdtGm() {
	gmevent.Register("udt.enter", func(player iface.IPlayer, args ...string) bool {
		var crossInt uint32 = 0
		if len(args) >= 2 {
			crossInt = utils.AtoUint32(args[1])
		}

		crossB := false
		if crossInt == 1 {
			crossB = true
		}
		st := &pb3.CommonSt{
			U32Param: utils.AtoUint32(args[0]),
		}
		var err error
		if crossB {
			err = player.EnterFightSrv(base.SmallCrossServer, fubendef.EnterUnknownDarkTemple, st)
		} else {
			err = player.EnterFightSrv(base.LocalFightServer, fubendef.EnterUnknownDarkTemple, st)
		}
		if nil != err {
			player.LogError("onSyncUnknownDarkTempleSceneSec error:%v", err)
			return false
		}
		return true
	}, 1)
	gmevent.Register("udt.getSceneNum", func(player iface.IPlayer, args ...string) bool {
		err := engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FReqUnknownDarkTempleScene, &pb3.CommonSt{U64Param: player.GetId(), U32Param: utils.AtoUint32(args[0]), U32Param2: engine.GetPfId(), U32Param3: engine.GetServerId()})
		if err != nil {
			player.LogError("err:%v,enter local %d failed", err, sysfuncid.G2FReqUnknownDarkTempleScene)
		}
		return true
	}, 1)
}
