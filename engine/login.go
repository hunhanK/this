/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2021/8/30 14:27
 */

package engine

import (
	"jjyz/base/custom_id"
	"jjyz/gameserver/gshare"
)

var (
	isDisableLogin bool
)

func SetDisableLogin(flag bool) {
	isDisableLogin = flag
}

func GetDisableLogin() bool {
	return isDisableLogin
}

func GetForbidCreatePlayer() bool {
	globalVar := gshare.GetStaticVar()
	return globalVar.ForbidCreatePlayer
}

func SetForbidCreatePlayer(flag bool) {
	globalVar := gshare.GetStaticVar()
	globalVar.ForbidCreatePlayer = flag

	gshare.SendDBMsg(custom_id.GMsgUpdateServerInfo, GetServerId(), flag)
}
