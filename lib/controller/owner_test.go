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

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/SENERGY-Platform/permissions-v2/pkg/client"
	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/SENERGY-Platform/process-model-repository/lib/database"
	"github.com/SENERGY-Platform/process-model-repository/lib/model"
)

// ownerDb keeps documents in memory; methods the move must not use panic through the nil embedded interface.
type ownerDb struct {
	database.Database
	docs      map[string]model.Process
	reads     int
	ownerSets int
	failSet   error
}

func (f *ownerDb) ReadProcess(_ context.Context, id string) (model.Process, bool, error) {
	f.reads++
	p, ok := f.docs[id]
	return p, ok, nil
}

func (f *ownerDb) SetProcessOwner(_ context.Context, id string, owner string) (bool, error) {
	f.ownerSets++
	if f.failSet != nil {
		return false, f.failSet
	}
	p, ok := f.docs[id]
	if !ok {
		return false, nil
	}
	p.Owner = owner
	f.docs[id] = p
	return true, nil
}

// ownerPerm hands out and stores copies, as the real client does through JSON.
type ownerPerm struct {
	client.Client
	resources map[string]client.ResourcePermissions
	gets      int
	sets      int
	failSet   error
}

func copyPermissions(in client.ResourcePermissions) (out client.ResourcePermissions) {
	b, err := json.Marshal(in)
	if err == nil {
		err = json.Unmarshal(b, &out)
	}
	if err != nil {
		panic(err)
	}
	return out
}

func (f *ownerPerm) GetResource(token string, topic string, id string) (client.Resource, error, int) {
	f.gets++
	if token != client.InternalAdminToken || topic != "processmodel" {
		return client.Resource{}, fmt.Errorf("unexpected token or topic %q", topic), http.StatusBadRequest
	}
	r, ok := f.resources[id]
	if !ok {
		return client.Resource{}, errors.New("unexpected statuscode 404: not found"), http.StatusNotFound
	}
	return client.Resource{Id: id, TopicId: topic, ResourcePermissions: copyPermissions(r)}, nil, http.StatusOK
}

func (f *ownerPerm) SetPermission(token string, topic string, id string, permissions client.ResourcePermissions) (client.ResourcePermissions, error, int) {
	f.sets++
	if f.failSet != nil {
		return permissions, f.failSet, http.StatusInternalServerError
	}
	if token != client.InternalAdminToken || topic != "processmodel" {
		return permissions, fmt.Errorf("unexpected token or topic %q", topic), http.StatusBadRequest
	}
	f.resources[id] = copyPermissions(permissions)
	return permissions, nil, http.StatusOK
}

const ownerTestLastUpdated = int64(1700000000)

type ownerFixture struct {
	ctrl *Controller
	db   *ownerDb
	perm *ownerPerm
}

// pm-1 belongs to user-a, is shared with user-b for reading, with a group and with the admin role.
func newOwnerFixture() *ownerFixture {
	f := &ownerFixture{
		db: &ownerDb{docs: map[string]model.Process{
			"pm-1": {Id: "pm-1", Name: "heating", Owner: "user-a", BpmnXml: "<bpmn/>", SvgXml: "<svg/>", Date: 42, Publish: true, PublishDate: "then", Description: "d", LastUpdatedUnix: ownerTestLastUpdated},
		}},
		perm: &ownerPerm{resources: map[string]client.ResourcePermissions{
			"pm-1": storedPermissions(),
		}},
	}
	f.ctrl = &Controller{db: f.db, perm: f.perm, config: config.Config{ProcessTopic: "processmodel"}}
	return f
}

func storedPermissions() client.ResourcePermissions {
	return client.ResourcePermissions{
		UserPermissions: map[string]client.PermissionsMap{
			"user-a": fullRights,
			"user-b": {Read: true},
		},
		GroupPermissions: map[string]client.PermissionsMap{"/team": {Read: true, Execute: true}},
		RolePermissions:  map[string]client.PermissionsMap{"admin": fullRights},
	}
}

