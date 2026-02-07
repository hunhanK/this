/**
 * @Author: LvYuMeng
 * @Date: 2024/3/7
 * @Desc:
**/

package pyy

import (
	"jjyz/base/custom_id/yydefine"
	"jjyz/gameserver/iface"
	"jjyz/gameserver/logicworker/actorsystem/pyymgr"
)

type YYPreviewSys struct {
	PlayerYYBase
}

func (s *YYPreviewSys) Login() {
}

func (s *YYPreviewSys) OnReconnect() {
}

func init() {
	pyymgr.RegPlayerYY(yydefine.YYPreview, func() iface.IPlayerYY {
		return &YYPreviewSys{}
	})
}
