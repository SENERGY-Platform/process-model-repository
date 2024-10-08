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
	"github.com/SENERGY-Platform/permissions-v2/pkg/client"
	"github.com/SENERGY-Platform/process-model-repository/lib"
	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/SENERGY-Platform/process-model-repository/lib/contextwg"
	"github.com/SENERGY-Platform/process-model-repository/lib/controller"
	"github.com/SENERGY-Platform/process-model-repository/lib/database"
	"github.com/SENERGY-Platform/process-model-repository/lib/model"
	"log"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestMigration(t *testing.T) {
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

	_, searchIp, err := OpenSearch(ctx, wg)
	if err != nil {
		t.Error(err)
		return
	}

	_, permIp, err := PermSearch(ctx, wg, false, conf.KafkaUrl, searchIp)
	if err != nil {
		t.Error(err)
		return
	}

	conf.PermissionsUrl = "http://" + permIp + ":8080"

	_, permv2Ip, err := PermissionsV2(ctx, wg, conf.MongoUrl, conf.KafkaUrl)
	if err != nil {
		t.Error(err)
		return
	}
	conf.PermissionsV2Url = "http://" + permv2Ip + ":8080"

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

	perm := client.New(conf.PermissionsV2Url)

	_, err, _ = perm.SetPermission(client.InternalAdminToken, conf.ProcessTopic, p2.Id, client.ResourcePermissions{
		UserPermissions: map[string]client.PermissionsMap{
			userid1: {Read: true, Write: true, Execute: true, Administrate: true},
			userid2: {Read: true, Write: true, Execute: true, Administrate: true},
		},
	})
	if err != nil {
		t.Error(err)
		return
	}

	_, err, _ = perm.SetPermission(client.InternalAdminToken, conf.ProcessTopic, p3.Id, client.ResourcePermissions{
		UserPermissions: map[string]client.PermissionsMap{
			userid2: {Read: true, Write: true, Execute: true, Administrate: true},
		},
	})
	if err != nil {
		t.Error(err)
		return
	}

	_, err, _ = perm.SetPermission(client.InternalAdminToken, conf.ProcessTopic, p4.Id, client.ResourcePermissions{
		UserPermissions: map[string]client.PermissionsMap{
			userid1: {Read: true, Write: true, Execute: true, Administrate: true},
		},
		RolePermissions: map[string]client.PermissionsMap{
			"foo": {Read: true, Write: true, Execute: true, Administrate: true},
		},
	})
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(2 * time.Second)

	//reset topic and permissions
	err, _ = perm.RemoveTopic(client.InternalAdminToken, conf.ProcessTopic)
	if err != nil {
		t.Error(err)
		return
	}
	_, err, _ = perm.SetTopic(client.InternalAdminToken, client.Topic{
		Id:                  conf.ProcessTopic,
		PublishToKafkaTopic: conf.ProcessTopic,
	})
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(2 * time.Second)

	//run migration
	db, err := database.New(ctx, conf)
	if err != nil {
		t.Error(err)
		return
	}

	ctrl, err := controller.New(conf, db, nil)
	if err != nil {
		t.Error(err)
		return
	}
	err = ctrl.RunMigrations()
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(2 * time.Second)

	//check result
	check := func(t *testing.T, id string, expected client.ResourcePermissions) {
		t.Helper()
		r, err, _ := perm.GetResource(client.InternalAdminToken, conf.ProcessTopic, id)
		if err != nil {
			t.Error(err)
			return
		}
		if !reflect.DeepEqual(r.ResourcePermissions, expected) {
			t.Errorf("\n%#v\n%#v\n", r.ResourcePermissions, expected)
			return
		}
	}

	check(t, p1.Id, client.ResourcePermissions{
		UserPermissions: map[string]client.PermissionsMap{
			userid1: {Read: true, Write: true, Execute: true, Administrate: true},
		},
		GroupPermissions: map[string]client.PermissionsMap{},
		RolePermissions:  map[string]client.PermissionsMap{},
	})
	check(t, p2.Id, client.ResourcePermissions{
		UserPermissions: map[string]client.PermissionsMap{
			userid1: {Read: true, Write: true, Execute: true, Administrate: true},
			userid2: {Read: true, Write: true, Execute: true, Administrate: true},
		},
		GroupPermissions: map[string]client.PermissionsMap{},
		RolePermissions:  map[string]client.PermissionsMap{},
	})
	check(t, p3.Id, client.ResourcePermissions{
		UserPermissions: map[string]client.PermissionsMap{
			userid2: {Read: true, Write: true, Execute: true, Administrate: true},
		},
		GroupPermissions: map[string]client.PermissionsMap{},
		RolePermissions:  map[string]client.PermissionsMap{},
	})
	check(t, p4.Id, client.ResourcePermissions{
		UserPermissions: map[string]client.PermissionsMap{
			userid1: {Read: true, Write: true, Execute: true, Administrate: true},
		},
		RolePermissions: map[string]client.PermissionsMap{
			"foo": {Read: true, Write: true, Execute: true, Administrate: true},
		},
		GroupPermissions: map[string]client.PermissionsMap{},
	})
}
