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

package smartservices

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUsageAsksWithTheCallersTokenAndParsesTheAnswer(t *testing.T) {
	var gotPath, gotAuth, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotMethod = r.URL.EscapedPath(), r.Header.Get("Authorization"), r.Method
		_, _ = w.Write([]byte(`{"releases":2,"instances":5,"readable":[{"id":"r1","design_id":"d1","name":"heating"}]}`))
	}))
	t.Cleanup(srv.Close)

	// A trailing slash on the configured address must not double up.
	usage, err := New(srv.URL+"/").Usage(context.Background(), "pm/1", "Bearer token-a")

	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodGet || gotPath != "/resource-usage/process-models/pm%2F1" || gotAuth != "Bearer token-a" {
		t.Errorf("request = %s %s with %q", gotMethod, gotPath, gotAuth)
	}
	if usage.Releases != 2 || usage.Instances != 5 || len(usage.Readable) != 1 ||
		usage.Readable[0].Id != "r1" || usage.Readable[0].DesignId != "d1" || usage.Readable[0].Name != "heating" {
		t.Errorf("usage = %+v", usage)
	}
}

func TestUsageOfAnUnusedProcessModelHasAnEmptyReadableList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"releases":0,"instances":0,"readable":null}`))
	}))
	t.Cleanup(srv.Close)

	usage, err := New(srv.URL).Usage(context.Background(), "pm-1", "")

	if err != nil {
		t.Fatal(err)
	}
	if usage.Releases != 0 || usage.Readable == nil || len(usage.Readable) != 0 {
		t.Errorf("usage = %#v", usage)
	}
}

// Every one of these is an unknown answer and must never read as "no smart service uses it".
func TestUsageFailsOnAnythingButACompleteAnswer(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		// A well-formed "unused" body, so that only the status can make these fail.
		{"unauthorized", http.StatusUnauthorized, `{"releases":0,"instances":0,"readable":[]}`},
		{"server error", http.StatusInternalServerError, `{"releases":0,"instances":0,"readable":[]}`},
		{"bad gateway text", http.StatusBadGateway, `bad gateway`},
		{"ok with an empty object", http.StatusOK, `{}`},
		{"ok without releases", http.StatusOK, `{"instances":0,"readable":[]}`},
		{"ok without instances", http.StatusOK, `{"releases":0,"readable":[]}`},
		{"ok with html from a proxy", http.StatusOK, `<html>maintenance</html>`},
		{"ok with an empty body", http.StatusOK, ``},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)

			if _, err := New(srv.URL).Usage(context.Background(), "pm-1", "Bearer t"); err == nil {
				t.Error("want an error")
			}
		})
	}
}

func TestUsageFailsWhenTheServiceIsDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	if _, err := New(url).Usage(context.Background(), "pm-1", "Bearer t"); err == nil {
		t.Error("want an error")
	}
}
