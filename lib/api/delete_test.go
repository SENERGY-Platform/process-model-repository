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

type fakeController struct {
	Controller
	err    error
	code   int
	id     string
	force  bool
	called bool
}

func (f *fakeController) DeleteProcess(_ auth.Token, id string, force bool) (error, int) {
	f.called, f.id, f.force = true, id, force
	return f.err, f.code
}

func deleteRequest(t *testing.T, ctrl Controller, target string) *httptest.ResponseRecorder {
	t.Helper()
	router := httprouter.New()
	ProcessEndpoints(config.Config{}, ctrl, router)
	token, err := auth.CreateToken("test", "user-a")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodDelete, target, nil)
	req.Header.Set("Authorization", token.Jwt())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestDeleteAnswersAUsedProcessModelWithTheUsage(t *testing.T) {
	ctrl := &fakeController{
		err: &model.InUseError{Usage: model.ResourceUsage{
			Releases: 2, Instances: 5, Readable: []model.ResourceRef{{Id: "r1", DesignId: "d1", Name: "heating"}},
		}},
		code: http.StatusConflict,
	}

	rec := deleteRequest(t, ctrl, "/processes/pm-1")

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	if body["error"] == "" || body["releases"] != float64(2) || body["instances"] != float64(5) {
		t.Errorf("body = %v", body)
	}
	readable, _ := body["readable"].([]any)
	if len(readable) != 1 || readable[0].(map[string]any)["design_id"] != "d1" || readable[0].(map[string]any)["name"] != "heating" || readable[0].(map[string]any)["id"] != "r1" {
		t.Errorf("readable = %v", body["readable"])
	}
	if ctrl.id != "pm-1" || ctrl.force {
		t.Errorf("controller got id %q force %v", ctrl.id, ctrl.force)
	}
}

func TestDeleteAnswersRefusalsWithoutUsageAsText(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code int
	}{
		{"smart-service-repository unavailable", model.ErrUsageUnavailable, http.StatusBadGateway},
		{"force by a non-admin", errors.New("force needs the admin role"), http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := deleteRequest(t, &fakeController{err: tc.err, code: tc.code}, "/processes/pm-1")

			if rec.Code != tc.code || !strings.Contains(rec.Body.String(), tc.err.Error()) {
				t.Errorf("status = %d, body = %q", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestDeletePassesForceOn(t *testing.T) {
	tests := []struct {
		target    string
		wantForce bool
	}{
		{"/processes/pm-1", false},
		{"/processes/pm-1?force=false", false},
		{"/processes/pm-1?force=true", true},
	}
	for _, tc := range tests {
		t.Run(tc.target, func(t *testing.T) {
			ctrl := &fakeController{code: http.StatusOK}

			rec := deleteRequest(t, ctrl, tc.target)

			if rec.Code != http.StatusOK || ctrl.force != tc.wantForce {
				t.Errorf("status = %d, force = %v, want 200 and %v", rec.Code, ctrl.force, tc.wantForce)
			}
		})
	}
}

func TestDeleteRejectsForceThatIsNoBoolean(t *testing.T) {
	ctrl := &fakeController{code: http.StatusOK}

	rec := deleteRequest(t, ctrl, "/processes/pm-1?force=maybe")

	if rec.Code != http.StatusBadRequest || ctrl.called {
		t.Errorf("status = %d, controller called = %v, want 400 and no call", rec.Code, ctrl.called)
	}
}
