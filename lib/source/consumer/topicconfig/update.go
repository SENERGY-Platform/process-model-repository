package topicconfig

import (
	"encoding/json"
	"github.com/go-zookeeper/zk"
	"time"
)

func Ensure(zkUrl string, topic string, config map[string]string) (err error) {
	c, _, err := zk.Connect([]string{zkUrl}, time.Second) //*10)
	if err != nil {
		return err
	}
	err = update(c, topic, config)
	if err == zk.ErrNoNode {
		err = Create(zkUrl, topic, 1, 1, config)
	}
	return err
}

func Update(zkUrl string, topic string, config map[string]string) (err error) {
	c, _, err := zk.Connect([]string{zkUrl}, time.Second) //*10)
	if err != nil {
		return err
	}
	return update(c, topic, config)
}

func update(c *zk.Conn, topic string, config map[string]string) (err error) {
	current, version, err := read(c, topic)
	if err != nil {
		return err
	}
	for key, value := range config {
		current[key] = value
	}
	err = set(c, topic, version, current)
	return
}

func Set(zkUrl string, topic string, version int32, config map[string]string) (err error) {
	c, _, err := zk.Connect([]string{zkUrl}, time.Second) //*10)
	if err != nil {
		return err
	}
	defer c.Close()
	return set(c, topic, version, config)
}

func set(c *zk.Conn, topic string, version int32, config map[string]string) (err error) {
	temp, err := json.Marshal(TopicConfigWrapper{
		Version: version,
		Config:  config,
	})
	if err != nil {
		return err
	}
	_, err = c.Set(getTopicPath(topic), temp, -1)
	return err
}
