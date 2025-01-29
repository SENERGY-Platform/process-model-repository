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
	"github.com/SENERGY-Platform/process-model-repository/lib/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"regexp"
	"strings"
	"time"
)

const processIdFieldName = "Id"
const processPublicFieldName = "Publish"

var processIdKey string
var processPublicKey string

func init() {
	var err error
	processIdKey, err = getBsonFieldName(model.Process{}, processIdFieldName)
	if err != nil {
		log.Fatal(err)
	}
	processPublicKey, err = getBsonFieldName(model.Process{}, processPublicFieldName)
	if err != nil {
		log.Fatal(err)
	}

	CreateCollections = append(CreateCollections, func(db *Mongo) error {
		collection := db.client.Database(db.config.MongoTable).Collection(db.config.MongoProcessCollection)
		//err = db.ensureIndex(collection, "processidindex", processIdKey, true, true)
		if err != nil {
			return err
		}
		err = db.ensureIndex(collection, "processpublicindex", processPublicKey, true, false)
		if err != nil {
			return err
		}
		return nil
	})
}

func (this *Mongo) ProcessCollection() *mongo.Collection {
	return this.client.Database(this.config.MongoTable).Collection(this.config.MongoProcessCollection)
}

func (this *Mongo) ReadProcess(ctx context.Context, id string) (process model.Process, exists bool, err error) {
	result := this.ProcessCollection().FindOne(ctx, bson.M{processIdKey: id})
	err = result.Err()
	if err == mongo.ErrNoDocuments {
		return process, false, nil
	}
	if err != nil {
		return
	}
	err = result.Decode(&process)
	if err == mongo.ErrNoDocuments {
		return process, false, nil
	}
	return process, true, err
}

func (this *Mongo) ReadAllPublicProcesses(ctx context.Context) (processes []model.Process, err error) {
	cursor, err := this.ProcessCollection().Find(ctx, bson.M{processPublicKey: true})
	if err != nil {
		return nil, err
	}
	err = cursor.All(ctx, &processes)
	return
}

func (this *Mongo) SetProcess(ctx context.Context, process model.Process) error {
	if process.LastUpdatedUnix == 0 {
		process.LastUpdatedUnix = time.Now().Unix()
	}
	_, err := this.ProcessCollection().ReplaceOne(ctx, bson.M{processIdKey: process.Id}, process, options.Replace().SetUpsert(true))
	return err
}

func (this *Mongo) DeleteProcess(ctx context.Context, id string) error {
	_, err := this.ProcessCollection().DeleteMany(ctx, bson.M{processIdKey: id})
	return err
}

func (this *Mongo) ListProcesses(ctx context.Context, listOptions model.ListOptions) (result []model.Process, total int64, err error) {
	opt := options.Find()
	if listOptions.Limit > 0 {
		opt.SetLimit(listOptions.Limit)
	}
	if listOptions.Offset > 0 {
		opt.SetSkip(listOptions.Offset)
	}

	if listOptions.SortBy == "" {
		listOptions.SortBy = "name.asc"
	}

	sortby := listOptions.SortBy
	sortby = strings.TrimSuffix(sortby, ".asc")
	sortby = strings.TrimSuffix(sortby, ".desc")

	direction := int32(1)
	if strings.HasSuffix(listOptions.SortBy, ".desc") {
		direction = int32(-1)
	}
	opt.SetSort(bson.D{{sortby, direction}})

	filter := bson.M{}
	if listOptions.Ids != nil {
		filter["_id"] = bson.M{"$in": listOptions.Ids}
	}
	search := strings.TrimSpace(listOptions.Search)
	if search != "" {
		escapedSearch := regexp.QuoteMeta(search)
		filter["$or"] = []interface{}{
			bson.M{"name": bson.M{"$regex": escapedSearch, "$options": "i"}},
			bson.M{"description": bson.M{"$regex": escapedSearch, "$options": "i"}},
		}
	}

	cursor, err := this.ProcessCollection().Find(ctx, filter, opt)
	if err != nil {
		return result, total, err
	}
	err = cursor.All(ctx, &result)
	if err != nil {
		return result, total, err
	}
	total, err = this.ProcessCollection().CountDocuments(ctx, filter)
	if err != nil {
		return result, total, err
	}
	return result, total, err
}

var CleanupLastUpdateTimeBuffer = time.Minute

func (this *Mongo) CheckIdList(ids []string) (missingInDb []string, missingInInput []string, err error) {
	idList, err := this.ProcessCollection().Distinct(context.Background(), processIdKey, bson.M{})
	if err != nil {
		return nil, nil, err
	}
	knownIds := map[string]bool{}
	for _, id := range idList {
		idString, ok := id.(string)
		if !ok {
			return nil, nil, errors.New("db id is not a string")
		}
		knownIds[idString] = true
	}
	for _, id := range ids {
		if !knownIds[id] {
			missingInDb = append(missingInDb, id)
		}
	}

	missingInInputList, err := this.ProcessCollection().Distinct(context.Background(), processIdKey, bson.M{
		processIdKey: bson.M{"$nin": ids},
		"$or": []interface{}{
			bson.M{"last_updated_unix": bson.M{"$exists": false}},
			bson.M{"last_updated_unix": bson.M{"$lt": time.Now().Add(-CleanupLastUpdateTimeBuffer).Unix()}},
		},
	})
	if err != nil {
		return nil, nil, err
	}
	for _, id := range missingInInputList {
		idString, ok := id.(string)
		if !ok {
			return nil, nil, errors.New("db id is not a string")
		}
		missingInInput = append(missingInInput, idString)
	}

	return missingInDb, missingInInput, nil
}
