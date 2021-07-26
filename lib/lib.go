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

package lib

import (
	"context"
	"github.com/SENERGY-Platform/process-model-repository/lib/api"
	"github.com/SENERGY-Platform/process-model-repository/lib/com"
	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/SENERGY-Platform/process-model-repository/lib/controller"
	"github.com/SENERGY-Platform/process-model-repository/lib/database"
	"github.com/SENERGY-Platform/process-model-repository/lib/source/consumer"
	"github.com/SENERGY-Platform/process-model-repository/lib/source/producer"
	"log"
)

/*
Start initializes
	kafka consumer
	kafka producer
	mongodb connection
	api server

on basectx.Done() all connections will be closed assync

if context contains a waitgroup as value (github.com/SENERGY-Platform/process-model-repository/lib/contextwg)
the waitgroup is informed about successfull close/disconnect
*/
func Start(basectx context.Context, conf config.Config) (err error) {
	ctx, cancel := context.WithCancel(basectx)
	defer func() {
		if err != nil {
			cancel()
		}
	}()
	db, err := database.New(ctx, conf)
	if err != nil {
		log.Println("ERROR: unable to connect to database", err)
		return err
	}

	perm, err := com.NewSecurity(conf)
	if err != nil {
		log.Println("ERROR: unable to create permission handler", err)
		return err
	}

	p, err := producer.New(ctx, conf)
	if err != nil {
		log.Println("ERROR: unable to create producer", err)
		return err
	}

	ctrl, err := controller.New(conf, db, perm, p)
	if err != nil {
		log.Println("ERROR: unable to start control", err)
		return err
	}

	err = consumer.Start(ctx, conf, ctrl)
	if err != nil {
		log.Println("ERROR: unable to start source", err)
		return err
	}

	api.Start(ctx, conf, ctrl)
	return
}
