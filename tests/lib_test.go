package tests

import (
	"context"
	"fmt"
	"github.com/SENERGY-Platform/process-model-repository/lib"
	"github.com/SENERGY-Platform/process-model-repository/lib/config"
	"github.com/SENERGY-Platform/process-model-repository/lib/contextwg"
	"github.com/SENERGY-Platform/process-model-repository/lib/model"
	"github.com/ory/dockertest/v3"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"
)

func Test(t *testing.T) {
	conf, err := config.Load("../config.json")
	if err != nil {
		log.Fatal("ERROR: unable to load config", err)
	}

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

	permsearch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "true")
	}))
	defer permsearch.Close()
	conf.PermissionsUrl = permsearch.URL

	time.Sleep(2 * time.Second)

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

	var p3 model.Process
	err = userjwt.PostJSON("http://localhost:"+conf.ServerPort+"/processes",
		model.Process{
			BpmnXml: createTestXmlString("p3"),
			SvgXml:  "svg3",
		}, &p3)
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(5 * time.Second)

	var p4 model.Process
	err = userjwt.PostJSON("http://localhost:"+conf.ServerPort+"/processes", model.Process{
		BpmnXml: createTestXmlString("p4"),
		SvgXml:  "svg4",
	}, &p4)
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(5 * time.Second)

	var p4c model.Process
	err = userjwt.PostJSON("http://localhost:"+conf.ServerPort+"/processes/"+p4.Id+"/publish", model.PublicCommand{Publish: true, Description: "publish_description4"}, &p4c)
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(5 * time.Second)

	for _, p := range []model.Process{p3, p4c} {
		r := model.Process{}
		err = userjwt.GetJSON("http://localhost:"+conf.ServerPort+"/processes/"+p.Id, &r)
		if err != nil {
			t.Error(err)
			return
		}
		if r.Id != p.Id {
			t.Fatal(p, r)
		}
		if r.SvgXml != p.SvgXml {
			t.Fatal(p, r)
		}
		if r.BpmnXml != p.BpmnXml {
			t.Fatal(p, r)
		}
	}

	list := []model.Process{}
	err = userjwt.GetJSON("http://localhost:"+conf.ServerPort+"/processes", &list)
	if err != nil {
		t.Error(err)
		return
	}

	if !reflect.DeepEqual(list, []model.Process{p4c}) {
		t.Fatal(list, "\n", p4c)
	}

	isStr := p4c.BpmnXml
	wantStr := createTestXmlString("p4")

	if isStr != wantStr {
		t.Fatal(isStr, "\n\n!=\n\n", wantStr)
	}
}

func createTestXmlString(processId string) (result string) {
	return `<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:bpmndi="http://www.omg.org/spec/BPMN/20100524/DI" xmlns:dc="http://www.omg.org/spec/DD/20100524/DC" xmlns:di="http://www.omg.org/spec/DD/20100524/DI" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" id="Definitions_1" targetNamespace="http://bpmn.io/schema/bpmn">
  <bpmndi:BPMNDiagram id="BPMNDiagram_1">
    <bpmndi:BPMNPlane bpmnElement="testor" id="BPMNPlane_1">
      <bpmndi:BPMNEdge bpmnElement="SequenceFlow_0r1kd9b" id="SequenceFlow_0r1kd9b_di">
        <di:waypoint x="188" y="300"/>
        <di:waypoint x="240" y="300"/>
      </bpmndi:BPMNEdge>
      <bpmndi:BPMNEdge bpmnElement="SequenceFlow_0w6aadb" id="SequenceFlow_0w6aadb_di">
        <di:waypoint x="340" y="300"/>
        <di:waypoint x="392" y="300"/>
      </bpmndi:BPMNEdge>
      <bpmndi:BPMNShape bpmnElement="StartEvent_1" id="_BPMNShape_StartEvent_2">
        <dc:Bounds height="36" width="36" x="152" y="282"/>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape bpmnElement="Task_0xjre54" id="Task_0xjre54_di">
        <dc:Bounds height="80" width="100" x="240" y="260"/>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape bpmnElement="EndEvent_1bcd04k" id="EndEvent_1bcd04k_di">
        <dc:Bounds height="36" width="36" x="392" y="282"/>
      </bpmndi:BPMNShape>
    </bpmndi:BPMNPlane>
  </bpmndi:BPMNDiagram>
  <bpmn:process id="` + processId + `" isExecutable="true">
    <bpmn:endEvent id="EndEvent_1bcd04k">
      <bpmn:incoming>SequenceFlow_0w6aadb</bpmn:incoming>
    </bpmn:endEvent>
    <bpmn:sequenceFlow id="SequenceFlow_0r1kd9b" sourceRef="StartEvent_1" targetRef="Task_0xjre54"/>
    <bpmn:sequenceFlow id="SequenceFlow_0w6aadb" sourceRef="Task_0xjre54" targetRef="EndEvent_1bcd04k"/>
    <bpmn:startEvent id="StartEvent_1">
      <bpmn:outgoing>SequenceFlow_0r1kd9b</bpmn:outgoing>
    </bpmn:startEvent>
    <bpmn:task id="Task_0xjre54" name="Test">
      <bpmn:incoming>SequenceFlow_0r1kd9b</bpmn:incoming>
      <bpmn:outgoing>SequenceFlow_0w6aadb</bpmn:outgoing>
    </bpmn:task>
  </bpmn:process>
</bpmn:definitions>`
}