func (f *ownerFixture) assertUnchanged(t *testing.T) {
	t.Helper()
	if f.perm.sets != 0 || f.db.ownerSets != 0 {
		t.Errorf("wrote permissions %d and owners %d times, want none", f.perm.sets, f.db.ownerSets)
	}
	if !reflect.DeepEqual(f.perm.resources["pm-1"], storedPermissions()) {
		t.Errorf("permissions = %+v", f.perm.resources["pm-1"])
	}
	if p := f.db.docs["pm-1"]; p.Owner != "user-a" || p.LastUpdatedUnix != ownerTestLastUpdated {
		t.Errorf("document = %+v", p)
	}
}

func TestSetProcessOwnerByANonAdminIsForbiddenBeforeAnyRead(t *testing.T) {
	f := newOwnerFixture()

	_, err, code := f.ctrl.SetProcessOwner(tokenOf(t, "user-a", "user"), "pm-1", "user-c")

	if err == nil || code != http.StatusForbidden {
		t.Fatalf("err = %v, code = %d, want 403", err, code)
	}
	if f.db.reads != 0 || f.perm.gets != 0 {
		t.Errorf("read %d documents and %d permissions before refusing", f.db.reads, f.perm.gets)
	}
	f.assertUnchanged(t)
}

func TestSetProcessOwnerRejectsAMissingOwner(t *testing.T) {
	for _, owner := range []string{"", " ", " user-c", "user-c\n"} {
		f := newOwnerFixture()

		_, err, code := f.ctrl.SetProcessOwner(tokenOf(t, "admin-a", "admin"), "pm-1", owner)

		if err == nil || code != http.StatusBadRequest {
			t.Errorf("owner %q: err = %v, code = %d, want 400", owner, err, code)
		}
		if f.db.reads != 0 || f.perm.gets != 0 {
			t.Errorf("owner %q: read %d documents and %d permissions", owner, f.db.reads, f.perm.gets)
		}
		f.assertUnchanged(t)
	}
}

func TestSetProcessOwnerOfAnUnknownIdIsNotFound(t *testing.T) {
	f := newOwnerFixture()

	_, err, code := f.ctrl.SetProcessOwner(tokenOf(t, "admin-a", "admin"), "pm-unknown", "user-c")

	if err == nil || code != http.StatusNotFound {
		t.Fatalf("err = %v, code = %d, want 404", err, code)
	}
	if f.perm.gets != 0 {
		t.Errorf("asked permissions-v2 %d times about an unknown id", f.perm.gets)
	}
	f.assertUnchanged(t)
}

func TestSetProcessOwnerMovesTheProcessModel(t *testing.T) {
	f := newOwnerFixture()
	before := f.db.docs["pm-1"]

	result, err, code := f.ctrl.SetProcessOwner(tokenOf(t, "admin-a", "admin"), "pm-1", "user-c")

	if err != nil || code != http.StatusOK {
		t.Fatalf("err = %v, code = %d", err, code)
	}
	want := storedPermissions()
	want.UserPermissions["user-c"] = fullRights
	if got := f.perm.resources["pm-1"]; !reflect.DeepEqual(got, want) {
		t.Errorf("permissions = %+v\nwant %+v", got, want)
	}
	stored := f.db.docs["pm-1"]
	wantDoc := before
	wantDoc.Owner = "user-c"
	if stored != wantDoc {
		t.Errorf("document = %+v\nwant only the owner changed: %+v", stored, wantDoc)
	}
	if !reflect.DeepEqual(result, stored) {
		t.Errorf("result = %+v, want the stored document %+v", result, stored)
	}
}

func TestSetProcessOwnerRaisesPartialRightsOfTheNewOwner(t *testing.T) {
	f := newOwnerFixture()

	_, err, code := f.ctrl.SetProcessOwner(tokenOf(t, "admin-a", "admin"), "pm-1", "user-b")

	if err != nil || code != http.StatusOK {
		t.Fatalf("err = %v, code = %d", err, code)
	}
	want := storedPermissions()
	want.UserPermissions["user-b"] = fullRights
	if got := f.perm.resources["pm-1"]; !reflect.DeepEqual(got, want) {
		t.Errorf("permissions = %+v\nwant %+v", got, want)
	}
}

