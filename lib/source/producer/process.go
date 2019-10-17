/*
 * Copyright 2019 InfAI (CC SES)
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package producer

import (
	"context"
	"encoding/json"
	"github.com/SENERGY-Platform/process-model-repository/lib/model"
	"github.com/segmentio/kafka-go"
	"log"
	"runtime/debug"
	"time"
)

type ProcessCommand struct {
	Command      string        `json:"command"`
	Id           string        `json:"id"`
	Owner        string        `json:"owner"`
	Processmodel model.Process `json:"processmodel"`
}

func (this *Producer) PublishProcessDelete(id string, userId string) error {
	cmd := ProcessCommand{Command: "DELETE", Id: id, Owner: userId}
	return this.PublishProcessCommand(cmd)
}

func (this *Producer) PublishProcessPut(id string, userId string, process model.Process) error {
	cmd := ProcessCommand{Command: "PUT", Id: id, Owner: userId, Processmodel: process}
	return this.PublishProcessCommand(cmd)
}

func (this *Producer) PublishProcessCommand(cmd ProcessCommand) error {
	if this.config.LogLevel == "DEBUG" {
		log.Println("DEBUG: produce process", cmd)
	}
	message, err := json.Marshal(cmd)
	if err != nil {
		debug.PrintStack()
		return err
	}
	err = this.process.WriteMessages(
		context.Background(),
		kafka.Message{
			Key:   []byte(cmd.Id),
			Value: message,
			Time:  time.Now(),
		},
	)
	if err != nil {
		log.Println("ERROR:", err)
		debug.PrintStack()
	}
	return err
}
