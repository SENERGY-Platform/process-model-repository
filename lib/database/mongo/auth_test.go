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
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/SENERGY-Platform/process-model-repository/lib/contextwg"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TestNew_Authenticates needs a throwaway server with access control; MONGO_AUTH_TEST_USER
// and MONGO_AUTH_TEST_PASSWORD are root credentials, used to create and remove the test users.
func TestNew_Authenticates(t *testing.T) {
	url, rootUser, rootPassword := os.Getenv("MONGO_AUTH_TEST_URL"), os.Getenv("MONGO_AUTH_TEST_USER"), os.Getenv("MONGO_AUTH_TEST_PASSWORD")
	if testing.Short() || url == "" || rootUser == "" || rootPassword == "" {
		t.Skip("needs MONGO_AUTH_TEST_URL, MONGO_AUTH_TEST_USER and MONGO_AUTH_TEST_PASSWORD, not in -short")
	}
	ctx := context.Background()
	root, err := mongo.Connect(ctx, options.Client().ApplyURI(url).SetAuth(options.Credential{Username: rootUser, Password: rootPassword, AuthSource: "admin"}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Disconnect(ctx) })

	suffix := randomHex(t)
	testDB, otherDB := "process_repository_auth_test_"+suffix, "process_repository_auth_other_"+suffix
	svcUser, svcPassword := "pmr-test-"+suffix, randomHex(t)
	otherUser, otherPassword := "pmr-other-"+suffix, randomHex(t)
	readUser, readPassword := "pmr-read-"+suffix, randomHex(t)
	createUser(t, root, svcUser, svcPassword, "readWrite", testDB)
	createUser(t, root, otherUser, otherPassword, "readWrite", otherDB)
	createUser(t, root, readUser, readPassword, "read", testDB)
	passwords := []string{svcPassword, otherPassword, readPassword, rootPassword}

	cfg := func(user, password string) config.Config {
		return config.Config{
			MongoUrl:               url,
			MongoUser:              user,
			MongoPassword:          password,
			MongoAuthSource:        "admin",
			MongoDatabase:          testDB,
			MongoProcessCollection: "process",
		}
	}

	t.Run("New with correct credentials", func(t *testing.T) {
		wg := &sync.WaitGroup{}
		svcCtx, cancel := context.WithCancel(contextwg.WithWaitGroup(ctx, wg))
		db, err := New(svcCtx, cfg(svcUser, svcPassword))
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		if _, _, err = db.ReadProcess(ctx, "doesnotexist"); err != nil {
			t.Errorf("query as the service user: %v", err)
		}
		cancel()
		wg.Wait()
		assertIndexExists(t, root, testDB, "process", "processpublicindex")
	})

	cases := []struct {
		name, user, password string
		wantErr              string
	}{
		{"correct credentials", svcUser, svcPassword, ""},
		{"no credentials", "", "", "mongo startup check failed: "},
		{"user of another database", otherUser, otherPassword, "mongo startup check failed: "},
		{"wrong password", svcUser, svcPassword + "-wrong", "mongo startup check failed: "},
		// listCollections passes for a read-only user, index creation does not.
		{"read-only user", readUser, readPassword, "Unauthorized"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			conf := cfg(c.user, c.password)
			if err := validateConfig(conf); err != nil {
				t.Fatal(err)
			}
			wg := &sync.WaitGroup{}
			svcCtx, cancel := context.WithCancel(contextwg.WithWaitGroup(ctx, wg))
			defer cancel()
			db, err := New(svcCtx, conf)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				cancel()
				wg.Wait()
				return
			}
			if err == nil {
				db.Disconnect()
				t.Fatalf("expected an error containing %q", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("unexpected error: %v", err)
			}
			for _, pw := range passwords {
				if strings.Contains(err.Error(), pw) {
					t.Error("error text contains a password")
				}
			}
		})
	}
}

// createUser registers the cleanup first, so a partly failed creation is removed as well.
func createUser(t *testing.T, root *mongo.Client, user, password, role, db string) {
	t.Helper()
	admin := root.Database("admin")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = admin.RunCommand(ctx, bson.D{{Key: "dropUser", Value: user}}).Err()
		_ = root.Database(db).Drop(ctx)
	})
	cmd := bson.D{
		{Key: "createUser", Value: user},
		{Key: "pwd", Value: password},
		{Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: role}, {Key: "db", Value: db}}}},
	}
	if err := admin.RunCommand(context.Background(), cmd).Err(); err != nil {
		t.Fatalf("create user: %v", err)
	}
}

func assertIndexExists(t *testing.T, root *mongo.Client, db, collection, want string) {
	t.Helper()
	specs, err := root.Database(db).Collection(collection).Indexes().ListSpecifications(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range specs {
		if s.Name == want {
			return
		}
	}
	names := []string{}
	for _, s := range specs {
		names = append(names, s.Name)
	}
	t.Errorf("index %q missing, have %v", want, names)
}

func randomHex(t *testing.T) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}
