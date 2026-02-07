package fightworker

import (
	"jjyz/base"
	"jjyz/gameserver/gshare"
)

const localFightName = "local"

func Startup() {
	AddFightClient(base.LocalFightServer, &FightHostInfo{
		Host: gshare.GameConf.LocalFightSrv,
		Name: localFightName,
	})

	for _, fight := range fights {
		if nil != fight {
			fight.StartUp()
		}
	}
}
