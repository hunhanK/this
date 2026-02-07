/**
 * @Author: PengZiMing
 * @Desc:
 * @Date: 2021/10/13 15:03
 */

package miscitem

type (
	BossTipContainer struct {
		bossTipIds *[]uint32
		bossTipMap map[uint32]struct{}
	}
)

func NewBossTipContainer(bossTipIds *[]uint32) *BossTipContainer {
	container := new(BossTipContainer)
	container.bossTipIds = bossTipIds
	container.bossTipMap = make(map[uint32]struct{})
	for _, id := range *bossTipIds {
		container.bossTipMap[id] = struct{}{}
	}
	return container
}

func (container *BossTipContainer) CheckBossTip(bossId uint32) bool {
	var _, ok = container.bossTipMap[bossId]
	return ok
}

func (container *BossTipContainer) ChangeTip(bossId uint32, need bool) {
	var _, ok = container.bossTipMap[bossId]
	if ok == need {
		return
	}
	if need {
		container.bossTipMap[bossId] = struct{}{}
		*(container.bossTipIds) = append(*(container.bossTipIds), bossId)
	} else {
		delete(container.bossTipMap, bossId)
		for pos, tBossId := range *container.bossTipIds {
			if tBossId == bossId {
				*(container.bossTipIds) = append((*container.bossTipIds)[:pos], (*container.bossTipIds)[pos+1:]...)
				break
			}
		}
	}
}
