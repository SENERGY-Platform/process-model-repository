package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/SENERGY-Platform/process-model-repository/lib"
	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/SENERGY-Platform/process-model-repository/lib/contextwg"
	"github.com/SENERGY-Platform/process-model-repository/lib/controller/auth"
	"github.com/SENERGY-Platform/process-model-repository/lib/model"
	"github.com/SENERGY-Platform/process-model-repository/lib/source/consumer/listener"
	"github.com/SENERGY-Platform/process-model-repository/lib/source/producer"
	"github.com/SENERGY-Platform/process-model-repository/lib/source/util"
	jwt_http_router "github.com/SmartEnergyPlatform/jwt-http-router"
	"github.com/ory/dockertest/v3"
	"github.com/segmentio/kafka-go"
	"log"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestUserDelete(t *testing.T) {
	conf, err := config.Load("../config.json")
	if err != nil {
		log.Fatal("ERROR: unable to load config", err)
	}

	conf.ConnectivityTest = false

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx = contextwg.WithWaitGroup(ctx, wg)

	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Error(err)
		return
	}

	_, mongoIp, err := MongoTestServer(pool, ctx)
	if err != nil {
		t.Error(err)
		return
	}

	conf.MongoUrl = "mongodb://" + mongoIp + ":27017"

	_, zkIp, err := Zookeeper(pool, ctx)
	if err != nil {
		t.Error(err)
		return
	}
	zookeeperUrl := zkIp + ":2181"

	conf.KafkaUrl, err = Kafka(pool, ctx, zookeeperUrl)
	if err != nil {
		t.Error(err)
		return
	}

	_, elasticIp, err := Elasticsearch(pool, ctx, wg)
	if err != nil {
		t.Error(err)
		return
	}

	_, permIp, err := PermSearch(pool, ctx, wg, conf.KafkaUrl, elasticIp)
	if err != nil {
		t.Error(err)
		return
	}
	conf.PermissionsUrl = "http://" + permIp + ":8080"

	time.Sleep(10 * time.Second)

	port, err := getFreePort()
	if err != nil {
		t.Error(err)
		return
	}
	conf.ServerPort = strconv.Itoa(port)

	err = lib.Start(ctx, conf)
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(2 * time.Second)

	broker, err := util.GetBroker(conf.KafkaUrl)
	if err != nil {
		t.Error(err)
		return
	}
	if len(broker) == 0 {
		t.Error("missing kafka broker")
		return
	}

	user1, err := auth.CreateToken("test", "user1")
	if err != nil {
		t.Error(err)
		return
	}
	user2, err := auth.CreateToken("test", "user2")
	if err != nil {
		t.Error(err)
		return
	}

	processModelIds := []string{}
	t.Run("create process models for user 1", testCreateModel(user1, conf, 10, &processModelIds))
	t.Run("create process models for user 2", testCreateModel(user2, conf, 10, &processModelIds))

	time.Sleep(20 * time.Second)

	t.Run("change permissions", func(t *testing.T) {
		permissions := &kafka.Writer{
			Addr:        kafka.TCP(broker...),
			Topic:       conf.PermissionsTopic,
			MaxAttempts: 10,
			Logger:      log.New(os.Stdout, "[TEST-KAFKA-PRODUCER] ", 0),
		}
		for i := 0; i < 4; i++ {
			id := processModelIds[i]
			err = setPermission(permissions, conf, user2.GetUserId(), id, "rwxa")
			if err != nil {
				t.Error(err)
				return
			}
		}
		for i := 10; i < 15; i++ {
			id := processModelIds[i]
			err = setPermission(permissions, conf, user1.GetUserId(), id, "rwxa")
			if err != nil {
				t.Error(err)
				return
			}
		}
		for i := 2; i < 4; i++ {
			id := processModelIds[i]
			err = setPermission(permissions, conf, user1.GetUserId(), id, "rx")
			if err != nil {
				t.Error(err)
				return
			}
		}
		for i := 12; i < 14; i++ {
			id := processModelIds[i]
			err = setPermission(permissions, conf, user2.GetUserId(), id, "rx")
			if err != nil {
				t.Error(err)
				return
			}
		}
	})

	time.Sleep(20 * time.Second)

	t.Run("check user1 before delete", checkUserProcesses(conf, user1, processModelIds[:15]))
	t.Run("check user2 before delete", checkUserProcesses(conf, user2, append(append([]string{}, processModelIds[:4]...), processModelIds[10:]...)))

	t.Run("delete user1", func(t *testing.T) {
		users := &kafka.Writer{
			Addr:        kafka.TCP(broker...),
			Topic:       conf.UsersTopic,
			MaxAttempts: 10,
			Logger:      log.New(os.Stdout, "[TEST-KAFKA-PRODUCER] ", 0),
		}
		cmd := listener.UserCommandMsg{
			Command: "DELETE",
			Id:      user1.GetUserId(),
		}
		message, err := json.Marshal(cmd)
		if err != nil {
			t.Error(err)
			return
		}
		err = users.WriteMessages(
			context.Background(),
			kafka.Message{
				Key:   []byte(user1.GetUserId()),
				Value: message,
				Time:  time.Now(),
			},
		)
	})

	time.Sleep(20 * time.Second)

	t.Run("check user1 after delete", checkUserProcesses(conf, user1, []string{}))
	t.Run("check user2 after delete", checkUserProcesses(conf, user2, append(append(append([]string{}, processModelIds[:4]...), processModelIds[10:12]...), processModelIds[14:]...)))
}

