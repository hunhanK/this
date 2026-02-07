package actorsystem

/*
	desc:仙缘墙系统
	author: twl
	time:	2023/12/05
*/

import (
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/manager"
	"jjyz/gameserver/net"
	"time"

	"github.com/gzjjyz/srvlib/utils"
)

const MarriageWallShowMax = 6      // 仙缘墙展示最大数
const MarriageWallRefresh = 5 * 60 // 仙缘墙展示最大数

// 仙缘墙
type MarriageWallSys struct {
	Base
	wallData        []*pb3.SimplyPlayerData
	nextRefreshTime uint32
}

func (sys *MarriageWallSys) OnInit() {
	sys.init()
}

func (sys *MarriageWallSys) OnLogin() {
	sys.refreshWall()
}

func (sys *MarriageWallSys) OnReconnect() {
	sys.refreshWall()
}

func (sys *MarriageWallSys) init() bool {
	if sys.wallData == nil {
		//初始化wallData
		sys.wallData = make([]*pb3.SimplyPlayerData, 0, MarriageWallShowMax)
	}
	return true
}

// 刷新仙缘墙
func (sys *MarriageWallSys) refreshWall() {
	now := uint32(time.Now().Unix())
	if now < sys.nextRefreshTime { // 没到时间
		return
	}
	// 清空wallData
	sys.wallData = make([]*pb3.SimplyPlayerData, 0, MarriageWallShowMax)
	// 获取符合条件的在线玩家
	conf := jsondata.GetCommonConf("marriageWallConds")
	if conf == nil {
		return
	}
	minLv, _ := conf.U32Vec[0], conf.U32Vec[1]
	myId := sys.GetPlayerData().GetActorId()
	mySex := custom_id.GetSexByJob(sys.GetMainData().Job)
	onlineFn := func(player iface.IPlayer) bool {
		if myId == player.GetId() { // 排除掉自己
			return false
		}
		// 等级>min
		if player.GetLevel() <= minLv {
			return false
		}
		if player.GetBinaryData().MarryData.MarryId > 0 {
			return false
		}
		// 异性
		tarSex := custom_id.GetSexByJob(player.GetJob())

		return tarSex != mySex
	}
	var ids []uint64
	playerLs := manager.GetOnlinePlayerByCond(onlineFn, MarriageWallShowMax) // 找所有
	for _, l := range playerLs {
		ids = append(ids, l.GetId())
	}
	// 多去少补
	getNum := MarriageWallShowMax - len(ids)
	if getNum > 0 { // 少了 去拿离线数据
		offlineFn := func(offlinePlayer *pb3.PlayerDataBase) bool {
			if myId == offlinePlayer.GetId() { // 排除掉自己
				return false
			}
			if utils.SliceContainsUint64(ids, offlinePlayer.GetId()) {
				return false
			}
			// 等级>min
			if offlinePlayer.Lv <= minLv {
				return false
			}

			if offlinePlayer.MarryId > 0 {
				return false
			}
			// 异性
			tarSex := custom_id.GetSexByJob(offlinePlayer.GetJob())
			return tarSex != mySex
		}
		offlineLs := manager.GetOfflinePlayerByCond(offlineFn, uint32(getNum))
		for _, l := range offlineLs {
			ids = append(ids, l.GetId())
		}
	}
	if len(ids) > MarriageWallShowMax { // 随机取出max个
		ids = ids[:MarriageWallShowMax]
	}
	// 排序后裁剪
	for _, id := range ids {
		simplyData := manager.GetSimplyData(id)
		// 加入wallData
		sys.wallData = append(sys.wallData, simplyData)
	}
	// todo
	sys.nextRefreshTime = now + MarriageWallRefresh
}

// 发送界面信息
func (sys *MarriageWallSys) c2sPackSend(_msg *base.Message) {
	sys.refreshWall() //刷新下
	rsp := &pb3.S2C_2_85{
		MyData:   manager.GetSimplyData(sys.owner.GetId()),
		WallData: sys.wallData,
	}
	sys.SendProto3(2, 85, rsp)
}

// 发送界面信息
func (sys *MarriageWallSys) c2sSetTags(msg *base.Message) error {
	var req pb3.C2S_2_86
	if err := pb3.Unmarshal(msg.Data, &req); nil != err {
		return neterror.ParamsInvalidError("UnpackPb3Msg SetTags :%v", err)
	}
	sys.owner.GetBinaryData().CharacterTags = req.CharacterTags
	rsp := &pb3.S2C_2_86{
		CharacterTags: sys.owner.GetBinaryData().CharacterTags,
	}
	sys.SendProto3(2, 86, rsp)
	return nil
}

func init() {
	RegisterSysClass(sysdef.SiMarriageWall, func() iface.ISystem {
		return &MarriageWallSys{}
	})
	net.RegisterSysProto(2, 85, sysdef.SiMarriageWall, (*MarriageWallSys).c2sPackSend)
	net.RegisterSysProto(2, 86, sysdef.SiMarriageWall, (*MarriageWallSys).c2sSetTags)
	//engine.RegQuestTargetProgress(custom_id.QttOneFourSymbolsLv, GetFourSymbolsLvByType)
	//gmevent.Register("FourSymbolsLv", gmFourSymbolsLvUp, 1)
	//gmevent.Register("compose", gmCompose, 1)
}
