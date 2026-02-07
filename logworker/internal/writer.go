package internal

import (
	"errors"
	"fmt"
	"github.com/gzjjyz/counter-go-sdk/counter"
	"github.com/gzjjyz/logger"
	micro "github.com/gzjjyz/simple-micro"
	"jjyz/base/cmd"
	"jjyz/base/pb3"
)

var (
	producer counter.Producer
)

func Init() error {
	cfg, flag := micro.MustMeta().GetRedisConn("log")
	if !flag {
		return errors.New("meta can not fight redis.log config")
	}
	var err error
	producer, err = counter.NewRedisProducer(counter.RedisConfig{
		Host:     cfg.Host,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	return err
}

func Flush() {
	if nil != producer {
		producer.Close()
	}
}

func PostLogData(cmdId cmd.GLConst, msg pb3.Message) {
	buf, err := pb3.Marshal(msg)
	if nil != err {
		logger.LogError("post log data error!! cmdId=%d, pb2:%v", cmdId, err)
		return
	}
	if err = producer.Add(cmdId.String(), buf); nil != err {
		logger.LogError("log counter producer add msg error! %v", err)
	}
}

func PostLogDataWithSuffix(pf uint32, topic string, msg pb3.Message) {
	topic = fmt.Sprintf("%s_%d", topic, pf)
	buf, err := pb3.Marshal(msg)
	if nil != err {
		logger.LogError("post log data error!! topic=%s, pb2:%v", topic, err)
		return
	}
	if err = producer.Add(topic, buf); nil != err {
		logger.LogError("log counter producer add msg error! %v", err)
	}
}
