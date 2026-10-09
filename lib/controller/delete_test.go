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
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/SENERGY-Platform/permissions-v2/pkg/client"
	"github.com/SENERGY-Platform/process-model-repository/lib/auth"
	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/SENERGY-Platform/process-model-repository/lib/database"
	"github.com/SENERGY-Platform/process-model-repository/lib/model"
)

type fakeDb struct {
	database.Database
	deleted []string
}

func (f *fakeDb) DeleteProcess(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

type fakePerm struct {
	client.Client
	removed []string
	allowed bool
}

func (f *fakePerm) CheckPermission(string, string, string, ...client.Permission) (bool, error, int) {
	return f.allowed, nil, http.StatusOK
}

func (f *fakePerm) RemoveResource(_ string, _ string, id string) (error, int) {
	f.removed = append(f.removed, id)
	return nil, http.StatusOK
}

type fakeUsage struct {
	usage model.ResourceUsage
	err   error
	// auth and id of the last question
	auth, id string
	asked    int
}

func (f *fakeUsage) Usage(_ context.Context, id string, auth string) (model.ResourceUsage, error) {
	f.asked++
	f.id, f.auth = id, auth
	return f.usage, f.err
}

type deleteFixture struct {
	ctrl  *Controller
	db    *fakeDb
	perm  *fakePerm
	usage *fakeUsage
	logs  *bytes.Buffer
}

func newDeleteFixture(t *testing.T, usage model.ResourceUsage) *deleteFixture {
	t.Helper()
	f := &deleteFixture{db: &fakeDb{}, perm: &fakePerm{allowed: true}, usage: &fakeUsage{usage: usage}, logs: &bytes.Buffer{}}
	f.ctrl = &Controller{db: f.db, perm: f.perm, usage: f.usage, config: config.Config{ProcessTopic: "processmodel"}}
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(f.logs, nil)))
	t.Cleanup(func() { slog.SetDefault(orig) })
	return f
}

func tokenOf(t *testing.T, user string, roles ...string) auth.Token {
	t.Helper()
	token, err := auth.CreateTokenWithRoles("test", user, roles)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func (f *deleteFixture) assertNothingDeleted(t *testing.T) {
	t.Helper()
	if len(f.db.deleted) != 0 || len(f.perm.removed) != 0 {
		t.Errorf("deleted from db %v and from permissions %v, want nothing", f.db.deleted, f.perm.removed)
	}
}

func (f *deleteFixture) assertDeleted(t *testing.T, id string) {
	t.Helper()
	if len(f.db.deleted) != 1 || f.db.deleted[0] != id || len(f.perm.removed) != 1 || f.perm.removed[0] != id {
		t.Errorf("deleted from db %v and from permissions %v, want only %q", f.db.deleted, f.perm.removed, id)
	}
}

var usedModel = model.ResourceUsage{
	Releases:  2,
	Instances: 5,
	Readable:  []model.ResourceRef{{Id: "r1", DesignId: "d1", Name: "heating"}},
}

func TestDeleteProcessRefusesAUsedProcessModel(t *testing.T) {
	f := newDeleteFixture(t, usedModel)
	token := tokenOf(t, "user-a")

	err, code := f.ctrl.DeleteProcess(token, "pm-1", false)

	var inUse *model.InUseError
	if !errors.As(err, &inUse) || code != http.StatusConflict {
		t.Fatalf("err = %v, code = %d, want an *InUseError and 409", err, code)
	}
	if inUse.Usage.Releases != 2 || inUse.Usage.Instances != 5 || len(inUse.Usage.Readable) != 1 || inUse.Usage.Readable[0].Name != "heating" {
		t.Errorf("usage = %+v", inUse.Usage)
	}
	if f.usage.id != "pm-1" || f.usage.auth != token.Jwt() {
		t.Errorf("asked about %q with %q, want pm-1 with the caller's token", f.usage.id, f.usage.auth)
	}
	f.assertNothingDeleted(t)
}

// Only releases count; instances without a release do not hold the process model.
func TestDeleteProcessDeletesAnUnusedProcessModel(t *testing.T) {
	f := newDeleteFixture(t, model.ResourceUsage{Releases: 0, Instances: 3, Readable: []model.ResourceRef{}})

	err, code := f.ctrl.DeleteProcess(tokenOf(t, "user-a"), "pm-1", false)

	if err != nil || code != http.StatusOK {
		t.Fatalf("err = %v, code = %d", err, code)
	}
	f.assertDeleted(t, "pm-1")
	if f.logs.Len() != 0 {
		t.Errorf("logged %q for an ordinary delete", f.logs.String())
	}
}

func TestDeleteProcessWithoutAnswerDeletesNothing(t *testing.T) {
	for _, force := range []bool{false, true} {
		f := newDeleteFixture(t, model.ResourceUsage{})
		f.usage.err = errors.New("smart-service-repository answered 500")

		err, code := f.ctrl.DeleteProcess(tokenOf(t, "admin-a", "admin"), "pm-1", force)

		if !errors.Is(err, model.ErrUsageUnavailable) || code != http.StatusBadGateway {
			t.Errorf("force=%v: err = %v, code = %d, want ErrUsageUnavailable and 502", force, err, code)
		}
		f.assertNothingDeleted(t)
	}
}

func TestDeleteProcessForceByAnAdminDeletesAndLogs(t *testing.T) {
	f := newDeleteFixture(t, usedModel)

	err, code := f.ctrl.DeleteProcess(tokenOf(t, "admin-a", "admin"), "pm-1", true)

	if err != nil || code != http.StatusOK {
		t.Fatalf("err = %v, code = %d", err, code)
	}
	f.assertDeleted(t, "pm-1")
	logged := f.logs.String()
	for _, want := range []string{"level=WARN", "process_model_id=pm-1", "user_id=admin-a", "releases=2", "instances=5"} {
		if !strings.Contains(logged, want) {
			t.Errorf("log %q lacks %q", logged, want)
		}
	}
}

func TestDeleteProcessForceByANonAdminIsForbidden(t *testing.T) {
	f := newDeleteFixture(t, usedModel)

	err, code := f.ctrl.DeleteProcess(tokenOf(t, "user-a", "user"), "pm-1", true)

	if err == nil || code != http.StatusForbidden {
		t.Fatalf("err = %v, code = %d, want 403", err, code)
	}
	f.assertNothingDeleted(t)
	if f.usage.asked != 0 {
		t.Errorf("asked the smart-service-repository %d times before refusing", f.usage.asked)
	}
}

func TestDeleteProcessWithoutRightsDoesNotAsk(t *testing.T) {
	f := newDeleteFixture(t, model.ResourceUsage{})
	f.perm.allowed = false

	err, code := f.ctrl.DeleteProcess(tokenOf(t, "user-a"), "pm-1", false)

	if err == nil || code != http.StatusForbidden {
		t.Fatalf("err = %v, code = %d, want 403", err, code)
	}
	f.assertNothingDeleted(t)
	if f.usage.asked != 0 {
		t.Errorf("asked the smart-service-repository %d times for a caller without rights", f.usage.asked)
	}
}

func TestNewRefusesAnEmptySmartServiceRepositoryUrl(t *testing.T) {
	_, err := New(config.Config{SmartServiceRepositoryUrl: ""}, &fakeDb{})
	if err == nil || !strings.Contains(err.Error(), "smart-service-repository") {
		t.Errorf("err = %v, want a refusal naming the smart-service-repository", err)
	}
}