func testCreateModel(token auth.Token, conf config.Config, count int, createdIds *[]string) func(t *testing.T) {
	return func(t *testing.T) {
		for i := 0; i < count; i++ {
			process := model.Process{}
			err := jwt_http_router.JwtImpersonate(token.Token).PostJSON("http://localhost:"+conf.ServerPort+"/processes",
				model.Process{
					BpmnXml: createTestXmlString("name_" + strconv.Itoa(i)),
					SvgXml:  "svg",
				}, &process)
			if err != nil {
				t.Error(err)
				return
			}
			*createdIds = append(*createdIds, process.Id)
		}
	}
}

func checkUserProcesses(conf config.Config, token auth.Token, expectedIdsOrig []string) func(t *testing.T) {
	return func(t *testing.T) {
		expectedIds := []string{}
		temp, err := json.Marshal(expectedIdsOrig)
		if err != nil {
			t.Error(err)
			return
		}
		err = json.Unmarshal(temp, &expectedIds)
		if err != nil {
			t.Error(err)
			return
		}

		req, err := http.NewRequest("GET", conf.PermissionsUrl+"/v3/resources/processmodel?rights=r&limit=100", nil)
		if err != nil {
			t.Error(err)
			return
		}
		req.Header.Set("Authorization", token.Token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Error(err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 300 {
			buf := new(bytes.Buffer)
			buf.ReadFrom(resp.Body)
			resp.Body.Close()
			log.Println("DEBUG: PermissionCheck()", buf.String())
			err = errors.New("access denied")
			t.Error(err)
			return
		}

		processes := []map[string]interface{}{}
		err = json.NewDecoder(resp.Body).Decode(&processes)
		if err != nil {
			t.Error(err)
			return
		}
		actualIds := []string{}
		for _, processe := range processes {
			id, ok := processe["id"].(string)
			if !ok {
				t.Error("expect process id to be string", processe)
				return
			}
			actualIds = append(actualIds, id)
		}
		sort.Strings(actualIds)
		sort.Strings(expectedIds)

		if !reflect.DeepEqual(actualIds, expectedIds) {
			a, _ := json.Marshal(actualIds)
			e, _ := json.Marshal(expectedIds)
			t.Error(string(a), "\n", string(e))
			return
		}
	}
}

func setPermission(permissions *kafka.Writer, conf config.Config, userId string, id string, right string) error {
	cmd := producer.PermCommandMsg{
		Command:  "PUT",
		Kind:     conf.ProcessTopic,
		Resource: id,
		User:     userId,
		Right:    right,
	}
	message, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	err = permissions.WriteMessages(
		context.Background(),
		kafka.Message{
			Key:   []byte(userId + "_" + conf.ProcessTopic + "_" + id),
			Value: message,
			Time:  time.Now(),
		},
	)
	return err
}
