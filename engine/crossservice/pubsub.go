package crossservice

import (
	"context"
	"encoding/json"
	"fmt"
	"jjyz/base/mqdef"
	"jjyz/base/pb3"
	"jjyz/base/pubsub"

	"github.com/gzjjyz/logger"
	uuid "github.com/satori/go.uuid"
)

func PubJsonNats(opcode pb3.CrossServiceNatsOpCode, data interface{}) error {
	logger.LogDebug("publish cross service nats")

	buf, err := json.Marshal(data)
	if nil != err {
		return err
	}

	msg := &pb3.CrossServiceGm{
		Id:   uuid.NewV4().String(),
		Op:   opcode,
		Data: buf,
	}

	bt, _ := json.Marshal(msg)

	err = pubsub.Publish(context.Background(), &pubsub.PublishRequest{
		Topic:       mqdef.CrossServiceGmTopic,
		Data:        bt,
		Persistence: false,
	})

	if err != nil {
		return fmt.Errorf("publish cross service gm message error! %v", err)
	}

	return nil
}
