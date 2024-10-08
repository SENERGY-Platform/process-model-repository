/*
 * Copyright 2024 InfAI (CC SES)
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
	"github.com/SENERGY-Platform/permissions-v2/pkg/client"
	model2 "github.com/SENERGY-Platform/permissions-v2/pkg/model"
	"github.com/SENERGY-Platform/process-model-repository/lib"
	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/SENERGY-Platform/process-model-repository/lib/contextwg"
	"github.com/SENERGY-Platform/process-model-repository/lib/model"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestList(t *testing.T) {
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

	_, zkIp, err := Zookeeper(ctx, wg)
	if err != nil {
		t.Error(err)
		return
	}
	zookeeperUrl := zkIp + ":2181"

	conf.KafkaUrl, err = Kafka(ctx, wg, zookeeperUrl)
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

	err = lib.Start(ctx, conf)
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(2 * time.Second)

	var p1 model.Process
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

	var p2 model.Process
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

	var p3 model.Process
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

	var p4 model.Process
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

	time.Sleep(5 * time.Second)

	_, err, _ = client.New(conf.PermissionsV2Url).SetPermission(client.InternalAdminToken, conf.ProcessTopic, p2.Id, client.ResourcePermissions{
		UserPermissions: map[string]model2.PermissionsMap{
			userid1: {Read: true, Write: true, Execute: true, Administrate: true},
			userid2: {Read: true, Write: true, Execute: true, Administrate: true},
		},
	})
	if err != nil {
		t.Error(err)
		return
	}

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

	t.Run("list all for admin", func(t *testing.T) {
		testList(client.InternalAdminToken, "/v2/processes", []model.Process{p1, p3, p2, p4})
	})

	t.Run("list for owner", func(t *testing.T) {
		testList(userjwt1, "/v2/processes", []model.Process{p1, p3, p2, p4})
	})

	t.Run("list for user2", func(t *testing.T) {
		testList(userjwt2, "/v2/processes", []model.Process{p2})
	})

	t.Run("sort", func(t *testing.T) {
		testList(userjwt1, "/v2/processes?sort=name.desc", []model.Process{p4, p2, p3, p1})
	})

	t.Run("limit offset", func(t *testing.T) {
		testList(userjwt1, "/v2/processes?limit=2&offset=1", []model.Process{p3, p2})
	})

	t.Run("ids", func(t *testing.T) {
		ids := strings.Join([]string{p3.Id, p4.Id}, ",")
		testList(userjwt1, "/v2/processes?ids="+url.QueryEscape(ids), []model.Process{p3, p4})
	})

	t.Run("search a", func(t *testing.T) {
		testList(userjwt1, "/v2/processes?search=a", []model.Process{p1, p3})
	})

	t.Run("search b", func(t *testing.T) {
		testList(userjwt1, "/v2/processes?search=b", []model.Process{p2, p4})
	})

	t.Run("search x", func(t *testing.T) {
		testList(userjwt1, "/v2/processes?search=x", []model.Process{p1, p4})
	})

	t.Run("search y", func(t *testing.T) {
		testList(userjwt1, "/v2/processes?search=y", []model.Process{p3, p2})
	})

	t.Run("search 1", func(t *testing.T) {
		testList(userjwt1, "/v2/processes?search=1", []model.Process{p1, p2})
	})

	t.Run("search 2", func(t *testing.T) {
		testList(userjwt1, "/v2/processes?search=2", []model.Process{p3, p4})
	})

	t.Run("search a 1", func(t *testing.T) {
		testList(userjwt1, "/v2/processes?search="+url.QueryEscape("a 1"), []model.Process{p1})
	})

}
