/**
 * @Author: LvYuMeng
 * @Date: 2025/2/24
 * @Desc:
**/

package actorsystem

import (
	"jjyz/base/custom_id/sysdef"
	"jjyz/base/custom_id/trialactivetype"
	"jjyz/base/jsondata"
	"jjyz/base/neterror"
	"jjyz/gameserver/iface"
)

type TrialActiveHandler struct {
	DoActive func(params *jsondata.TrialActiveParams) error
	DoForget func(params *jsondata.TrialActiveParams) error
}

type TrialActiveFunc func(player iface.IPlayer, params *jsondata.TrialActiveParams)

var trialActiveMap = map[uint32]func(player iface.IPlayer) (*TrialActiveHandler, error){
	trialactivetype.ActiveTypeFaGul:       newTrialActiveFaGul,
	trialactivetype.ActiveTypeFashion:     newTrialActiveFashion,
	trialactivetype.ActiveTypeWingFashion: newTrialWingFashion,
}

func getTrialActiveHandler(player iface.IPlayer, activeType uint32) (*TrialActiveHandler, error) {
	if fn, ok := trialActiveMap[activeType]; ok {
		return fn(player)
	}

	return nil, neterror.ParamsInvalidError("not reg")
}

func newTrialActiveFaGul(player iface.IPlayer) (*TrialActiveHandler, error) {
	sys, ok := player.GetSysObj(sysdef.SiFaGul).(*FaGulSys)
	if !ok || !sys.IsOpen() {
		return nil, neterror.SysNotExistError("sys %d not found", sysdef.SiFaGul)
	}

	return sys.newTrialActiveSt()
}

func newTrialActiveFashion(player iface.IPlayer) (*TrialActiveHandler, error) {
	sys, ok := player.GetSysObj(sysdef.SiFashion).(*FashionSys)
	if !ok || !sys.IsOpen() {
		return nil, neterror.SysNotExistError("sys %d not found", sysdef.SiFashion)
	}

	return sys.newTrialActiveSt()
}

func newTrialWingFashion(player iface.IPlayer) (*TrialActiveHandler, error) {
	sys, ok := player.GetSysObj(sysdef.SiFairyWingFashion).(*FairyWingFashionSys)
	if !ok || !sys.IsOpen() {
		return nil, neterror.SysNotExistError("sys %d not found", sysdef.SiFairyWingFashion)
	}

	return sys.newTrialActiveSt()
}
