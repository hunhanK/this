/**
 * @Author: ChenJunJi
 * @Desc:
 * @Date: 2021/9/26 13:58
 */

package gateworker

import (
	"sync"
)

var (
	UserIdMap sync.Map
	BootMap   = make(map[uint64]*BootObj)
)

func GetGateUserByUserId(id uint32) *UserSt {
	tmp, ok := UserIdMap.Load(id)
	if !ok {
		return nil
	}
	if user, ok := tmp.(*UserSt); ok {
		return user
	}
	return nil
}

func AddGateUser(user *UserSt) {
	if nil == user {
		return
	}
	UserIdMap.Store(user.UserId, user)
}

func DelGateUserByUserId(id uint32) {
	UserIdMap.Delete(id)
}
