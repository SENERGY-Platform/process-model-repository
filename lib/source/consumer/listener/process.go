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

package listener

import (
	"encoding/json"
	"errors"
	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/SENERGY-Platform/process-model-repository/lib/source/producer"
)

func init() {
	Factories = append(Factories, ProcessListenerFactory)
}

func ProcessListenerFactory(config config.Config, control Controller) (topic string, listener Listener, err error) {
	return config.ProcessTopic, func(msg []byte) (err error) {
		command := producer.ProcessCommand{}
		err = json.Unmarshal(msg, &command)
		if err != nil {
			return
		}
		switch command.Command {
		case "PUT":
			return control.SetProcess(command.Processmodel)
		case "DELETE":
			return control.DeleteProcess(command.Id)
		case "RIGHTS":
			return nil
		}
		return errors.New("unable to handle command: " + string(msg))
	}, nil
}
