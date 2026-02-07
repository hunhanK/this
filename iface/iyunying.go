/**
 * @Author: HeXinLi
 * @Desc: 运营活动接口
 * @Date: 2021/9/28 21:56
 */

package iface

import (
	"jjyz/base/custom_id"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
)

type IYunYing interface {
	Init(oTime, eTime uint32)
	OnInit()
	IsOpen() bool
	OnOpen()
	OnEnd()
	BeforeNewDay()
	NewDay()
	GetClass() uint32
	GetOpenDay() uint32
	GetOpenTime() uint32
	GetEndTime() uint32
	SetEndTime(eTime uint32)
	SetConfIdx(idx uint32)
	GetConfIdx() uint32
	SetId(id uint32)
	GetId() uint32
	QuestEvent(qt, id, count uint32)
	GetYYStateInfo() *pb3.YYStateInfo
	PlayerUseDiamond(count int64)
	PlayerCharge(*custom_id.ActorEventCharge)
	BugFix()
	PlayerLogin(player IPlayer)
	PlayerReconnect(player IPlayer)
	Broadcast(sysId, cmdId uint16, msg pb3.Message)
	BroadcastYYStateInfo()
	ServerStopSaveData()
	ResetData()
}

type IPlayerYY interface {
	Init(player IPlayer, oTime, eTime uint32)
	OnInit()
	IsOpen() bool
	OnOpen()
	OnEnd()
	Login()
	OnAfterLogin()
	OnLogout()
	OnReconnect()
	BeforeNewDay()
	NewDay()
	GetClass() uint32
	GetOpenDay() uint32
	GetOpenTime() uint32
	GetEndTime() uint32
	SetEndTime(eTime uint32)
	GetPlayer() IPlayer
	GetYYData() *pb3.PlayerYYData
	SetConfIdx(idx uint32)
	GetConfIdx() uint32
	SetId(id uint32)
	GetId() uint32
	QuestEvent(qt, id, count uint32)
	GetYYStateInfo() *pb3.YYStateInfo
	PlayerUseDiamond(count int64)
	PlayerCharge(*custom_id.ActorEventCharge)
	BugFix()
	ResetData()
	MergeFix()
	CmdYYFix()
	OnLoginFight()
	IsUseRealCharge() bool
}

type IPlayerYYMgr interface {
	SendPlayerYYInfo(msg *pb3.S2C_127_255)
}

type IYYMoneyHandler interface {
	CanAddYYMoney(mt uint32) bool
}

type IYYLotteryChangeLibConf interface {
	GetChangeLibConf(libId uint32) *jsondata.LotteryLibConf
}

type IPYYSummerSurfDraw interface {
	GetHitItemNums() uint32
}

type IYYReachStandardScore interface {
	GetReachStandardScore(playerId uint64) int64
}
