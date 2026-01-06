package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SENERGY-Platform/permissions-v2/pkg/client"
	"github.com/SENERGY-Platform/process-model-repository/lib"
	"github.com/SENERGY-Platform/process-model-repository/lib/auth"
	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/SENERGY-Platform/process-model-repository/lib/contextwg"
	"github.com/SENERGY-Platform/process-model-repository/lib/model"
	"github.com/SENERGY-Platform/process-model-repository/lib/source/consumer/listener"
	"github.com/segmentio/kafka-go"
)

func TestUserDelete(t *testing.T) {
	conf, err := config.Load("../config.json")
	if err != nil {
		log.Fatal("ERROR: unable to load config", err)
	}

	conf.Debug = true
	conf.ConnectivityTest = false

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx = contextwg.WithWaitGroup(ctx, wg)

	_, mongoIp, err := MongoTestServer(ctx, wg)
	if err != nil {
		t.Error(err)
		return
	}

	conf.MongoUrl = "mongodb://" + mongoIp + ":27017"

	conf.KafkaUrl, err = Kafka(ctx, wg)
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(10 * time.Second)

	_, permIp, err := PermissionsV2(ctx, wg, conf.MongoUrl, conf.KafkaUrl)
	if err != nil {
		t.Error(err)
		return
	}
	conf.PermissionsV2Url = "http://" + permIp + ":8080"

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
		for i := 0; i < 4; i++ {
			id := processModelIds[i]
			err = setPermission(conf, user2.GetUserId(), id, "rwxa")
			if err != nil {
				t.Error(err)
				return
			}
		}
		for i := 10; i < 15; i++ {
			id := processModelIds[i]
			err = setPermission(conf, user1.GetUserId(), id, "rwxa")
			if err != nil {
				t.Error(err)
				return
			}
		}
		for i := 2; i < 4; i++ {
			id := processModelIds[i]
			err = setPermission(conf, user1.GetUserId(), id, "rx")
			if err != nil {
				t.Error(err)
				return
			}
		}
		for i := 12; i < 14; i++ {
			id := processModelIds[i]
			err = setPermission(conf, user2.GetUserId(), id, "rx")
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
			Addr:        kafka.TCP(conf.KafkaUrl),
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
			err := PostJSON(token.Jwt(), "http://localhost:"+conf.ServerPort+"/processes",
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

		req, err := http.NewRequest("GET", "http://localhost:"+conf.ServerPort+"/v2/processes?rights=r&limit=100", nil)
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
			id, ok := processe["_id"].(string)
			if !ok {
				t.Errorf("expect process id to be string, got %#v \n%#v\n", processe["_id"], processe)
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

func setPermission(conf config.Config, userId string, id string, right string) error {
	c := client.New(conf.PermissionsV2Url)
	old, err, code := c.GetResource(client.InternalAdminToken, conf.ProcessTopic, id)
	if err != nil && code != http.StatusNotFound {
		return err
	}
	old.UserPermissions[userId] = client.PermissionsMap{
		Read:         strings.Contains(right, "r"),
		Write:        strings.Contains(right, "2"),
		Execute:      strings.Contains(right, "x"),
		Administrate: strings.Contains(right, "a"),
	}
	_, err, _ = c.SetPermission(client.InternalAdminToken, conf.ProcessTopic, id, old.ResourcePermissions)
	return err
}
