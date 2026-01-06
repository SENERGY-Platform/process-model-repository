/*
 * Copyright 2025 InfAI (CC SES)
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

package tests

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/SENERGY-Platform/permissions-v2/pkg/client"
	"github.com/SENERGY-Platform/process-model-repository/lib"
	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/SENERGY-Platform/process-model-repository/lib/contextwg"
	"github.com/SENERGY-Platform/process-model-repository/lib/database/mongo"
	"github.com/SENERGY-Platform/process-model-repository/lib/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestMongoCheckIdList(t *testing.T) {
	conf, err := config.Load("../config.json")
	if err != nil {
		log.Fatal("ERROR: unable to load config", err)
	}
	conf.Debug = true
	conf.ConnectivityTest = false

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx = contextwg.WithWaitGroup(ctx, wg)

	_, mongoIp, err := MongoTestServer(ctx, wg)
	if err != nil {
		t.Error(err)
		return
	}

	conf.MongoUrl = "mongodb://" + mongoIp + ":27017"

	db, err := mongo.New(ctx, conf)
	if err != nil {
		t.Error(err)
		return
	}

	idList := []string{"missing_in_db"}
	expectedMissingInDb := []string{"missing_in_db"}
	expectedMissingInInput := []string{}
	t.Run("create entry with idList entry", func(t *testing.T) {
		id := "process"
		idList = append(idList, id)
		err = db.SetProcess(ctx, model.Process{
			Id:              id,
			Name:            id,
			LastUpdatedUnix: 1700000000,
		})
		if err != nil {
			t.Error(err)
			return
		}
	})
	t.Run("create entry without idList entry ", func(t *testing.T) {
		id := "process-missing-in-list"
		expectedMissingInInput = append(expectedMissingInInput, id)
		err = db.SetProcess(ctx, model.Process{
			Id:              id,
			Name:            id,
			LastUpdatedUnix: 1700000000,
		})
		if err != nil {
			t.Error(err)
			return
		}
	})
	t.Run("create fresh entry without idList entry", func(t *testing.T) {
		id := "fresh-process-missing-in-list"
		err = db.SetProcess(ctx, model.Process{
			Id:              id,
			Name:            id,
			LastUpdatedUnix: time.Now().Unix(),
		})
		if err != nil {
			t.Error(err)
			return
		}
	})
	t.Run("create legacy entry with idList entry", func(t *testing.T) {
		id := "legacy-process"
		idList = append(idList, id)
		_, err = db.ProcessCollection().ReplaceOne(ctx, bson.M{"_id": id}, bson.M{"_id": id}, options.Replace().SetUpsert(true))
		if err != nil {
			t.Error(err)
			return
		}
	})
	t.Run("create legacy entry without idList entry ", func(t *testing.T) {
		id := "legacy-process-missing-in-list"
		expectedMissingInInput = append(expectedMissingInInput, id)
		_, err = db.ProcessCollection().ReplaceOne(ctx, bson.M{"_id": id}, bson.M{"_id": id}, options.Replace().SetUpsert(true))
		if err != nil {
			t.Error(err)
			return
		}
	})

	t.Run("check", func(t *testing.T) {
		missingInDb, missingInList, err := db.CheckIdList(idList)
		if err != nil {
			t.Error(err)
			return
		}
		sort.Strings(missingInDb)
		sort.Strings(missingInList)
		sort.Strings(expectedMissingInInput)
		sort.Strings(expectedMissingInDb)
		if !reflect.DeepEqual(missingInList, expectedMissingInInput) {
			t.Errorf("\na=%#v\ne=%#v\n", missingInList, expectedMissingInInput)
		}
		if !reflect.DeepEqual(missingInDb, expectedMissingInDb) {
			t.Errorf("\na=%#v\ne=%#v\n", missingInDb, expectedMissingInDb)
		}
	})
}

func TestCleanup(t *testing.T) {
	backup := mongo.CleanupLastUpdateTimeBuffer
	mongo.CleanupLastUpdateTimeBuffer = time.Millisecond
	defer func() {
		mongo.CleanupLastUpdateTimeBuffer = backup
	}()

	conf, err := config.Load("../config.json")
	if err != nil {
		log.Fatal("ERROR: unable to load config", err)
	}
	conf.Debug = true
	conf.ConnectivityTest = false

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx = contextwg.WithWaitGroup(ctx, wg)

	_, mongoIp, err := MongoTestServer(ctx, wg)
	if err != nil {
		t.Error(err)
		return
	}

	conf.MongoUrl = "mongodb://" + mongoIp + ":27017"

	conf.KafkaUrl, err = Kafka(ctx, wg)
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(2 * time.Second)

	_, permIp, err := PermissionsV2(ctx, wg, conf.MongoUrl, conf.KafkaUrl)
	if err != nil {
		t.Error(err)
		return
	}
	conf.PermissionsV2Url = "http://" + permIp + ":8080"

	port, err := getFreePort()
	if err != nil {
		t.Error(err)
		return
	}
	conf.ServerPort = strconv.Itoa(port)

	db, ctrl, err := lib.StartGetInternals(ctx, conf)
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(time.Second)

	var p1 model.Process
	var p2 model.Process
	var p3 model.Process
	var p4 model.Process
	t.Run("create entries", func(t *testing.T) {
		err = PostJSON(userjwt1, "http://localhost:"+conf.ServerPort+"/processes",
			model.Process{
				Name:        "a 1",
				Description: "x",
				BpmnXml:     createTestXmlString("p1"),
				SvgXml:      "svg3",
			}, &p1)
		if err != nil {
			t.Error(err)
			return
		}

		err = PostJSON(userjwt1, "http://localhost:"+conf.ServerPort+"/processes", model.Process{
			Name:        "b 1",
			Description: "y",
			BpmnXml:     createTestXmlString("p2"),
			SvgXml:      "svg4",
		}, &p2)
		if err != nil {
			t.Error(err)
			return
		}

		err = PostJSON(userjwt1, "http://localhost:"+conf.ServerPort+"/processes", model.Process{
			Name:        "a 2",
			Description: "y",
			BpmnXml:     createTestXmlString("p3"),
			SvgXml:      "svg4",
		}, &p3)
		if err != nil {
			t.Error(err)
			return
		}

		err = PostJSON(userjwt1, "http://localhost:"+conf.ServerPort+"/processes", model.Process{
			Name:        "b 2",
			Description: "x",
			BpmnXml:     createTestXmlString("p4"),
			SvgXml:      "svg4",
		}, &p4)
		if err != nil {
			t.Error(err)
			return
		}
	})

	testList := func(token string, path string, expected []model.Process) func(t *testing.T) {
		return func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "http://localhost:"+conf.ServerPort+path, nil)
			if err != nil {
				t.Error(err)
				return
			}
			req.Header.Set("Authorization", token)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Error(resp.StatusCode)
				return
			}
			actual := []model.Process{}
			err = json.NewDecoder(resp.Body).Decode(&actual)
			if err != nil {
				t.Error(err)
				return
			}
			if !reflect.DeepEqual(actual, expected) {
				t.Errorf("\n%#v\n%#v\n", actual, expected)
				return
			}
		}
	}

	t.Run("list before cleanup", func(t *testing.T) {
		testList(userjwt1, "/v2/processes", []model.Process{p1, p3, p2, p4})
	})

	time.Sleep(time.Second)

	t.Run("remove p2 from permissions", func(t *testing.T) {
		err, _ = client.New(conf.PermissionsV2Url).RemoveResource(client.InternalAdminToken, conf.ProcessTopic, p2.Id)
		if err != nil {
			t.Error(err)
			return
		}
	})

	t.Run("remove p3 from db", func(t *testing.T) {
		err = db.DeleteProcess(context.Background(), p3.Id)
		if err != nil {
			t.Error(err)
			return
		}
	})

	t.Run("call cleanup", func(t *testing.T) {
		removedPermissions, removedProcesses, err := ctrl.Cleanup()
		if err != nil {
			t.Error(err)
		}
		if removedProcesses != 1 {
			t.Errorf("\na=%#v\ne=%#v\n", removedProcesses, 1)
		}
		if removedPermissions != 1 {
			t.Errorf("\na=%#v\ne=%#v\n", removedPermissions, 1)
		}
	})

	t.Run("list after cleanup", func(t *testing.T) {
		testList(userjwt1, "/v2/processes", []model.Process{p1, p4})
	})
}