func TestSetProcessOwnerTwiceChangesNothingTheSecondTime(t *testing.T) {
	f := newOwnerFixture()
	admin := tokenOf(t, "admin-a", "admin")
	first, err, _ := f.ctrl.SetProcessOwner(admin, "pm-1", "user-c")
	if err != nil {
		t.Fatal(err)
	}
	permissionsAfterFirst := copyPermissions(f.perm.resources["pm-1"])
	f.perm.sets, f.db.ownerSets = 0, 0

	second, err, code := f.ctrl.SetProcessOwner(admin, "pm-1", "user-c")

	if err != nil || code != http.StatusOK {
		t.Fatalf("err = %v, code = %d", err, code)
	}
	if f.perm.sets != 0 || f.db.ownerSets != 0 {
		t.Errorf("repeat wrote permissions %d and owners %d times", f.perm.sets, f.db.ownerSets)
	}
	if !reflect.DeepEqual(second, first) || !reflect.DeepEqual(f.perm.resources["pm-1"], permissionsAfterFirst) {
		t.Errorf("repeat changed the result %+v -> %+v or the permissions", first, second)
	}
}

func TestSetProcessOwnerWithoutPermissionsEntryChangesNothing(t *testing.T) {
	f := newOwnerFixture()
	delete(f.perm.resources, "pm-1")

	_, err, code := f.ctrl.SetProcessOwner(tokenOf(t, "admin-a", "admin"), "pm-1", "user-c")

	if err == nil || code != http.StatusInternalServerError {
		t.Fatalf("err = %v, code = %d, want 500", err, code)
	}
	if f.perm.sets != 0 || f.db.ownerSets != 0 || f.db.docs["pm-1"].Owner != "user-a" {
		t.Errorf("wrote permissions %d and owners %d times", f.perm.sets, f.db.ownerSets)
	}
}

func TestSetProcessOwnerKeepsTheDocumentWhenPermissionsFail(t *testing.T) {
	f := newOwnerFixture()
	f.perm.failSet = errors.New("permissions-v2 down")

	_, err, code := f.ctrl.SetProcessOwner(tokenOf(t, "admin-a", "admin"), "pm-1", "user-c")

	if err == nil || code != http.StatusInternalServerError {
		t.Fatalf("err = %v, code = %d, want 500", err, code)
	}
	if f.db.ownerSets != 0 || f.db.docs["pm-1"].Owner != "user-a" {
		t.Errorf("document owner set %d times to %q after the permissions failed", f.db.ownerSets, f.db.docs["pm-1"].Owner)
	}
}

func TestSetProcessOwnerRepeatCompletesAfterTheDocumentWriteFailed(t *testing.T) {
	f := newOwnerFixture()
	f.db.failSet = errors.New("mongo down")
	admin := tokenOf(t, "admin-a", "admin")

	_, err, code := f.ctrl.SetProcessOwner(admin, "pm-1", "user-c")

	if err == nil || code != http.StatusInternalServerError {
		t.Fatalf("err = %v, code = %d, want 500", err, code)
	}
	if f.perm.resources["pm-1"].UserPermissions["user-c"] != fullRights || f.db.docs["pm-1"].Owner != "user-a" {
		t.Fatalf("after the failure: permissions %+v, owner %q", f.perm.resources["pm-1"], f.db.docs["pm-1"].Owner)
	}

	f.db.failSet = nil
	f.perm.sets = 0
	result, err, code := f.ctrl.SetProcessOwner(admin, "pm-1", "user-c")

	if err != nil || code != http.StatusOK || result.Owner != "user-c" || f.db.docs["pm-1"].Owner != "user-c" {
		t.Fatalf("repeat: err = %v, code = %d, owner %q", err, code, f.db.docs["pm-1"].Owner)
	}
	if f.perm.sets != 0 {
		t.Errorf("repeat wrote the permissions again %d times", f.perm.sets)
	}
}
