package producer

import (
	"context"
	"encoding/json"
	"github.com/segmentio/kafka-go"
	"log"
	"runtime/debug"
	"time"
)

type PermCommandMsg struct {
	Command  string `json:"command"`
	Kind     string
	Resource string
	User     string
	Group    string
	Right    string
}

func (this *Producer) PublishDeleteUserRights(resource string, id string, userId string) error {
	cmd := PermCommandMsg{
		Command:  "DELETE",
		Kind:     resource,
		Resource: id,
		User:     userId,
	}
	if this.config.LogLevel == "DEBUG" {
		log.Println("DEBUG: produce Location", cmd)
	}
	message, err := json.Marshal(cmd)
	if err != nil {
		debug.PrintStack()
		return err
	}
	err = this.permissions.WriteMessages(
		context.Background(),
		kafka.Message{
			Key:   []byte(userId + "_" + resource + "_" + id),
			Value: message,
			Time:  time.Now(),
		},
	)
	if err != nil {
		debug.PrintStack()
	}
	return err
}
