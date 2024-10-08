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
	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/SENERGY-Platform/process-model-repository/lib/contextwg"
	"github.com/segmentio/kafka-go"
	"io"
	"log"
	"os"
)

type Producer struct {
	config  config.Config
	process *kafka.Writer
}

func New(ctx context.Context, conf config.Config) (*Producer, error) {
	process, err := getProducer(conf.KafkaUrl, conf.ProcessTopic, conf.LogLevel == "DEBUG")
	if err != nil {
		return nil, err
	}
	contextwg.Add(ctx, 1)
	go func() {
		<-ctx.Done()
		log.Println("close kafka producer:", process.Close())
		contextwg.Done(ctx)
	}()
	return &Producer{config: conf, process: process}, nil
}

func getProducer(broker string, topic string, debug bool) (writer *kafka.Writer, err error) {
	var logger *log.Logger
	if debug {
		logger = log.New(os.Stdout, "[KAFKA-PRODUCER] ", 0)
	} else {
		logger = log.New(io.Discard, "", 0)
	}
	writer = &kafka.Writer{
		Addr:        kafka.TCP(broker),
		Topic:       topic,
		Logger:      logger,
		MaxAttempts: 10,
		Async:       false,
		BatchSize:   1,
		Balancer:    &kafka.Hash{},
		ErrorLogger: log.New(os.Stderr, "KAFKA", 0),
	}
	return writer, err
}
