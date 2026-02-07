package activity

import (
	"jjyz/base"
	"jjyz/base/custom_id"
	"jjyz/base/custom_id/playerfuncid"
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/iface"

	"github.com/gzjjyz/logger"
)

func init() {
	engine.RegisterSysCall(sysfuncid.F2GHuryChicksInfoReq, func(_ []byte) {
		engine.CallFightSrvFunc(base.LocalFightServer, sysfuncid.G2FHuryChickInfoRes, &pb3.G2FHuryChickInfoRes{
			Info: gshare.GetStaticVar().HuryChick,
		})
	})

	engine.RegisterSysCall(sysfuncid.F2GStoreHuryChicksInfoReq, func(buf []byte) {
		var req pb3.F2GStoreHuryChickInfoReq
		if err := pb3.Unmarshal(buf, &req); err != nil {
			logger.LogError("F2GStoreHuryChicksInfoReq Unmarshal error:%v", err)
		}

		gshare.GetStaticVar().HuryChick = req.Info
	})

	engine.RegisterActorCallFunc(playerfuncid.HuryChickenEnter, func(player iface.IPlayer, buf []byte) {
		event.TriggerEvent(player, custom_id.AeCompleteRetrieval, sysdef.SiHuryChicken, 1)
	})
}
