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

package mongo

import (
	"context"
	"errors"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const replicaSetURL = "mongodb://mongo-0.mongo:27017,mongo-1.mongo:27017/?replicaSet=rs0&readPreference=primary"

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.Config
		wantErr error
	}{
		{"no auth", config.Config{MongoDatabase: "process_repository"}, nil},
		{"user and password", config.Config{MongoDatabase: "process_repository", MongoUser: "u", MongoPassword: "p"}, nil},
		{"password without user", config.Config{MongoDatabase: "process_repository", MongoPassword: "p"}, nil},
		{"user without password", config.Config{MongoDatabase: "process_repository", MongoUser: "u"}, ErrMissingPassword},
		{"empty database", config.Config{}, ErrEmptyDatabase},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateConfig(tt.cfg); !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestClientOptions_AuthWhenUserGiven(t *testing.T) {
	opts := clientOptions(config.Config{
		MongoUrl:        replicaSetURL,
		MongoUser:       "process-repository",
		MongoPassword:   "s3cr3t",
		MongoAuthSource: "admin",
		MongoDatabase:   "process_repository",
	})
	if err := opts.Validate(); err != nil {
		t.Fatal(err)
	}
	want := &options.Credential{Username: "process-repository", Password: "s3cr3t", AuthSource: "admin"}
	if !reflect.DeepEqual(opts.Auth, want) {
		t.Errorf("auth = %+v, want %+v", opts.Auth, want)
	}
}

func TestClientOptions_NoAuthWhenUserEmpty(t *testing.T) {
	// A password without a user must not switch auth on.
	opts := clientOptions(config.Config{
		MongoUrl:        "mongodb://localhost:27017",
		MongoPassword:   "s3cr3t",
		MongoAuthSource: "admin",
		MongoDatabase:   "process_repository",
	})
	if err := opts.Validate(); err != nil {
		t.Fatal(err)
	}
	if opts.Auth != nil {
		t.Errorf("auth = %+v, want nil", opts.Auth)
	}
}

func TestClientOptions_ConfiguredCredentialsReplaceURICredentials(t *testing.T) {
	opts := clientOptions(config.Config{
		MongoUrl:        "mongodb://old:oldpw@localhost:27017/?authSource=other&authMechanism=SCRAM-SHA-1",
		MongoUser:       "process-repository",
		MongoPassword:   "newpw",
		MongoAuthSource: "admin",
		MongoDatabase:   "process_repository",
	})
	if err := opts.Validate(); err != nil {
		t.Fatal(err)
	}
	want := &options.Credential{Username: "process-repository", Password: "newpw", AuthSource: "admin"}
	if !reflect.DeepEqual(opts.Auth, want) {
		t.Errorf("auth = %+v, want %+v", opts.Auth, want)
	}
}

func TestClientOptions_URIPassedUnchanged(t *testing.T) {
	opts := clientOptions(config.Config{MongoUrl: replicaSetURL, MongoDatabase: "process_repository"})
	if err := opts.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := opts.GetURI(); got != replicaSetURL {
		t.Errorf("uri = %q, want %q", got, replicaSetURL)
	}
	if want := []string{"mongo-0.mongo:27017", "mongo-1.mongo:27017"}; !reflect.DeepEqual(opts.Hosts, want) {
		t.Errorf("hosts = %v, want %v", opts.Hosts, want)
	}
	if opts.ReplicaSet == nil || *opts.ReplicaSet != "rs0" {
		t.Errorf("replica set = %v, want rs0", opts.ReplicaSet)
	}
}

func TestClientOptions_NoSchemeAdded(t *testing.T) {
	opts := clientOptions(config.Config{MongoUrl: "localhost:27017", MongoDatabase: "process_repository"})
	if err := opts.Validate(); err == nil {
		t.Fatal("expected an error for a url without scheme")
	}
}

// unreachableURL points at a port that was just free, so only the startup check can fail.
func unreachableURL(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
	return "mongodb://" + addr + "/?directConnection=true"
}

// The startup check would fail as well, so these check for the specific validation error
// and that New never reaches the network for it.
func TestNew_RejectsBeforeConnecting(t *testing.T) {
	cases := map[string]config.Config{
		"empty database":        {MongoUser: "process-repository", MongoPassword: "s3cr3t"},
		"user without password": {MongoUser: "process-repository", MongoDatabase: "process_repository"},
	}
	want := map[string]error{"empty database": ErrEmptyDatabase, "user without password": ErrMissingPassword}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			cfg.MongoUrl = unreachableURL(t)
			db, err := New(context.Background(), cfg)
			if !errors.Is(err, want[name]) {
				t.Fatalf("err = %v, want %v", err, want[name])
			}
			if db != nil {
				t.Error("expected no db on failure")
			}
			if strings.Contains(err.Error(), "s3cr3t") {
				t.Errorf("error leaks the password: %v", err)
			}
		})
	}
}

func TestConnect_StartupCheckFailsWithoutServer(t *testing.T) {
	const password = "pw-must-not-appear-7f3a"
	cfg := config.Config{
		MongoUrl:        unreachableURL(t),
		MongoUser:       "process-repository",
		MongoPassword:   password,
		MongoAuthSource: "admin",
		MongoDatabase:   "process_repository",
	}
	begin := time.Now()
	client, err := connect(context.Background(), clientOptions(cfg), cfg.MongoDatabase, 500*time.Millisecond)
	if err == nil {
		t.Fatal("expected an error when the server is unreachable")
	}
	if client != nil {
		t.Error("expected no client on failure")
	}
	if !strings.HasPrefix(err.Error(), "mongo startup check failed: ") {
		t.Errorf("unexpected error: %v", err)
	}
	if strings.Contains(err.Error(), password) {
		t.Error("error text contains the password")
	}
	if elapsed := time.Since(begin); elapsed > 5*time.Second {
		t.Errorf("connect took %v, the timeout was not applied", elapsed)
	}
}
