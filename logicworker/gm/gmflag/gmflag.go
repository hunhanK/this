/**
 * @Author: HeXinLi
 * @Desc:
 * @Date: 2022/10/18 19:36
 */

package gmflag

import (
	"jjyz/base/pb3"
	"jjyz/gameserver/gshare"
)

func GetGmCmdData() *pb3.GmCmdData {
	sst := gshare.GetStaticVar()
	if nil == sst.GmCmdData {
		sst.GmCmdData = new(pb3.GmCmdData)
	}
	return sst.GmCmdData
}
