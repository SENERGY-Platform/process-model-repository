/*
 * Copyright 2019 InfAI (CC SES)
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
	"fmt"
	"reflect"
	"time"

	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/SENERGY-Platform/process-model-repository/lib/contextwg"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsoncodec"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Mongo struct {
	config config.Config
	client *mongo.Client
}

var CreateCollections = []func(db *Mongo) error{}

var (
	ErrEmptyDatabase   = errors.New("mongo database name must not be empty")
	ErrMissingPassword = errors.New("mongo password must not be empty when a mongo user is set")
)

func New(ctx context.Context, conf config.Config) (*Mongo, error) {
	if err := validateConfig(conf); err != nil {
		return nil, err
	}
	client, err := connect(ctx, clientOptions(conf), conf.MongoDatabase, 10*time.Second)
	if err != nil {
		return nil, err
	}
	db := &Mongo{config: conf, client: client}
	for _, creators := range CreateCollections {
		err = creators(db)
		if err != nil {
			client.Disconnect(context.Background())
			return nil, err
		}
	}
	contextwg.Add(ctx, 1)
	go func() {
		<-ctx.Done()
		disconnectTimeout, _ := getTimeoutContext(context.Background())
		conf.GetLogger().Info("disconnect from mongodb", "result", client.Disconnect(disconnectTimeout))
		contextwg.Done(ctx)
	}()
	return db, nil
}

func validateConfig(conf config.Config) error {
	if conf.MongoDatabase == "" {
		return ErrEmptyDatabase
	}
	if conf.MongoUser != "" && conf.MongoPassword == "" {
		return ErrMissingPassword
	}
	return nil
}

// clientOptions applies the credentials after the URI, so they replace any user, password,
// authSource and authMechanism given in MongoUrl.
func clientOptions(conf config.Config) *options.ClientOptions {
	opts := options.Client().ApplyURI(conf.MongoUrl)
	if conf.MongoUser != "" {
		opts.SetAuth(options.Credential{
			Username:   conf.MongoUser,
			Password:   conf.MongoPassword,
			AuthSource: conf.MongoAuthSource,
		})
	}
	return opts
}

// connect runs an authorized listCollections on database because Connect is lazy and ping
// needs no authentication; an unreachable server or wrong or missing credentials then fail
// here instead of at the first query. On failure the client is disconnected.
func connect(ctx context.Context, opts *options.ClientOptions, database string, timeout time.Duration) (*mongo.Client, error) {
	connectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client, err := mongo.Connect(connectCtx, opts)
	if err != nil {
		return nil, err
	}
	checkCtx, checkCancel := context.WithTimeout(ctx, timeout)
	defer checkCancel()
	listOpts := options.ListCollections().SetNameOnly(true).SetAuthorizedCollections(true)
	if _, err = client.Database(database).ListCollectionNames(checkCtx, bson.D{}, listOpts); err != nil {
		disconnectTimeout, disconnectCancel := getTimeoutContext(context.Background())
		defer disconnectCancel()
		_ = client.Disconnect(disconnectTimeout)
		return nil, fmt.Errorf("mongo startup check failed: %w", err)
	}
	return client, nil
}

func (this *Mongo) CreateId() string {
	return uuid.NewString()
}

func (this *Mongo) Transaction(ctx context.Context) (resultCtx context.Context, close func(success bool) error, err error) {
	if !this.config.MongoReplSet {
		return ctx, func(bool) error { return nil }, nil
	}
	session, err := this.client.StartSession()
	if err != nil {
		return nil, nil, err
	}
	err = session.StartTransaction()
	if err != nil {
		return nil, nil, err
	}

	//create session context; callback is executed synchronously and the error is passed on as error of WithSession
	_ = mongo.WithSession(ctx, session, func(sessionContext mongo.SessionContext) error {
		resultCtx = sessionContext
		return nil
	})

	return resultCtx, func(success bool) error {
		defer session.EndSession(context.Background())
		var err error
		if success {
			err = session.CommitTransaction(resultCtx)
		} else {
			err = session.AbortTransaction(resultCtx)
		}
		if err != nil {
			this.config.GetLogger().Error("unable to finish mongo transaction", "error", err)
		}
		return err
	}, nil
}

func (this *Mongo) ensureIndex(collection *mongo.Collection, indexname string, indexKey string, asc bool, unique bool) error {
	ctx, _ := getTimeoutContext(context.Background())
	var direction int32 = -1
	if asc {
		direction = 1
	}
	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{indexKey, direction}},
		Options: options.Index().SetName(indexname).SetUnique(unique),
	})
	return err
}

func (this *Mongo) ensureCompoundIndex(collection *mongo.Collection, indexname string, asc bool, unique bool, indexKeys ...string) error {
	ctx, _ := getTimeoutContext(context.Background())
	var direction int32 = -1
	if asc {
		direction = 1
	}
	keys := []bson.E{}
	for _, key := range indexKeys {
		keys = append(keys, bson.E{Key: key, Value: direction})
	}
	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D(keys),
		Options: options.Index().SetName(indexname).SetUnique(unique),
	})
	return err
}

func (this *Mongo) Disconnect() {
	this.config.GetLogger().Info("disconnected from mongodb", "result", this.client.Disconnect(context.Background()))
}

func getBsonFieldName(obj interface{}, fieldName string) (bsonName string, err error) {
	field, found := reflect.TypeOf(obj).FieldByName(fieldName)
	if !found {
		return "", errors.New("field '" + fieldName + "' not found")
	}
	tags, err := bsoncodec.DefaultStructTagParser.ParseStructTags(field)
	return tags.Name, err
}

func getTimeoutContext(basectx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(basectx, 10*time.Second)
}
