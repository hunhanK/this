/**
 * @Author: lzp
 * @Date: 2024/7/18
 * @Desc:
**/

package beasttidemgr

import (
	"jjyz/base/custom_id/yydefine"
	pyy "jjyz/gameserver/logicworker/actorsystem/yy"
	"jjyz/gameserver/logicworker/actorsystem/yymgr"
)

func AutoDonateBeastTide() {
	allYY := yymgr.GetAllYY(yydefine.YYBeastTide)
	for _, v := range allYY {
		if sys, ok := v.(*pyy.YYBeastTide); ok && sys.IsOpen() {
			sys.AutoDonateBeastTide()
		}
	}
}
func AutoDonateDemonSubduing() {
	allYY := yymgr.GetAllYY(yydefine.YYDemonSubduing)
	for _, v := range allYY {
		if sys, ok := v.(*pyy.DemonSubduing); ok && sys.IsOpen() {
			sys.AutoDonateDemonSubduing()
		}
	}
}

func GetBeastTideAddition(effType uint32) int64 {
	allYY := yymgr.GetAllYY(yydefine.YYBeastTide)
	for _, v := range allYY {
		if sys, ok := v.(*pyy.YYBeastTide); ok && sys.IsOpen() {
			return sys.GetBeastTideAddition(effType)
		}
	}
	return 0
}
