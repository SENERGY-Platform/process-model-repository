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
	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/SENERGY-Platform/process-model-repository/lib/source/consumer/listener"
	"log"
)

func Start(config config.Config, control listener.Controller) (stop func(), err error) {
	closer := []func(){}
	stop = func() {
		for _, c := range closer {
			c()
		}
	}
	for _, factory := range listener.Factories {
		topic, handler, err := factory(config, control)
		if err != nil {
			log.Println("ERROR: listener.factory", topic, err)
			return stop, err
		}
		consumer, err := NewConsumer(config.ZookeeperUrl, config.GroupId, topic, func(topic string, msg []byte) error {
			if config.Debug {
				log.Println("DEBUG: consume", topic, string(msg))
			}
			return handler(msg)
		}, func(err error, consumer *Consumer) {
			log.Fatal(err)
		})
		if err != nil {
			stop()
			return stop, err
		}
		closer = append(closer, consumer.Stop)
	}
	return stop, err
}
