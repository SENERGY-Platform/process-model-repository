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
	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/SENERGY-Platform/process-model-repository/lib/controller"
	"github.com/SENERGY-Platform/process-model-repository/lib/database"
	"github.com/SENERGY-Platform/process-model-repository/lib/source/consumer"
	"log"
	"time"
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
	_, _, err = StartGetInternals(basectx, conf)
	return err
}

func StartGetInternals(basectx context.Context, conf config.Config) (db database.Database, ctrl *controller.Controller, err error) {
	ctx, cancel := context.WithCancel(basectx)
	defer func() {
		if err != nil {
			cancel()
		}
	}()
	db, err = database.New(ctx, conf)
	if err != nil {
		log.Println("ERROR: unable to connect to database", err)
		return db, ctrl, err
	}

	ctrl, err = controller.New(conf, db)
	if err != nil {
		log.Println("ERROR: unable to start control", err)
		return db, ctrl, err
	}

	cleanupInterval, err := time.ParseDuration(conf.CleanupInterval)
	if err != nil {
		log.Println("ERROR: unable to parse cleanup interval", err)
		return db, ctrl, err
	}
	ctrl.StartCleanupLoop(ctx, cleanupInterval)

	err = consumer.Start(ctx, conf, ctrl)
	if err != nil {
		log.Println("ERROR: unable to start source", err)
		return db, ctrl, err
	}

	api.Start(ctx, conf, ctrl)
	return
}
