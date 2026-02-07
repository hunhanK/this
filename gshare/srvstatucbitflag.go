/**
 * @Author: zjj
 * @Date: 2025/2/17
 * @Desc:
**/

package gshare

import "github.com/gzjjyz/srvlib/utils"

const (
	SrvStatusByCanMerge        uint32 = 1 // 可以合服状态
	SrvStatusByConnSmallCross  uint32 = 2 // 连接小跨服中
	SrvStatusByConnMediumCross uint32 = 3 // 连接中跨服中
)

func GetSrvStatusFlag() uint64 {
	staticVar := GetStaticVar()
	return staticVar.SrvStatusFlag
}

func SetSrvStatusFlag(flag uint32) {
	staticVar := GetStaticVar()
	staticVar.SrvStatusFlag = utils.SetBit64(staticVar.SrvStatusFlag, flag)
}

func IsSrvStatusFlag(flag uint32) bool {
	staticVar := GetStaticVar()
	return utils.IsSetBit64(staticVar.SrvStatusFlag, flag)
}

func ClearSrvStatusFlag(flag uint32) {
	staticVar := GetStaticVar()
	staticVar.SrvStatusFlag = utils.ClearBit64(staticVar.SrvStatusFlag, flag)
}
