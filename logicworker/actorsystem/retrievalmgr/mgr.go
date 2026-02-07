/**
 * @Author: zjj
 * @Date: 2024/5/28
 * @Desc:
**/

package retrievalmgr

import (
	"jjyz/base/jsondata"
	"jjyz/gameserver/iface"
	"sync"
)

const (
	RetrievalByEqualFuBen uint32 = 1 // 装备副本自定义资源找回奖励
)

var (
	lock sync.Mutex
)

var retrievalMgr = make(map[uint32]RetrievalRewardsHandle)

type RetrievalRewardsHandle func(actor iface.IPlayer, count, consumeId uint32) jsondata.StdRewardVec

func Reg(id uint32, f RetrievalRewardsHandle) {
	lock.Lock()
	defer lock.Unlock()
	_, ok := retrievalMgr[id]
	if ok {
		panic("already registered retrieval handle")
	}
	retrievalMgr[id] = f
}

func Get(id uint32) RetrievalRewardsHandle {
	f, ok := retrievalMgr[id]
	if !ok {
		return nil
	}
	return f
}
