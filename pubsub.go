package main

import (
	"context"
	"encoding/json"
	"fmt"
	"jjyz/base/mqdef"
	"jjyz/base/pb3"
	"jjyz/base/pubsub"
	"jjyz/gameserver/engine"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/mq"
	"jjyz/gameserver/redisworker/redismid"

	"github.com/gzjjyz/confloader"
	"github.com/gzjjyz/logger"
	micro "github.com/gzjjyz/simple-micro"
)

func loadNatsCfg(natsCfg *pubsub.Config) error {
	metaPath := micro.MustMeta().GetConfigPath()
	cfgLoader := confloader.NewLoader(metaPath, natsCfg)
	if err := cfgLoader.Load(); err != nil {
		return fmt.Errorf("load nats config error! %v", err)
	}
	return nil
}

func initPubSub(log logger.ILogger) error {
	var natsCfg pubsub.Config
	if err := loadNatsCfg(&natsCfg); err != nil {
		return err
	}

	pubsub.NewGlobalPubSub(log, pubsub.ProviderNats)

	natsCfg.Provider = pubsub.ProviderNats
	natsCfg.Nats.StreamName = mqdef.GameServerStream

	// 为每个 server 创建一个消费者
	// 确保每个 server 只消费自己的消息
	natsCfg.Nats.DurableName = fmt.Sprintf(mqdef.GameServerConsumer, engine.GetPfId(), engine.GetServerId())

	if err := pubsub.Init(context.Background(), &natsCfg); err != nil {
		return fmt.Errorf("init pubsub error! %v", err)
	}

	handler := func(ctx context.Context, msg *pubsub.Message) error {
		var m pb3.GameServerMsg
		err := json.Unmarshal(msg.Data, &m)
		if err != nil {
			logger.LogError("unmarshal msg error! %v", err)
			return nil
		}

		if err := mq.OnMQMessage(m.Op, m.Data); nil != err {
			logger.LogError("on mq message error! %v", err)
		}

		return nil
	}

	err := pubsub.Subscribe(context.Background(),
		pubsub.SubscribeRequest{
			Topic: fmt.Sprintf(mqdef.GameServerWithJs, engine.GetPfId(), engine.GetServerId()), Persistence: false,
		},
		handler)
	if err != nil {
		return err
	}

	if err := mqRegisterSmallCrossEnterHandler(); err != nil {
		return err
	}
	if err := mqRegisterMediumCrossEnterHandler(); err != nil {
		return err
	}

	return nil
}

func mqRegisterSmallCrossEnterHandler() error {
	handler := func(ctx context.Context, _ *pubsub.Message) error {
		//  收到跨服匹配通知后，到 redisworker 中获取最新的跨服信息
		gshare.SendRedisMsg(redismid.EnterSmallCross)
		return nil
	}

	err := pubsub.Subscribe(context.Background(),
		pubsub.SubscribeRequest{
			Topic: fmt.Sprintf(mqdef.GameServerSmallCross, engine.GetPfId(), engine.GetServerId()),
		},
		handler)
	if err != nil {
		return fmt.Errorf("subscribe small cross enter error! %v", err)
	}

	return nil
}

func mqRegisterMediumCrossEnterHandler() error {
	handler := func(ctx context.Context, _ *pubsub.Message) error {
		//  收到跨服匹配通知后，到 redisworker 中获取最新的跨服信息
		gshare.SendRedisMsg(redismid.EnterMediumCross)
		return nil
	}

	err := pubsub.Subscribe(context.Background(),
		pubsub.SubscribeRequest{
			Topic: fmt.Sprintf(mqdef.GameServerMediumCross, engine.GetPfId(), engine.GetServerId()),
		},
		handler)
	if err != nil {
		return fmt.Errorf("subscribe medium cross enter error! %v", err)
	}

	return nil
}
