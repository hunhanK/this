/**
 * @Author: PengZiMing
 * @Desc: 处理任务进度的逻辑
 * @Date: 2022/6/8 10:17
 */

package quest

import (
	"jjyz/base/custom_id"
	"jjyz/base/jsondata"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/iface"

	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/srvlib/utils"
)

type ProgressBase struct {
	SkippedCheckProgress bool
}

func (p *ProgressBase) AcceptQuestInitProgress(actor iface.IPlayer, progress []uint32, targets []*jsondata.QuestTargetConf) []uint32 {
	progress = make([]uint32, len(targets))
	for idx, line := range targets {
		qtt := line.Type
		if qtt <= 0 || qtt > custom_id.QttMax {
			logger.LogStack("任务目标类型超出范围")
			continue
		}
		var recordCount uint32
		def := custom_id.QttVec[qtt]
		if def.IsRecord {
			recordCount = actor.GetTaskRecord(line.Type, line.Ids)
			logger.LogDebug("got quest record, type:%d id:%+v, count:%d", line.Type, line.Ids, recordCount)
		} else {
			recordCount, _ = engine.GetQuestTargetProgress(actor, line.Type, line.Ids)
		}
		if recordCount > 0 {
			if recordCount >= line.Count {
				progress[idx] = line.Count
			} else {
				progress[idx] = recordCount
			}
		}
	}
	return progress
}

func (p *ProgressBase) CheckProgress(progress []uint32, targets []*jsondata.QuestTargetConf) bool {
	if p.SkippedCheckProgress {
		return true
	}
	len1, len2 := len(progress), len(targets)
	if len1 < len2 {
		return false
	}
	for idx, target := range targets {
		if progress[idx] < target.Count {
			return false
		}
	}
	return true
}

func (p *ProgressBase) QuestEventProgress(progress []uint32, targets []*jsondata.QuestTargetConf,
	qt, id, count uint32, add bool) ([]uint32, bool, bool) {

	change, checkFinish := false, false
	for idx, target := range targets {
		if target.Type != qt {
			continue //不是当前目标
		}
		if len(target.Ids) > 0 && !utils.SliceContainsUint32(target.Ids, id) {
			continue
		}
		var old uint32 = 0
		len1 := len(progress)
		if len1 > idx {
			old = progress[idx]
			if old >= target.Count {
				continue
			}
		} else {
			len2 := len(targets)
			for i := len1; i < len2; i++ {
				progress = append(progress, 0)
			}
		}

		val := count
		if add {
			val += old
		}
		change = old != val
		if val >= target.Count {
			progress[idx] = target.Count
			checkFinish = true
		} else {
			progress[idx] = val
		}
	}
	return progress, change, checkFinish
}

func (p *ProgressBase) QuestEventRangeProgress(progress []uint32, targets []*jsondata.QuestTargetConf,
	qtt, tVal, preVal, qtype uint32) ([]uint32, bool, bool) {

	change, checkFinish := false, false
	for idx, target := range targets {
		if target.Type != qtt {
			continue //不是当前目标
		}
		if len(target.Ids) > 0 && qtype != custom_id.QTYPE_CHANGE && tVal < target.Ids[0] {
			continue
		}

		var old uint32 = 0
		len1 := len(progress)
		if len1 > idx {
			old = progress[idx]
			if old >= target.Count {
				continue
			}
		} else {
			len2 := len(targets)
			for i := len1; i < len2; i++ {
				progress = append(progress, 0)
			}
		}

		val := old
		switch qtype {
		case custom_id.QTYPE_ADD:
			val += 1
		case custom_id.QTYPE_DEL:
			if val < 1 {
				continue
			}
			val -= 1
		case custom_id.QTYPE_CHANGE:
			if preVal < target.Ids[0] {
				if tVal >= target.Ids[0] {
					val += 1
				}
			} else {
				if tVal < target.Ids[0] {
					if val >= 1 {
						continue
					}
					val -= 1
				}
			}
		default:
			continue
		}
		change = old != val
		if val >= target.Count {
			progress[idx] = target.Count
			checkFinish = true
		} else {
			progress[idx] = val
		}
	}
	return progress, change, checkFinish
}

func (p *ProgressBase) QuestEventRangeProgress2(progress []uint32, targets []*jsondata.QuestTargetConf,
	actor iface.IPlayer, qtt uint32, args ...interface{}) ([]uint32, bool, bool) {

	change, checkFinish := false, false
	for idx, target := range targets {
		if target.Type != qtt {
			continue //不是当前目标
		}
		var old uint32 = 0
		len1 := len(progress)
		if len1 > idx {
			old = progress[idx]
			if old >= target.Count {
				continue
			}
		} else {
			len2 := len(targets)
			for i := len1; i < len2; i++ {
				progress = append(progress, 0)
			}
		}
		val, _ := engine.GetQuestTargetProgress(actor, qtt, target.Ids, args...)
		if !change {
			change = old != val
		}
		if val >= target.Count {
			progress[idx] = target.Count
			checkFinish = true
		} else {
			progress[idx] = val
		}
	}
	return progress, change, checkFinish
}
