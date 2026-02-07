package fashion

import (
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/attrdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/iface"
)

// 装扮升星 如果装扮上有技能将会自动学习
// 升星后会自动要求重算属性
type UpStar struct {
	Fashions map[uint32]*pb3.DressData
	CheckJob bool

	MinLv uint32

	AttrSysId             uint32
	LogId                 pb3.LogId
	GetLvConfHandler      func(fahsionId, lv uint32) *jsondata.FashionStarConf
	GetFashionConfHandler func(fashionId uint32) *jsondata.FashionMeta

	AfterUpstarCb   func(fashionId uint32)
	AfterActivateCb func(fashionId uint32)
}

func (u *UpStar) Init() error {
	if u.Fashions == nil {
		return neterror.ParamsInvalidError("Fashions is nil")
	}

	if u.AttrSysId < attrdef.SysBegin || u.AttrSysId >= attrdef.SysEnd {
		return neterror.ParamsInvalidError("AttrSysId is invalid")
	}

	if u.LogId == 0 {
		return neterror.ParamsInvalidError("LogId is invalid")
	}

	if u.GetLvConfHandler == nil {
		return neterror.ParamsInvalidError("GetLvConfHandler is nil")
	}

	if u.GetFashionConfHandler == nil {
		return neterror.ParamsInvalidError("GetFashionConfHandler is nil")
	}

	if u.AfterUpstarCb == nil {
		return neterror.ParamsInvalidError("AfterUpstarCb is nil")
	}

	if u.AfterActivateCb == nil {
		return neterror.ParamsInvalidError("AfterActivateCb is nil")
	}
	return nil
}

func (u *UpStar) CanActive(player iface.IPlayer, fashionId uint32, checkConsume bool) (bool, error) {
	fashionConf := u.GetFashionConfHandler(fashionId)
	if nil == fashionConf {
		return false, neterror.ParamsInvalidError("invalid fashionId")
	}

	if u.CheckJob && fashionConf.Job != player.GetJob() {
		return false, neterror.ParamsInvalidError("invalid fashionId job not meat")
	}

	if fashionConf.Circle > 0 && player.GetCircle() < fashionConf.Circle {
		return false, neterror.ParamsInvalidError("invalid fashionId circle not meat")
	}
	if fashionConf.NirvanaLv > 0 && player.GetNirvanaLevel() < fashionConf.NirvanaLv {
		return false, neterror.ParamsInvalidError("invalid fashionId nirvanaLevel not meat")
	}

	lvConf := u.GetLvConfHandler(fashionId, u.MinLv)

	if lvConf == nil {
		return false, neterror.ParamsInvalidError("invalid fashionId %d lv %d", fashionId, u.MinLv)
	}

	if checkConsume && !player.CheckConsumeByConf(lvConf.Consumes, false, u.LogId) {
		return false, neterror.ConsumeFailedError("not enough")
	}

	return true, nil
}

func (u *UpStar) Activate(player iface.IPlayer, fashionId uint32, autoBuy bool, noCheckConsume bool) error {
	fashionConf := u.GetFashionConfHandler(fashionId)
	if nil == fashionConf {
		return neterror.ParamsInvalidError("invalid fashionId")
	}

	canActive, err := u.CanActive(player, fashionId, false)
	if !canActive {
		return err
	}

	fashionData, ok := u.Fashions[fashionId]
	if ok {
		return neterror.ParamsInvalidError("fashionId %d already activate", fashionId)
	}

	fashionData = &pb3.DressData{
		Id:   fashionId,
		Star: u.MinLv,
	}

	lvConf := u.GetLvConfHandler(fashionId, u.MinLv)

	if lvConf == nil {
		return neterror.ParamsInvalidError("invalid fashionId %d lv %d", fashionId, u.MinLv)
	}

	if !noCheckConsume && !player.ConsumeByConf(lvConf.Consumes, autoBuy, common.ConsumeParams{LogId: u.LogId}) {
		return neterror.ConsumeFailedError("not enough")
	}
	u.Fashions[fashionId] = fashionData

	u.afterActivate(player, fashionId)
	player.TriggerEvent(custom_id.AeActiveFashion, &custom_id.FashionSetEvent{
		SetId:     fashionConf.SetId,
		FType:     fashionConf.FType,
		FashionId: fashionId,
	})
	player.TriggerEvent(custom_id.AeRareTitleActiveFashion, &custom_id.FashionSetEvent{
		FType:     fashionConf.FType,
		FashionId: fashionId,
	})
	return nil
}

func (u *UpStar) Upstar(player iface.IPlayer, fashionId uint32, autoBuy bool) error {
	fashionConf := u.GetFashionConfHandler(fashionId)
	if nil == fashionConf {
		return neterror.ParamsInvalidError("invalid fashionId")
	}

	if u.CheckJob && fashionConf.Job != player.GetJob() {
		return neterror.ParamsInvalidError("invalid fashionId job not meat")
	}

	nextLv := uint32(0)

	fashioData, ok := u.Fashions[fashionId]
	if !ok {
		return neterror.ParamsInvalidError("fashionId %d not activate", fashionId)
	}

	nextLv = fashioData.Star + 1

	lvConf := u.GetLvConfHandler(fashionId, nextLv)

	if lvConf == nil {
		return neterror.ParamsInvalidError("invalid fashionId %d lv %d", fashionId, nextLv)
	}

	if !player.ConsumeByConf(lvConf.Consumes, autoBuy, common.ConsumeParams{LogId: u.LogId}) {
		return neterror.ConsumeFailedError("not enough")
	}

	fashioData.Star = nextLv

	u.afterLvUp(player, fashionId)
	return nil
}

func (u *UpStar) afterActivate(player iface.IPlayer, fashionId uint32) {
	u.AfterActivateCb(fashionId)
	player.GetAttrSys().ResetSysAttr(u.AttrSysId)
	return
}

func (u *UpStar) afterLvUp(player iface.IPlayer, fashioId uint32) {
	u.AfterUpstarCb(fashioId)
	player.GetAttrSys().ResetSysAttr(u.AttrSysId)

	return
}
