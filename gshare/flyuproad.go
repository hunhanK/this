/**
 * @Author: zjj
 * @Date: 2023/11/22
 * @Desc:
**/

package gshare

const (
	// 一个基数 用于计算登仙榜得分
	BaseFlyUpRoadScore = uint32(60 * 60 * 24 * 365 * 2)
)

func GetFlyUpRoadBastTimeAt() uint32 {
	return GetOpenServerTime() + BaseFlyUpRoadScore
}
