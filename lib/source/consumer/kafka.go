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

package consumer

import (
	"context"
	"github.com/SENERGY-Platform/process-model-repository/lib/contextwg"
	"github.com/SENERGY-Platform/process-model-repository/lib/source/util"
	"github.com/segmentio/kafka-go"
	"io"
	"io/ioutil"
	"log"
	"sync"
	"time"
)

func NewConsumer(ctx context.Context, bootstrapUrl string, groupid string, topic string, listener func(topic string, msg []byte) error, errorhandler func(err error, consumer *Consumer)) (consumer *Consumer, err error) {
	consumer = &Consumer{ctx: ctx, groupId: groupid, bootstrapUrl: bootstrapUrl, topic: topic, listener: listener, errorhandler: errorhandler}
	err = consumer.start()
	return
}

type Consumer struct {
	count        int
	bootstrapUrl string
	groupId      string
	topic        string
	ctx          context.Context
	listener     func(topic string, msg []byte) error
	errorhandler func(err error, consumer *Consumer)
	mux          sync.Mutex
}

func (this *Consumer) start() error {
	log.Println("DEBUG: consume topic: \"" + this.topic + "\"")
	broker, err := util.GetBroker(this.bootstrapUrl)
	if err != nil {
		log.Println("ERROR: unable to get broker list", err)
		return err
	}
	err = util.InitTopic(this.bootstrapUrl, this.topic)
	if err != nil {
		log.Println("WARNING: unable to create topic", err)
		err = nil
	}
	r := kafka.NewReader(kafka.ReaderConfig{
		CommitInterval: 0, //synchronous commits
		Brokers:        broker,
		GroupID:        this.groupId,
		Topic:          this.topic,
		MaxWait:        1 * time.Second,
		Logger:         log.New(ioutil.Discard, "", 0),
		ErrorLogger:    log.New(ioutil.Discard, "", 0),
	})
	contextwg.Add(this.ctx, 1)
	go func() {
		defer contextwg.Done(this.ctx)
		for {
			select {
			case <-this.ctx.Done():
				log.Println("close kafka reader:", this.topic, r.Close())
				return
			default:
				m, err := r.FetchMessage(this.ctx)
				if err == io.EOF || err == context.Canceled {
					log.Println("close consumer for topic ", this.topic)
					return
				}
				if err != nil {
					log.Println("ERROR: while consuming topic ", this.topic, err)
					this.errorhandler(err, this)
					return
				}
				err = this.listener(m.Topic, m.Value)
				if err != nil {
					log.Println("ERROR: unable to handle message (no commit)", err)
				} else {
					err = r.CommitMessages(this.ctx, m)
					if err != nil {
						log.Println("ERROR: unable to commit message consumption", err)
					}
				}
			}
		}
	}()
	return err
}
