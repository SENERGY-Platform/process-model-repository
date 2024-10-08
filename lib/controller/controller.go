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

package controller

import (
	"github.com/SENERGY-Platform/permissions-v2/pkg/client"
	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/SENERGY-Platform/process-model-repository/lib/database"
)

func New(config config.Config, db database.Database, producer Producer) (ctrl *Controller, err error) {
	ctrl = &Controller{
		db:       db,
		producer: producer,
		config:   config,
		perm:     client.New(config.PermissionsV2Url),
	}
	_, err, _ = ctrl.perm.SetTopic(client.InternalAdminToken, client.Topic{
		Id:                  config.ProcessTopic,
		PublishToKafkaTopic: config.ProcessTopic,
	})
	if err != nil {
		return nil, err
	}
	err = ctrl.RunMigrations()
	if err != nil {
		return nil, err
	}
	return ctrl, nil
}

type Controller struct {
	db       database.Database
	producer Producer
	config   config.Config
	perm     client.Client
}
