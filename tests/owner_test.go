/*
 * Copyright 2026 InfAI (CC SES)
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
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SENERGY-Platform/permissions-v2/pkg/client"
	"github.com/SENERGY-Platform/process-model-repository/lib/api"
	"github.com/SENERGY-Platform/process-model-repository/lib/auth"
	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/SENERGY-Platform/process-model-repository/lib/contextwg"
	"github.com/SENERGY-Platform/process-model-repository/lib/controller"
	"github.com/SENERGY-Platform/process-model-repository/lib/database"
	"github.com/SENERGY-Platform/process-model-repository/lib/model"
)

func TestSetOwner(t *testing.T) {
	conf, err := config.Load("../config.json")
	if err != nil {
		t.Fatal(err)
	}
	conf.Debug = true
	conf.ConnectivityTest = false

	wg := &sync.WaitGroup{}
	defer wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = contextwg.WithWaitGroup(ctx, wg)

	// The service reaches the containers through their mapped ports, which also works where the
	// container network is not routable from the host (rootless Docker); it starts without the
	// Kafka consumer, which the move does not use.
	mongoPort, mongoIp, err := MongoTestServer(ctx, wg)
	if err != nil {
		t.Fatal(err)
	}
	kafkaUrl, err := Kafka(ctx, wg)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)
	permPort, _, err := PermissionsV2(ctx, wg, "mongodb://"+mongoIp+":27017", kafkaUrl)
	if err != nil {
		t.Fatal(err)
	}
	conf.MongoUrl = "mongodb://localhost:" + mongoPort
	conf.PermissionsV2Url = "http://localhost:" + permPort
	port, err := getFreePort()
	if err != nil {
		t.Fatal(err)
	}
	conf.ServerPort = strconv.Itoa(port)
	db, err := database.New(ctx, conf)
	if err != nil {
		t.Fatal(err)
	}
	ctrl, err := controller.New(conf, db)
	if err != nil {
		t.Fatal(err)
	}
	api.Start(ctx, conf, ctrl)
	time.Sleep(time.Second)

	tokenOf := func(user string, roles ...string) auth.Token {
		token, err := auth.CreateTokenWithRoles("test", user, roles)
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	author, reader, target := tokenOf("author", "user"), tokenOf("reader", "user"), tokenOf("target", "user")
	admin := tokenOf("admin-user", "admin")
	base := "http://localhost:" + conf.ServerPort
	perm := client.New(conf.PermissionsV2Url)

	var created model.Process
	err = PostJSON(author.Jwt(), base+"/processes", model.Process{Name: "heating", BpmnXml: createTestXmlString("p1"), SvgXml: "svg"}, &created)
	if err != nil || created.Id == "" || created.Owner != "author" {
		t.Fatalf("create: %v, %+v", err, created)
	}

	// An old update time, so that a move which touched it would show within the same second too.
	created.LastUpdatedUnix = 1700000000
	if err = db.SetProcess(context.Background(), created); err != nil {
		t.Fatal(err)
	}

	stored, err, _ := perm.GetResource(client.InternalAdminToken, conf.ProcessTopic, created.Id)
	if err != nil {
		t.Fatal(err)
	}
	stored.UserPermissions["reader"] = client.PermissionsMap{Read: true}
	stored.GroupPermissions = map[string]client.PermissionsMap{"/team": {Read: true, Execute: true}}
	_, err, _ = perm.SetPermission(client.InternalAdminToken, conf.ProcessTopic, created.Id, stored.ResourcePermissions)
	if err != nil {
		t.Fatal(err)
	}
	before, err, _ := perm.GetResource(client.InternalAdminToken, conf.ProcessTopic, created.Id)
	if err != nil {
		t.Fatal(err)
	}

	put := func(token auth.Token, id string, body string) (code int, result model.Process) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPut, base+"/processes/"+id+"/owner", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", token.Jwt())
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
		}
		return resp.StatusCode, result
	}
	assertState := func(t *testing.T, owner string, permissions client.ResourcePermissions) {
		t.Helper()
		doc, exists, err := db.ReadProcess(context.Background(), created.Id)
		if err != nil || !exists || doc.Owner != owner {
			t.Errorf("document: %v, exists %v, owner %q, want %q", err, exists, doc.Owner, owner)
		}
		actual, err, _ := perm.GetResource(client.InternalAdminToken, conf.ProcessTopic, created.Id)
		if err != nil || !reflect.DeepEqual(actual.ResourcePermissions, permissions) {
			t.Errorf("permissions: %v\n%#v\nwant\n%#v", err, actual.ResourcePermissions, permissions)
		}
	}
	listOwned := func(t *testing.T, token auth.Token) (ids []string) {
		t.Helper()
		var list []model.Process
		if err := GetJSON(token.Jwt(), base+"/v2/processes?owner="+token.GetUserId(), &list); err != nil {
			t.Fatal(err)
		}
		for _, p := range list {
			ids = append(ids, p.Id)
		}
		return ids
	}

	t.Run("non-admin is forbidden", func(t *testing.T) {
		if code, _ := put(author, created.Id, `{"owner": "target"}`); code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", code)
		}
		assertState(t, "author", before.ResourcePermissions)
	})
	t.Run("missing owner", func(t *testing.T) {
		if code, _ := put(admin, created.Id, `{}`); code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", code)
		}
		assertState(t, "author", before.ResourcePermissions)
	})
	t.Run("unknown id", func(t *testing.T) {
		if code, _ := put(admin, "unknown-id", `{"owner": "target"}`); code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", code)
		}
		if _, err, code := perm.GetResource(client.InternalAdminToken, conf.ProcessTopic, "unknown-id"); code != http.StatusNotFound {
			t.Errorf("permissions of the unknown id: %v, %d", err, code)
		}
	})

	want := before.ResourcePermissions
	want.UserPermissions = map[string]client.PermissionsMap{}
	for k, v := range before.UserPermissions {
		want.UserPermissions[k] = v
	}
	want.UserPermissions["target"] = client.PermissionsMap{Read: true, Write: true, Execute: true, Administrate: true}

	var moved model.Process
	t.Run("move", func(t *testing.T) {
		var code int
		code, moved = put(admin, created.Id, `{"owner": "target"}`)
		if code != http.StatusOK {
			t.Fatalf("status = %d", code)
		}
		// last_updated_unix included: the smart-service-repository reads a newer one as a content change.
		wantMoved := created
		wantMoved.Owner = "target"
		if moved != wantMoved {
			t.Errorf("moved = %+v\nwant only the owner changed: %+v", moved, wantMoved)
		}
		assertState(t, "target", want)
		if ids := listOwned(t, target); !reflect.DeepEqual(ids, []string{created.Id}) {
			t.Errorf("owner=target lists %v", ids)
		}
		if ids := listOwned(t, author); len(ids) != 0 {
			t.Errorf("owner=author lists %v", ids)
		}
		var readable model.Process
		if err := GetJSON(reader.Jwt(), base+"/processes/"+created.Id, &readable); err != nil || readable.Owner != "target" {
			t.Errorf("reader: %v, %+v", err, readable)
		}
	})
	t.Run("repeat", func(t *testing.T) {
		code, again := put(admin, created.Id, `{"owner": "target"}`)
		if code != http.StatusOK || again != moved {
			t.Errorf("status = %d, %+v\nfirst %+v", code, again, moved)
		}
		assertState(t, "target", want)
	})
	t.Run("owner write of an unknown id inserts nothing", func(t *testing.T) {
		found, err := db.SetProcessOwner(context.Background(), "unknown-id", "target")
		if err != nil || found {
			t.Fatalf("found = %v, err = %v", found, err)
		}
		if _, exists, err := db.ReadProcess(context.Background(), "unknown-id"); err != nil || exists {
			t.Errorf("exists = %v, err = %v", exists, err)
		}
	})
}
