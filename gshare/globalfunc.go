package gshare

import (
	"jjyz/gameserver/redisworker/redismid"
)

var (
	SendRedisMsg func(id redismid.RedisMID, params ...interface{})
)
