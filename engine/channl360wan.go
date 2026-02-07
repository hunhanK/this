/**
 * @Author: zjj
 * @Date: 2025/2/28
 * @Desc:
**/

package engine

import (
	"github.com/gzjjyz/srvlib/utils"
	"strings"
)

const (
	Uni360AppId     = 30002 // 应用ID
	Uni360DitchId   = 13000 // exe微端 渠道ID
	Uni360OLDitchId = 13100 // 网页 渠道ID
	Uni360KeyWan    = uint64(Uni360DitchId) | uint64(Uni360AppId)<<32
	Uni360KeyWanOL  = uint64(Uni360OLDitchId) | uint64(Uni360AppId)<<32
)

type _360AppInfo struct {
	Gkey     string
	LoginKey string
	Dept     int64
	Gid      int64
}

var _360AppInfoMap = map[uint64]*_360AppInfo{
	Uni360KeyWan: {
		Gkey:     "lxzj",
		LoginKey: "lTe9KJbLlEzb5mlBwq639z8p5qKPAXAs",
		Dept:     38,
		Gid:      2098,
	},
	Uni360KeyWanOL: {
		Gkey:     "lxzjol",
		LoginKey: "f0ada1b2931422aed0ca5b38181808e2",
		Dept:     38,
		Gid:      2230,
	},
}

var (
	GMSet360Wan     bool
	GMSet360DitchId uint32
	GMSet360AppId   uint32
)

func Is360Wan(ditchId uint32) bool {
	if GMSet360Wan {
		return true
	}
	_, ok := _360AppInfoMap[utils.Make64(ditchId, GetAppId())]
	if ok {
		return true
	}
	return false
}

func Get360WanInfo(ditchId uint32) *_360AppInfo {
	if GMSet360Wan {
		return _360AppInfoMap[utils.Make64(GMSet360DitchId, GMSet360AppId)]
	}
	info, ok := _360AppInfoMap[utils.Make64(ditchId, GetAppId())]
	if !ok {
		return nil
	}
	return info
}

func SetGM360Wan(gMSet360DitchId, gMSet360AppId uint32) {
	GMSet360Wan = !GMSet360Wan
	GMSet360DitchId = gMSet360DitchId
	GMSet360AppId = gMSet360AppId
}

func Get360WanUserId(account string) string {
	var platformUniquePlayerId = account
	split := strings.Split(account, "_")
	if len(split) > 0 {
		platformUniquePlayerId = split[len(split)-1]
	}
	return platformUniquePlayerId
}
