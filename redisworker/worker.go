package redisworker

import (
	"context"
	"errors"
	"github.com/gzjjyz/logger"
	"github.com/gzjjyz/safeworker"
	micro "github.com/gzjjyz/simple-micro"
	"github.com/redis/go-redis/v9"
	"jjyz/base/custom_id"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/gshare/event"
	"jjyz/gameserver/redisworker/redismid"
	"time"
)

var (
	client *redis.Client

	router   = safeworker.NewRouter(time.Millisecond * 5)
	Register = func(id redismid.RedisMID, cb safeworker.MsgHdlType) {
		router.Register(safeworker.MsgIdType(id), cb)
	}
)

func NewRedisWorker() (worker *safeworker.Worker, err error) {
	config, _ := micro.MustMeta().GetRedisConn("data")
	if nil == config {
		return nil, errors.New("redis data config is nil")
	}

	client = redis.NewClient(&redis.Options{
		Addr:     config.Host,
		Password: config.Password,
		DB:       config.DB,
	})
	if err = client.Ping(context.Background()).Err(); nil != err {
		return nil, err
	}

	worker, err = safeworker.NewWorker(
		safeworker.WithName("redisWorker"),
		safeworker.WithBeforeLoop(func() {
			event.TriggerSysEvent(custom_id.SeRedisWorkerInitDone)
		}),
		safeworker.WithLoopFunc(func() {}),
		safeworker.WithChSize(10000),
		safeworker.WithRouter(router))

	if nil != err {
		logger.LogError("new redis worker error. %v", err)
	}

	gshare.SendRedisMsg = func(id redismid.RedisMID, params ...interface{}) {
		worker.SendMsg(safeworker.MsgIdType(id), params...)
	}
	return
}
