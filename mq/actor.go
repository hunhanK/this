package mq

import (
	"context"
	"fmt"
	"jjyz/base/mqdef"
	"jjyz/base/pb3"
	"jjyz/base/pubsub"
	"jjyz/gameserver/gshare"
	"jjyz/gameserver/logicworker/manager"

	"github.com/gzjjyz/logger"
)

var (
	topic string
)

func PubCrossFightPlayerDataMqHandle(pfId uint32, srvId uint32, actorId uint64, data []byte) error {
	logger.LogDebug("pub msg , srvId is %v , actorId is %v", srvId, actorId)

	m := &pb3.CrossFightPlayerDataMqMsg{
		SrvId:    srvId,
		ActorId:  actorId,
		PfId:     pfId,
		BaseData: manager.GetData(actorId, gshare.ActorDataBase).(*pb3.PlayerDataBase),
	}

	buf, err := pb3.Marshal(m)
	if err != nil {
		logger.LogError("err:%v", err)
		return err
	}

	if len(topic) <= 0 {
		topic = fmt.Sprintf(mqdef.PlayerBaseDataTopic, pfId, srvId)
	}

	err = pubsub.Publish(context.Background(), &pubsub.PublishRequest{
		Topic:       topic,
		Data:        buf,
		Persistence: false,
	})
	if err != nil {
		return fmt.Errorf("publish msg to nats occur err:%v", err)
	}

	return nil
}
