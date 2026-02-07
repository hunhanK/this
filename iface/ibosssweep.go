/**
 * @Author: LvYuMeng
 * @Date: 2025/1/9
 * @Desc:
**/

package iface

import "jjyz/base/jsondata"

type IBossSweep interface {
	BossSweepChecker(monId uint32) bool
	BossSweepSettle(monId, sceneId uint32, rewards jsondata.StdRewardVec) bool
}
