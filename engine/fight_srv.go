/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2021/8/28 17:07
 */

package engine

import (
	"jjyz/base"
	"jjyz/base/pb3"
)

var (
	IsConnectCrossSrv func() bool
)

var (
	SendToFight               func(ftype base.ServerType, cmd uint16, msg pb3.Message) error
	FightClientExistPredicate func(ftype base.ServerType) bool // 判断战斗服是否已经连接
	GetMediumCrossCamp        func() uint64
	GetSmallCrossCamp         func() uint32
)
