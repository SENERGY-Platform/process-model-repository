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

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SENERGY-Platform/process-model-repository/lib/auth"
	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/SENERGY-Platform/process-model-repository/lib/model"
	"github.com/julienschmidt/httprouter"
)

type ownerController struct {
	Controller
	result model.Process
	err    error
	code   int
	called bool
	token  auth.Token
	id     string
	owner  string
}

func (f *ownerController) SetProcessOwner(token auth.Token, id string, owner string) (model.Process, error, int) {
	f.called, f.token, f.id, f.owner = true, token, id, owner
	return f.result, f.err, f.code
}

func ownerRequest(t *testing.T, ctrl Controller, target string, body string) *httptest.ResponseRecorder {
	t.Helper()
	router := httprouter.New()
	ProcessEndpoints(config.Config{}, ctrl, router)
	token, err := auth.CreateTokenWithRoles("test", "admin-a", []string{"admin"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, target, strings.NewReader(body))
	req.Header.Set("Authorization", token.Jwt())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestOwnerAnswersTheMovedProcessModel(t *testing.T) {
	ctrl := &ownerController{result: model.Process{Id: "pm-1", Name: "heating", Owner: "user-c"}, code: http.StatusOK}

	rec := ownerRequest(t, ctrl, "/processes/pm-1/owner", `{"owner": "user-c"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	if ctrl.id != "pm-1" || ctrl.owner != "user-c" || ctrl.token.GetUserId() != "admin-a" || !ctrl.token.IsAdmin() {
		t.Errorf("controller got id %q, owner %q, user %q", ctrl.id, ctrl.owner, ctrl.token.GetUserId())
	}
	var body model.Process
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	if body != ctrl.result {
		t.Errorf("body = %+v", body)
	}
}

func TestOwnerPassesAMissingOwnerOnAsEmpty(t *testing.T) {
	ctrl := &ownerController{err: errors.New("owner must be a user id"), code: http.StatusBadRequest}

	rec := ownerRequest(t, ctrl, "/processes/pm-1/owner", `{}`)

	if !ctrl.called || ctrl.owner != "" || rec.Code != http.StatusBadRequest {
		t.Errorf("called %v with owner %q, status %d", ctrl.called, ctrl.owner, rec.Code)
	}
}

func TestOwnerRejectsABodyThatIsNoJson(t *testing.T) {
	ctrl := &ownerController{code: http.StatusOK}

	rec := ownerRequest(t, ctrl, "/processes/pm-1/owner", `user-c`)

	if rec.Code != http.StatusBadRequest || ctrl.called {
		t.Errorf("status = %d, controller called = %v, want 400 and no call", rec.Code, ctrl.called)
	}
}

func TestOwnerAnswersRefusalsWithTheirStatus(t *testing.T) {
	for _, code := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError} {
		ctrl := &ownerController{err: errors.New("refused"), code: code}

		rec := ownerRequest(t, ctrl, "/processes/pm-1/owner", `{"owner": "user-c"}`)

		if rec.Code != code || !strings.Contains(rec.Body.String(), "refused") {
			t.Errorf("status = %d, body %q, want %d", rec.Code, rec.Body.String(), code)
		}
	}
}
