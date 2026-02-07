/**
 * @Author: lzp
 * @Date: 2025/5/8
 * @Desc:
**/

package actorsystem

import (
	"github.com/gzjjyz/srvlib/utils"
	"jjyz/base"
	"jjyz/base/common"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/net"
)

type AntiComicsSys struct {
	Base
}

func (sys *AntiComicsSys) OnReconnect() {
	sys.s2cInfo()
}

func (sys *AntiComicsSys) OnLogin() {
	sys.s2cInfo()
}

func (sys *AntiComicsSys) OnOpen() {
	sys.s2cInfo()
}

func (sys *AntiComicsSys) OnNewDay() {
	if data := sys.GetBinaryData(); nil != data {
		data.ComicsRewards = data.ComicsRewards[:0]
	}
	sys.s2cInfo()
}

func (sys *AntiComicsSys) GetData() []uint32 {
	if sys.GetBinaryData().ComicsRewards == nil {
		sys.GetBinaryData().ComicsRewards = make([]uint32, 0)
	}
	return sys.GetBinaryData().ComicsRewards
}

func (sys *AntiComicsSys) s2cInfo() {
	data := sys.GetData()
	msg := &pb3.S2C_2_225{
		Ids: data,
	}
	sys.SendProto3(2, 225, msg)
}

func (sys *AntiComicsSys) c2sFetchReward(msg *base.Message) error {
	var req pb3.C2S_2_226
	if err := msg.UnpackagePbmsg(&req); nil != err {
		return err
	}

	conf := jsondata.GetAntiComicsConf(req.Id)
	if conf == nil {
		return neterror.ConfNotFoundError("id: %d config not found", req.Id)
	}

	if gshare.GetOpenServerDay() >= jsondata.GlobalUint("fzznEndDay") {
		return neterror.ParamsInvalidError("open day limit")
	}

	idL := sys.GetData()
	if utils.SliceContainsUint32(idL, req.Id) {
		return neterror.ParamsInvalidError("id: %d rewards is fetched", req.Id)
	}

	idL = append(idL, req.Id)
	sys.GetBinaryData().ComicsRewards = idL

	if len(conf.Awards) > 0 {
		engine.GiveRewards(sys.owner, conf.Awards, common.EngineGiveRewardParam{
			LogId: pb3.LogId_LogAntiComicsRewards,
		})
	}

	sys.SendProto3(2, 226, &pb3.S2C_2_226{Id: req.Id})
	return nil
}

func init() {
	RegisterSysClass(sysdef.SiAntiComics, func() iface.ISystem {
		return &AntiComicsSys{}
	})

	event.RegActorEvent(custom_id.AeNewDay, func(actor iface.IPlayer, args ...interface{}) {
		if sys, ok := actor.GetSysObj(sysdef.SiAntiComics).(*AntiComicsSys); ok && sys.IsOpen() {
			sys.OnNewDay()
		}
	})

	net.RegisterSysProto(2, 226, sysdef.SiAntiComics, (*AntiComicsSys).c2sFetchReward)
}
