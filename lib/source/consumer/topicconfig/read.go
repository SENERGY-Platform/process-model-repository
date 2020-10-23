package topicconfig

import (
	"encoding/json"
	"github.com/go-zookeeper/zk"
	"log"
	"time"
)

type TopicConfigWrapper struct {
	Version int32             `json:"version"`
	Config  map[string]string `json:"config"`
}

func Read(zkUrl string, topic string) (config map[string]string, version int32, err error) {
	c, _, err := zk.Connect([]string{zkUrl}, time.Second) //*10)
	if err != nil {
		return config, version, err
	}
	defer c.Close()
	return read(c, topic)
}

func read(c *zk.Conn, topic string) (config map[string]string, version int32, err error) {
	temp, stat, err := c.Get(getTopicPath(topic))
	if err != nil {
		return config, version, err
	}
	wrapper := TopicConfigWrapper{}
	err = json.Unmarshal(temp, &wrapper)
	version = wrapper.Version
	config = wrapper.Config
	log.Println("DEBUG version", wrapper.Version, stat.Version, stat.Cversion, stat.Aversion)
	return
}
