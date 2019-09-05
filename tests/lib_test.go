package tests

import (
	"encoding/json"
	"fmt"
	"github.com/SENERGY-Platform/process-model-repository/lib"
	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/SENERGY-Platform/process-model-repository/lib/model"
	"github.com/ory/dockertest"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func TestMigration(t *testing.T) {
	conf, err := config.Load("../config.json")
	if err != nil {
		log.Fatal("ERROR: unable to load config", err)
	}

	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Fatal(err)
	}

	amqpCloser, _, amqpIp, err := Amqp(pool)
	defer amqpCloser()
	if err != nil {
		t.Fatal(err)
	}

	mongoCloser, _, mongoIp, err := MongoTestServer(pool)
	defer mongoCloser()
	if err != nil {
		t.Fatal(err)
	}

	conf.MongoUrl = "mongodb://" + mongoIp + ":27017"

	closeZk, _, zkIp, err := ZookeeperContainer(pool)
	defer closeZk()
	if err != nil {
		t.Fatal(err)
	}
	conf.ZookeeperUrl = zkIp + ":2181"

	closeKafka, err := KafkaContainer(pool, conf.ZookeeperUrl)
	defer closeKafka()
	if err != nil {
		t.Fatal(err)
	}

	permsearch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "true")
	}))
	defer permsearch.Close()
	conf.PermissionsUrl = permsearch.URL

	repoCloser, _, oldHost, err := OldProcessRepo(pool, "amqp://guest:guest@"+amqpIp+":5672/", conf.MongoUrl, conf.PermissionsUrl)
	defer repoCloser()
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Second)

	port, err := getFreePort()
	if err != nil {
		t.Fatal(err)
	}
	conf.ServerPort = strconv.Itoa(port)

	stop, err := lib.Start(conf)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	var p1 model.Process
	err = userjwt.PostJSON("http://"+oldHost+":8081/process", map[string]interface{}{"svgXML": "svg1", "process": map[string]map[string]map[string]string{"definitions": {"process": {"_id": "p1"}}}}, &p1)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(5 * time.Second)

	var p2 model.Process
	err = userjwt.PostJSON("http://"+oldHost+":8081/process", map[string]interface{}{"svgXML": "svg2", "process": map[string]map[string]map[string]string{"definitions": {"process": {"_id": "p2"}}}}, &p2)
	if err != nil {
		t.Fatal(err)
	}

	//time.Sleep(5 * time.Second)
	//
	//var p2c model.Process
	//err = userjwt.PostJSON("http://"+oldHost+":8081/process/"+p2.Id+"/publish", model.PublicCommand{Publish: true, Description: "publish_description2"}, &p2c)
	//if err != nil {
	//	t.Fatal(err)
	//}

	time.Sleep(5 * time.Second)

	var p3 model.Process
	err = userjwt.PostJSON("http://localhost:"+conf.ServerPort+"/processes", map[string]interface{}{"svgXML": "svg3", "process": map[string]map[string]map[string]string{"definitions": {"process": {"_id": "p3"}}}}, &p3)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(5 * time.Second)

	var p4 model.Process
	err = userjwt.PostJSON("http://localhost:"+conf.ServerPort+"/processes", map[string]interface{}{"svgXML": "svg4", "process": map[string]map[string]map[string]string{"definitions": {"process": {"_id": "p4"}}}}, &p4)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(5 * time.Second)

	var p4c model.Process
	err = userjwt.PostJSON("http://localhost:"+conf.ServerPort+"/processes/"+p4.Id+"/publish", model.PublicCommand{Publish: true, Description: "publish_description4"}, &p4c)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(5 * time.Second)

	for _, p := range []model.Process{p1, p2, p3, p4c} {
		r := model.Process{}
		err = userjwt.GetJSON("http://localhost:"+conf.ServerPort+"/processes/"+p.Id, &r)
		if err != nil {
			t.Fatal(err)
		}
		if r.Id != p.Id {
			t.Fatal(p, r)
		}
		if r.SvgXml != p.SvgXml {
			t.Fatal(p, r)
		}
		if !reflect.DeepEqual(r.Process, p.Process) {
			t.Fatal(p, r)
		}
	}

	list := []model.Process{}
	err = userjwt.GetJSON("http://localhost:"+conf.ServerPort+"/processes", &list)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(list, []model.Process{p4c}) {
		t.Fatal(list, "\n", p4c)
	}

	isStr, err := json.Marshal(p4c.Process)
	if err != nil {
		t.Fatal(err)
	}

	wantStr, err := json.Marshal(map[string]map[string]map[string]string{"definitions": {"process": {"_id": "p4"}}})
	if err != nil {
		t.Fatal(err)
	}

	var a interface{}
	var b interface{}
	err = json.Unmarshal(isStr, &a)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(wantStr, &b)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(a, b) {
		t.Fatal(a, b, string(isStr), string(wantStr))
	}
}
