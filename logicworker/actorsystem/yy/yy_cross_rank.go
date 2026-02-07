/**
 * @Author: LvYuMeng
 * @Date: 2025/4/21
 * @Desc:
**/

package yy

import (
	"github.com/gzjjyz/logger"
	"jjyz/base"
	"jjyz/base/custom_id/sysfuncid"
	"jjyz/base/pb3"
	"jjyz/gameserver/engine"
)

// yyCrossRankActInfoSync 同步运营活动开启信息
func yyCrossRankActInfoSync(info *pb3.G2CSyncYYCrossRankOpen) {
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CSyncYYCrossRankOpen, info)

	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
}

// yyCrossRankDataSync 同步排行数据
func yyCrossRankDataSync(info *pb3.G2CSyncYYCrossRankValue) {
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CSyncYYCrossRankValue, info)

	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
}

// yyCrossRankInfoReq 请求跨服排行
func yyCrossRankInfoReq(info *pb3.G2CReqYYCrossRankInfo) {
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CReqYYCrossRankInfo, info)

	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
}

// yyCrossRankInfoReq 请求结算Fra
func yyCrossRankAwardsCalc(info *pb3.G2CCalcRankAwards) {
	err := engine.CallFightSrvFunc(base.SmallCrossServer, sysfuncid.G2CCalcRankAwards, info)

	if err != nil {
		logger.LogError("err:%v", err)
		return
	}
}
