/**
 * @Author: LvYuMeng
 * @Date: 2025/5/12
 * @Desc:
**/

package pyy

import (
	"jjyz/base/custom_id/yydefine"
	"jjyz/base/jsondata"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
)

type YYDrawRateModifySys struct {
	PlayerYYBase
}

func (s *YYDrawRateModifySys) Login() {
}

func (s *YYDrawRateModifySys) OnReconnect() {
}

func (s *YYDrawRateModifySys) GetChangeLibConf(libId uint32) *jsondata.LotteryLibConf {
	conf := jsondata.GetYYDrawRateModifyConf(s.ConfName, s.ConfIdx)
	if nil == conf {
		return nil
	}

	rateConf, ok := conf.LibConf[libId]
	if !ok {
		return nil
	}

	libConf := jsondata.ShallowCopyLotteryLibConf(libId)
	if nil == libConf {
		return nil
	}

	libConf.Rate = rateConf.Rate
	libConf.MaxLucky = rateConf.MaxLucky
	libConf.MinLucky = rateConf.MinLucky
	libConf.StageCount = rateConf.StageCount
	libConf.MaxStageCount = rateConf.MaxStageCount
	libConf.MaxRoundCount = rateConf.MaxRoundCount

	return libConf
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYDrawRateModify, func() iface.IPlayerYY {
		return &YYDrawRateModifySys{}
	})
}
