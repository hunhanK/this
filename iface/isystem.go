package iface

import (
	"jjyz/base/custom_id/itemdef"
	"jjyz/base/jsondata"
	"jjyz/base/pb3"
)

type ISystemMgr interface {
	OnLogin()
	OnAfterLogin()
	OnLogout()
	OnLoginFight()
	OnReconnect()
	OneSecLoop()
}

type ISystem interface {
	GetSysId() uint32
	Init(sysId uint32, sysMgr ISystemMgr, player IPlayer)
	IsOpen() bool
	Open()
	Close()
	OnOpen()
	OnClose()
	OnSave()
	OnInit()
	OnLogin()
	OnAfterLogin()
	OnLogout()
	OnDestroy()
	OnLoginFight()
	OnReconnect()
	OneSecLoop()
	DataFix()
}

type IMailSys interface {
	OnServerMailLoaded()
	SendDbLoadMail(uint64)
}

type ISkillSys interface {
	PackFightSrvSkill(createData *pb3.CreateActorData)
}

type IImmortalSoulSys interface {
	PackFightSrvBattleSoul(createData *pb3.CreateActorData)
}

type ILawSys interface {
	PackFightSrvLawInfo(createData *pb3.CreateActorData)
}

type IDomainSys interface {
	PackFightSrvDomainInfo(createData *pb3.CreateActorData)
}

type ICustomTipSys interface {
	PackFightCustomTip(createData *pb3.CreateActorData)
}

type IPackCreateData interface {
	PackCreateData(createData *pb3.CreateActorData)
}

type IBagSys interface {
	IContainer
	CanAddItem(param *itemdef.ItemParamSt, overlay bool) bool
	GetAddItemNeedGridCount(param *itemdef.ItemParamSt, overlay bool) int64
	AddItem(param *itemdef.ItemParamSt) bool
	AddItemPtr(item *pb3.ItemSt, bTip bool, logId pb3.LogId) bool
	DeleteItemPtr(item *pb3.ItemSt, count int64, logId pb3.LogId) bool
	DeleteItem(param *itemdef.ItemParamSt) bool
	GetItemCount(itemId uint32, bind int8) (count int64)
	RemoveItemByHandle(handle uint64, logId pb3.LogId) bool
	FindItemByHandle(handle uint64) *pb3.ItemSt
}

type IContainer interface {
	AvailableCount() uint32
	Split(handle uint64, count int64, logId pb3.LogId) uint64
	Sort() (success bool, updateVec []*pb3.ItemSt, deleteVec []uint64)
	CheckItemBind(itemId uint32, isBind bool) bool
}

type IGuildSys interface {
	GetGuildBasicById(id uint64) *pb3.GuildBasicInfo
}

type IFlyUpRoadSys interface {
	GetCompleteFlyUpWorldQuestTimeAt() uint32
}

type IFashionActiveCheck interface {
	CheckFashionActive(fashionId uint32) bool
}

type IEquip interface {
	CheckEquipPosTakeOn(pos uint32) bool
}

// IFashionChecker 时装检查接口
type IFashionChecker interface {
	IFashionActiveCheck
	GetFashionQuality(uint32) uint32
	GetFashionBaseAttr(fashionId uint32) jsondata.AttrVec
}

type IMaxStarChecker interface {
	IsMaxStar(id uint32) bool
}
