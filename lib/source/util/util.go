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

package util

import (
	"github.com/segmentio/kafka-go"
	"net"
	"strconv"
)

func GetBroker(bootstrapUrl string) (brokers []string, err error) {
	return getBroker(bootstrapUrl)
}

func getBroker(bootstrapUrl string) (result []string, err error) {
	conn, err := kafka.Dial("tcp", bootstrapUrl)
	if err != nil {
		return result, err
	}
	defer conn.Close()
	brokers, err := conn.Brokers()
	if err != nil {
		return result, err
	}
	for _, broker := range brokers {
		result = append(result, net.JoinHostPort(broker.Host, strconv.Itoa(broker.Port)))
	}
	return result, nil
}

func InitTopic(bootstrapUrl string, topics ...string) (err error) {
	conn, err := kafka.Dial("tcp", bootstrapUrl)
	if err != nil {
		return err
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return err
	}
	var controllerConn *kafka.Conn
	controllerConn, err = kafka.Dial("tcp", net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		return err
	}
	defer controllerConn.Close()

	topicConfigs := []kafka.TopicConfig{}

	for _, topic := range topics {
		topicConfigs = append(topicConfigs, kafka.TopicConfig{
			Topic:             topic,
			NumPartitions:     1,
			ReplicationFactor: 1,
			ConfigEntries: []kafka.ConfigEntry{
				{
					ConfigName:  "retention.ms",
					ConfigValue: "-1",
				},
				{
					ConfigName:  "retention.bytes",
					ConfigValue: "-1",
				},
				{
					ConfigName:  "cleanup.policy",
					ConfigValue: "compact",
				},
				{
					ConfigName:  "delete.retention.ms",
					ConfigValue: "86400000",
				},
				{
					ConfigName:  "segment.ms",
					ConfigValue: "604800000",
				},
				{
					ConfigName:  "min.cleanable.dirty.ratio",
					ConfigValue: "0.1",
				},
			},
		})
	}
	return controllerConn.CreateTopics(topicConfigs...)
}
